package main

import (
	"context"
	"fmt"
	bao "github.com/RockInMars/openbao-sdk-go"
	"github.com/RockInMars/openbao-sdk-go/examples/internal/bootstrap"
	"os"
)

func main() {
	if e := bootstrap.Run(func(ctx context.Context, c *bao.Client) error {
		fmt.Println("SDK service started; awaiting cancellation")
		<-ctx.Done()
		return nil
	}); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
