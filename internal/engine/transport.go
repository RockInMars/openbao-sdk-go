package engine

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sync"
	"time"
)

var ErrResponseTooLarge = errors.New("response body exceeds configured limit")
var ErrInvalidResponse = errors.New("invalid response encoding")

type exchangeKey struct{}

// Exchange captures bounded response bytes before an upstream client's own
// error parser can consume them. It is request-local and never logged.
type Exchange struct {
	mu         sync.Mutex
	response   *Response
	err        error
	dispatched bool
}

func NewExchange(ctx context.Context) (context.Context, *Exchange) {
	x := &Exchange{}
	return context.WithValue(ctx, exchangeKey{}, x), x
}
func (x *Exchange) Snapshot() (*Response, error, bool) {
	x.mu.Lock()
	defer x.mu.Unlock()
	return x.response, x.err, x.dispatched
}
func (x *Exchange) capture(r *Response, e error) {
	if x == nil {
		return
	}
	x.mu.Lock()
	x.response = r
	x.err = e
	x.mu.Unlock()
}
func (x *Exchange) dispatch() {
	if x == nil {
		return
	}
	x.mu.Lock()
	x.dispatched = true
	x.mu.Unlock()
}
func IsLoopbackHost(host string) bool {
	ip, e := netip.ParseAddr(host)
	return e == nil && ip.IsLoopback()
}

type TransportConfig struct {
	TLS                     *tls.Config
	DialTimeout             time.Duration
	TLSHandshakeTimeout     time.Duration
	IdleTimeout             time.Duration
	MaxIdle, MaxIdlePerHost int
	MaxResponseHeaderBytes  int64
	MaxResponseBytes        int64
	ProxyURL                string
}
type Transport struct {
	base *http.Transport
	max  int64
}

func NewTransport(c TransportConfig) (*Transport, error) {
	if c.TLS == nil || c.TLS.InsecureSkipVerify || c.TLS.MinVersion < tls.VersionTLS12 || c.MaxResponseBytes <= 0 {
		return nil, errors.New("invalid transport configuration")
	}
	b := &http.Transport{TLSClientConfig: c.TLS.Clone(), DialContext: (&net.Dialer{Timeout: c.DialTimeout, KeepAlive: 30 * time.Second}).DialContext, TLSHandshakeTimeout: c.TLSHandshakeTimeout, MaxIdleConns: c.MaxIdle, MaxIdleConnsPerHost: c.MaxIdlePerHost, IdleConnTimeout: c.IdleTimeout, MaxResponseHeaderBytes: c.MaxResponseHeaderBytes, ForceAttemptHTTP2: true, ExpectContinueTimeout: time.Second}
	if c.ProxyURL != "" {
		u, e := url.Parse(c.ProxyURL)
		if e != nil {
			return nil, errors.New("invalid proxy configuration")
		}
		b.Proxy = http.ProxyURL(u)
	}
	return &Transport{base: b, max: c.MaxResponseBytes}, nil
}
func (t *Transport) CloseIdleConnections() { t.base.CloseIdleConnections() }
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	x, _ := req.Context().Value(exchangeKey{}).(*Exchange)
	if req.Context().Err() != nil {
		return nil, req.Context().Err()
	}
	x.dispatch()
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		x.capture(nil, err)
		return nil, err
	}
	defer resp.Body.Close()
	out := &Response{Status: resp.StatusCode, Header: resp.Header.Clone()}
	var rd io.Reader = resp.Body
	if enc := resp.Header.Get("Content-Encoding"); enc != "" && !resp.Uncompressed {
		if enc != "gzip" {
			x.capture(out, ErrInvalidResponse)
			return nil, ErrInvalidResponse
		}
		g, e := gzip.NewReader(rd)
		if e != nil {
			x.capture(out, ErrInvalidResponse)
			return nil, ErrInvalidResponse
		}
		defer g.Close()
		rd = g
		resp.Header.Del("Content-Encoding")
		resp.Header.Del("Content-Length")
		resp.ContentLength = -1
		resp.Uncompressed = true
	}
	data, e := io.ReadAll(io.LimitReader(rd, t.max+1))
	if int64(len(data)) > t.max {
		clear(data)
		x.capture(out, ErrResponseTooLarge)
		return nil, ErrResponseTooLarge
	}
	if e != nil {
		clear(data)
		x.capture(out, e)
		return nil, e
	}
	out.Body = data
	x.capture(out, nil)
	resp.Body = io.NopCloser(bytes.NewReader(data))
	return resp, nil
}

// NewHTTPClient disables the second redirect layer. The caller separately
// disables the official client's redirect handling.
func NewHTTPClient(t *Transport) *http.Client {
	return &http.Client{Transport: t, Timeout: 0, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}
