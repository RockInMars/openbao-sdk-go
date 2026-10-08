// Package pki contains the public SDK contract types.
package pki

import (
	"crypto/x509"
	"github.com/RockInMars/openbao-sdk-go/sensitive"
	"net/netip"
	"time"
)

type IssueRequest struct {
	Role              string
	CommonName        string
	DNSNames          []string
	EmailNames        []string
	IPAddresses       []netip.Addr
	URISANs           []string
	TTL               time.Duration
	ExcludeCNFromSANs bool
}
type SignCSRRequest struct {
	Role   string
	CSRPEM sensitive.Bytes
	TTL    time.Duration
}
type Certificate struct {
	CertificatePEM    []byte
	IssuingCAPEM      []byte
	CAChainPEM        [][]byte
	SerialNumber      string
	NotBefore         time.Time
	NotAfter          time.Time
	FingerprintSHA256 string
}
type IssuedCertificate struct {
	Certificate Certificate
	PrivateKey  sensitive.Bytes // PKCS#8 PEM。
	RequestID   string
}
type SignedCertificate struct {
	Certificate Certificate // 不包含私钥字段。
	RequestID   string
}
type CertificateRecord struct {
	CertificatePEM []byte
	SerialNumber   string
	NotBefore      time.Time
	NotAfter       time.Time
	RevokedAt      *time.Time // 只在服务器返回相应证据时填。
	RequestID      string
}
type ChainResult struct {
	CertificatesPEM [][]byte
	RequestID       string
}
type RevokeResult struct {
	RevokedAt time.Time
	RequestID string
}
type VerifyPolicy struct {
	TrustedRootsPEM [][]byte  // 调用方受信配置；不是响应随附根。
	CurrentTime     time.Time // 零值使用当前时间。
	RequiredEKUs    []x509.ExtKeyUsage
	ExpectedCN      string
	ExpectedDNS     []string
	ExpectedURI     []string
	MinRemainingTTL time.Duration
}
