package baoerr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestErrorClassificationAndRedaction(t *testing.T) {
	e := &Error{Code: CodeDeadlineExceeded, Operation: "KV_CREATE", Effect: EffectUnknown, Message: "untrusted-fixture"}
	wrapped := fmt.Errorf("outer: %w", e)
	if !errors.Is(wrapped, context.DeadlineExceeded) || !HasUnknownOutcome(wrapped) || !IsCode(wrapped, CodeDeadlineExceeded) {
		t.Fatal("error identity lost")
	}
	for _, render := range []string{e.Error(), fmt.Sprintf("%+v %#v", e, e)} {
		if strings.Contains(render, "untrusted-fixture") {
			t.Fatal("unsafe message")
		}
	}
	b, err := json.Marshal(e)
	if err != nil || strings.Contains(string(b), "untrusted-fixture") {
		t.Fatal("unsafe serialization")
	}
	if (&Error{Code: CodeCanceled}).Unwrap() != context.Canceled {
		t.Fatal("context identity")
	}
	if HasUnknownOutcome(nil) || IsCode(nil, CodeClosed) {
		t.Fatal("nil classification")
	}
}
