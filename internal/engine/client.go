package engine

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/RockInMars/openbao-sdk-go/baoerr"
	"github.com/RockInMars/openbao-sdk-go/internal/jsondoc"
	"net/url"
	"strings"
	"time"
)

type ExecutorConfig struct {
	RequestTimeout, IssueTimeout, LoginTimeout, RenewTimeout time.Duration
	Concurrency                                              int
	MaxRequestBytes                                          int64
	MaxAttempts                                              int
	BaseDelay, MaxDelay                                      time.Duration
	Namespace                                                string
}
type Executor struct {
	sender Sender
	cfg    ExecutorConfig
	slots  chan struct{}
}
type Call struct {
	Operation  Operation
	Path       string
	Query      url.Values
	Payload    []byte
	Credential func(context.Context) (string, error)
}

func NewExecutor(s Sender, c ExecutorConfig) *Executor {
	return &Executor{sender: s, cfg: c, slots: make(chan struct{}, c.Concurrency)}
}
func (e *Executor) CloseIdleConnections() { e.sender.CloseIdleConnections() }
func (e *Executor) Budget(op Operation) time.Duration {
	switch op {
	case PKIIssue, PKISignCSR:
		return e.cfg.IssueTimeout
	case AppRoleLogin:
		return e.cfg.LoginTimeout
	case TokenRenew:
		return e.cfg.RenewTimeout
	}
	return e.cfg.RequestTimeout
}
func (e *Executor) Execute(parent context.Context, c Call) (*Response, error) {
	d, ok := Lookup(c.Operation)
	if !ok || parent == nil {
		return nil, SafeError(baoerr.CodeInvalidArgument, c.Operation, 0, baoerr.EffectNone)
	}
	p := strings.TrimPrefix(c.Path, "/v1/")
	if p == c.Path {
		return nil, SafeError(baoerr.CodeInvalidArgument, c.Operation, 0, baoerr.EffectNone)
	}
	if c.Operation == KVList {
		p = strings.TrimSuffix(p, "/")
	}
	if ValidatePath(p) != nil || int64(len(c.Payload)) > e.cfg.MaxRequestBytes {
		return nil, SafeError(baoerr.CodeInvalidArgument, c.Operation, 0, baoerr.EffectNone)
	}
	if len(c.Payload) > 0 {
		if _, err := jsondoc.Object(c.Payload); err != nil {
			return nil, SafeError(baoerr.CodeInvalidArgument, c.Operation, 0, baoerr.EffectNone)
		}
	}
	ctx, cancel := context.WithTimeout(parent, e.Budget(c.Operation))
	defer cancel()
	if ctx.Err() != nil {
		return nil, contextError(ctx, c.Operation, 0, baoerr.EffectNone)
	}
	if d.Kind != AuthAction && d.Kind != HealthProbe {
		select {
		case e.slots <- struct{}{}:
			defer func() { <-e.slots }()
		case <-ctx.Done():
			return nil, contextError(ctx, c.Operation, 0, baoerr.EffectNone)
		}
	}
	attempts := 1
	if d.Kind == SafeRead {
		attempts = e.cfg.MaxAttempts
	}
	for n := 1; n <= attempts; n++ {
		if ctx.Err() != nil {
			return nil, contextError(ctx, c.Operation, n-1, baoerr.EffectNone)
		}
		token := ""
		if c.Credential != nil {
			var err error
			token, err = c.Credential(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return nil, contextError(ctx, c.Operation, n-1, baoerr.EffectNone)
				}
				var be *baoerr.Error
				if errors.As(err, &be) && be != nil {
					return nil, SafeError(be.Code, c.Operation, n-1, baoerr.EffectNone)
				}
				return nil, SafeError(baoerr.CodeAuthenticationFailed, c.Operation, n-1, baoerr.EffectNone)
			}
			if !validToken(token) {
				return nil, SafeError(baoerr.CodeAuthenticationFailed, c.Operation, n-1, baoerr.EffectNone)
			}
		} else if d.Kind != AuthAction && d.Kind != HealthProbe {
			return nil, SafeError(baoerr.CodeNotReady, c.Operation, n-1, baoerr.EffectNone)
		}
		if ctx.Err() != nil {
			return nil, contextError(ctx, c.Operation, n-1, baoerr.EffectNone)
		}
		r, err := e.sender.Send(ctx, Request{Method: d.Method, Path: c.Path, Query: c.Query, Body: c.Payload, Token: token, Namespace: e.cfg.Namespace})
		if r == nil {
			r = &Response{}
		}
		r.Attempts = n
		r.RequestID = requestID(r.Body)
		retry := false
		if err != nil {
			err, retry = transportError(err, c.Operation, d, n)
			if be, ok := err.(*baoerr.Error); ok {
				be.HTTPStatus = r.Status
				be.RequestID = r.RequestID
			}
		} else {
			if d.Kind == HealthProbe {
				switch r.Status {
				case 200, 429, 472, 473, 501, 503:
					return r, nil
				}
			}
			switch r.Status {
			case 200:
				if len(r.Body) == 0 && !d.Void {
					return r, InvalidResponse(r, c.Operation)
				}
				if d.Void && len(r.Body) > 0 {
					if _, e := DecodeObject(r, c.Operation); e != nil {
						return r, e
					}
				}
				return r, nil
			case 204:
				if d.Void {
					return r, nil
				}
				return r, InvalidResponse(r, c.Operation)
			default:
				err, retry = responseError(r, c.Operation, d)
			}
		}
		if !retry || n >= attempts {
			return r, err
		}
		delay, again := retryDelay(ctx, n, e.cfg, r.Header)
		if !again {
			return r, err
		}
		r.Zero()
		if wait(ctx, delay) != nil {
			return nil, contextError(ctx, c.Operation, n, baoerr.EffectNone)
		}
	}
	return nil, SafeError(baoerr.CodeUnavailable, c.Operation, attempts, baoerr.EffectNone)
}
func validToken(s string) bool {
	if len(s) == 0 || len(s) > 64<<10 {
		return false
	}
	for i := range s {
		if s[i] < 0x21 || s[i] > 0x7e {
			return false
		}
	}
	return true
}

// Only UUID-shaped server identifiers leave the decode boundary. Other forms
// are omitted, rather than accidentally copying an arbitrary response string.
func requestID(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	v, err := jsondoc.Object(raw)
	if err != nil {
		return ""
	}
	s, ok := v["request_id"].(string)
	if !ok || len(s) != 36 {
		return ""
	}
	for i, b := range []byte(s) {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if b != '-' {
				return ""
			}
		} else if !(b >= '0' && b <= '9' || b >= 'a' && b <= 'f' || b >= 'A' && b <= 'F') {
			return ""
		}
	}
	return s
}

// DecodeObject is shared by module and authentication codecs; it preserves
// JSON numbers and rejects duplicate keys before any typed field extraction.
func DecodeObject(r *Response, op Operation) (map[string]any, error) {
	if r == nil {
		return nil, InvalidResponse(r, op)
	}
	o, e := jsondoc.Object(r.Body)
	if e != nil {
		return nil, InvalidResponse(r, op)
	}
	if v, ok := o["errors"]; ok {
		a, ok := v.([]any)
		if !ok || len(a) > 0 {
			return nil, InvalidResponse(r, op)
		}
	}
	return o, nil
}
func Data(r *Response, op Operation) (map[string]any, error) {
	o, e := DecodeObject(r, op)
	if e != nil {
		return nil, e
	}
	m, ok := o["data"].(map[string]any)
	if !ok {
		return nil, InvalidResponse(r, op)
	}
	return m, nil
}
func Int(v any) (int, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return 0, false
	}
	i, e := n.Int64()
	if e != nil || int64(int(i)) != i {
		return 0, false
	}
	return int(i), true
}
