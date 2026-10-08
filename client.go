package bao

import (
	"context"
	"errors"
	"github.com/RockInMars/openbao-sdk-go/baoerr"
	"github.com/RockInMars/openbao-sdk-go/diagnostics"
	"github.com/RockInMars/openbao-sdk-go/internal/authn"
	"github.com/RockInMars/openbao-sdk-go/internal/engine"
	"github.com/RockInMars/openbao-sdk-go/observe"
	"sync"
	"time"
)

// Client has a fixed cluster, namespace and authentication scope. Do not copy it.
type Client struct {
	cfg            Config
	engine, health *engine.Executor
	manager        *authn.Manager
	observer       observe.Observer
	mu             sync.Mutex
	lifecycle      string
	starting       *startAttempt
	run            context.Context
	cancel         context.CancelFunc
	requests       context.Context
	cancelRequests context.CancelFunc
	active         int
	drained        chan struct{}
	closeDone      chan struct{}
}

type startAttempt struct {
	done chan struct{}
	err  error
}

// New validates configuration and prepares a client without network I/O or
// background goroutines. It copies configured RoleID and in-memory TLS secrets;
// failed construction clears those copies, leaving caller-owned values intact.
// Call Start before business operations and Close when the client is no longer used.
func New(cfg Config, opts ...Option) (*Client, error) { return newClient(cfg, false, opts...) }
func newClient(cfg Config, loopbackHTTP bool, opts ...Option) (*Client, error) {
	c, err := normalizeConfig(cfg, loopbackHTTP)
	if err != nil {
		return nil, err
	}
	return newClientFromConfig(c, opts...)
}

// newClientFromConfig takes ownership of the copies made by normalizeConfig.
func newClientFromConfig(c Config, opts ...Option) (client *Client, err error) {
	defer func() {
		if client == nil {
			zeroConfigSecrets(c)
		}
	}()
	settings := clientOptions{}
	for _, o := range opts {
		if o.apply == nil {
			return nil, invalid("OPTION")
		}
		if err := o.apply(&settings); err != nil {
			return nil, err
		}
	}
	business, err := transportFor(c)
	if err != nil {
		return nil, err
	}
	sender, err := newProtocolSender(c.Address, c.Namespace.Path, engine.NewHTTPClient(business), business)
	if err != nil {
		business.CloseIdleConnections()
		return nil, err
	}
	healthTr, err := transportFor(c)
	if err != nil {
		business.CloseIdleConnections()
		return nil, err
	}
	healthSender, err := newProtocolSender(c.Address, "", engine.NewHTTPClient(healthTr), healthTr)
	if err != nil {
		business.CloseIdleConnections()
		healthTr.CloseIdleConnections()
		return nil, err
	}
	ec := engine.ExecutorConfig{RequestTimeout: c.Timeouts.Request, IssueTimeout: c.Timeouts.PKIIssue, LoginTimeout: c.Timeouts.Login, RenewTimeout: c.Timeouts.Renew, Concurrency: c.Limits.MaxConcurrentRequests, MaxRequestBytes: c.Limits.MaxRequestBytes, MaxAttempts: c.ReadRetry.MaxAttempts, BaseDelay: c.ReadRetry.BaseDelay, MaxDelay: c.ReadRetry.MaxDelay, Namespace: c.Namespace.Path}
	ex := engine.NewExecutor(sender, ec)
	ec.Namespace = ""
	hx := engine.NewExecutor(healthSender, ec)
	// This context owns requests even before Start (unauthenticated health).
	// Constructing it does not start a goroutine or perform any network I/O.
	requests, cancelRequests := context.WithCancel(context.Background())
	manager := authn.New(c.Auth, ex, nil, func(ctx context.Context, op engine.Operation, attempts int) {
		if settings.observer != nil {
			settings.observer.Observe(ctx, observe.Event{Operation: string(op), ClusterAlias: c.ClusterAlias, Attempts: attempts})
		}
	})
	return &Client{cfg: c, engine: ex, health: hx, manager: manager, observer: settings.observer, lifecycle: "CREATED", requests: requests, cancelRequests: cancelRequests}, nil
}
func contextErr(ctx context.Context, op engine.Operation) error {
	if ctx == nil {
		return invalid(string(op))
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return engine.SafeError(baoerr.CodeCanceled, op, 0, baoerr.EffectNone)
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return engine.SafeError(baoerr.CodeDeadlineExceeded, op, 0, baoerr.EffectNone)
	}
	return nil
}

// Start initializes authentication. Its non-nil context owns the client's running
// lifetime, so use a service-lifetime context, not a short request context.
// ExternalToken checks the provider snapshot locally; success does not prove
// server reachability or permissions. ManagedAppRole logs in and manages refresh.
func (c *Client) Start(ctx context.Context) error {
	if err := contextErr(ctx, "START"); err != nil {
		return err
	}
	c.mu.Lock()
	switch c.lifecycle {
	case "CLOSING", "CLOSED":
		c.mu.Unlock()
		return engine.SafeError(baoerr.CodeClosed, "START", 0, baoerr.EffectNone)
	case "READY":
		run := c.run
		c.mu.Unlock()
		if run != nil {
			return contextErr(run, "START")
		}
		return nil
	case "STARTING":
		attempt := c.starting
		c.mu.Unlock()
		select {
		case <-attempt.done:
			return attempt.err
		case <-ctx.Done():
			return contextErr(ctx, "START")
		}
	}
	run, cancel := context.WithCancel(ctx)
	c.run = run
	c.cancel = cancel
	c.lifecycle = "STARTING"
	attempt := &startAttempt{done: make(chan struct{})}
	c.starting = attempt
	c.active++
	c.mu.Unlock()
	loginCtx, loginCancel := context.WithTimeout(run, c.cfg.Timeouts.Login)
	// Refresh uses the login budget, while the continuing loop uses run's lifetime.
	err := c.manager.Refresh(loginCtx)
	loginCancel()
	if err == nil {
		err = c.manager.Run(run)
	}
	if err == nil && run.Err() != nil {
		err = contextErr(run, "START")
	}
	c.mu.Lock()
	if c.lifecycle == "CLOSING" || c.lifecycle == "CLOSED" {
		err = engine.SafeError(baoerr.CodeClosed, "START", 0, baoerr.EffectNone)
	} else if err != nil {
		c.lifecycle = "CREATED"
		cancel()
	} else {
		c.lifecycle = "READY"
	}
	attempt.err = err
	c.active--
	c.signalDrain()
	close(attempt.done)
	c.mu.Unlock()
	return err
}
func (c *Client) signalDrain() {
	if c.active == 0 && c.drained != nil {
		select {
		case <-c.drained:
		default:
			close(c.drained)
		}
	}
}

// Close stops admission, drains operations within ctx, cancels remaining work,
// and clears SDK-owned credentials. It is safe to call repeatedly, including
// before Start. Use a separate non-nil shutdown context. Close does not revoke
// server credentials or erase shared providers, caller inputs, or revealed copies.
func (c *Client) Close(ctx context.Context) error {
	if ctx == nil {
		return invalid("CLOSE")
	}
	c.mu.Lock()
	if c.lifecycle == "CLOSED" {
		c.mu.Unlock()
		return nil
	}
	if c.lifecycle == "CLOSING" {
		done := c.closeDone
		c.mu.Unlock()
		select {
		case <-done:
			return nil
		case <-ctx.Done():
			return contextErr(ctx, "CLOSE")
		}
	}
	starting := c.lifecycle == "STARTING"
	c.lifecycle = "CLOSING"
	c.closeDone = make(chan struct{})
	c.drained = make(chan struct{})
	c.signalDrain()
	drained := c.drained
	cancel := c.cancel
	c.mu.Unlock()
	if starting && cancel != nil {
		cancel()
	}
	stopErr := c.manager.Stop(ctx)
	var err error
	select {
	case <-drained:
	case <-ctx.Done():
		err = contextErr(ctx, "CLOSE")
	}
	if cancel != nil {
		cancel()
	}
	// The service run context may never have existed; all admitted health
	// requests still have to stop once the shutdown drain budget is exhausted.
	c.cancelRequests()
	c.engine.CloseIdleConnections()
	c.health.CloseIdleConnections()
	c.manager.Zero()
	zeroConfigSecrets(c.cfg)
	c.mu.Lock()
	c.lifecycle = "CLOSED"
	close(c.closeDone)
	c.mu.Unlock()
	if err != nil {
		return err
	}
	return stopErr
}

// State returns a local lifecycle/authentication snapshot without network I/O.
// Ready is not proof that any particular server operation is authorized.
func (c *Client) State() diagnostics.ClientState {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.manager.State()
	s.Lifecycle = c.lifecycle
	if c.lifecycle != "READY" {
		s.Ready = false
	} else if c.run != nil && c.run.Err() != nil {
		s.Ready = false
		s.AuthState = "AUTH_UNAVAILABLE"
		s.LastErrorCode = baoerr.CodeCanceled
	} else if s.AuthState == "DEGRADED" || s.AuthState == "AUTH_UNAVAILABLE" {
		s.Lifecycle = s.AuthState
	}
	return s
}
func (c *Client) admit(ctx context.Context, op engine.Operation) (context.Context, func(), error) {
	if e := contextErr(ctx, op); e != nil {
		return nil, nil, e
	}
	c.mu.Lock()
	if c.lifecycle == "CLOSED" || c.lifecycle == "CLOSING" {
		c.mu.Unlock()
		return nil, nil, engine.SafeError(baoerr.CodeClosed, op, 0, baoerr.EffectNone)
	}
	if c.lifecycle != "READY" && op != engine.ClusterHealth {
		c.mu.Unlock()
		return nil, nil, engine.SafeError(baoerr.CodeNotReady, op, 0, baoerr.EffectNone)
	}
	c.active++
	run := c.run
	requests := c.requests
	c.mu.Unlock()
	joined, cancel := context.WithCancel(ctx)
	stop := func() bool { return false }
	if run != nil {
		stop = context.AfterFunc(run, cancel)
		if run.Err() != nil {
			cancel()
		}
	}
	stopRequests := context.AfterFunc(requests, cancel)
	if requests.Err() != nil {
		cancel()
	}
	return joined, func() {
		stopRequests()
		stop()
		cancel()
		c.mu.Lock()
		c.active--
		c.signalDrain()
		c.mu.Unlock()
	}, nil
}
func (c *Client) execute(parent context.Context, call engine.Call, mount string, decode func(*engine.Response) error) (err error) {
	started := time.Now()
	var resp *engine.Response
	defer func() {
		event := observe.Event{Operation: string(call.Operation), ClusterAlias: c.cfg.ClusterAlias, MountLabel: mount, Duration: time.Since(started)}
		if resp != nil {
			event.Attempts = resp.Attempts
			event.HTTPStatus = resp.Status
			event.RequestID = resp.RequestID
			resp.Zero()
		}
		var be *baoerr.Error
		if errors.As(err, &be) && be != nil {
			event.ErrorCode = be.Code
			if event.Attempts == 0 {
				event.Attempts = be.Attempts
			}
		}
		if c.observer != nil {
			func() { defer func() { _ = recover() }(); c.observer.Observe(parent, event) }()
		}
	}()
	if parent == nil {
		return invalid(string(call.Operation))
	}
	budget, cancel := context.WithTimeout(parent, c.engine.Budget(call.Operation))
	defer cancel()
	ctx, done, e := c.admit(budget, call.Operation)
	if e != nil {
		return e
	}
	defer done()
	ex := c.engine
	if call.Operation == engine.ClusterHealth {
		ex = c.health
		call.Credential = nil
	} else {
		call.Credential = c.manager.Credential
	}
	resp, err = ex.Execute(ctx, call)
	if err != nil {
		return err
	}
	if decode != nil {
		return decode(resp)
	}
	return nil
}
