package main

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	bao "git.example.com/infra/openbao-sdk-go"
	"git.example.com/infra/openbao-sdk-go/examples/internal/bootstrap"
	"git.example.com/infra/openbao-sdk-go/kv"
	"git.example.com/infra/openbao-sdk-go/pki"
	"os"
	"time"
)

func run(ctx context.Context, c *bao.Client) error {
	path := os.Getenv("SDK_BAO_SECRET_PATH")
	generation := os.Getenv("SDK_BAO_GENERATION")
	operation := os.Getenv("SDK_BAO_OPERATION_ID")
	cn := os.Getenv("SDK_BAO_CERT_CN")
	trustFile := os.Getenv("SDK_BAO_ISSUER_TRUST_FILE")
	if path == "" || generation == "" || operation == "" || cn == "" || trustFile == "" {
		return errors.New("explicit secret path, generation, operation, CN and independent issuer trust file required")
	}
	roots, e := os.ReadFile(trustFile)
	if e != nil {
		return errors.New("issuer trust file unreadable")
	}
	p, e := c.PKI(os.Getenv("SDK_BAO_PKI_MOUNT"))
	if e != nil {
		return e
	}
	k, e := c.KVv2(os.Getenv("SDK_BAO_KV_MOUNT"))
	if e != nil {
		return e
	}
	issued, e := p.Issue(ctx, pki.IssueRequest{Role: os.Getenv("SDK_BAO_PKI_ROLE"), CommonName: cn, TTL: 10 * time.Minute})
	if e != nil {
		return e
	}
	defer issued.PrivateKey.Zero()
	if e = pki.ValidateBundle(*issued, pki.VerifyPolicy{TrustedRootsPEM: [][]byte{roots}, RequiredEKUs: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, ExpectedCN: cn, MinRemainingTTL: 5 * time.Minute}); e != nil {
		return e
	}
	private := issued.PrivateKey.RevealCopy()
	defer clear(private)
	doc, e := kv.NewDocument(map[string]any{"schema_version": 1, "operation_id": operation, "resource_generation": generation, "certificate_pem": string(issued.Certificate.CertificatePEM), "private_key_pem": string(private), "ca_chain_pem": issued.Certificate.CAChainPEM})
	if e != nil {
		return e
	}
	defer doc.Zero()
	stored, e := k.Create(ctx, path, doc)
	if e != nil {
		return errors.Join(e, errors.New("certificate was already issued; do not rerun this one-shot example to recover"))
	}
	read, e := k.ReadRef(ctx, stored.Ref)
	if e != nil {
		return e
	}
	defer read.Data.Zero()
	var saved map[string]any
	if e = read.Data.Decode(&saved); e != nil {
		return e
	}
	if saved["operation_id"] != operation || saved["resource_generation"] != generation {
		return errors.New("credential generation mismatch")
	}
	fmt.Printf("Validated and stored credential version %d; persist the exact Ref in your application journal\n", stored.Ref.Version)
	// See examples/credentialworkflow for durable claim + unknown-result recovery.
	return nil
}
func main() {
	if e := bootstrap.Run(run); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
