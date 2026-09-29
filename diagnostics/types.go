// Package diagnostics contains the public SDK contract types.
package diagnostics

import (
	"time"
)

type ClientState struct {
	Lifecycle      string
	AuthState      string
	Ready          bool
	TokenExpiresAt *time.Time
	LastErrorCode  string
}
type Health struct {
	Initialized bool
	Sealed      bool
	Standby     bool
	HTTPStatus  int
	Version     string
	ClusterID   string
}
type ReadProbe struct {
	Mount   string
	Path    string
	Version int
}
type Readiness struct {
	Ready     bool
	ErrorCode string
}
