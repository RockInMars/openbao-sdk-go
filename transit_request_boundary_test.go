package bao

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/RockInMars/openbao-sdk-go/baoerr"
	"github.com/RockInMars/openbao-sdk-go/sensitive"
	"github.com/RockInMars/openbao-sdk-go/transit"
)

type transitRequestFixture struct {
	name, action, wireBody string
	metadata, response     map[string]any
	call                   func(context.Context, *TransitClient) error
	callerInput            sensitive.Bytes
	callerBytes            []byte
}

func transitRequestFixtures(t *testing.T) []transitRequestFixture {
	t.Helper()
	messageRaw := bytes.Repeat([]byte("x"), 48)
	message := sensitive.NewBytes(messageRaw)
	digestRaw := make([]byte, 32)
	digest := sensitive.NewBytes(digestRaw)
	t.Cleanup(message.Zero)
	t.Cleanup(digest.Zero)
	encodedMessage := strings.Repeat("eHh4", 16)
	encodedDigest := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	private := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	signerMetadata := keyMeta("key", "ed25519", map[string]any{"2": map[string]any{"public_key": publicText(t, private.Public(), "ed25519")}}, false)
	digestPrivate, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	digestFixture := signFixture{profile: transit.ECDSAP256SHA256ASN1, typ: "ecdsa-p256", key: digestPrivate}
	digestMetadata := keyMeta("key", "ecdsa-p256", map[string]any{"2": map[string]any{"public_key": publicText(t, digestPrivate.Public(), "ecdsa-p256")}}, false)
	cipherMetadata := keyMeta("key", "aes256-gcm96", map[string]any{"1": 1, "2": 2}, true)
	macMetadata := keyMeta("key", "hmac", map[string]any{"1": 1, "2": 2}, false)
	wrappedSignature := "vault:v2:" + strings.Repeat("A", 86) + "=="
	wrappedDigestSignature := "vault:v2:MAYCAQECAQE="
	wrappedCipher := "vault:v1:" + strings.Repeat("A", 38) + "=="
	wrappedMAC := "vault:v2:" + encodedDigest
	valid := map[string]any{"valid": true}
	fixtures := []transitRequestFixture{
		{
			name: "sign", action: "sign", wireBody: fmt.Sprintf(`{"input":"%s","key_version":2,"prehashed":false}`, encodedMessage), metadata: signerMetadata,
			response: map[string]any{"signature": fixtureWrapped(2, ed25519.Sign(private, messageRaw)), "key_version": 2},
			call: func(ctx context.Context, client *TransitClient) error {
				_, err := client.Sign(ctx, transit.SignRequest{KeyName: "key", KeyVersion: 2, Profile: transit.Ed25519Message, Message: message})
				return err
			},
		},
		{
			name: "sign digest", action: "sign", wireBody: fmt.Sprintf(`{"hash_algorithm":"sha2-256","input":"%s","key_version":2,"marshaling_algorithm":"asn1","prehashed":true}`, encodedDigest), metadata: digestMetadata,
			response: map[string]any{"signature": fixtureWrapped(2, signLocal(t, digestFixture, digestRaw, true)), "key_version": 2},
			call: func(ctx context.Context, client *TransitClient) error {
				_, err := client.SignDigest(ctx, transit.SignDigestRequest{KeyName: "key", KeyVersion: 2, Profile: transit.ECDSAP256SHA256ASN1, Digest: digest})
				return err
			},
		},
		{
			name: "verify", action: "verify", wireBody: fmt.Sprintf(`{"input":"%s","prehashed":false,"signature":"%s"}`, encodedMessage, wrappedSignature), metadata: signerMetadata, response: valid,
			call: func(ctx context.Context, client *TransitClient) error {
				_, err := client.Verify(ctx, transit.VerifyRequest{KeyName: "key", ExpectedVersion: 2, Profile: transit.Ed25519Message, Message: message, Signature: transit.Signature{Wrapped: wrappedSignature, Version: 2, Profile: transit.Ed25519Message}})
				return err
			},
		},
		{
			name: "verify digest", action: "verify", wireBody: fmt.Sprintf(`{"hash_algorithm":"sha2-256","input":"%s","marshaling_algorithm":"asn1","prehashed":true,"signature":"%s"}`, encodedDigest, wrappedDigestSignature), metadata: digestMetadata, response: valid,
			call: func(ctx context.Context, client *TransitClient) error {
				_, err := client.VerifyDigest(ctx, transit.VerifyDigestRequest{KeyName: "key", ExpectedVersion: 2, Profile: transit.ECDSAP256SHA256ASN1, Digest: digest, Signature: transit.Signature{Wrapped: wrappedDigestSignature, Version: 2, Profile: transit.ECDSAP256SHA256ASN1}})
				return err
			},
		},
		{
			name: "encrypt", action: "encrypt", wireBody: fmt.Sprintf(`{"context":"%s","key_version":2,"plaintext":"%s"}`, encodedMessage, encodedMessage), metadata: cipherMetadata,
			response: map[string]any{"ciphertext": "vault:v2:" + strings.Repeat("A", 38) + "==", "key_version": 2},
			call: func(ctx context.Context, client *TransitClient) error {
				_, err := client.Encrypt(ctx, transit.EncryptRequest{KeyName: "key", KeyVersion: 2, Plaintext: message, Context: message})
				return err
			},
		},
		{
			name: "decrypt", action: "decrypt", wireBody: fmt.Sprintf(`{"ciphertext":"%s","context":"%s"}`, wrappedCipher, encodedMessage), metadata: cipherMetadata,
			response: map[string]any{"plaintext": encodedMessage},
			call: func(ctx context.Context, client *TransitClient) error {
				result, err := client.Decrypt(ctx, transit.DecryptRequest{KeyName: "key", Ciphertext: transit.Ciphertext{Wrapped: wrappedCipher, Version: 1}, Context: message})
				if result != nil {
					result.Plaintext.Zero()
				}
				return err
			},
		},
		{
			name: "rewrap", action: "rewrap", wireBody: fmt.Sprintf(`{"ciphertext":"%s","context":"%s","key_version":2}`, wrappedCipher, encodedMessage), metadata: cipherMetadata,
			response: map[string]any{"ciphertext": "vault:v2:" + strings.Repeat("A", 38) + "==", "key_version": 2},
			call: func(ctx context.Context, client *TransitClient) error {
				_, err := client.Rewrap(ctx, transit.RewrapRequest{KeyName: "key", TargetVersion: 2, Ciphertext: transit.Ciphertext{Wrapped: wrappedCipher, Version: 1}, Context: message})
				return err
			},
		},
		{
			name: "hmac", action: "hmac", wireBody: fmt.Sprintf(`{"input":"%s","key_version":2}`, encodedMessage), metadata: macMetadata, response: map[string]any{"hmac": wrappedMAC},
			call: func(ctx context.Context, client *TransitClient) error {
				_, err := client.HMAC(ctx, transit.HMACRequest{KeyName: "key", KeyVersion: 2, Message: message})
				return err
			},
		},
		{
			name: "hmac verify", action: "verify", wireBody: fmt.Sprintf(`{"hmac":"%s","input":"%s"}`, wrappedMAC, encodedMessage), metadata: macMetadata, response: valid,
			call: func(ctx context.Context, client *TransitClient) error {
				_, err := client.HMACVerify(ctx, transit.HMACVerifyRequest{KeyName: "key", ExpectedVersion: 2, Message: message, WrappedHMAC: wrappedMAC})
				return err
			},
		},
	}
	for index := range fixtures {
		fixtures[index].callerInput = message
		fixtures[index].callerBytes = messageRaw
		if strings.Contains(fixtures[index].name, "digest") {
			fixtures[index].callerInput = digest
			fixtures[index].callerBytes = digestRaw
		}
	}
	return fixtures
}

func TestTransitEncodedPayloadBoundary(t *testing.T) {
	for _, fixture := range transitRequestFixtures(t) {
		t.Run(fixture.name, func(t *testing.T) {
			for _, scenario := range []struct {
				name                                       string
				overLimit, metadataFails, canceled, closed bool
				code                                       string
				reads, writes                              int32
			}{
				{name: "equal limit", reads: 1, writes: 1},
				{name: "one byte over", overLimit: true, code: baoerr.CodeInvalidArgument},
				{name: "over before failed metadata", overLimit: true, metadataFails: true, code: baoerr.CodeInvalidArgument},
				{name: "over before cancellation", overLimit: true, canceled: true, code: baoerr.CodeInvalidArgument},
				{name: "equal limit canceled", canceled: true, code: baoerr.CodeCanceled},
				{name: "equal limit failed metadata", metadataFails: true, code: baoerr.CodeUnavailable, reads: 1},
				{name: "over before closed", overLimit: true, closed: true, code: baoerr.CodeInvalidArgument},
				{name: "equal limit closed", closed: true, code: baoerr.CodeClosed},
			} {
				t.Run(scenario.name, func(t *testing.T) {
					var reads, writes atomic.Int32
					provider := &countingProvider{}
					cfg := serverConfig(t, func(writer http.ResponseWriter, request *http.Request) {
						if request.Method == http.MethodGet {
							reads.Add(1)
							if request.URL.Path != "/v1/transit/keys/key" {
								t.Errorf("metadata path: %s", request.URL.Path)
							}
							if scenario.metadataFails {
								writer.WriteHeader(http.StatusServiceUnavailable)
								_, _ = writer.Write([]byte(`{"errors":["fixture unavailable"]}`))
								return
							}
							jsonData(writer, fixture.metadata)
							return
						}
						writes.Add(1)
						wantPath := "/v1/transit/" + fixture.action + "/key"
						if strings.HasPrefix(fixture.name, "hmac") {
							wantPath += "/sha2-256"
						}
						body, err := io.ReadAll(request.Body)
						defer clear(body)
						if err != nil || request.Method != http.MethodPost || request.URL.Path != wantPath || string(body) != fixture.wireBody {
							t.Errorf("unexpected business request: %s %s (%v)", request.Method, request.URL.Path, err)
						}
						jsonData(writer, fixture.response)
					})
					cfg.Auth.TokenProvider = provider
					cfg.ReadRetry.MaxAttempts = 1
					cfg.Limits.MaxRequestBytes = int64(len(fixture.wireBody))
					if scenario.overLimit {
						cfg.Limits.MaxRequestBytes--
					}
					client := startedClient(t, cfg)
					transitClient, err := client.Transit("transit")
					if err != nil {
						t.Fatal(err)
					}
					before := provider.n.Load()
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					if scenario.canceled {
						cancel()
					}
					if scenario.closed {
						if err := client.Close(context.Background()); err != nil {
							t.Fatal(err)
						}
					}
					err = fixture.call(ctx, transitClient)
					if scenario.code == "" {
						if err != nil {
							t.Fatalf("equal-limit request failed: %v", err)
						}
					} else {
						var boundaryError *baoerr.Error
						if !baoerr.IsCode(err, scenario.code) || !errors.As(err, &boundaryError) || boundaryError.Effect != baoerr.EffectNone {
							t.Errorf("expected %s / NONE, got %v", scenario.code, err)
						}
					}
					if reads.Load() != scenario.reads || writes.Load() != scenario.writes {
						t.Errorf("requests: metadata=%d business=%d, want %d/%d", reads.Load(), writes.Load(), scenario.reads, scenario.writes)
					}
					if scenario.reads == 0 && provider.n.Load() != before {
						t.Error("local rejection acquired an authentication snapshot")
					}
					remaining := fixture.callerInput.RevealCopy()
					defer clear(remaining)
					if !bytes.Equal(remaining, fixture.callerBytes) {
						t.Error("operation changed caller-owned input")
					}
				})
			}
		})
	}
}
