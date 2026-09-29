// Package credentialworkflow is an application-level example, NOT SDK runtime.
// Durable Journal and Store implementations belong to the consuming service.
package credentialworkflow

import (
	"context"
	"errors"
	"git.example.com/infra/openbao-sdk-go/kv"
	"git.example.com/infra/openbao-sdk-go/sensitive"
	"time"
)

var (
	ErrNotStored = errors.New("positively established absence")
	ErrUnknown   = errors.New("credential result unknown; reconcile or use a reviewed new generation, do not reissue automatically")
	ErrMismatch  = errors.New("credential generation or exact reference mismatch")
)

type Phase string

const (
	Claimed Phase = "CLAIMED"
	Unknown Phase = "UNKNOWN"
	Ready   Phase = "READY"
)

type Generation struct{ OperationID, ResourceGeneration, Path string }
type Stored struct {
	Generation Generation
	Ref        kv.Ref
}
type Record struct {
	Generation Generation
	Ref        kv.Ref
	Phase      Phase
}
type Issuer interface {
	Issue(context.Context, Generation) (sensitive.Bytes, error)
}

// Find must validate operation_id/resource_generation INSIDE the stored version.
// ErrNotStored means established absence; do NOT blindly map OpenBao's ambiguous
// 404/not_found_or_hidden to it. Verify permissions and an application-owned
// generation journal before deciding absence. Return any ambiguity as an error.
type Store interface {
	Find(context.Context, Generation) (*Stored, error)
	Create(context.Context, Generation, sensitive.Bytes) (*Stored, error)
}

// Claim is a durable atomic insert-if-absent on the generation. A database unique
// constraint/CAS implements it; a process mutex is not sufficient across replicas.
// Save must preserve the generation and reject conflicting transitions/references.
type Journal interface {
	Load(context.Context, Generation) (*Record, error)
	Claim(context.Context, Record) (bool, error)
	Save(context.Context, Record) error
}
type Coordinator struct {
	Issuer  Issuer
	Store   Store
	Journal Journal
}

func (w Coordinator) Prepare(ctx context.Context, g Generation) (*Record, error) {
	if ctx == nil || ctx.Err() != nil {
		if ctx != nil {
			return nil, ctx.Err()
		}
		return nil, ErrMismatch
	}
	if g.OperationID == "" || g.ResourceGeneration == "" || g.Path == "" || w.Issuer == nil || w.Store == nil || w.Journal == nil {
		return nil, ErrMismatch
	}
	previous, e := w.Journal.Load(ctx, g)
	if e != nil && !errors.Is(e, ErrNotStored) {
		return nil, e
	}
	if previous != nil && previous.Generation != g {
		return nil, ErrMismatch
	}
	stored, e := w.Store.Find(ctx, g)
	if e == nil {
		if stored == nil {
			return nil, ErrMismatch
		}
		if previous != nil && previous.Ref.Version > 0 && previous.Ref != stored.Ref {
			return nil, ErrMismatch
		}
		return w.complete(ctx, g, stored)
	}
	if !errors.Is(e, ErrNotStored) {
		return nil, e
	}
	// A prior durable claim survives process death after Issue. Even if the key was
	// never saved, restarting must not blindly issue a second certificate.
	if previous != nil {
		return nil, ErrUnknown
	}
	owned, e := w.Journal.Claim(ctx, Record{Generation: g, Phase: Claimed})
	if e != nil {
		return nil, e
	}
	if !owned {
		return nil, ErrUnknown
	}
	material, e := w.Issuer.Issue(ctx, g)
	defer material.Zero()
	if e != nil {
		return nil, errors.Join(e, w.Journal.Save(ctx, Record{Generation: g, Phase: Unknown}))
	}
	if material.Len() == 0 {
		return nil, ErrUnknown
	}
	stored, e = w.Store.Create(ctx, g, material)
	if e != nil {
		// Keep this exact material in memory while reconciling. Never call Issue here.
		reconciled, readErr := w.Store.Find(ctx, g)
		if readErr != nil {
			return nil, errors.Join(e, ErrUnknown)
		}
		stored = reconciled
	}
	return w.complete(ctx, g, stored)
}
func (w Coordinator) complete(ctx context.Context, g Generation, s *Stored) (*Record, error) {
	if s == nil || s.Generation != g || s.Ref.Path != g.Path || s.Ref.Version <= 0 || s.Ref.ClusterAlias == "" || s.Ref.Mount == "" {
		return nil, ErrMismatch
	}
	r := Record{Generation: g, Ref: s.Ref, Phase: Ready}
	if e := w.Journal.Save(ctx, r); e != nil {
		return nil, e
	}
	return &r, nil
}

type MaterialState struct {
	Ready, Validated bool
	Version          int
	ValidUntil       time.Time
}

// Both MQTT and business credentials must be validated and unexpired before a
// registration credential is issued. Authentication/authorization stay upstream.
func CanRegister(mqtt, business MaterialState) bool {
	now := time.Now()
	return mqtt.Ready && mqtt.Validated && mqtt.Version > 0 && mqtt.ValidUntil.After(now) && business.Ready && business.Validated && business.Version > 0 && business.ValidUntil.After(now)
}
