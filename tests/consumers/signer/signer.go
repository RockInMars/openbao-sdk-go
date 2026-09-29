// Package signer is a second independent module using the fixed-version API.
package signer

import (
	"context"
	bao "git.example.com/infra/openbao-sdk-go"
	"git.example.com/infra/openbao-sdk-go/sensitive"
	"git.example.com/infra/openbao-sdk-go/transit"
)

type Signer struct {
	t       *bao.TransitClient
	key     string
	version int
}

func New(c *bao.Client, mount, key string, version int) (*Signer, error) {
	tr, e := c.Transit(mount)
	if e != nil {
		return nil, e
	}
	return &Signer{t: tr, key: key, version: version}, nil
}
func (s *Signer) Sign(ctx context.Context, message sensitive.Bytes) (*transit.SignResult, error) {
	return s.t.Sign(ctx, transit.SignRequest{KeyName: s.key, KeyVersion: s.version, Profile: transit.ECDSAP256SHA256ASN1, Message: message})
}
