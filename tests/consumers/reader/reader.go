// Package reader is an independent consumer module, not a workspace replacement.
package reader

import (
	"context"
	bao "git.example.com/infra/openbao-sdk-go"
	"git.example.com/infra/openbao-sdk-go/kv"
)

type Reader struct{ k *bao.KVClient }

func New(c *bao.Client, mount string) (*Reader, error) {
	k, e := c.KVv2(mount)
	if e != nil {
		return nil, e
	}
	return &Reader{k: k}, nil
}
func (r *Reader) Read(ctx context.Context, path string, version int) (*kv.ReadResult, error) {
	return r.k.ReadVersion(ctx, path, version)
}
