package main

import (
	"context"
	"errors"
	"fmt"
	bao "git.example.com/infra/openbao-sdk-go"
	"git.example.com/infra/openbao-sdk-go/examples/internal/bootstrap"
	"git.example.com/infra/openbao-sdk-go/kv"
	"os"
)

func run(ctx context.Context, c *bao.Client) error {
	path := os.Getenv("SDK_BAO_SECRET_PATH")
	if path == "" {
		return errors.New("explicit unique SDK_BAO_SECRET_PATH is required")
	}
	k, e := c.KVv2(os.Getenv("SDK_BAO_KV_MOUNT"))
	if e != nil {
		return e
	}
	doc, e := kv.ParseDocument([]byte(`{"example_integer":9007199254740993,"schema_version":1}`))
	if e != nil {
		return e
	}
	defer doc.Zero()
	created, e := k.Create(ctx, path, doc)
	if e != nil {
		return e
	} // No silent overwrite or issue retry.
	exact, e := k.ReadRef(ctx, created.Ref)
	if e != nil {
		return e
	}
	defer exact.Data.Zero()
	updated, e := k.CompareAndSwap(ctx, path, created.Ref.Version, doc)
	if e != nil {
		return e
	}
	fmt.Printf("Created version %d and CAS version %d; secret values omitted\n", created.Ref.Version, updated.Ref.Version)
	return nil
}
func main() {
	if e := bootstrap.Run(run); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
