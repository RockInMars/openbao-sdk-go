package credentialworkflow

import (
	"context"
	"errors"
	"github.com/RockInMars/openbao-sdk-go/baoerr"
	"github.com/RockInMars/openbao-sdk-go/kv"
	"github.com/RockInMars/openbao-sdk-go/sensitive"
	"testing"
	"time"
)

type issueFixture struct {
	calls int
	err   error
}

func (f *issueFixture) Issue(ctx context.Context, g Generation) (sensitive.Bytes, error) {
	f.calls++
	return sensitive.NewBytes([]byte("generated-material")), f.err
}

type storageFixture struct {
	saved        *Stored
	unknownWrite bool
}

func (f *storageFixture) Find(ctx context.Context, g Generation) (*Stored, error) {
	if f.saved == nil {
		return nil, ErrNotStored
	}
	cp := *f.saved
	return &cp, nil
}
func (f *storageFixture) Create(ctx context.Context, g Generation, b sensitive.Bytes) (*Stored, error) {
	f.saved = &Stored{Generation: g, Ref: kv.Ref{ClusterAlias: "fixture", Namespace: "", Mount: "kv", Path: g.Path, Version: 1}}
	if f.unknownWrite {
		return nil, &baoerr.Error{Code: baoerr.CodeUnavailable, Effect: baoerr.EffectUnknown}
	}
	return f.Find(ctx, g)
}

type journalFixture struct {
	saved            *Record
	failCompleteOnce bool
}

func (f *journalFixture) Claim(ctx context.Context, r Record) (bool, error) {
	if f.saved != nil {
		return false, nil
	}
	f.saved = &r
	return true, nil
}
func (f *journalFixture) Load(ctx context.Context, g Generation) (*Record, error) {
	if f.saved == nil {
		return nil, ErrNotStored
	}
	cp := *f.saved
	return &cp, nil
}
func (f *journalFixture) Save(ctx context.Context, r Record) error {
	if r.Phase == Ready && f.failCompleteOnce {
		f.failCompleteOnce = false
		return errors.New("association unavailable")
	}
	f.saved = &r
	return nil
}
func generation() Generation {
	return Generation{OperationID: "op-1", ResourceGeneration: "generation-1", Path: "fixture/generation-1"}
}
func TestCredentialWorkflowRecovery(t *testing.T) {
	ctx := context.Background()
	i := &issueFixture{}
	s := &storageFixture{}
	j := &journalFixture{failCompleteOnce: true}
	w := Coordinator{Issuer: i, Store: s, Journal: j}
	if _, e := w.Prepare(ctx, generation()); e == nil {
		t.Fatal("database association failure hidden")
	}
	out, e := w.Prepare(ctx, generation())
	if e != nil || out.Phase != Ready || out.Ref.Version != 1 || i.calls != 1 {
		t.Fatal("recovery did not reuse exact ref")
	}
}
func TestCredentialWorkflowUnknownDoesNotReissue(t *testing.T) {
	i := &issueFixture{err: &baoerr.Error{Code: baoerr.CodeDeadlineExceeded, Effect: baoerr.EffectUnknown}}
	w := Coordinator{Issuer: i, Store: &storageFixture{}, Journal: &journalFixture{}}
	for range 2 {
		if _, e := w.Prepare(context.Background(), generation()); e == nil {
			t.Fatal("unknown issue accepted")
		}
	}
	if i.calls != 1 {
		t.Fatal("unknown issue was repeated")
	}
}
func TestCredentialWorkflowUnknownKVReconciles(t *testing.T) {
	i := &issueFixture{}
	w := Coordinator{Issuer: i, Store: &storageFixture{unknownWrite: true}, Journal: &journalFixture{}}
	r, e := w.Prepare(context.Background(), generation())
	if e != nil || r.Phase != Ready || i.calls != 1 {
		t.Fatal("committed KV result not reconciled")
	}
}
func TestCredentialWorkflowGenerationMismatch(t *testing.T) {
	wrong := generation()
	wrong.OperationID = "other"
	i := &issueFixture{}
	w := Coordinator{Issuer: i, Store: &storageFixture{saved: &Stored{Generation: wrong}}, Journal: &journalFixture{}}
	if _, e := w.Prepare(context.Background(), generation()); e == nil || i.calls != 0 {
		t.Fatal("foreign generation adopted")
	}
}
func TestRegistrationRequiresBothMaterials(t *testing.T) {
	if CanRegister(MaterialState{Ready: true, Version: 1, Validated: true, ValidUntil: time.Now().Add(time.Hour)}, MaterialState{}) {
		t.Fatal("missing business credentials accepted")
	}
	if !CanRegister(MaterialState{Ready: true, Version: 1, Validated: true, ValidUntil: time.Now().Add(time.Hour)}, MaterialState{Ready: true, Version: 2, Validated: true, ValidUntil: time.Now().Add(time.Hour)}) {
		t.Fatal("both validated credentials rejected")
	}
}

// A faulty adapter must not turn a missing row into a nil dereference.
type nilStorage struct{ storageFixture }

func (f *nilStorage) Find(context.Context, Generation) (*Stored, error) { return nil, nil }
func TestCredentialWorkflowRejectsNilStored(t *testing.T) {
	g := generation()
	i := &issueFixture{}
	j := &journalFixture{saved: &Record{Generation: g, Ref: kv.Ref{Version: 1}}}
	w := Coordinator{Issuer: i, Store: &nilStorage{}, Journal: j}
	if _, e := w.Prepare(context.Background(), g); !errors.Is(e, ErrMismatch) {
		t.Fatal("nil stored result not rejected")
	}
	if i.calls != 0 {
		t.Fatal("faulty storage caused reissue")
	}
}
