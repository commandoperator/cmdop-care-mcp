// Package ratelimit provides a simple per-tool, per-process token bucket.
// It is defense-in-depth against a steered/compromised MCP client polling a
// tool as a covert exfiltration channel (security review §7, §8, §12) — this
// artifact runs unsupervised on arbitrary public users' machines, unlike the
// internal product, so the MCP layer itself now caps call rate rather than
// relying solely on the relay's own limits (which this artifact does not
// control and may not have any).
package ratelimit

import (
	"fmt"
	"sync"

	"golang.org/x/time/rate"
)

// Limiter caps calls per tool name using one token bucket per tool.
type Limiter struct {
	mu      sync.Mutex
	buckets map[string]*rate.Limiter
	rate    rate.Limit
	burst   int
}

// New builds a Limiter allowing `perSecond` steady-state calls per tool with
// a burst of `burst` calls. A tool name not seen before gets its own fresh
// bucket lazily, so tool registration order does not matter.
func New(perSecond float64, burst int) *Limiter {
	return &Limiter{
		buckets: make(map[string]*rate.Limiter),
		rate:    rate.Limit(perSecond),
		burst:   burst,
	}
}

// Allow reports whether a call to the named tool may proceed right now. It
// never blocks — a denied call should fail fast with a clear error, not
// stall the stdio loop.
func (l *Limiter) Allow(tool string) bool {
	l.mu.Lock()
	b, ok := l.buckets[tool]
	if !ok {
		b = rate.NewLimiter(l.rate, l.burst)
		l.buckets[tool] = b
	}
	l.mu.Unlock()
	return b.Allow()
}

// ErrRateLimited is returned (wrapped with the tool name) by callers that
// choose to surface Allow()==false as an error.
type ErrRateLimited struct{ Tool string }

func (e *ErrRateLimited) Error() string {
	return fmt.Sprintf("rate limit exceeded for tool %q — slow down and retry shortly", e.Tool)
}
