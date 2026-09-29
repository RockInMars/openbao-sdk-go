// Package bootstrap is example-only service wiring, not implicit SDK configuration.
package bootstrap

import (
	"context"
	"errors"
	bao "git.example.com/infra/openbao-sdk-go"
	"git.example.com/infra/openbao-sdk-go/auth"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var ErrConfiguration = errors.New("example requires explicit SDK_BAO_ADDRESS, CLUSTER, NAMESPACE_MODE and controlled TOKEN_FILE; named mode also requires NAMESPACE")

func ConfigFrom(get func(string) string) (bao.Config, error) {
	if get == nil {
		return bao.Config{}, ErrConfiguration
	}
	mode := bao.NamespaceMode(get("SDK_BAO_NAMESPACE_MODE"))
	ns := get("SDK_BAO_NAMESPACE")
	if mode != bao.NamespaceRoot && mode != bao.NamespaceNamed || mode == bao.NamespaceRoot && ns != "" || mode == bao.NamespaceNamed && ns == "" {
		return bao.Config{}, ErrConfiguration
	}
	if get("SDK_BAO_ADDRESS") == "" || get("SDK_BAO_CLUSTER") == "" || get("SDK_BAO_TOKEN_FILE") == "" {
		return bao.Config{}, ErrConfiguration
	}
	provider, e := auth.NewTokenFile(get("SDK_BAO_TOKEN_FILE"))
	if e != nil {
		return bao.Config{}, e
	}
	return bao.Config{Address: get("SDK_BAO_ADDRESS"), ClusterAlias: get("SDK_BAO_CLUSTER"), Namespace: bao.NamespaceConfig{Mode: mode, Path: ns}, Auth: auth.Config{Mode: auth.ExternalToken, TokenProvider: provider}, TLS: bao.TLSConfig{CAFile: get("SDK_BAO_CA_FILE")}}, nil
}

// Run keeps the service context alive through all API calls; only Close receives
// a fresh bounded shutdown context. Callbacks never receive raw authentication.
func Run(action func(context.Context, *bao.Client) error) (err error) {
	if action == nil {
		return ErrConfiguration
	}
	cfg, e := ConfigFrom(os.Getenv)
	if e != nil {
		return e
	}
	c, e := bao.New(cfg)
	if e != nil {
		return e
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	defer func() {
		shutdown, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		err = errors.Join(err, c.Close(shutdown))
	}()
	if e = c.Start(ctx); e != nil {
		return e
	}
	return action(ctx, c)
}
