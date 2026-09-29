package testenv

import (
	"context"
	"net/url"
)

func (c *Cluster) bootstrap(ctx context.Context, admin []byte) error {
	for _, ns := range []string{"sdk-a", "sdk-b"} {
		if e := c.call(ctx, "POST", "", "sys/namespaces/"+ns, admin, map[string]any{}, nil); e != nil {
			return e
		}
		for _, m := range []struct {
			path, kind string
			options    map[string]string
		}{{"kv", "kv", map[string]string{"version": "2"}}, {"pki", "pki", nil}, {"transit", "transit", nil}} {
			if e := c.call(ctx, "POST", ns, "sys/mounts/"+m.path, admin, map[string]any{"type": m.kind, "options": m.options}, nil); e != nil {
				return e
			}
		}
		// Only bootstrap has metadata-write permission. Runtime clients must
		// read versions with future deletion_time without treating them as gone.
		if e := c.call(ctx, "POST", ns, "kv/metadata/fixture/scheduled", admin, map[string]any{"delete_version_after": "1h"}, nil); e != nil {
			return e
		}
		if e := c.call(ctx, "POST", ns, "sys/auth/sdk-role", admin, map[string]any{"type": "approle"}, nil); e != nil {
			return e
		}
		var ca struct {
			Data struct {
				Certificate string `json:"certificate"`
			} `json:"data"`
		}
		if e := c.call(ctx, "POST", ns, "pki/root/generate/internal", admin, map[string]any{"common_name": "SDK isolated issuer " + ns, "ttl": "24h", "key_type": "ec", "key_bits": 256}, &ca); e != nil || ca.Data.Certificate == "" {
			return errFixture
		}
		role := map[string]any{"allowed_domains": []string{"terminal.test"}, "allow_bare_domains": true, "allow_subdomains": true, "allowed_uri_sans": []string{"spiffe://fixture/*"}, "client_flag": true, "server_flag": false, "key_type": "ec", "key_bits": 256, "max_ttl": "1h", "ttl": "10m"}
		if e := c.call(ctx, "POST", ns, "pki/roles/device", admin, role, nil); e != nil {
			return e
		}
		if e := c.call(ctx, "POST", ns, "transit/config/keys", admin, map[string]any{"disable_upsert": true}, nil); e != nil {
			return e
		}
		for _, k := range []struct {
			name, kind string
			derived    bool
		}{{"ecdsa", "ecdsa-p256", false}, {"rsa", "rsa-2048", false}, {"ed25519", "ed25519", false}, {"cipher", "aes256-gcm96", false}, {"derived", "aes256-gcm96", true}, {"mac", "hmac", false}} {
			args := map[string]any{"type": k.kind, "derived": k.derived, "exportable": false, "allow_plaintext_backup": false}
			if k.kind == "hmac" {
				args["key_size"] = 32
			}
			if e := c.call(ctx, "POST", ns, "transit/keys/"+k.name, admin, args, nil); e != nil {
				return e
			}
			if e := c.call(ctx, "POST", ns, "transit/keys/"+k.name+"/rotate", admin, map[string]any{}, nil); e != nil {
				return e
			}
		}
		for name, policy := range map[string]string{"provision": runtimePolicy(), "control": controlPolicy(), "maintenance": `path "pki/revoke" { capabilities = ["update"] }`} {
			if e := c.call(ctx, "PUT", ns, "sys/policies/acl/"+name, admin, map[string]any{"policy": policy}, nil); e != nil {
				return e
			}
		}
		s := Space{Namespace: ns, IssuerCA: []byte(ca.Data.Certificate)}
		var e error
		if s.Provision, e = c.identity(ctx, ns, "provision", admin); e != nil {
			return e
		}
		if s.Control, e = c.identity(ctx, ns, "control", admin); e != nil {
			return e
		}
		if s.Maintenance, e = c.identity(ctx, ns, "maintenance", admin); e != nil {
			return e
		}
		if s.Replacement, e = c.identity(ctx, ns, "provision", admin); e != nil {
			return e
		}
		c.Spaces = append(c.Spaces, s)
	}
	return nil
}
func (c *Cluster) identity(ctx context.Context, ns, policy string, admin []byte) (Identity, error) {
	role := "sdk-" + policy
	args := map[string]any{"token_policies": []string{policy}, "token_no_default_policy": true, "token_num_uses": 0, "token_ttl": "12s", "token_max_ttl": "36s", "secret_id_ttl": "1h", "secret_id_num_uses": 0}
	if e := c.call(ctx, "POST", ns, "auth/sdk-role/role/"+role, admin, args, nil); e != nil {
		return Identity{}, e
	}
	var rid struct {
		Data struct {
			RoleID string `json:"role_id"`
		} `json:"data"`
	}
	var sid struct {
		Data struct {
			SecretID string `json:"secret_id"`
		} `json:"data"`
	}
	if e := c.call(ctx, "GET", ns, "auth/sdk-role/role/"+url.PathEscape(role)+"/role-id", admin, nil, &rid); e != nil {
		return Identity{}, e
	}
	if e := c.call(ctx, "POST", ns, "auth/sdk-role/role/"+url.PathEscape(role)+"/secret-id", admin, map[string]any{}, &sid); e != nil {
		return Identity{}, e
	}
	var token struct {
		Auth struct {
			ClientToken string `json:"client_token"`
		} `json:"auth"`
	}
	if e := c.call(ctx, "POST", ns, "auth/token/create", admin, map[string]any{"policies": []string{policy}, "no_default_policy": true, "ttl": "10m", "num_uses": 0}, &token); e != nil {
		return Identity{}, e
	}
	if token.Auth.ClientToken == "" || rid.Data.RoleID == "" || sid.Data.SecretID == "" {
		return Identity{}, errFixture
	}
	return Identity{Token: []byte(token.Auth.ClientToken), RoleID: []byte(rid.Data.RoleID), SecretID: []byte(sid.Data.SecretID)}, nil
}
