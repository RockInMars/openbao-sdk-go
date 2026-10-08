// Package transit contains the public SDK contract types.
package transit

import (
	"github.com/RockInMars/openbao-sdk-go/sensitive"
)

type Profile string

const (
	ECDSAP256SHA256ASN1 Profile = "ecdsa-p256-sha256-asn1"
	RSAPSSSHA256        Profile = "rsa-pss-sha256-saltlen-hash"
	Ed25519Message      Profile = "ed25519-message"
)

type KeyRef struct {
	ClusterAlias string
	Namespace    string
	Mount        string
	Name         string
	Version      int
}
type Signature struct {
	Wrapped string // 原始 vault:vN:...；默认观察日志不输出。
	Version int
	Profile Profile
}
type SignRequest struct {
	KeyName    string
	KeyVersion int
	Profile    Profile
	Message    sensitive.Bytes
}
type SignDigestRequest struct {
	KeyName    string
	KeyVersion int
	Profile    Profile
	Digest     sensitive.Bytes
}
type VerifyRequest struct {
	KeyName         string
	ExpectedVersion int
	Profile         Profile
	Message         sensitive.Bytes
	Signature       Signature
}
type VerifyDigestRequest struct {
	KeyName         string
	ExpectedVersion int
	Profile         Profile
	Digest          sensitive.Bytes
	Signature       Signature
}
type SignResult struct {
	Key       KeyRef
	Signature Signature
	RequestID string
}
type VerifyResult struct {
	Valid     bool
	RequestID string
}
type PublicKeyResult struct {
	Key       KeyRef
	KeyType   string
	SPKIDER   []byte // 统一为 DER SubjectPublicKeyInfo。
	PEM       []byte
	RequestID string
}
type KeyMetadata struct {
	Name                 string
	Type                 string
	LatestVersion        int
	MinEncryptionVersion int
	MinDecryptionVersion int
	Exportable           bool
	DeletionAllowed      bool
	SupportsSigning      bool
	SupportsEncryption   bool
	VersionNumbers       []int
	RequestID            string
}

type Ciphertext struct {
	Wrapped string
	Version int
}
type EncryptRequest struct {
	KeyName    string
	KeyVersion int
	Plaintext  sensitive.Bytes
	Context    sensitive.Bytes // 非 derived key 必须为空。
}
type DecryptRequest struct {
	KeyName    string
	Ciphertext Ciphertext
	Context    sensitive.Bytes
}
type RewrapRequest struct {
	KeyName       string
	TargetVersion int
	Ciphertext    Ciphertext
	Context       sensitive.Bytes
}
type CipherResult struct {
	Ciphertext Ciphertext
	RequestID  string
}
type DecryptResult struct {
	Plaintext sensitive.Bytes
	RequestID string
}
type HMACRequest struct {
	KeyName    string
	KeyVersion int
	Message    sensitive.Bytes // V1 固定 SHA-256。
}
type HMACResult struct {
	Wrapped   string
	Version   int
	RequestID string
}
type HMACVerifyRequest struct {
	KeyName         string
	ExpectedVersion int
	Message         sensitive.Bytes
	WrappedHMAC     string
}
