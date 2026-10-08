package engine

import (
	"context"
	"crypto/x509"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/RockInMars/openbao-sdk-go/baoerr"
)

type boundarySender func(context.Context, Request) (*Response, error)

func (f boundarySender) Send(ctx context.Context, r Request) (*Response, error) { return f(ctx, r) }
func (f boundarySender) CloseIdleConnections()                                  {}
func boundaryExecutor(s boundarySender) *Executor {
	return NewExecutor(s, ExecutorConfig{RequestTimeout: time.Second, IssueTimeout: 2 * time.Second, LoginTimeout: 3 * time.Second, RenewTimeout: 4 * time.Second, Concurrency: 1, MaxRequestBytes: 128, MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: time.Second})
}
func boundaryCall(op Operation) Call {
	return Call{Operation: op, Path: "/v1/mount/item", Credential: func(context.Context) (string, error) { return "test-token", nil }}
}

func TestExecuteRejectsInputAndCredentialsBeforeSend(t *testing.T) {
	var sends atomic.Int32
	e := boundaryExecutor(func(context.Context, Request) (*Response, error) {
		sends.Add(1)
		return &Response{Status: 200, Body: []byte(`{}`)}, nil
	})
	for _, change := range []func(*Call){
		func(c *Call) { c.Operation = "UNKNOWN" }, func(c *Call) { c.Path = "/not-v1" }, func(c *Call) { c.Path = "/v1/../x" },
		func(c *Call) { c.Payload = []byte(strings.Repeat("a", 129)) }, func(c *Call) { c.Payload = []byte(`[]`) },
	} {
		c := boundaryCall(KVReadVersion)
		change(&c)
		if _, err := e.Execute(context.Background(), c); !baoerr.IsCode(err, baoerr.CodeInvalidArgument) {
			t.Fatal(err)
		}
	}
	if _, err := e.Execute(nil, boundaryCall(KVReadVersion)); !baoerr.IsCode(err, baoerr.CodeInvalidArgument) {
		t.Fatal(err)
	}
	for _, token := range []string{"", "bad\ntoken", strings.Repeat("a", 65537)} {
		c := boundaryCall(KVReadVersion)
		c.Credential = func(context.Context) (string, error) { return token, nil }
		if _, err := e.Execute(context.Background(), c); !baoerr.IsCode(err, baoerr.CodeAuthenticationFailed) {
			t.Fatal(err)
		}
	}
	for _, failure := range []error{errors.New("secret must not escape"), SafeError(baoerr.CodePermissionDenied, KVReadVersion, 0, baoerr.EffectNone)} {
		c := boundaryCall(KVReadVersion)
		c.Credential = func(context.Context) (string, error) { return "", failure }
		if _, err := e.Execute(context.Background(), c); err == nil || strings.Contains(err.Error(), "secret must not escape") {
			t.Fatal("provider failure leaked or accepted")
		}
	}
	c := boundaryCall(KVReadVersion)
	c.Credential = nil
	if _, err := e.Execute(context.Background(), c); !baoerr.IsCode(err, baoerr.CodeNotReady) {
		t.Fatal(err)
	}
	if sends.Load() != 0 {
		t.Fatal("rejected call was sent")
	}
}

func TestBudgetAndUnauthenticatedHealth(t *testing.T) {
	e := boundaryExecutor(func(_ context.Context, r Request) (*Response, error) {
		if r.Token != "" {
			t.Error("unexpected token")
		}
		return &Response{Status: 503}, nil
	})
	for op, want := range map[Operation]time.Duration{PKIIssue: 2 * time.Second, PKISignCSR: 2 * time.Second, AppRoleLogin: 3 * time.Second, TokenRenew: 4 * time.Second, KVReadVersion: time.Second} {
		if e.Budget(op) != want {
			t.Fatalf("wrong budget for %s", op)
		}
	}
	for _, status := range []int{200, 429, 472, 473, 501, 503} {
		e.sender = boundarySender(func(context.Context, Request) (*Response, error) { return &Response{Status: status}, nil })
		c := boundaryCall(ClusterHealth)
		c.Credential = nil
		if r, err := e.Execute(context.Background(), c); err != nil || r.Status != status {
			t.Fatalf("health %d: %v", status, err)
		}
	}
}

func TestQueuedCancellationDoesNotSendAndReleasesSlot(t *testing.T) {
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	done := make(chan error, 1)
	var sends atomic.Int32
	e := boundaryExecutor(func(ctx context.Context, _ Request) (*Response, error) {
		if sends.Add(1) == 1 {
			entered <- struct{}{}
			select {
			case <-release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return &Response{Status: 200, Body: []byte(`{}`)}, nil
	})
	go func() { _, err := e.Execute(context.Background(), boundaryCall(KVReadVersion)); done <- err }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("first call did not acquire slot")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := e.Execute(ctx, boundaryCall(KVReadVersion))
	close(release)
	if !baoerr.IsCode(err, baoerr.CodeDeadlineExceeded) || sends.Load() != 1 {
		t.Fatalf("queued call sent: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, err := e.Execute(context.Background(), boundaryCall(KVReadVersion)); err != nil || sends.Load() != 2 {
		t.Fatal("slot leaked", err)
	}
}

func TestSuccessBodyAndVoidContracts(t *testing.T) {
	for _, tc := range []struct {
		op     Operation
		status int
		body   string
		ok     bool
	}{
		{KVReadVersion, 200, "", false}, {KVReadVersion, 204, "", false}, {KVDelete, 200, "", true}, {KVDelete, 204, "", true},
		{KVDelete, 200, `{"errors":[]}`, true}, {KVDelete, 200, `{"errors":["bad"]}`, false}, {KVDelete, 200, `broken`, false},
	} {
		e := boundaryExecutor(func(context.Context, Request) (*Response, error) {
			return &Response{Status: tc.status, Body: []byte(tc.body)}, nil
		})
		_, err := e.Execute(context.Background(), boundaryCall(tc.op))
		if (err == nil) != tc.ok {
			t.Fatalf("%s %d %q: %v", tc.op, tc.status, tc.body, err)
		}
	}
}

func TestHTTPErrorEnvelopeAndCASClassification(t *testing.T) {
	write, _ := Lookup(KVCAS)
	for _, tc := range []struct {
		status     int
		body, code string
		unknown    bool
	}{
		{400, `{"errors":["check-and-set parameter did not match the current version"]}`, baoerr.CodeCASConflict, false},
		{400, `{"errors":["check-and-set parameter did not match the current version!"]}`, baoerr.CodeInvalidArgument, false},
		{400, `{"errors":["check-and-set parameter did not match the current version","extra"]}`, baoerr.CodeInvalidArgument, false},
		{401, `{"errors":["denied"]}`, baoerr.CodeAuthenticationFailed, false}, {403, `{"errors":["denied"]}`, baoerr.CodePermissionDenied, false},
		{400, `{}`, baoerr.CodeInvalidResponse, true}, {401, `{"errors":[]}`, baoerr.CodeInvalidResponse, true}, {403, `{"errors":[1]}`, baoerr.CodeInvalidResponse, true},
		{403, `{"errors":[""]}`, baoerr.CodeInvalidResponse, true}, {418, `{}`, baoerr.CodeInvalidResponse, true}, {503, `broken`, baoerr.CodeInvalidResponse, true},
		{307, "", baoerr.CodeRedirectBlocked, true}, {308, "", baoerr.CodeRedirectBlocked, true},
	} {
		err, retry := responseError(&Response{Status: tc.status, Body: []byte(tc.body), Attempts: 1}, KVCAS, write)
		if !baoerr.IsCode(err, tc.code) || baoerr.HasUnknownOutcome(err) != tc.unknown || retry {
			t.Fatalf("HTTP %d: %v retry=%v", tc.status, err, retry)
		}
	}
}

func TestTransportFailureClassification(t *testing.T) {
	write, _ := Lookup(PKIIssue)
	for _, tc := range []struct {
		cause   error
		sent    bool
		code    string
		unknown bool
	}{
		{errors.New("connection"), false, baoerr.CodeUnavailable, false}, {errors.New("connection"), true, baoerr.CodeUnavailable, true},
		{context.Canceled, true, baoerr.CodeCanceled, true}, {context.DeadlineExceeded, true, baoerr.CodeDeadlineExceeded, true},
		{ErrResponseTooLarge, true, baoerr.CodeResponseTooLarge, true}, {ErrInvalidResponse, true, baoerr.CodeInvalidResponse, true},
		{x509.UnknownAuthorityError{}, true, baoerr.CodeUnavailable, false},
	} {
		err, retry := transportError(&TransportFailure{Cause: tc.cause, Sent: tc.sent}, PKIIssue, write, 1)
		if !baoerr.IsCode(err, tc.code) || baoerr.HasUnknownOutcome(err) != tc.unknown || retry {
			t.Fatalf("transport classified incorrectly: %v", err)
		}
	}
	read, _ := Lookup(KVReadVersion)
	_, retry := transportError(errors.New("connection"), KVReadVersion, read, 1)
	if !retry {
		t.Fatal("safe transport read cannot retry")
	}
}

func TestRetryWaitCancellationStopsFurtherAttempts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var sends atomic.Int32
	e := boundaryExecutor(func(context.Context, Request) (*Response, error) {
		sends.Add(1)
		time.AfterFunc(10*time.Millisecond, cancel)
		return &Response{Status: 503, Header: http.Header{"Retry-After": []string{"1"}}, Body: []byte(`{}`)}, nil
	})
	e.cfg.RequestTimeout = 3 * time.Second
	_, err := e.Execute(ctx, boundaryCall(KVReadVersion))
	if !baoerr.IsCode(err, baoerr.CodeCanceled) || sends.Load() != 1 {
		t.Fatal("retry continued after cancellation", err)
	}
}
