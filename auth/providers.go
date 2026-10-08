package auth

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/RockInMars/openbao-sdk-go/baoerr"
	"github.com/RockInMars/openbao-sdk-go/sensitive"
)

const maxCredentialBytes = 64 << 10

var generationKey struct {
	once sync.Once
	key  [32]byte
	err  error
}

func credentialError(code string) error {
	return &baoerr.Error{Code: code, Effect: baoerr.EffectNone, Operation: "credential.provider", Message: "credential provider failed"}
}
func contextFailure(ctx context.Context) error {
	if ctx == nil {
		return credentialError(baoerr.CodeInvalidArgument)
	}
	if ctx.Err() == context.Canceled {
		return credentialError(baoerr.CodeCanceled)
	}
	if ctx.Err() == context.DeadlineExceeded {
		return credentialError(baoerr.CodeDeadlineExceeded)
	}
	return nil
}
func validCredential(b []byte) bool {
	if len(b) == 0 || len(b) > maxCredentialBytes {
		return false
	}
	for _, c := range b {
		if c < 33 || c > 126 {
			return false
		}
	}
	return true
}
func generation(value []byte) (string, error) {
	generationKey.once.Do(func() { _, generationKey.err = rand.Read(generationKey.key[:]) })
	if generationKey.err != nil {
		return "", credentialError(baoerr.CodeUnavailable)
	}
	m := hmac.New(sha256.New, generationKey.key[:])
	m.Write(value)
	return hex.EncodeToString(m.Sum(nil)), nil
}

type staticToken struct {
	value      sensitive.Bytes
	generation string
}

// NewStaticToken copies the token; it does not assume an unlimited lifetime or renew it.
// Each Snapshot returns an independent value owned by its consumer. Direct callers
// must Zero that snapshot after use. Closing a client does not erase the provider,
// which may be shared by other clients.
func NewStaticToken(token sensitive.Bytes) (TokenProvider, error) {
	raw := token.RevealCopy()
	defer clear(raw)
	if !validCredential(raw) {
		return nil, credentialError(baoerr.CodeInvalidArgument)
	}
	g, err := generation(raw)
	if err != nil {
		return nil, err
	}
	return &staticToken{value: sensitive.NewBytes(raw), generation: g}, nil
}
func (p *staticToken) Snapshot(ctx context.Context) (TokenSnapshot, error) {
	if err := contextFailure(ctx); err != nil {
		return TokenSnapshot{}, err
	}
	raw := p.value.RevealCopy()
	defer clear(raw)
	return TokenSnapshot{Token: sensitive.NewBytes(raw), Generation: p.generation}, nil
}

// SnapshotOwnedByConsumer checks exact identity so an embedding wrapper cannot
// accidentally transfer ownership of a different Snapshot implementation.
func (p *staticToken) SnapshotOwnedByConsumer(actual any) bool {
	q, ok := actual.(*staticToken)
	return ok && q == p
}

type tokenFile struct{ path string }
type secretIDFile struct {
	path string
	use  SecretIDUse
}

func validFilePath(path string) bool {
	return strings.TrimSpace(path) != "" && !strings.ContainsAny(path, "\x00\r\n")
}

// NewTokenFile accepts a trusted deployment path, including trusted symlink mounts.
// The path's parent directory must not be writable by an untrusted local principal.
// Each successful Snapshot is independent and must be Zeroed by its consumer.
func NewTokenFile(path string) (TokenProvider, error) {
	if !validFilePath(path) {
		return nil, credentialError(baoerr.CodeInvalidArgument)
	}
	return &tokenFile{path: path}, nil
}

// NewSecretIDFile reads a trusted deployment file on each Current call. Each
// successful snapshot is independent and must be Zeroed by its consumer.
func NewSecretIDFile(path string, use SecretIDUse) (SecretIDProvider, error) {
	if !validFilePath(path) || (use != ReusableSecretID && use != SingleUseSecretID) {
		return nil, credentialError(baoerr.CodeInvalidArgument)
	}
	return &secretIDFile{path: path, use: use}, nil
}
func readCredential(ctx context.Context, path string) (sensitive.Bytes, string, error) {
	if err := contextFailure(ctx); err != nil {
		return sensitive.Bytes{}, "", err
	}
	// Preflight excludes FIFOs/devices before opening. The opened descriptor is
	// checked again, so atomic replacement cannot bypass the actual size/type check.
	st, err := os.Stat(path)
	if err != nil || !st.Mode().IsRegular() {
		return sensitive.Bytes{}, "", credentialError(baoerr.CodeAuthenticationFailed)
	}
	f, err := os.Open(path)
	if err != nil {
		return sensitive.Bytes{}, "", credentialError(baoerr.CodeAuthenticationFailed)
	}
	defer f.Close()
	st, err = f.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Size() > maxCredentialBytes {
		return sensitive.Bytes{}, "", credentialError(baoerr.CodeAuthenticationFailed)
	}
	raw, err := io.ReadAll(io.LimitReader(f, maxCredentialBytes+1))
	defer clear(raw)
	if err != nil || len(raw) > maxCredentialBytes {
		return sensitive.Bytes{}, "", credentialError(baoerr.CodeAuthenticationFailed)
	}
	if err = contextFailure(ctx); err != nil {
		return sensitive.Bytes{}, "", err
	}
	value := bytes.TrimSpace(raw)
	if !validCredential(value) {
		return sensitive.Bytes{}, "", credentialError(baoerr.CodeAuthenticationFailed)
	}
	g, err := generation(value)
	if err != nil {
		return sensitive.Bytes{}, "", err
	}
	return sensitive.NewBytes(value), g, nil
}
func (p *tokenFile) Snapshot(ctx context.Context) (TokenSnapshot, error) {
	b, g, err := readCredential(ctx, p.path)
	if err != nil {
		return TokenSnapshot{}, err
	}
	return TokenSnapshot{Token: b, Generation: g}, nil
}

func (p *tokenFile) SnapshotOwnedByConsumer(actual any) bool {
	q, ok := actual.(*tokenFile)
	return ok && q == p
}

func (p *secretIDFile) Current(ctx context.Context) (SecretIDSnapshot, error) {
	b, g, err := readCredential(ctx, p.path)
	if err != nil {
		return SecretIDSnapshot{}, err
	}
	return SecretIDSnapshot{SecretID: b, Generation: g, Use: p.use}, nil
}

func (p *secretIDFile) SnapshotOwnedByConsumer(actual any) bool {
	q, ok := actual.(*secretIDFile)
	return ok && q == p
}
