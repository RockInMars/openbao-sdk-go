package testenv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIntegrationOptionsFailClosed(t *testing.T) {
	for _, in := range []Options{{}, {Version: "latest"}, {Version: "2.7.0", BinaryPath: "bao", BinarySHA256: strings.Repeat("a", 64)}, {Version: "2.7.0", Image: "openbao/openbao:latest"}, {Version: "2.7.0", Image: "openbao/openbao@sha256:" + strings.Repeat("a", 64), BinaryPath: "/tmp/bao"}} {
		if err := in.Validate(); err == nil {
			t.Fatal("unsafe integration options accepted")
		}
	}
}
func TestIntegrationBinaryDigest(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bao")
	if err := os.WriteFile(p, []byte("not-a-server"), 0700); err != nil {
		t.Fatal(err)
	}
	o := Options{Version: "2.7.0", BinaryPath: p, BinarySHA256: strings.Repeat("a", 64)}
	if err := o.Validate(); err == nil {
		t.Fatal("unverified binary accepted")
	}
}
func TestIntegrationPolicyIsolation(t *testing.T) {
	provision := runtimePolicy()
	control := controlPolicy()
	if strings.Contains(control, "kv/") || strings.Contains(control, "pki/") {
		t.Fatal("control identity receives terminal credentials")
	}
	for _, p := range []string{provision, control} {
		if strings.Contains(p, `path "*"`) || strings.Contains(p, "sudo") || strings.Contains(p, "sys/mounts") || strings.Contains(p, "export/") {
			t.Fatal("unsafe runtime policy")
		}
	}
	if !strings.Contains(provision, "kv/data/fixture/*") || !strings.Contains(control, "transit/sign/ecdsa") {
		t.Fatal("required least privilege paths absent")
	}
}
func TestIntegrationEnvironmentInput(t *testing.T) {
	t.Setenv("BAO_TEST_ADDRESS", "https://production.example.invalid")
	if _, err := OptionsFromEnv(); err == nil {
		t.Fatal("external address mode accepted")
	}
}
