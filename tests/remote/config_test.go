package remotecheck

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testManifest(t *testing.T, change func(map[string]any)) (string, string) {
	t.Helper()
	raw, err := os.ReadFile("../../deploy/test/remote-manifest.example.json")
	if err != nil {
		t.Fatal(err)
	}
	var plan map[string]any
	if json.Unmarshal(raw, &plan) != nil {
		t.Fatal("invalid fixture")
	}
	now := time.Now().UTC().Truncate(time.Second)
	plan["issued_at"] = now.Add(-time.Second).Format(time.RFC3339)
	plan["expires_at"] = now.Add(19 * time.Minute).Format(time.RFC3339)
	plan["run_id"] = "remote-sdk-20261001-0123456789abcdef"
	plan["resource_prefix"] = "sdk-20261001-0123456789abcdef"
	if change != nil {
		change(plan)
	}
	raw, err = json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "manifest.json")
	if os.WriteFile(path, raw, 0600) != nil {
		t.Fatal("fixture write failed")
	}
	digest := sha256.Sum256(raw)
	return path, hex.EncodeToString(digest[:])
}

func fixtureEnv() map[string]string {
	return map[string]string{
		"BAO_REMOTE_ADDRESS": "https://kms.jiup9.com:443", "BAO_REMOTE_CONFIRMED": "yes",
		"BAO_REMOTE_ENVIRONMENT": "nonproduction", "BAO_REMOTE_NAMESPACE_MODE": "named",
		"BAO_REMOTE_NAMESPACE": "sdk-test", "BAO_REMOTE_TOKEN": "synthetic-token",
		"BAO_REMOTE_MODE": "readonly", "BAO_REMOTE_KV_MOUNT": "kv", "BAO_REMOTE_KV_PATH": "fixture/check",
		"BAO_REMOTE_KV_VERSION": "1", "BAO_REMOTE_KV_MARKER": "synthetic-marker",
		"BAO_REMOTE_RUN_ID": "local-fixture",
	}
}

func TestConfigRequiresExplicitInputs(t *testing.T) {
	for _, key := range []string{"BAO_REMOTE_TOKEN", "BAO_REMOTE_NAMESPACE_MODE", "BAO_REMOTE_CONFIRMED", "BAO_REMOTE_ENVIRONMENT", "BAO_REMOTE_KV_MOUNT", "BAO_REMOTE_KV_VERSION"} {
		e := fixtureEnv()
		delete(e, key)
		if _, err := loadConfig(func(k string) string { return e[k] }); err == nil {
			t.Errorf("missing %s accepted", key)
		}
	}
}

func TestConfigRejectsTargetsAndTraversal(t *testing.T) {
	for key, values := range map[string][]string{
		"BAO_REMOTE_ADDRESS":   {"http://kms.jiup9.com", "https://other.invalid", "https://kms.jiup9.com:8443", "https://kms.jiup9.com@other.invalid", "https://kms.jiup9.com?x=1"},
		"BAO_REMOTE_KV_PATH":   {"../private", "a/%2f/b", "a\\b"},
		"BAO_REMOTE_MODE":      {"all", "write", ""},
		"BAO_REMOTE_NAMESPACE": {"", "../root"},
	} {
		for _, value := range values {
			e := fixtureEnv()
			e[key] = value
			if _, err := loadConfig(func(k string) string { return e[k] }); err == nil {
				t.Errorf("accepted invalid %s", key)
			}
		}
	}
}

func TestWritesRequirePrefixAndCleanupAuthority(t *testing.T) {
	e := fixtureEnv()
	e["BAO_REMOTE_MODE"] = "isolated"
	if _, err := loadConfig(func(k string) string { return e[k] }); err == nil {
		t.Fatal("write without authority")
	}
	e["BAO_REMOTE_WRITE_PREFIX"] = "sdk-validation"
	e["BAO_REMOTE_ALLOW_WRITE"] = "yes"
	e["BAO_REMOTE_ALLOW_CLEANUP"] = "yes"
	if _, err := loadConfig(func(k string) string { return e[k] }); err != nil {
		t.Fatal("valid synthetic config rejected")
	}
}

func TestFullRemoteModeRequiresDedicatedResources(t *testing.T) {
	base := fixtureEnv()
	base["BAO_REMOTE_MODE"] = "isolated"
	base["BAO_REMOTE_WRITE_PREFIX"] = "sdk-validation"
	base["BAO_REMOTE_RUN_ID"] = "remote-sdk-20261001-0123456789abcdef-core"
	base["BAO_REMOTE_KV_PATH"] = "sdk-validation/remote-sdk-20261001-0123456789abcdef-core/seed"
	base["BAO_REMOTE_KV_MOUNT"] = "sdk-20261001-0123456789abcdef-kv"
	base["BAO_REMOTE_KV_MARKER"] = "owned-remote-sdk-20261001-0123456789abcdef"
	base["BAO_REMOTE_ALLOW_WRITE"] = "yes"
	base["BAO_REMOTE_ALLOW_CLEANUP"] = "yes"
	base["BAO_REMOTE_ALLOW_SOFT_DELETE"] = "yes"
	base["BAO_REMOTE_FULL"] = "yes"
	base["BAO_REMOTE_FULL_PART"] = "core"
	base["BAO_REMOTE_MANIFEST"], base["BAO_REMOTE_MANIFEST_SHA256"] = testManifest(t, nil)
	if _, err := loadConfig(func(k string) string { return base[k] }); err == nil {
		t.Fatal("full remote mode accepted without dedicated resources")
	}
	base["BAO_REMOTE_TRANSIT_MOUNT"] = "sdk-20261001-0123456789abcdef-transit"
	base["BAO_REMOTE_TRANSIT_KEY"] = "cipher"
	base["BAO_REMOTE_TRANSIT_TYPE"] = "aes256-gcm96"
	base["BAO_REMOTE_TRANSIT_SIGN_KEY"] = "sign"
	base["BAO_REMOTE_TRANSIT_HMAC_KEY"] = "mac"
	base["BAO_REMOTE_PKI_MOUNT"] = "sdk-20261001-0123456789abcdef-pki"
	base["BAO_REMOTE_PKI_ROLE"] = "device"
	base["BAO_REMOTE_PKI_DNS"] = "node.sdk-test.invalid"
	base["BAO_REMOTE_PKI_ROOT_FILE"] = "synthetic-root.pem"
	base["BAO_REMOTE_ALLOW_SIGN_CSR"] = "yes"
	base["BAO_REMOTE_ALLOW_REVOKE"] = "yes"
	base["BAO_REMOTE_APPROLE_MOUNT"] = "sdk-20261001-0123456789abcdef-approle"
	base["BAO_REMOTE_APPROLE_ROLE_ID"] = "synthetic-role-id"
	base["BAO_REMOTE_APPROLE_SECRET_ID"] = "synthetic-secret-id"
	base["BAO_REMOTE_NEGATIVE_TOKEN"] = "synthetic-negative-token"
	base["BAO_REMOTE_NEGATIVE_MOUNT"] = base["BAO_REMOTE_KV_MOUNT"]
	base["BAO_REMOTE_NEGATIVE_PATH"] = base["BAO_REMOTE_KV_PATH"]
	if _, err := loadConfig(func(k string) string { return base[k] }); err != nil {
		t.Fatal("valid full synthetic config rejected")
	}
	old := make(map[string]string, len(base))
	for k, v := range base {
		old[k] = v
	}
	old["BAO_REMOTE_RUN_ID"] = "remote-sdk-full-20260930-06-core"
	old["BAO_REMOTE_KV_PATH"] = "sdk-validation/remote-sdk-full-20260930-06-core/seed"
	old["BAO_REMOTE_NEGATIVE_PATH"] = old["BAO_REMOTE_KV_PATH"]
	old["BAO_REMOTE_KV_MARKER"] = "owned-remote-sdk-full-20260930-06"
	if _, err := loadConfig(func(k string) string { return old[k] }); err == nil {
		t.Fatal("previous full run identifier accepted")
	}
	for _, key := range []string{"BAO_REMOTE_TRANSIT_SIGN_KEY", "BAO_REMOTE_TRANSIT_HMAC_KEY", "BAO_REMOTE_APPROLE_SECRET_ID"} {
		e := make(map[string]string, len(base))
		for k, v := range base {
			e[k] = v
		}
		delete(e, key)
		if _, err := loadConfig(func(k string) string { return e[k] }); err == nil {
			t.Errorf("missing %s accepted", key)
		}
	}
	base["BAO_REMOTE_APPROLE_MOUNT"] = "../root"
	if _, err := loadConfig(func(k string) string { return base[k] }); err == nil {
		t.Fatal("traversal in AppRole mount accepted")
	}
	base["BAO_REMOTE_APPROLE_MOUNT"] = "sdk-20261001-0123456789abcdef-approle"
	for _, part := range []string{"", "all", "transit"} {
		base["BAO_REMOTE_FULL_PART"] = part
		if part == "transit" {
			base["BAO_REMOTE_RUN_ID"] = "remote-sdk-20261001-0123456789abcdef-transit"
			base["BAO_REMOTE_KV_PATH"] = "sdk-validation/remote-sdk-20261001-0123456789abcdef-transit/seed"
			base["BAO_REMOTE_NEGATIVE_PATH"] = base["BAO_REMOTE_KV_PATH"]
			if _, err := loadConfig(func(k string) string { return base[k] }); err != nil {
				t.Fatal("transit-only full part rejected")
			}
			continue
		}
		if _, err := loadConfig(func(k string) string { return base[k] }); err == nil {
			t.Errorf("invalid full part %q accepted", part)
		}
	}
	base["BAO_REMOTE_FULL_PART"] = "core"
	base["BAO_REMOTE_RUN_ID"] = "remote-sdk-20261001-0123456789abcdef-core"
	base["BAO_REMOTE_KV_PATH"] = "sdk-validation/remote-sdk-20261001-0123456789abcdef-core/seed"
	base["BAO_REMOTE_NEGATIVE_PATH"] = base["BAO_REMOTE_KV_PATH"]
	for _, namespace := range []string{"root", "other"} {
		base["BAO_REMOTE_NAMESPACE_MODE"] = "named"
		base["BAO_REMOTE_NAMESPACE"] = namespace
		if _, err := loadConfig(func(k string) string { return base[k] }); err == nil {
			t.Errorf("full mode accepted namespace %s", namespace)
		}
	}
}

func TestFullRemotePermissionsStayOnConfiguredMounts(t *testing.T) {
	c := configuration{mode: "isolated", full: true, fullPart: "core", mount: "owned-kv", path: "sdk-validation/full-run/seed", prefix: "sdk-validation", runID: "full-run",
		transitMount: "owned-transit", transitKey: "cipher", transitSignKey: "sign", transitHMACKey: "mac", transitType: "aes256-gcm96",
		pkiMount: "owned-pki", pkiRole: "device", softDelete: true, pkiRevoke: true}
	h := harness{c: c}
	permissions := h.requiredPermissions()
	for path := range permissions {
		if len(path) >= 4 && path[:4] == "sys/" {
			t.Fatalf("system path in runtime permissions: %s", path)
		}
	}
	for _, path := range []string{"owned-kv/metadata/sdk-validation/full-run", "owned-pki/issue/device", "owned-pki/ca_chain"} {
		if _, ok := permissions[path]; !ok {
			t.Errorf("missing full-mode permission: %s", path)
		}
	}
	c.fullPart = "transit"
	c.runID = "full-transit"
	c.path = "sdk-validation/full-transit/seed"
	transitPermissions := (&harness{c: c}).requiredPermissions()
	for _, path := range []string{"owned-kv/data/sdk-validation/full-transit/seed", "owned-transit/rewrap/cipher",
		"owned-transit/sign/sign", "owned-transit/hmac/mac/sha2-256"} {
		if _, ok := transitPermissions[path]; !ok {
			t.Errorf("missing transit-only permission: %s", path)
		}
	}
	for _, path := range []string{"owned-kv/data/sdk-validation/full-transit/kv",
		"owned-kv/metadata/sdk-validation/full-transit/kv", "owned-kv/metadata/sdk-validation/full-transit",
		"owned-pki/issue/device"} {
		if _, ok := transitPermissions[path]; ok {
			t.Errorf("transit-only run requests unused permission: %s", path)
		}
	}
}
