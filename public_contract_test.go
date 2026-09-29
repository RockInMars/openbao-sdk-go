package bao_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	bao "git.example.com/infra/openbao-sdk-go"
	"git.example.com/infra/openbao-sdk-go/diagnostics"
	"git.example.com/infra/openbao-sdk-go/kv"
	"git.example.com/infra/openbao-sdk-go/pki"
	"git.example.com/infra/openbao-sdk-go/transit"
)

// These compile-time consumer contracts pin every specified runtime method.
var _ interface {
	Start(context.Context) error
	Close(context.Context) error
	State() diagnostics.ClientState
	KVv2(string) (*bao.KVClient, error)
	PKI(string) (*bao.PKIClient, error)
	Transit(string) (*bao.TransitClient, error)
	ClusterHealth(context.Context) (*diagnostics.Health, error)
	CheckReady(context.Context, diagnostics.ReadProbe) (*diagnostics.Readiness, error)
} = (*bao.Client)(nil)
var _ interface {
	Create(context.Context, string, kv.Document) (*kv.WriteResult, error)
	CompareAndSwap(context.Context, string, int, kv.Document) (*kv.WriteResult, error)
	ReadVersion(context.Context, string, int) (*kv.ReadResult, error)
	ReadLatest(context.Context, string) (*kv.ReadResult, error)
	ReadRef(context.Context, kv.Ref) (*kv.ReadResult, error)
	ReadMetadata(context.Context, string) (*kv.Metadata, error)
	List(context.Context, string) (*kv.ListResult, error)
	DeleteVersions(context.Context, string, []int) error
	UndeleteVersions(context.Context, string, []int) error
} = (*bao.KVClient)(nil)
var _ interface {
	Issue(context.Context, pki.IssueRequest) (*pki.IssuedCertificate, error)
	SignCSR(context.Context, pki.SignCSRRequest) (*pki.SignedCertificate, error)
	ReadCertificate(context.Context, string) (*pki.CertificateRecord, error)
	ReadIssuerChain(context.Context) (*pki.ChainResult, error)
	Revoke(context.Context, string) (*pki.RevokeResult, error)
} = (*bao.PKIClient)(nil)
var _ interface {
	Sign(context.Context, transit.SignRequest) (*transit.SignResult, error)
	SignDigest(context.Context, transit.SignDigestRequest) (*transit.SignResult, error)
	Verify(context.Context, transit.VerifyRequest) (*transit.VerifyResult, error)
	VerifyDigest(context.Context, transit.VerifyDigestRequest) (*transit.VerifyResult, error)
	ReadKeyMetadata(context.Context, string) (*transit.KeyMetadata, error)
	ReadPublicKey(context.Context, string, int) (*transit.PublicKeyResult, error)
	Encrypt(context.Context, transit.EncryptRequest) (*transit.CipherResult, error)
	Decrypt(context.Context, transit.DecryptRequest) (*transit.DecryptResult, error)
	Rewrap(context.Context, transit.RewrapRequest) (*transit.CipherResult, error)
	HMAC(context.Context, transit.HMACRequest) (*transit.HMACResult, error)
	HMACVerify(context.Context, transit.HMACVerifyRequest) (*transit.VerifyResult, error)
} = (*bao.TransitClient)(nil)

func TestPublicRuntimeHasNoAdminEscape(t *testing.T) {
	for _, v := range []any{(*bao.Client)(nil), (*bao.KVClient)(nil), (*bao.PKIClient)(nil), (*bao.TransitClient)(nil)} {
		typ := reflect.TypeOf(v)
		for i := 0; i < typ.NumMethod(); i++ {
			name := typ.Method(i).Name
			for _, forbidden := range []string{"Raw", "Export", "Destroy", "CreateKey", "Rotate", "Unseal", "SetToken", "SetNamespace"} {
				if strings.Contains(name, forbidden) {
					t.Fatal("unexpected runtime administrative escape")
				}
			}
		}
	}
}
