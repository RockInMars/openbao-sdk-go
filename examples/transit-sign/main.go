package main

import (
	"context"
	"errors"
	"fmt"
	bao "git.example.com/infra/openbao-sdk-go"
	"git.example.com/infra/openbao-sdk-go/examples/internal/bootstrap"
	"git.example.com/infra/openbao-sdk-go/sensitive"
	"git.example.com/infra/openbao-sdk-go/transit"
	"os"
	"strconv"
)

func run(ctx context.Context, c *bao.Client) error {
	version, e := strconv.Atoi(os.Getenv("SDK_BAO_KEY_VERSION"))
	if e != nil || version <= 0 {
		return errors.New("positive explicit SDK_BAO_KEY_VERSION required")
	}
	tr, e := c.Transit(os.Getenv("SDK_BAO_TRANSIT_MOUNT"))
	if e != nil {
		return e
	}
	msg := sensitive.NewBytes([]byte("example-domain:v1:non-secret-message"))
	defer msg.Zero()
	name := os.Getenv("SDK_BAO_KEY_NAME")
	profile := transit.Profile(os.Getenv("SDK_BAO_SIGN_PROFILE"))
	signed, e := tr.Sign(ctx, transit.SignRequest{KeyName: name, KeyVersion: version, Profile: profile, Message: msg})
	if e != nil {
		return e
	}
	verified, e := tr.Verify(ctx, transit.VerifyRequest{KeyName: name, ExpectedVersion: version, Profile: profile, Message: msg, Signature: signed.Signature})
	if e != nil {
		return e
	}
	if !verified.Valid {
		return errors.New("signature validation failed")
	}
	public, e := tr.ReadPublicKey(ctx, name, version)
	if e != nil {
		return e
	}
	fmt.Printf("Signature verified using exact version %d (%s); input/signature omitted\n", public.Key.Version, public.KeyType)
	return nil
}
func main() {
	if e := bootstrap.Run(run); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
