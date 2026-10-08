package bao_test

import (
	"context"
	"fmt"
	"time"

	bao "github.com/RockInMars/openbao-sdk-go"
	"github.com/RockInMars/openbao-sdk-go/auth"
	"github.com/RockInMars/openbao-sdk-go/sensitive"
)

// This example uses a placeholder provider and makes no network requests.
// ExternalToken startup checks its snapshot, not server reachability or rights.
func ExampleNew() {
	input := sensitive.NewBytes([]byte("example-only-token"))
	provider, err := auth.NewStaticToken(input)
	input.Zero() // NewStaticToken owns a separate copy.
	if err != nil {
		panic(err)
	}
	client, err := bao.New(bao.Config{
		Address: "https://bao.invalid", ClusterAlias: "example",
		Namespace: bao.NamespaceConfig{Mode: bao.NamespaceRoot},
		Auth:      auth.Config{Mode: auth.ExternalToken, TokenProvider: provider},
	})
	if err != nil {
		panic(err)
	}
	serviceCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := client.Start(serviceCtx); err != nil {
		_ = client.Close(context.Background())
		panic(err)
	}
	fmt.Println(client.State().Lifecycle)
	shutdownCtx, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if err := client.Close(shutdownCtx); err != nil {
		panic(err)
	}
	fmt.Println(client.State().Lifecycle)
	// Output:
	// READY
	// CLOSED
}
