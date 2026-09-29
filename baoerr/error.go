// Package baoerr exposes stable, sanitized SDK failures and independent effect semantics.
package baoerr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
)

type Effect string

const (
	EffectNone      Effect = "none"
	EffectUnknown   Effect = "unknown"
	EffectConfirmed Effect = "confirmed"
)
const (
	CodeInvalidArgument      = "invalid_argument"
	CodeAuthenticationFailed = "authentication_failed"
	CodePermissionDenied     = "permission_denied"
	CodeNotFoundOrHidden     = "not_found_or_hidden"
	CodeVersionUnavailable   = "version_unavailable"
	CodeCASConflict          = "cas_conflict"
	CodeCanceled             = "canceled"
	CodeDeadlineExceeded     = "deadline_exceeded"
	CodeUnavailable          = "unavailable"
	CodeInvalidResponse      = "invalid_response"
	CodeResponseTooLarge     = "response_too_large"
	CodeRedirectBlocked      = "redirect_blocked"
	CodeNotReady             = "not_ready"
	CodeClosed               = "closed"
)

type Error struct {
	Code       string
	Operation  string
	HTTPStatus int
	RequestID  string
	Attempts   int
	Effect     Effect
	Message    string
}

// Error deliberately ignores arbitrary Message contents: raw upstream messages
// never become a safe error merely by being placed in this public struct.
func (e *Error) Error() string {
	if e == nil {
		return "openbao SDK error"
	}
	return "openbao SDK: " + safeCode(e.Code)
}
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	switch e.Code {
	case CodeCanceled:
		return context.Canceled
	case CodeDeadlineExceeded:
		return context.DeadlineExceeded
	}
	return nil
}
func IsCode(err error, code string) bool {
	var e *Error
	return errors.As(err, &e) && e != nil && e.Code == code
}
func HasUnknownOutcome(err error) bool {
	var e *Error
	return errors.As(err, &e) && e != nil && e.Effect == EffectUnknown
}
func (e *Error) String() string             { return e.Error() }
func (e *Error) GoString() string           { return e.Error() }
func (e *Error) Format(s fmt.State, v rune) { _, _ = io.WriteString(s, e.Error()) }
func (e *Error) LogValue() slog.Value       { return slog.StringValue(e.Error()) }
func (e *Error) MarshalJSON() ([]byte, error) {
	if e == nil {
		return []byte("null"), nil
	}
	return json.Marshal(struct {
		Code       string `json:"code"`
		HTTPStatus int    `json:"http_status"`
		Attempts   int    `json:"attempts"`
		Effect     Effect `json:"effect"`
		Message    string `json:"message"`
	}{safeCode(e.Code), e.HTTPStatus, e.Attempts, safeEffect(e.Effect), e.Error()})
}
func safeCode(c string) string {
	switch c {
	case CodeInvalidArgument, CodeAuthenticationFailed, CodePermissionDenied, CodeNotFoundOrHidden, CodeVersionUnavailable, CodeCASConflict, CodeCanceled, CodeDeadlineExceeded, CodeUnavailable, CodeInvalidResponse, CodeResponseTooLarge, CodeRedirectBlocked, CodeNotReady, CodeClosed:
		return c
	}
	return "sdk_error"
}
func safeEffect(e Effect) Effect {
	switch e {
	case EffectNone, EffectUnknown, EffectConfirmed:
		return e
	}
	return EffectUnknown
}
