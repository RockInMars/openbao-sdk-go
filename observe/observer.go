// Package observe contains the public SDK contract types.
package observe

import (
	"context"
	"time"
)

type Event struct {
	Operation    string
	ClusterAlias string
	MountLabel   string
	Duration     time.Duration
	Attempts     int
	HTTPStatus   int
	ErrorCode    string
	RequestID    string
}
type Observer interface {
	Observe(ctx context.Context, event Event)
}
