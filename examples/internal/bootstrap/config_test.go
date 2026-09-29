package bootstrap

import "testing"

func TestExplicitExampleConfiguration(t *testing.T) {
	values := map[string]string{"SDK_BAO_ADDRESS": "https://127.0.0.1:8200", "SDK_BAO_CLUSTER": "isolated", "SDK_BAO_NAMESPACE_MODE": "root", "SDK_BAO_TOKEN_FILE": "/tmp/controlled-token-file"}
	cfg, e := ConfigFrom(func(k string) string { return values[k] })
	if e != nil {
		t.Fatal(e)
	}
	if cfg.Namespace.Mode != "root" || cfg.Address != values["SDK_BAO_ADDRESS"] {
		t.Fatal("explicit config not preserved")
	}
	delete(values, "SDK_BAO_NAMESPACE_MODE")
	if _, e = ConfigFrom(func(k string) string { return values[k] }); e == nil {
		t.Fatal("namespace defaulted")
	}
	values["SDK_BAO_NAMESPACE_MODE"] = "named"
	if _, e = ConfigFrom(func(k string) string { return values[k] }); e == nil {
		t.Fatal("named namespace missing")
	}
}
