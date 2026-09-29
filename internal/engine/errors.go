package engine

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"git.example.com/infra/openbao-sdk-go/baoerr"
	"git.example.com/infra/openbao-sdk-go/internal/jsondoc"
	"time"
)

type TransportFailure struct {
	Cause error
	Sent  bool
}

func (e *TransportFailure) Error() string { return "transport failed" }
func (e *TransportFailure) Unwrap() error { return e.Cause }
func SafeError(code string, op Operation, attempts int, effect baoerr.Effect) *baoerr.Error {
	return &baoerr.Error{Code: code, Operation: string(op), Attempts: attempts, Effect: effect, Message: "OpenBao operation failed"}
}
func contextError(ctx context.Context, op Operation, attempts int, effect baoerr.Effect) error {
	code := baoerr.CodeCanceled
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		code = baoerr.CodeDeadlineExceeded
	}
	return SafeError(code, op, attempts, effect)
}
func isTLSFailure(err error) bool {
	var a *tls.CertificateVerificationError
	var b x509.UnknownAuthorityError
	var c x509.HostnameError
	var d x509.CertificateInvalidError
	var record tls.RecordHeaderError
	return errors.As(err, &a) || errors.As(err, &b) || errors.As(err, &c) || errors.As(err, &d) || errors.As(err, &record)
}
func transportError(err error, op Operation, d Definition, n int) (error, bool) {
	sent := true
	var tf *TransportFailure
	if errors.As(err, &tf) {
		sent = tf.Sent
	}
	effect := baoerr.EffectNone
	if sent && SideEffect(d.Kind) {
		effect = baoerr.EffectUnknown
	}
	code := baoerr.CodeUnavailable
	retry := d.Kind == SafeRead
	switch {
	case errors.Is(err, context.Canceled):
		code = baoerr.CodeCanceled
		retry = false
	case errors.Is(err, context.DeadlineExceeded):
		code = baoerr.CodeDeadlineExceeded
		retry = false
	case errors.Is(err, ErrResponseTooLarge):
		code = baoerr.CodeResponseTooLarge
		retry = false
	case errors.Is(err, ErrInvalidResponse):
		code = baoerr.CodeInvalidResponse
		retry = false
	case isTLSFailure(err):
		effect = baoerr.EffectNone
		retry = false
	}
	return SafeError(code, op, n, effect), retry
}
func responseError(r *Response, op Operation, d Definition) (error, bool) {
	effect := baoerr.EffectNone
	if SideEffect(d.Kind) {
		effect = baoerr.EffectUnknown
	}
	code := baoerr.CodeUnavailable
	retry := false
	if r.Status >= 300 && r.Status < 400 {
		code = baoerr.CodeRedirectBlocked
		return statusError(r, op, code, effect), false
	}
	var obj map[string]any
	if len(r.Body) > 0 {
		var e error
		obj, e = jsondoc.Object(r.Body)
		if e != nil {
			return statusError(r, op, baoerr.CodeInvalidResponse, effect), false
		}
	}
	// An arbitrary proxy object (or missing body) is not proof of a server-side
	// rejection. Preserve UNKNOWN for writes until an API error envelope exists.
	if r.Status == 400 || r.Status == 401 || r.Status == 403 {
		list, ok := obj["errors"].([]any)
		if !ok || len(list) == 0 {
			return statusError(r, op, baoerr.CodeInvalidResponse, effect), false
		}
		for _, item := range list {
			text, ok := item.(string)
			if !ok || text == "" {
				return statusError(r, op, baoerr.CodeInvalidResponse, effect), false
			}
		}
	}
	switch r.Status {
	case 400:
		code = baoerr.CodeInvalidArgument
		effect = baoerr.EffectNone
		if op == KVCreate || op == KVCAS {
			if list, ok := obj["errors"].([]any); ok && len(list) == 1 {
				if s, ok := list[0].(string); ok && s == "check-and-set parameter did not match the current version" {
					code = baoerr.CodeCASConflict
				}
			}
		}
	case 401:
		code = baoerr.CodeAuthenticationFailed
		effect = baoerr.EffectNone
	case 403:
		code = baoerr.CodePermissionDenied
		effect = baoerr.EffectNone
	case 404:
		code = baoerr.CodeNotFoundOrHidden
		// Keep 404's public code, but a proxy-shaped/missing body does not prove
		// a mutation was never executed. Only an API rejection permits NONE.
		if apiRejection(obj) {
			effect = baoerr.EffectNone
		}
		if (op == KVReadVersion || op == KVReadLatest) && deletedMetadata(obj) {
			code = baoerr.CodeVersionUnavailable
		}
	case 429, 500, 502, 503, 504:
		retry = d.Kind == SafeRead
	default:
		code = baoerr.CodeInvalidResponse
	}
	return statusError(r, op, code, effect), retry
}
func deletedMetadata(root map[string]any) bool {
	data, ok := root["data"].(map[string]any)
	if !ok {
		return false
	}
	m, ok := data["metadata"].(map[string]any)
	if !ok {
		return false
	}
	n, ok := m["version"].(json.Number)
	if !ok {
		return false
	}
	v, e := n.Int64()
	if e != nil || v <= 0 {
		return false
	}
	if destroyed, ok := m["destroyed"].(bool); ok && destroyed {
		return true
	}
	s, ok := m["deletion_time"].(string)
	if !ok || s == "" {
		return false
	}
	dt, e := time.Parse(time.RFC3339Nano, s)
	// A future auto-delete schedule cannot explain a missing version.
	return e == nil && !dt.IsZero() && !dt.After(time.Now())
}
func statusError(r *Response, op Operation, code string, effect baoerr.Effect) *baoerr.Error {
	e := SafeError(code, op, r.Attempts, effect)
	e.HTTPStatus = r.Status
	e.RequestID = r.RequestID
	return e
}
func InvalidResponse(r *Response, op Operation) error {
	d, _ := Lookup(op)
	effect := baoerr.EffectNone
	if SideEffect(d.Kind) {
		effect = baoerr.EffectUnknown
	}
	if r == nil {
		return SafeError(baoerr.CodeInvalidResponse, op, 0, effect)
	}
	return statusError(r, op, baoerr.CodeInvalidResponse, effect)
}

func apiRejection(obj map[string]any) bool {
	list, ok := obj["errors"].([]any)
	if !ok || len(list) == 0 {
		return false
	}
	for _, v := range list {
		s, ok := v.(string)
		if !ok || s == "" {
			return false
		}
	}
	return true
}
