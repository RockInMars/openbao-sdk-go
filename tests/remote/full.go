package remotecheck

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"os"
	"strings"
	"time"

	bao "github.com/RockInMars/openbao-sdk-go"
	"github.com/RockInMars/openbao-sdk-go/auth"
	"github.com/RockInMars/openbao-sdk-go/diagnostics"
	"github.com/RockInMars/openbao-sdk-go/pki"
	"github.com/RockInMars/openbao-sdk-go/sensitive"
	"github.com/RockInMars/openbao-sdk-go/transit"
)

// Full mode supplements the bounded, opt-in remote checks. It uses only the
// fixture's exact paths and persists no secret values or raw server responses.
func (h *harness) testFullKV() bool {
	read, err := h.kv.ReadLatest(h.ctx, h.c.path)
	if err == nil {
		var data map[string]string
		err = read.Data.Decode(&data)
		if err == nil && (read.Ref.Version != h.c.version || data["sdk_test_marker"] != h.c.marker) {
			err = errors.New("latest version mismatch")
		}
		read.Data.Zero()
	}
	if !h.check("kv_read_latest", err) {
		return false
	}
	ref, err := h.kv.ReadRef(h.ctx, read.Ref)
	if err == nil {
		var data map[string]string
		err = ref.Data.Decode(&data)
		if err == nil && data["sdk_test_marker"] != h.c.marker {
			err = errors.New("reference mismatch")
		}
		ref.Data.Zero()
	}
	if !h.check("kv_read_ref", err) {
		return false
	}
	meta, err := h.kv.ReadMetadata(h.ctx, h.c.path)
	if err == nil && meta.CurrentVersion != h.c.version {
		err = errors.New("metadata version mismatch")
	}
	if !h.check("kv_metadata", err) {
		return false
	}
	entries, err := h.kv.List(h.ctx, h.c.prefix+"/"+h.c.runID)
	if err == nil {
		found := false
		for _, e := range entries.Entries {
			if e.Name == "seed" && !e.IsFolder {
				found = true
			}
		}
		if !found {
			err = errors.New("owned seed absent from listing")
		}
	}
	return h.check("kv_list", err)
}

func (h *harness) testFullTransit() bool {
	if h.b.limit-h.b.count() < 24 {
		return h.check("full_transit_budget", errors.New("insufficient request reserve"))
	}
	c, err := h.sdk.Transit(h.c.transitMount)
	if err != nil {
		return h.check("full_transit_factory", err)
	}
	message := sensitive.NewBytes([]byte("sdk-remote-full-synthetic-message"))
	defer message.Zero()
	meta, err := c.ReadKeyMetadata(h.ctx, h.c.transitKey)
	if err == nil && (meta.Type != "aes256-gcm96" || meta.LatestVersion < 2) {
		err = errors.New("cipher key rotation missing")
	}
	if !h.check("transit_rewrap_key", err) {
		return false
	}
	old, err := c.Encrypt(h.ctx, transit.EncryptRequest{KeyName: h.c.transitKey, KeyVersion: 1, Plaintext: message})
	if !h.check("transit_rewrap_source", err) {
		return false
	}
	rewrapped, err := c.Rewrap(h.ctx, transit.RewrapRequest{KeyName: h.c.transitKey, TargetVersion: meta.LatestVersion, Ciphertext: old.Ciphertext})
	if err == nil && rewrapped.Ciphertext.Version != meta.LatestVersion {
		err = errors.New("rewrap version mismatch")
	}
	if !h.check("transit_rewrap", err) {
		return false
	}
	clear, err := c.Decrypt(h.ctx, transit.DecryptRequest{KeyName: h.c.transitKey, Ciphertext: rewrapped.Ciphertext})
	if err == nil {
		actual := clear.Plaintext.RevealCopy()
		if !bytes.Equal(actual, []byte("sdk-remote-full-synthetic-message")) {
			err = errors.New("rewrap plaintext mismatch")
		}
		for i := range actual {
			actual[i] = 0
		}
		clear.Plaintext.Zero()
	}
	if !h.check("transit_rewrap_decrypt", err) {
		return false
	}

	signMeta, err := c.ReadKeyMetadata(h.ctx, h.c.transitSignKey)
	if err == nil && (signMeta.Type != "ecdsa-p256" || !signMeta.SupportsSigning || signMeta.LatestVersion < 1) {
		err = errors.New("sign key mismatch")
	}
	if !h.check("transit_sign_key", err) {
		return false
	}
	profile := transit.ECDSAP256SHA256ASN1
	signature, err := c.Sign(h.ctx, transit.SignRequest{KeyName: h.c.transitSignKey, KeyVersion: signMeta.LatestVersion, Profile: profile, Message: message})
	if !h.check("transit_sign", err) {
		return false
	}
	verified, err := c.Verify(h.ctx, transit.VerifyRequest{KeyName: h.c.transitSignKey, ExpectedVersion: signMeta.LatestVersion, Profile: profile, Message: message, Signature: signature.Signature})
	if err == nil && !verified.Valid {
		err = errors.New("signature invalid")
	}
	if !h.check("transit_verify", err) {
		return false
	}
	digestRaw := sha256.Sum256([]byte("sdk-remote-full-synthetic-message"))
	digest := sensitive.NewBytes(digestRaw[:])
	defer digest.Zero()
	digestSignature, err := c.SignDigest(h.ctx, transit.SignDigestRequest{KeyName: h.c.transitSignKey, KeyVersion: signMeta.LatestVersion, Profile: profile, Digest: digest})
	if !h.check("transit_sign_digest", err) {
		return false
	}
	verified, err = c.VerifyDigest(h.ctx, transit.VerifyDigestRequest{KeyName: h.c.transitSignKey, ExpectedVersion: signMeta.LatestVersion, Profile: profile, Digest: digest, Signature: digestSignature.Signature})
	if err == nil && !verified.Valid {
		err = errors.New("digest signature invalid")
	}
	if !h.check("transit_verify_digest", err) {
		return false
	}
	public, err := c.ReadPublicKey(h.ctx, h.c.transitSignKey, signMeta.LatestVersion)
	if err == nil {
		key, parseErr := x509.ParsePKIXPublicKey(public.SPKIDER)
		parts := strings.Split(digestSignature.Signature.Wrapped, ":")
		if parseErr != nil || public.KeyType != "ecdsa-p256" || len(parts) != 3 {
			err = errors.New("public key invalid")
		} else {
			encoded, decodeErr := base64.StdEncoding.DecodeString(parts[2])
			pub, ok := key.(*ecdsa.PublicKey)
			if decodeErr != nil || !ok || !ecdsa.VerifyASN1(pub, digestRaw[:], encoded) {
				err = errors.New("public key does not verify this run's signature")
			}
		}
	}
	if !h.check("transit_public_key", err) {
		return false
	}

	macMeta, err := c.ReadKeyMetadata(h.ctx, h.c.transitHMACKey)
	if err == nil && (macMeta.Type != "hmac" || macMeta.LatestVersion < 1) {
		err = errors.New("HMAC key mismatch")
	}
	if !h.check("transit_hmac_key", err) {
		return false
	}
	mac, err := c.HMAC(h.ctx, transit.HMACRequest{KeyName: h.c.transitHMACKey, KeyVersion: macMeta.LatestVersion, Message: message})
	if !h.check("transit_hmac", err) {
		return false
	}
	verified, err = c.HMACVerify(h.ctx, transit.HMACVerifyRequest{KeyName: h.c.transitHMACKey, ExpectedVersion: macMeta.LatestVersion, Message: message, WrappedHMAC: mac.Wrapped})
	if err == nil && !verified.Valid {
		err = errors.New("HMAC invalid")
	}
	return h.check("transit_hmac_verify", err)
}

func (h *harness) testFullPKI() (ok bool) {
	if h.b.limit-h.b.count() < 7 {
		return h.check("full_pki_budget", errors.New("insufficient request reserve"))
	}
	client, err := h.sdk.PKI(h.c.pkiMount)
	if err != nil {
		return h.check("full_pki_factory", err)
	}
	i := len(h.output.Resources)
	h.output.Resources = append(h.output.Resources, resource{Kind: "certificate", Mount: h.c.pkiMount, Path: h.c.pkiRole, State: "issuance_pending"})
	if !h.save() {
		return false
	}
	issued, err := client.Issue(h.ctx, pki.IssueRequest{Role: h.c.pkiRole, CommonName: h.c.pkiDNS, TTL: 5 * time.Minute})
	var rootPEM []byte
	if err == nil {
		defer issued.PrivateKey.Zero()
		h.output.Resources[i].Path = issued.Certificate.SerialNumber
		h.output.Resources[i].State = "retained_until_expiry"
		if !h.save() {
			return false
		}
		defer func() {
			if h.journalFailed {
				ok = false
				return
			}
			h.output.Resources[i].State = "revocation_pending"
			if !h.save() {
				ok = false
				return
			}
			revoked, revokeErr := client.Revoke(h.ctx, issued.Certificate.SerialNumber)
			if revokeErr == nil && revoked.RevokedAt.IsZero() {
				revokeErr = errors.New("revocation time missing")
			}
			if !h.check("pki_issue_revoke", revokeErr) {
				ok = false
				return
			}
			h.output.Resources[i].State = "revoked_record_retained"
			if !h.save() {
				ok = false
			}
		}()
		var readErr error
		rootPEM, readErr = os.ReadFile(h.c.pkiRoot)
		if readErr != nil {
			err = errors.New("trusted root unavailable")
		} else {
			err = pki.ValidateBundle(*issued, pki.VerifyPolicy{TrustedRootsPEM: [][]byte{rootPEM}, RequiredEKUs: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, ExpectedCN: h.c.pkiDNS, ExpectedDNS: []string{h.c.pkiDNS}})
		}
	}
	if !h.check("pki_issue", err) {
		return false
	}
	record, err := client.ReadCertificate(h.ctx, issued.Certificate.SerialNumber)
	if err == nil && !bytes.Equal(record.CertificatePEM, issued.Certificate.CertificatePEM) {
		err = errors.New("certificate read mismatch")
	}
	if !h.check("pki_read_certificate", err) {
		return false
	}
	chain, err := client.ReadIssuerChain(h.ctx)
	if err == nil {
		rootBlock, _ := pem.Decode(rootPEM)
		found := false
		for _, certificate := range chain.CertificatesPEM {
			block, _ := pem.Decode(certificate)
			if rootBlock != nil && block != nil && bytes.Equal(rootBlock.Bytes, block.Bytes) {
				found = true
			}
		}
		if !found {
			err = errors.New("issuer chain omits this run's CA")
		}
	}
	if !h.check("pki_read_issuer_chain", err) {
		return false
	}
	return true
}

type fullSecretProvider struct{ value string }

func (p fullSecretProvider) Current(ctx context.Context) (auth.SecretIDSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return auth.SecretIDSnapshot{}, err
	}
	return auth.SecretIDSnapshot{SecretID: sensitive.NewBytes([]byte(p.value)), Generation: "remote-test-fixture", Use: auth.ReusableSecretID}, nil
}

func (h *harness) testFullAppRole() bool {
	if h.b.limit-h.b.count() < 4 {
		return h.check("approle_budget", errors.New("insufficient request reserve"))
	}
	roleID := sensitive.NewBytes([]byte(h.c.appRoleRoleID))
	defer roleID.Zero()
	client, err := bao.New(bao.Config{
		Address: h.c.address, ClusterAlias: "remote-test", Namespace: bao.NamespaceConfig{Mode: bao.NamespaceMode(h.c.namespaceMode), Path: h.c.namespace},
		Auth:     auth.Config{Mode: auth.ManagedAppRole, AppRole: &auth.AppRoleConfig{Mount: strings.TrimPrefix(h.c.appRoleMount, "auth/"), RoleID: roleID, SecretIDProvider: fullSecretProvider{value: h.c.appRoleSecretID}}},
		TLS:      bao.TLSConfig{CAFile: h.c.caFile, ClientCertFile: h.c.certFile, ClientKeyFile: h.c.keyFile},
		Timeouts: bao.TimeoutConfig{Request: 5 * time.Second, Dial: 3 * time.Second, TLSHandshake: 3 * time.Second},
		Limits:   bao.LimitConfig{MaxConcurrentRequests: 1}, ReadRetry: bao.ReadRetryConfig{MaxAttempts: 1},
	}, bao.WithObserver(h.b))
	if err != nil {
		return h.check("approle_start", err)
	}
	if !h.check("approle_start", client.Start(h.ctx)) {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = client.Close(closeCtx)
		return false
	}
	ready, err := client.CheckReady(h.ctx, diagnostics.ReadProbe{Mount: h.c.mount, Path: h.c.path, Version: h.c.version})
	if err == nil && !ready.Ready {
		err = errors.New("AppRole read probe not ready")
	}
	good := h.check("approle_ready", err)
	closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	closed := h.check("approle_close", client.Close(closeCtx))
	return good && closed
}
