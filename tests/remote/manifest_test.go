package remotecheck

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"
	"time"
)

func manifestConfig() configuration {
	return configuration{address: "https://kms.jiup9.com:443", environment: "nonproduction", namespaceMode: "named", namespace: "sdk-test",
		runID: "remote-sdk-20261001-0123456789abcdef-core", fullPart: "core", mount: "sdk-20261001-0123456789abcdef-kv",
		transitMount: "sdk-20261001-0123456789abcdef-transit", pkiMount: "sdk-20261001-0123456789abcdef-pki",
		appRoleMount: "sdk-20261001-0123456789abcdef-approle", marker: "owned-remote-sdk-20261001-0123456789abcdef"}
}

func TestManifestRejectsScopeBudgetAndTimeBeforeHarness(t *testing.T) {
	for name, change := range map[string]func(map[string]any){
		"target":     func(m map[string]any) { m["target"] = "https://other.invalid" },
		"allowlist":  func(m map[string]any) { m["target_allowlist"] = []string{"https://other.invalid"} },
		"namespace":  func(m map[string]any) { m["namespace"] = "root" },
		"prefix":     func(m map[string]any) { m["resource_prefix"] = "business" },
		"extra":      func(m map[string]any) { m["credential"] = "synthetic" },
		"budget":     func(m map[string]any) { m["request_budget"].(map[string]any)["total"] = 121 },
		"resources":  func(m map[string]any) { m["resource_budget"] = 10 },
		"operations": func(m map[string]any) { m["allowed_operations"] = []string{"all"} },
		"expired": func(m map[string]any) {
			m["issued_at"] = "2000-01-01T00:00:00Z"
			m["expires_at"] = "2000-01-01T00:20:00Z"
		},
		"future": func(m map[string]any) {
			m["issued_at"] = "2999-01-01T00:00:00Z"
			m["expires_at"] = "2999-01-01T00:20:00Z"
		},
		"window": func(m map[string]any) { m["expires_at"] = time.Now().UTC().Add(time.Hour).Format(time.RFC3339) },
	} {
		t.Run(name, func(t *testing.T) {
			path, sha := testManifest(t, change)
			get := func(k string) string {
				if k == "BAO_REMOTE_MANIFEST" {
					return path
				}
				return sha
			}
			c := manifestConfig()
			if bindManifest(&c, get) == nil {
				t.Fatal("unsafe manifest accepted")
			}
		})
	}
}

func TestManifestBindsDigestAndRejectsDuplicateJSON(t *testing.T) {
	path, sha := testManifest(t, nil)
	get := func(k string) string {
		if k == "BAO_REMOTE_MANIFEST" {
			return path
		}
		return sha
	}
	c := manifestConfig()
	if bindManifest(&c, get) != nil || c.manifestSHA != sha || c.expiresAt.IsZero() {
		t.Fatal("valid manifest rejected")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw = append([]byte(`{"schema_version":1,`), raw[1:]...)
	if os.WriteFile(path, raw, 0600) != nil {
		t.Fatal("write failed")
	}
	if bindManifest(&c, get) == nil {
		t.Fatal("tampered digest accepted")
	}
	digest := sha256.Sum256(raw)
	sha = hex.EncodeToString(digest[:])
	if bindManifest(&c, get) == nil {
		t.Fatal("duplicate JSON accepted")
	}
}

func TestFullHarnessDeadlineUsesManifest(t *testing.T) {
	c, err := loadConfig(func(k string) string { return fixtureEnv()[k] })
	if err != nil {
		t.Fatal(err)
	}
	c.full, c.fullPart, c.manifestSHA = true, "core", "synthetic-digest"
	c.expiresAt = time.Now().Add(time.Minute)
	h, err := newHarness(c, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer h.cancel()
	deadline, _ := h.ctx.Deadline()
	if deadline.After(c.expiresAt.Add(time.Millisecond)) || time.Until(deadline) > time.Minute {
		t.Fatal("manifest deadline ignored")
	}
	h.sdk.Close(h.ctx)
	c.expiresAt = time.Now().Add(-time.Second)
	if _, err := newHarness(c, nil); err == nil {
		t.Fatal("expired manifest accepted")
	}
}
