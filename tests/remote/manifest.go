package remotecheck

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"regexp"
	"strings"
	"time"
)

// This is a closed test profile, not a general remote-execution configuration.
type runManifest struct {
	SchemaVersion     int            `json:"schema_version"`
	TargetAllowlist   []string       `json:"target_allowlist"`
	Target            string         `json:"target"`
	Environment       string         `json:"environment"`
	NamespaceMode     string         `json:"namespace_mode"`
	Namespace         string         `json:"namespace"`
	RunID             string         `json:"run_id"`
	ResourcePrefix    string         `json:"resource_prefix"`
	BootstrapMount    string         `json:"bootstrap_mount"`
	IssuedAt          string         `json:"issued_at"`
	ExpiresAt         string         `json:"expires_at"`
	RequestBudget     map[string]int `json:"request_budget"`
	ResourceBudget    int            `json:"resource_budget"`
	Ownership         string         `json:"ownership"`
	Cleanup           string         `json:"cleanup"`
	AllowedOperations []string       `json:"allowed_operations"`
}

func bindManifest(c *configuration, get func(string) string) error {
	bad := errors.New("remote manifest rejected")
	f, err := os.Open(get("BAO_REMOTE_MANIFEST"))
	if err != nil {
		return bad
	}
	raw, err := io.ReadAll(io.LimitReader(f, 16385))
	f.Close()
	if err != nil || len(raw) > 16384 {
		return bad
	}
	digest := sha256.Sum256(raw)
	want := get("BAO_REMOTE_MANIFEST_SHA256")
	if hex.EncodeToString(digest[:]) != want {
		return bad
	}
	check := json.NewDecoder(bytes.NewReader(raw))
	if uniqueJSONValue(check, 0) != nil {
		return bad
	}
	if _, err = check.Token(); err != io.EOF {
		return bad
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return bad
	}
	keys := strings.Fields("schema_version target_allowlist target environment namespace_mode namespace run_id resource_prefix bootstrap_mount issued_at expires_at request_budget resource_budget ownership cleanup allowed_operations")
	if len(fields) != len(keys) {
		return bad
	}
	for _, key := range keys {
		if _, ok := fields[key]; !ok {
			return bad
		}
	}
	var m runManifest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&m) != nil {
		return bad
	}
	if m.SchemaVersion != 1 || m.Target != "https://kms.jiup9.com:443" || len(m.TargetAllowlist) != 1 || m.TargetAllowlist[0] != m.Target ||
		m.Environment != "nonproduction" || m.NamespaceMode != "named" || m.Namespace != "sdk-test" || m.BootstrapMount != "auth/approle" ||
		!regexp.MustCompile(`^remote-sdk-[0-9]{8}-[a-f0-9]{16}$`).MatchString(m.RunID) || m.ResourcePrefix != strings.TrimPrefix(m.RunID, "remote-") ||
		m.ResourceBudget != 9 || m.Ownership != "create_only_with_identity" || m.Cleanup != "reverse_created_resources_before_expiry" {
		return bad
	}
	if len(m.RequestBudget) != 4 || m.RequestBudget["total"] != 120 || m.RequestBudget["core"] != 43 || m.RequestBudget["transit"] != 32 || m.RequestBudget["cleanup"] != 15 {
		return bad
	}
	ops := []string{"bootstrap_approle", "create_isolated_mounts", "create_acl_policies_cas", "create_runtime_tokens", "seed_kv_cas", "configure_owned_transit_pki_approle", "run_sdk_core", "run_sdk_transit", "cleanup_owned_resources"}
	if len(m.AllowedOperations) != len(ops) {
		return bad
	}
	for i, op := range ops {
		if m.AllowedOperations[i] != op {
			return bad
		}
	}
	stamp := regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$`)
	issued, err1 := time.Parse(time.RFC3339, m.IssuedAt)
	expires, err2 := time.Parse(time.RFC3339, m.ExpiresAt)
	now := time.Now()
	if !stamp.MatchString(m.IssuedAt) || !stamp.MatchString(m.ExpiresAt) || err1 != nil || err2 != nil || !expires.After(issued) ||
		expires.Sub(issued) > 20*time.Minute || now.Before(issued) || !now.Before(expires) {
		return bad
	}
	if c.fullPart != "core" && c.fullPart != "transit" {
		return bad
	}
	if c.address != m.Target || c.environment != m.Environment || c.namespaceMode != m.NamespaceMode || c.namespace != m.Namespace ||
		c.runID != m.RunID+"-"+c.fullPart || c.mount != m.ResourcePrefix+"-kv" || c.transitMount != m.ResourcePrefix+"-transit" ||
		c.pkiMount != m.ResourcePrefix+"-pki" || c.appRoleMount != m.ResourcePrefix+"-approle" || c.marker != "owned-"+m.RunID {
		return bad
	}
	c.manifestSHA, c.expiresAt = want, expires
	return nil
}

// encoding/json permits duplicate keys. Reject them before decoding the profile.
func uniqueJSONValue(d *json.Decoder, depth int) error {
	if depth > 8 {
		return errors.New("manifest nesting rejected")
	}
	token, err := d.Token()
	if err != nil {
		return err
	}
	delim, compound := token.(json.Delim)
	if !compound {
		return nil
	}
	if delim != '{' && delim != '[' {
		return errors.New("manifest shape rejected")
	}
	seen := map[string]bool{}
	for d.More() {
		if delim == '{' {
			key, err := d.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return errors.New("manifest duplicate field")
			}
			seen[name] = true
		}
		if err := uniqueJSONValue(d, depth+1); err != nil {
			return err
		}
	}
	_, err = d.Token()
	return err
}
