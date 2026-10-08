package auth

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/RockInMars/openbao-sdk-go/sensitive"
)

type snapshotOwnershipMarker interface {
	SnapshotOwnedByConsumer(any) bool
}

func TestBuiltinSnapshotOwnership(t *testing.T) {
	input := sensitive.NewBytes([]byte("fixture-token"))
	defer input.Zero()
	static, err := NewStaticToken(input)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "credential")
	if err := os.WriteFile(path, []byte("fixture-file-credential"), 0600); err != nil {
		t.Fatal(err)
	}
	file, err := NewTokenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	secret, err := NewSecretIDFile(path, ReusableSecretID)
	if err != nil {
		t.Fatal(err)
	}
	for name, provider := range map[string]any{"static": static, "token-file": file, "secret-file": secret} {
		t.Run(name, func(t *testing.T) {
			marker, ok := provider.(snapshotOwnershipMarker)
			if !ok || !marker.SnapshotOwnedByConsumer(provider) {
				t.Fatal("built-in provider did not transfer its independent snapshot")
			}
			wrapper := &struct{ snapshotOwnershipMarker }{marker}
			if wrapper.SnapshotOwnedByConsumer(wrapper) || marker.SnapshotOwnedByConsumer(nil) {
				t.Fatal("promoted ownership marker accepted another provider")
			}
		})
	}
	for _, provider := range []TokenProvider{static, file} {
		first, err := provider.Snapshot(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		second, err := provider.Snapshot(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		defer second.Token.Zero()
		first.Token.Zero()
		if second.Token.Len() == 0 {
			t.Fatal("releasing one snapshot erased another")
		}
	}
	first, err := secret.Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	second, err := secret.Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer second.SecretID.Zero()
	first.SecretID.Zero()
	if second.SecretID.Len() == 0 {
		t.Fatal("releasing one SecretID snapshot erased another")
	}
}
