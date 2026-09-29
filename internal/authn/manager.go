// Package authn owns authentication state. No business request triggers a
// refresh or replay; only this coordinator's lifecycle evidence can do so.
package authn

import (
	"context"
	"errors"
	"math/rand/v2"
	"sync"
	"time"

	"git.example.com/infra/openbao-sdk-go/auth"
	"git.example.com/infra/openbao-sdk-go/baoerr"
	"git.example.com/infra/openbao-sdk-go/diagnostics"
	"git.example.com/infra/openbao-sdk-go/internal/engine"
	"git.example.com/infra/openbao-sdk-go/sensitive"
)

type flight struct {
	done chan struct{}
	err  error
}
type Manager struct {
	cfg                auth.Config
	executor           *engine.Executor
	clock              Clock
	mu                 sync.Mutex
	token              sensitive.Bytes
	expires, next      time.Time
	renewable          bool
	sequence           uint64
	knownExternal      bool
	externalValidUntil time.Time
	last               error
	failures           int
	used               map[string]struct{}
	refreshing         *flight
	loopContext        context.Context
	loopCancel         context.CancelFunc
	loopDone           chan struct{}
	closed             bool
}

func New(c auth.Config, e *engine.Executor, clock Clock) *Manager {
	if clock == nil {
		clock = realClock{}
	}
	return &Manager{cfg: c, executor: e, clock: clock, used: make(map[string]struct{})}
}
func safeContext(ctx context.Context) error {
	if ctx == nil {
		return engine.SafeError(baoerr.CodeInvalidArgument, "AUTH", 0, baoerr.EffectNone)
	}
	switch ctx.Err() {
	case context.Canceled:
		return engine.SafeError(baoerr.CodeCanceled, "AUTH", 0, baoerr.EffectNone)
	case context.DeadlineExceeded:
		return engine.SafeError(baoerr.CodeDeadlineExceeded, "AUTH", 0, baoerr.EffectNone)
	}
	return nil
}
func providerError(ctx context.Context) error {
	if e := safeContext(ctx); e != nil {
		return e
	}
	return engine.SafeError(baoerr.CodeAuthenticationFailed, "AUTH", 0, baoerr.EffectNone)
}
func validSecret(s []byte) bool {
	if len(s) == 0 || len(s) > 64<<10 {
		return false
	}
	for _, b := range s {
		if b < 33 || b > 126 {
			return false
		}
	}
	return true
}
func safeTokenSnapshot(ctx context.Context, p auth.TokenProvider) (s auth.TokenSnapshot, err error) {
	defer func() {
		if recover() != nil {
			err = providerError(ctx)
		}
	}()
	return p.Snapshot(ctx)
}
func safeSecretSnapshot(ctx context.Context, p auth.SecretIDProvider) (s auth.SecretIDSnapshot, err error) {
	defer func() {
		if recover() != nil {
			err = providerError(ctx)
		}
	}()
	return p.Current(ctx)
}
func (m *Manager) Credential(ctx context.Context) (string, error) {
	if e := safeContext(ctx); e != nil {
		return "", e
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return "", engine.SafeError(baoerr.CodeClosed, "AUTH", 0, baoerr.EffectNone)
	}
	m.mu.Unlock()
	if m.cfg.Mode == auth.ExternalToken {
		s, err := safeTokenSnapshot(ctx, m.cfg.TokenProvider)
		if err != nil {
			e := providerError(ctx)
			m.recordExternal(false, time.Time{}, e)
			return "", e
		}
		raw := s.Token.RevealCopy()
		defer clear(raw)
		if !validSecret(raw) || !s.ValidUntil.IsZero() && !m.clock.Now().Before(s.ValidUntil) {
			e := providerError(ctx)
			m.recordExternal(false, s.ValidUntil, e)
			return "", e
		}
		if e := safeContext(ctx); e != nil {
			return "", e
		}
		m.recordExternal(true, s.ValidUntil, nil)
		return string(raw), nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return "", engine.SafeError(baoerr.CodeClosed, "AUTH", 0, baoerr.EffectNone)
	}
	if m.token.Len() == 0 || !m.clock.Now().Before(m.expires) {
		return "", providerError(ctx)
	}
	raw := m.token.RevealCopy()
	defer clear(raw)
	return string(raw), nil
}
func (m *Manager) recordExternal(ok bool, until time.Time, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.knownExternal = ok
	m.externalValidUntil = until
	m.last = err
}
func (m *Manager) State() diagnostics.ClientState {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := diagnostics.ClientState{AuthState: "AUTH_UNAVAILABLE"}
	if m.last != nil {
		var e *baoerr.Error
		if errors.As(m.last, &e) && e != nil {
			s.LastErrorCode = e.Code
		}
	}
	if m.closed {
		s.AuthState = "CLOSED"
		return s
	}
	until := m.expires
	if m.cfg.Mode == auth.ExternalToken {
		until = m.externalValidUntil
		s.Ready = m.knownExternal && (until.IsZero() || m.clock.Now().Before(until))
	} else {
		s.Ready = m.token.Len() > 0 && m.clock.Now().Before(until)
	}
	if !until.IsZero() {
		copy := until
		s.TokenExpiresAt = &copy
	}
	if s.Ready {
		if m.last != nil {
			s.AuthState = "DEGRADED"
		} else {
			s.AuthState = "READY"
		}
	}
	return s
}

// Refresh combines concurrent login/renewal attempts. A backoff after a failure
// applies even after expiry; expired credentials are never returned meanwhile.
func (m *Manager) Refresh(ctx context.Context) error {
	if e := safeContext(ctx); e != nil {
		return e
	}
	if m.cfg.Mode == auth.ExternalToken {
		_, e := m.Credential(ctx)
		return e
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return engine.SafeError(baoerr.CodeClosed, "AUTH", 0, baoerr.EffectNone)
	}
	if f := m.refreshing; f != nil {
		m.mu.Unlock()
		select {
		case <-f.done:
			return f.err
		case <-ctx.Done():
			return safeContext(ctx)
		}
	}
	now := m.clock.Now()
	if !m.next.IsZero() && now.Before(m.next) {
		e := m.last
		m.mu.Unlock()
		return e
	}
	f := &flight{done: make(chan struct{})}
	m.refreshing = f
	renew := m.renewable && m.token.Len() > 0 && now.Before(m.expires)
	seq := m.sequence
	raw := m.token.RevealCopy()
	m.mu.Unlock()
	defer clear(raw)
	var err error
	if renew {
		err = m.renew(ctx, raw, seq)
	} else {
		err = m.login(ctx)
	}
	m.mu.Lock()
	if err != nil {
		m.last = err
		m.failures++
		capSeconds := 1 << min(m.failures-1, 5)
		if capSeconds > 30 {
			capSeconds = 30
		}
		// Lower bounded jitter suppresses a tight login loop on unavailable files.
		delay := time.Second + time.Duration(rand.Int64N(int64(time.Duration(capSeconds)*time.Second)))
		m.next = m.clock.Now().Add(delay)
		if renew && (baoerr.IsCode(err, baoerr.CodeAuthenticationFailed) || baoerr.IsCode(err, baoerr.CodePermissionDenied) || baoerr.IsCode(err, baoerr.CodeInvalidArgument)) {
			m.renewable = false
		}
	}
	f.err = err
	m.refreshing = nil
	close(f.done)
	m.mu.Unlock()
	return err
}

// Run starts only lifecycle work after the caller has completed bounded Refresh.
func (m *Manager) Run(ctx context.Context) error {
	if e := safeContext(ctx); e != nil {
		return e
	}
	if m.cfg.Mode == auth.ExternalToken {
		return nil
	}
	for {
		m.mu.Lock()
		if m.closed {
			m.mu.Unlock()
			return engine.SafeError(baoerr.CodeClosed, "AUTH", 0, baoerr.EffectNone)
		}
		if done := m.loopDone; done != nil {
			select {
			case <-done:
				m.loopDone = nil
				m.loopCancel = nil
				m.loopContext = nil
			default:
				if m.loopContext != nil && m.loopContext.Err() == nil {
					m.mu.Unlock()
					return nil
				}
				m.mu.Unlock()
				select {
				case <-done:
					continue
				case <-ctx.Done():
					return safeContext(ctx)
				}
			}
		}
		if e := safeContext(ctx); e != nil {
			m.mu.Unlock()
			return e
		}
		run, cancel := context.WithCancel(ctx)
		done := make(chan struct{})
		m.loopContext = run
		m.loopCancel = cancel
		m.loopDone = done
		m.mu.Unlock()
		go func() { defer close(done); m.run(run) }()
		return nil
	}
}
func (m *Manager) run(ctx context.Context) {
	for {
		m.mu.Lock()
		next := m.next
		expiry := m.expires
		m.mu.Unlock()
		now := m.clock.Now()
		delay := next.Sub(now)
		if delay <= 0 {
			delay = time.Millisecond
		}
		if now.Before(expiry) && expiry.Before(next) {
			delay = expiry.Sub(now)
		}
		timer := m.clock.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C():
			timer.Stop()
		}
		if ctx.Err() != nil {
			return
		}
		_ = m.Refresh(ctx)
	}
}

// Stop ends lifecycle work without invalidating tokens used by admitted calls.
func (m *Manager) Stop(ctx context.Context) error {
	if e := safeContext(ctx); e != nil {
		m.mu.Lock()
		if m.loopCancel != nil {
			m.loopCancel()
		}
		m.mu.Unlock()
		return e
	}
	m.mu.Lock()
	cancel, done := m.loopCancel, m.loopDone
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return safeContext(ctx)
	}
}

// Zero is called after draining or canceling admitted requests.
func (m *Manager) Zero() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	m.token.Zero()
	if m.loopCancel != nil {
		m.loopCancel()
	}
}
