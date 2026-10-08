//go:build remote

package remotecheck

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// This test is absent from normal builds. The CLI compiles it without credentials,
// then explicitly enables just this target in the credential-bearing process.
func TestRemoteService(t *testing.T) {
	if os.Getenv("BAO_REMOTE_EXECUTE") != "yes" {
		t.Fatal("remote execution was not enabled")
	}
	c, err := loadConfig(os.Getenv)
	if err != nil {
		t.Fatal("remote configuration rejected")
	}
	directory := os.Getenv("BAO_REMOTE_OUTPUT_DIR")
	if directory == "" {
		t.Fatal("evidence directory required")
	}
	seq := 0
	persist := func(r result) error {
		seq++
		path := filepath.Join(directory, fmt.Sprintf("checkpoint-%03d.json", seq))
		return writeResult(path, r)
	}
	h, err := newHarness(c, persist)
	if err != nil {
		t.Fatal("remote initialization rejected")
	}
	r := h.run()
	if writeResult(filepath.Join(directory, "result.json"), r) != nil {
		t.Fatal("result persistence failed")
	}
	if r.Status != "PASS" {
		t.Fatal("remote checks did not pass; inspect structured evidence")
	}
}

func writeResult(path string, r result) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	err = json.NewEncoder(f).Encode(r)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}
