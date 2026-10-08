package remotecheck

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"net/http"
	"os"
	"regexp"
	"time"

	"github.com/RockInMars/openbao-sdk-go/baoerr"
	"github.com/RockInMars/openbao-sdk-go/pki"
	"github.com/RockInMars/openbao-sdk-go/sensitive"
	"github.com/RockInMars/openbao-sdk-go/transit"
)

func (h *harness) optionalScenarios() bool {
	if h.c.negativeToken != "" {
		// A denial only proves isolation after this identity has authenticated.
		token := h.c.token
		h.c.token = h.c.negativeToken
		identity, _, identityErr := h.raw(http.MethodGet, "auth/token/lookup-self", nil)
		if identityErr == nil {
			identityErr = checkRemainingTokenBudget(identity, 6, 2)
		}
		if identityErr != nil {
			h.c.token = token
			return h.check("permission_identity", identityErr)
		}
		if !h.check("permission_identity", nil) {
			h.c.token = token
			return false
		}
		_, status, _ := h.raw(http.MethodGet, h.c.negativeMount+"/data/"+h.c.negativePath+"?version=1", nil)
		h.c.token = token
		var err error
		if status != http.StatusForbidden {
			err = errors.New("explicit permission denial required")
		}
		if !h.check("permission_negative", err) {
			return false
		}
	}
	if h.c.transitMount != "" && !h.testTransit() {
		return false
	}
	if h.c.pkiMount != "" && !h.testPKI() {
		return false
	}
	return true
}

func (h *harness) testTransit() bool {
	if h.b.limit-h.b.count() < 15 {
		return h.check("transit_budget", errors.New("insufficient budget"))
	}
	c, factoryErr := h.sdk.Transit(h.c.transitMount)
	if factoryErr != nil {
		return h.check("transit_factory", factoryErr)
	}
	meta, err := c.ReadKeyMetadata(h.ctx, h.c.transitKey)
	if err == nil && (meta.Type != h.c.transitType || meta.LatestVersion <= 0) {
		err = errors.New("key type mismatch")
	}
	if !h.check("transit_existing_key", err) {
		return false
	}
	message := sensitive.NewBytes([]byte("sdk-remote-synthetic-message"))
	defer message.Zero()
	if h.c.transitType == "aes256-gcm96" {
		if !meta.SupportsEncryption {
			return h.check("transit_encrypt", errors.New("key capability mismatch"))
		}
		cipher, err := c.Encrypt(h.ctx, transit.EncryptRequest{KeyName: h.c.transitKey, KeyVersion: meta.LatestVersion, Plaintext: message})
		if !h.check("transit_encrypt", err) {
			return false
		}
		clear, err := c.Decrypt(h.ctx, transit.DecryptRequest{KeyName: h.c.transitKey, Ciphertext: cipher.Ciphertext})
		if err == nil {
			raw := clear.Plaintext.RevealCopy()
			err = nil
			if !bytes.Equal(raw, []byte("sdk-remote-synthetic-message")) {
				err = errors.New("plaintext mismatch")
			}
			for i := range raw {
				raw[i] = 0
			}
			clear.Plaintext.Zero()
		}
		if !h.check("transit_decrypt", err) {
			return false
		}
		// Corrupt the authenticated ciphertext while retaining its wire syntax.
		wrapped := []byte(cipher.Ciphertext.Wrapped)
		i := len(wrapped) - 5
		if i < 0 {
			return h.check("transit_tamper", errors.New("ciphertext malformed"))
		}
		if wrapped[i] == 'A' {
			wrapped[i] = 'B'
		} else {
			wrapped[i] = 'A'
		}
		_, err = c.Decrypt(h.ctx, transit.DecryptRequest{KeyName: h.c.transitKey, Ciphertext: transit.Ciphertext{Wrapped: string(wrapped), Version: cipher.Ciphertext.Version}})
		var e *baoerr.Error
		if !errors.As(err, &e) || e.HTTPStatus != http.StatusBadRequest {
			return h.check("transit_tamper", errors.New("tamper was not rejected"))
		}
		return h.check("transit_tamper", nil)
	}
	if !meta.SupportsSigning {
		return h.check("transit_sign", errors.New("key capability mismatch"))
	}
	profile := transit.RSAPSSSHA256
	if meta.Type == "ecdsa-p256" {
		profile = transit.ECDSAP256SHA256ASN1
	}
	if meta.Type == "ed25519" {
		profile = transit.Ed25519Message
	}
	signed, err := c.Sign(h.ctx, transit.SignRequest{KeyName: h.c.transitKey, KeyVersion: meta.LatestVersion, Profile: profile, Message: message})
	if !h.check("transit_sign", err) {
		return false
	}
	verified, err := c.Verify(h.ctx, transit.VerifyRequest{KeyName: h.c.transitKey, ExpectedVersion: meta.LatestVersion, Profile: profile, Message: message, Signature: signed.Signature})
	if err == nil && !verified.Valid {
		err = errors.New("signature invalid")
	}
	if !h.check("transit_verify", err) {
		return false
	}
	tampered := sensitive.NewBytes([]byte("sdk-remote-tampered"))
	defer tampered.Zero()
	verified, err = c.Verify(h.ctx, transit.VerifyRequest{KeyName: h.c.transitKey, ExpectedVersion: meta.LatestVersion, Profile: profile, Message: tampered, Signature: signed.Signature})
	if err == nil && verified.Valid {
		err = errors.New("tamper accepted")
	}
	return h.check("transit_tamper", err)
}

func (h *harness) testPKI() bool {
	client, factoryErr := h.sdk.PKI(h.c.pkiMount)
	if factoryErr != nil {
		return h.check("pki_factory", factoryErr)
	}
	deadline, _ := h.ctx.Deadline()
	if h.b.limit-h.b.count() < 5 || time.Until(deadline) < 60*time.Second {
		return h.check("pki_budget", errors.New("insufficient budget"))
	}
	rootPEM, err := os.ReadFile(h.c.pkiRoot)
	roots := x509.NewCertPool()
	if err != nil || !roots.AppendCertsFromPEM(rootPEM) {
		return h.check("pki_trust", errors.New("trusted root unavailable"))
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return h.check("pki_csr", errors.New("random unavailable"))
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: h.c.pkiDNS}, DNSNames: []string{h.c.pkiDNS}}, key)
	if err != nil {
		return h.check("pki_csr", errors.New("CSR failed"))
	}
	csr := sensitive.NewBytes(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}))
	defer csr.Zero()
	i := len(h.output.Resources)
	h.output.Resources = append(h.output.Resources, resource{Kind: "certificate", Mount: h.c.pkiMount, Path: h.c.pkiRole, State: "issuance_pending"})
	if !h.save() {
		return false
	}
	signed, err := client.SignCSR(h.ctx, pki.SignCSRRequest{Role: h.c.pkiRole, CSRPEM: csr, TTL: 5 * time.Minute})
	if err == nil {
		h.output.Resources[i].Path = signed.Certificate.SerialNumber
		h.output.Resources[i].State = "retained_until_expiry"
		if !h.save() {
			return false
		}
	}
	if !h.check("pki_sign_csr", err) {
		return false
	}
	serial := signed.Certificate.SerialNumber
	if !regexp.MustCompile(`^[0-9a-fA-F:-]{1,128}$`).MatchString(serial) {
		return h.check("pki_serial", errors.New("invalid serial"))
	}
	block, _ := pem.Decode(signed.Certificate.CertificatePEM)
	if block == nil {
		err = errors.New("certificate invalid")
	} else {
		cert, parseErr := x509.ParseCertificate(block.Bytes)
		err = parseErr
		if err == nil {
			intermediates := x509.NewCertPool()
			for _, chain := range signed.Certificate.CAChainPEM {
				intermediates.AppendCertsFromPEM(chain)
			}
			intermediates.AppendCertsFromPEM(signed.Certificate.IssuingCAPEM)
			_, err = cert.Verify(x509.VerifyOptions{Roots: roots, Intermediates: intermediates, DNSName: h.c.pkiDNS, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}})
			pub, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
			actual, _ := x509.MarshalPKIXPublicKey(cert.PublicKey)
			if !bytes.Equal(pub, actual) || len(cert.DNSNames) != 1 || cert.DNSNames[0] != h.c.pkiDNS || len(cert.IPAddresses) != 0 || len(cert.URIs) != 0 || cert.IsCA || time.Until(cert.NotAfter) > 6*time.Minute {
				err = errors.New("certificate policy mismatch")
			}
		}
	}
	valid := h.check("pki_verify", err)
	if h.c.pkiRevoke && !h.journalFailed {
		h.output.Resources[i].State = "revocation_pending"
		if !h.save() {
			return false
		}
		_, revokeErr := client.Revoke(h.ctx, serial)
		if !h.check("pki_revoke", revokeErr) {
			return false
		}
		h.output.Resources[i].State = "revoked_record_retained"
		if !h.save() {
			return false
		}
	}
	return valid
}
