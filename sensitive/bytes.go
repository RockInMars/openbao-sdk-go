// Package sensitive contains explicitly revealed, best-effort erasable secret material.
package sensitive

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
)

type cell struct {
	mu    sync.RWMutex
	value []byte
}

// Bytes never formats or marshals its contents implicitly. Value copies share an
// erasure handle; use NewBytes(b.RevealCopy()) for an independently owned value.
// Zero cannot erase copies previously revealed or made by the Go runtime.
type Bytes struct{ cell *cell }

func NewBytes(value []byte) Bytes { return Bytes{cell: &cell{value: append([]byte(nil), value...)}} }
func (b Bytes) RevealCopy() []byte {
	if b.cell == nil {
		return nil
	}
	b.cell.mu.RLock()
	defer b.cell.mu.RUnlock()
	return append([]byte(nil), b.cell.value...)
}
func (b *Bytes) Zero() {
	if b == nil || b.cell == nil {
		return
	}
	b.cell.mu.Lock()
	defer b.cell.mu.Unlock()
	clear(b.cell.value)
	b.cell.value = nil
}
func (b Bytes) Len() int {
	if b.cell == nil {
		return 0
	}
	b.cell.mu.RLock()
	defer b.cell.mu.RUnlock()
	return len(b.cell.value)
}
func (b Bytes) String() string                    { return "[REDACTED]" }
func (b Bytes) GoString() string                  { return "[REDACTED]" }
func (b Bytes) Format(state fmt.State, verb rune) { _, _ = io.WriteString(state, "[REDACTED]") }
func (b Bytes) LogValue() slog.Value              { return slog.StringValue("[REDACTED]") }
func (b Bytes) MarshalJSON() ([]byte, error) {
	return nil, errors.New("sensitive material requires explicit reveal")
}
