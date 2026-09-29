package authn

import (
	"context"
	"encoding/json"
	"math"
	"math/rand/v2"
	"time"

	"git.example.com/infra/openbao-sdk-go/auth"
	"git.example.com/infra/openbao-sdk-go/baoerr"
	"git.example.com/infra/openbao-sdk-go/internal/engine"
	"git.example.com/infra/openbao-sdk-go/sensitive"
)

func (m *Manager) login(parent context.Context) error {
	ctx, cancel := context.WithTimeout(parent, m.executor.Budget(engine.AppRoleLogin))
	defer cancel()
	started := m.clock.Now()
	if m.cfg.AppRole == nil {
		return providerError(ctx)
	}
	snap, err := safeSecretSnapshot(ctx, m.cfg.AppRole.SecretIDProvider)
	if err != nil {
		return providerError(ctx)
	}
	secret := snap.SecretID.RevealCopy()
	role := m.cfg.AppRole.RoleID.RevealCopy()
	defer clear(secret)
	defer clear(role)
	if !validSecret(secret) || !validSecret(role) || len(snap.Generation) == 0 || len(snap.Generation) > 256 || (snap.Use != auth.ReusableSecretID && snap.Use != auth.SingleUseSecretID) {
		return providerError(ctx)
	}
	if err = safeContext(ctx); err != nil {
		return err
	}
	if snap.Use == auth.SingleUseSecretID {
		m.mu.Lock()
		_, used := m.used[snap.Generation]
		if !used {
			m.used[snap.Generation] = struct{}{}
		}
		m.mu.Unlock()
		if used {
			return providerError(ctx)
		}
	}
	body, err := json.Marshal(map[string]string{"role_id": string(role), "secret_id": string(secret)})
	if err != nil {
		return providerError(ctx)
	}
	defer clear(body)
	resp, err := m.executor.Execute(ctx, engine.Call{Operation: engine.AppRoleLogin, Path: "/v1/auth/" + m.cfg.AppRole.Mount + "/login", Payload: body})
	if resp != nil {
		defer resp.Zero()
	}
	if err != nil {
		return err
	}
	return m.publish(resp, engine.AppRoleLogin, started, 0)
}
func (m *Manager) renew(parent context.Context, token []byte, seq uint64) error {
	ctx, cancel := context.WithTimeout(parent, m.executor.Budget(engine.TokenRenew))
	defer cancel()
	started := m.clock.Now()
	resp, err := m.executor.Execute(ctx, engine.Call{Operation: engine.TokenRenew, Path: "/v1/auth/token/renew-self", Payload: []byte(`{}`), Credential: func(context.Context) (string, error) { return string(token), nil }})
	if resp != nil {
		defer resp.Zero()
	}
	if err != nil {
		return err
	}
	return m.publish(resp, engine.TokenRenew, started, seq)
}
func (m *Manager) publish(r *engine.Response, op engine.Operation, started time.Time, expected uint64) error {
	root, err := engine.DecodeObject(r, op)
	if err != nil {
		return err
	}
	a, ok := root["auth"].(map[string]any)
	if !ok {
		return engine.InvalidResponse(r, op)
	}
	value, ok := a["client_token"].(string)
	if !ok || !validSecret([]byte(value)) {
		return engine.InvalidResponse(r, op)
	}
	ttl, ok := engine.Int(a["lease_duration"])
	if !ok || ttl <= 0 || int64(ttl) > math.MaxInt64/int64(time.Second) {
		return engine.InvalidResponse(r, op)
	}
	renewable, ok := a["renewable"].(bool)
	if !ok {
		return engine.InvalidResponse(r, op)
	}
	if uses, exists := a["num_uses"]; exists {
		n, ok := engine.Int(uses)
		if !ok || n != 0 {
			return engine.InvalidResponse(r, op)
		}
	}
	expires := started.Add(time.Duration(ttl) * time.Second)
	now := m.clock.Now()
	remaining := expires.Sub(now)
	if remaining <= 0 {
		return engine.InvalidResponse(r, op)
	}
	b := sensitive.NewBytes([]byte(value))
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		b.Zero()
		return engine.SafeError(baoerr.CodeClosed, op, r.Attempts, baoerr.EffectUnknown)
	}
	if expected != 0 && m.sequence != expected {
		b.Zero()
		return engine.InvalidResponse(r, op)
	}
	m.token.Zero()
	m.token = b
	m.expires = expires
	m.renewable = renewable
	m.sequence++
	// Refresh at 55–65% of the actual remaining lease, not the previous TTL.
	fraction := 0.55 + rand.Float64()*0.10
	m.next = now.Add(time.Duration(float64(remaining) * fraction))
	m.last = nil
	m.failures = 0
	return nil
}
