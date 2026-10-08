package auth

import (
	"context"
	"github.com/RockInMars/openbao-sdk-go/sensitive"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStaticTokenSnapshots(t *testing.T) {
	b := sensitive.NewBytes([]byte("fixture-token"))
	p, err := NewStaticToken(b)
	if err != nil {
		t.Fatal(err)
	}
	b.Zero()
	a, err := p.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	raw := a.Token.RevealCopy()
	if string(raw) != "fixture-token" {
		t.Fatal("static input ownership")
	}
	clear(raw)
	a.Token.Zero()
	z, err := p.Snapshot(context.Background())
	if err != nil || z.Token.Len() == 0 {
		t.Fatal("snapshots share mutable secret")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = p.Snapshot(ctx); err == nil {
		t.Fatal("canceled provider succeeded")
	}
	if _, err = NewStaticToken(sensitive.NewBytes([]byte("bad\ntoken"))); err == nil {
		t.Fatal("accepted control character")
	}
}
func TestTokenFileAtomicReplacement(t *testing.T) {
	d := t.TempDir()
	path := filepath.Join(d, "token")
	if err := os.WriteFile(path, []byte(" fixture-token \n"), 0600); err != nil {
		t.Fatal(err)
	}
	p, err := NewTokenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	a, err := p.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	next := filepath.Join(d, "next")
	os.WriteFile(next, []byte("fixture-token"), 0600)
	os.Rename(next, path)
	b, err := p.Snapshot(context.Background())
	if err != nil || a.Generation != b.Generation {
		t.Fatal("same content changed generation")
	}
	os.WriteFile(next, []byte("fixture-replacement"), 0600)
	os.Rename(next, path)
	c, err := p.Snapshot(context.Background())
	if err != nil || a.Generation == c.Generation {
		t.Fatal("new content did not change generation")
	}
	os.Remove(path)
	if _, err = p.Snapshot(context.Background()); err == nil {
		t.Fatal("stale token silently reused")
	}
}
func TestTokenFileValidation(t *testing.T) {
	d := t.TempDir()
	path := filepath.Join(d, "token")
	p, err := NewTokenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"", "\n \t", "internal space", "private\x00fixture", strings.Repeat("x", 65537)} {
		os.WriteFile(path, []byte(raw), 0600)
		_, e := p.Snapshot(context.Background())
		if e == nil {
			t.Fatal("invalid token accepted")
		}
		if raw != "" && strings.Contains(e.Error(), raw) {
			t.Fatal("secret included in error")
		}
	}
	if _, err = NewTokenFile(""); err == nil {
		t.Fatal("empty file name accepted")
	}
	os.Remove(path)
	os.Mkdir(path, 0700)
	if _, err = p.Snapshot(context.Background()); err == nil {
		t.Fatal("directory accepted")
	}
}
func TestSecretIDGenerationAndUse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sid")
	os.WriteFile(path, []byte("fixture-secret-id"), 0600)
	p, err := NewSecretIDFile(path, SingleUseSecretID)
	if err != nil {
		t.Fatal(err)
	}
	a, err := p.Current(context.Background())
	if err != nil || a.Use != SingleUseSecretID || a.Generation == "" {
		t.Fatal("missing secret-id snapshot")
	}
	os.WriteFile(path, []byte("fixture-secret-id"), 0600)
	b, err := p.Current(context.Background())
	if err != nil || a.Generation != b.Generation {
		t.Fatal("generation not content based")
	}
	if _, err = NewSecretIDFile(path, ""); err == nil {
		t.Fatal("implicit use mode")
	}
}
