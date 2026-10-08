package remotecheck

import (
	"context"
	"strings"
	"sync"

	"github.com/RockInMars/openbao-sdk-go/observe"
)

// The observer only counts attempts. It never records labels, IDs, or payloads.
type budget struct {
	mu          sync.Mutex
	used, limit int
	cancel      context.CancelFunc
	categories  map[string]int
}

func (b *budget) Observe(_ context.Context, e observe.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.used += e.Attempts
	category := "sdk_other"
	if e.Operation == "APPROLE_LOGIN" || e.Operation == "TOKEN_RENEW" {
		category = "authn"
	} else {
		for _, prefix := range []string{"kv", "pki", "transit", "health"} {
			if strings.Contains(strings.ToLower(e.Operation), prefix) {
				category = prefix
				break
			}
		}
	}
	b.categories[category] += e.Attempts
	if b.used >= b.limit {
		b.cancel()
	}
}
func (b *budget) count() int { b.mu.Lock(); defer b.mu.Unlock(); return b.used }
func (b *budget) reserve(category string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.used >= b.limit {
		return false
	}
	b.used++
	b.categories[category]++
	return true
}

func (b *budget) counts() map[string]int {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := map[string]int{}
	for k, v := range b.categories {
		out[k] = v
	}
	return out
}
