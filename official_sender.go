package bao

import (
	"context"
	"net/http"
	"net/url"

	"git.example.com/infra/openbao-sdk-go/internal/engine"
	baoapi "github.com/openbao/openbao/api/v2"
)

// newProtocolSender is the only third-party dependency seam. Production always
// uses api/v2; supplemental offline contract tests overlay this file explicitly.
func newProtocolSender(address, namespace string, h *http.Client, t *engine.Transport) (engine.Sender, error) {
	cfg := baoapi.NewConfig()
	if cfg.Error != nil {
		return nil, invalid("OFFICIAL_CLIENT")
	}
	cfg.Address = address
	cfg.HttpClient = h
	cfg.DisableEnvironment = true
	cfg.MaxRetries = 0
	cfg.DisableRedirects = true
	cfg.Timeout = 0
	cfg.Logger = nil
	cfg.OutputCurlString = false
	cfg.OutputPolicy = false
	cfg.DefaultStrongConsistency = false
	base, err := baoapi.NewClient(cfg)
	if err != nil {
		return nil, invalid("OFFICIAL_CLIENT")
	}
	base.SetClientTimeout(0)
	// api/v2 re-applies the client's namespace in RawRequestWithContext. Set it
	// once at construction; never change it as requests or token snapshots change.
	if namespace != "" {
		base.SetNamespace(namespace)
	}
	return &officialSender{base: base, namespace: namespace, transport: t}, nil
}

type officialSender struct {
	base      *baoapi.Client
	namespace string
	transport *engine.Transport
}

func (s *officialSender) CloseIdleConnections() { s.transport.CloseIdleConnections() }
func (s *officialSender) Send(ctx context.Context, in engine.Request) (*engine.Response, error) {
	if in.Namespace != s.namespace {
		return nil, &engine.TransportFailure{Cause: engine.ErrInvalidResponse}
	}
	ctx, capture := engine.NewExchange(ctx)
	req := s.base.NewRequest(in.Method, in.Path)
	req.URL.Path = in.Path
	req.URL.RawPath = ""
	req.ClientToken = in.Token
	req.Params = make(url.Values, len(in.Query))
	for k, v := range in.Query {
		req.Params[k] = append([]string(nil), v...)
	}
	if len(in.Body) > 0 {
		// The HTTP stack can retain body readers during cancellation. Give it an
		// independent GC-owned buffer rather than racing a caller's best-effort Zero.
		req.BodyBytes = append([]byte(nil), in.Body...)
		req.Headers.Set("Content-Type", "application/json")
	}
	raw, err := s.base.RawRequestWithContext(ctx, req)
	if raw != nil && raw.Body != nil {
		_ = raw.Body.Close()
	}
	out, captureErr, sent := capture.Snapshot()
	if captureErr != nil {
		return out, &engine.TransportFailure{Cause: captureErr, Sent: sent}
	}
	if out != nil && out.Status >= 300 {
		return out, nil
	} // classify bounded error/redirect ourselves
	if err != nil {
		return out, &engine.TransportFailure{Cause: err, Sent: sent}
	}
	return out, nil
}
