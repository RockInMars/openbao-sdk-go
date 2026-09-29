package engine

import (
	"context"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func retryDelay(ctx context.Context, n int, c ExecutorConfig, h http.Header) (time.Duration, bool) {
	cap := c.BaseDelay
	for i := 1; i < n; i++ {
		if cap >= c.MaxDelay/2 {
			cap = c.MaxDelay
			break
		}
		cap *= 2
	}
	if cap > c.MaxDelay {
		cap = c.MaxDelay
	}
	delay := time.Duration(0)
	if cap > 0 {
		delay = time.Duration(rand.Int64N(int64(cap)))
	}
	if v := h.Get("Retry-After"); v != "" {
		var retryAfter time.Duration
		if seconds, e := strconv.ParseInt(strings.TrimSpace(v), 10, 64); e == nil {
			if seconds < 0 || seconds > int64(c.MaxDelay/time.Second) {
				return 0, false
			}
			retryAfter = time.Duration(seconds) * time.Second
		} else if date, e := http.ParseTime(v); e == nil {
			retryAfter = time.Until(date)
			if retryAfter < 0 {
				retryAfter = 0
			}
		} else {
			return 0, false
		}
		if retryAfter > c.MaxDelay {
			return 0, false
		}
		if retryAfter > delay {
			delay = retryAfter
		}
	}
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) <= delay {
		return 0, false
	}
	return delay, true
}
func wait(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
