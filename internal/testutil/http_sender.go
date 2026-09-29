// Package testutil contains test fixtures, not a production client backend.
package testutil

import (
	"bytes"
	"context"
	"git.example.com/infra/openbao-sdk-go/internal/engine"
	"net/http"
)

type HTTPSender struct {
	Address, Namespace string
	Client             *http.Client
	Transport          *engine.Transport
}

func NewHTTPSender(address, namespace string, c *http.Client, t *engine.Transport) *HTTPSender {
	return &HTTPSender{address, namespace, c, t}
}
func (s *HTTPSender) CloseIdleConnections() { s.Transport.CloseIdleConnections() }
func (s *HTTPSender) Send(ctx context.Context, r engine.Request) (*engine.Response, error) {
	ctx, x := engine.NewExchange(ctx)
	uri := s.Address + r.Path
	if len(r.Query) > 0 {
		uri += "?" + r.Query.Encode()
	}
	req, e := http.NewRequestWithContext(ctx, r.Method, uri, bytes.NewReader(append([]byte(nil), r.Body...)))
	if e != nil {
		return nil, &engine.TransportFailure{Cause: e}
	}
	req.Header.Set("Content-Type", "application/json")
	if r.Token != "" {
		req.Header.Set("X-Vault-Token", r.Token)
	}
	if s.Namespace != "" {
		req.Header.Set("X-Vault-Namespace", s.Namespace)
	}
	resp, err := s.Client.Do(req)
	if resp != nil {
		resp.Body.Close()
	}
	out, captureErr, sent := x.Snapshot()
	if captureErr != nil {
		return out, &engine.TransportFailure{Cause: captureErr, Sent: sent}
	}
	if err != nil {
		return out, &engine.TransportFailure{Cause: err, Sent: sent}
	}
	return out, nil
}
