package baoerr

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

func TestNilUnknownAndAllCodes(t *testing.T) {
	var nilErr *Error
	if nilErr.Error() == "" || nilErr.Unwrap() != nil || nilErr.String() == "" || nilErr.GoString() == "" {
		t.Fatal("nil error methods")
	}
	b, e := nilErr.MarshalJSON()
	if e != nil || string(b) != "null" {
		t.Fatal("nil error JSON")
	}
	for _, code := range []string{CodeInvalidArgument, CodeAuthenticationFailed, CodePermissionDenied, CodeNotFoundOrHidden, CodeVersionUnavailable, CodeCASConflict, CodeCanceled, CodeDeadlineExceeded, CodeUnavailable, CodeInvalidResponse, CodeResponseTooLarge, CodeRedirectBlocked, CodeNotReady, CodeClosed, "unsafe-secret-sentinel"} {
		v := &Error{Code: code, Operation: "unsafe-secret-sentinel", RequestID: "unsafe-secret-sentinel", Message: "unsafe-secret-sentinel", Effect: Effect("unsafe-secret-sentinel")}
		b, e := json.Marshal(v)
		if e != nil || strings.Contains(string(b), "unsafe-secret-sentinel") {
			t.Fatal("unsafe error output")
		}
		if v.LogValue().Kind() != slog.KindString || strings.Contains(fmt.Sprint(v), "unsafe-secret-sentinel") {
			t.Fatal("unsafe log value")
		}
		if code != CodeCanceled && code != CodeDeadlineExceeded && v.Unwrap() != nil {
			t.Fatal("unsafe cause retained")
		}
	}
	for _, effect := range []Effect{EffectNone, EffectConfirmed, EffectUnknown} {
		v := &Error{Code: CodeUnavailable, Effect: effect}
		b, _ := json.Marshal(v)
		if !strings.Contains(string(b), string(effect)) {
			t.Fatal("effect lost")
		}
	}
	if (&Error{Code: CodeDeadlineExceeded}).Unwrap() != context.DeadlineExceeded {
		t.Fatal("deadline cause lost")
	}
}
