package bao

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"github.com/RockInMars/openbao-sdk-go/internal/engine"
	"github.com/RockInMars/openbao-sdk-go/internal/pemutil"
	"io"
	"os"
)

const maxTLSMaterialBytes = 4 << 20

func readTLSFile(path string) ([]byte, error) {
	// Inputs are trusted deployment files, never request-controlled paths.
	// Validate type before opening (FIFO open can otherwise block New).
	st, e := os.Stat(path)
	if e != nil || !st.Mode().IsRegular() || st.Size() > maxTLSMaterialBytes {
		return nil, invalid("TLS")
	}
	f, e := os.Open(path)
	if e != nil {
		return nil, invalid("TLS")
	}
	defer f.Close()
	st, e = f.Stat()
	if e != nil || !st.Mode().IsRegular() || st.Size() > maxTLSMaterialBytes {
		return nil, invalid("TLS")
	}
	b, e := io.ReadAll(io.LimitReader(f, maxTLSMaterialBytes+1))
	if e != nil || len(b) > maxTLSMaterialBytes {
		clear(b)
		return nil, invalid("TLS")
	}
	return b, nil
}
func transportFor(c Config) (*engine.Transport, error) {
	tc := &tls.Config{MinVersion: c.TLS.MinVersion, ServerName: c.TLS.ServerName}
	ca := c.TLS.CAPEM
	if c.TLS.CAFile != "" {
		b, e := readTLSFile(c.TLS.CAFile)
		if e != nil {
			return nil, e
		}
		defer clear(b)
		if len(bytes.TrimSpace(b)) == 0 {
			return nil, invalid("TLS")
		}
		ca = b
	}
	if len(ca) > 0 {
		if len(ca) > maxTLSMaterialBytes {
			return nil, invalid("TLS")
		}
		pool := x509.NewCertPool()
		rest := bytes.TrimSpace(ca)
		count := 0
		for len(rest) > 0 {
			if !bytes.HasPrefix(rest, []byte("-----BEGIN CERTIFICATE-----")) {
				return nil, invalid("TLS")
			}
			block, next := pemutil.Decode(rest)
			if block == nil || block.Type != "CERTIFICATE" || len(block.Headers) > 0 {
				return nil, invalid("TLS")
			}
			cert, e := x509.ParseCertificate(block.Bytes)
			if e != nil {
				return nil, invalid("TLS")
			}
			pool.AddCert(cert)
			count++
			rest = bytes.TrimSpace(next)
		}
		if count == 0 {
			return nil, invalid("TLS")
		}
		tc.RootCAs = pool
	}
	var certPEM, keyPEM []byte
	if c.TLS.ClientCertFile != "" {
		var e error
		certPEM, e = readTLSFile(c.TLS.ClientCertFile)
		if e != nil {
			return nil, e
		}
		defer clear(certPEM)
		keyPEM, e = readTLSFile(c.TLS.ClientKeyFile)
		if e != nil {
			return nil, e
		}
		defer clear(keyPEM)
	} else {
		certPEM = c.TLS.ClientCertPEM
		keyPEM = c.TLS.ClientKeyPEM.RevealCopy()
		defer clear(keyPEM)
	}
	if len(certPEM) > 0 {
		if len(certPEM) > maxTLSMaterialBytes || len(keyPEM) > maxTLSMaterialBytes {
			return nil, invalid("TLS")
		}
		if !tlsPEMFraming(certPEM, false) || !tlsPEMFraming(keyPEM, true) {
			return nil, invalid("TLS")
		}
		cert, e := tls.X509KeyPair(certPEM, keyPEM)
		if e != nil {
			return nil, invalid("TLS")
		}
		tc.Certificates = []tls.Certificate{cert}
	}
	tr, e := engine.NewTransport(engine.TransportConfig{TLS: tc, DialTimeout: c.Timeouts.Dial, TLSHandshakeTimeout: c.Timeouts.TLSHandshake, IdleTimeout: c.Network.IdleConnTimeout, MaxIdle: c.Network.MaxIdleConnections, MaxIdlePerHost: c.Network.MaxIdlePerHost, MaxResponseHeaderBytes: c.Limits.MaxResponseHeaderBytes, MaxResponseBytes: c.Limits.MaxResponseBytes, ProxyURL: c.Network.ProxyURL})
	if e != nil {
		return nil, invalid("TLS")
	}
	return tr, nil
}

// tls.X509KeyPair searches past malformed/unrelated PEM. Reject those inputs
// before parsing the matching pair; all supported TLS private-key formats remain.
func tlsPEMFraming(raw []byte, key bool) bool {
	rest := bytes.TrimSpace(raw)
	count := 0
	for len(rest) > 0 {
		block, next := pemutil.Decode(rest)
		if block == nil {
			return false
		}
		clear(block.Bytes)
		if len(block.Headers) != 0 {
			return false
		}
		if key {
			if count != 0 || (block.Type != "PRIVATE KEY" && block.Type != "RSA PRIVATE KEY" && block.Type != "EC PRIVATE KEY") {
				return false
			}
		} else if block.Type != "CERTIFICATE" {
			return false
		}
		count++
		rest = bytes.TrimSpace(next)
	}
	return count > 0
}
