// Package pemutil enforces non-skipping PEM framing. Size and block-type limits
// belong to callers, which know whether the input is a key, CSR or certificate.
package pemutil

import (
	"bytes"
	"encoding/pem"
)

// Decode reads a PEM block starting exactly at the first byte. Unlike pem.Decode
// it never searches past an invalid first block for a later valid one. On
// failure it returns the original input, without exposing partially decoded data.
func Decode(raw []byte) (*pem.Block, []byte) {
	marker := []byte("-----BEGIN ")
	if !bytes.HasPrefix(raw, marker) {
		return nil, raw
	}
	block, rest := pem.Decode(raw)
	if block == nil {
		return nil, raw
	}
	if bytes.Count(raw[:len(raw)-len(rest)], marker) != 1 {
		clear(block.Bytes)
		return nil, raw
	}
	return block, rest
}
