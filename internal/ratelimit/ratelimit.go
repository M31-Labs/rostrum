// Package ratelimit provides small in-memory guards for public intake
// endpoints. Every limiter here is process-local: state resets when the
// process restarts, which is enough for a single-process demo deployment to
// stop one session or one IP address from growing the store without bound
// (SE-3b in the security-hardening spec).
package ratelimit

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"m31labs.dev/gosx/session"
)

// Counter caps the number of allowed events per key over the lifetime of the
// process. Use it for a hard cap such as "at most 5 submissions per
// session", where the count must never roll back down on its own.
type Counter struct {
	mu     sync.Mutex
	limit  int
	counts map[string]int
}

// NewCounter creates a Counter that allows at most limit events per key. A
// non-positive limit becomes 1, since a zero cap would silently lock out
// every caller instead of disabling the guard.
func NewCounter(limit int) *Counter {
	if limit <= 0 {
		limit = 1
	}
	return &Counter{limit: limit, counts: make(map[string]int)}
}

// Allow reports whether key may proceed, and records the event when it does.
// An empty key always proceeds, because it means the caller could not
// establish an identity and rejecting it would only reward that failure. A
// false result means the caller must not perform the guarded action; Allow
// never records a partial event.
func (c *Counter) Allow(key string) bool {
	if key == "" {
		return true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.counts[key] >= c.limit {
		return false
	}
	c.counts[key]++
	return true
}

// Count returns the current recorded count for key without recording a new
// event. Tests use this to assert state.
func (c *Counter) Count(key string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.counts[key]
}

// TokenBucket smooths a rate of events per key over time: each key refills
// toward capacity continuously and spends one token per allowed event. Use
// it for a rolling cap such as "10 per hour per IP address", where a caller
// should regain capacity over time rather than being blocked forever.
type TokenBucket struct {
	mu       sync.Mutex
	capacity float64
	refill   float64 // tokens added per second
	buckets  map[string]*bucketState
	now      func() time.Time
}

type bucketState struct {
	tokens   float64
	lastSeen time.Time
}

// NewTokenBucket creates a bucket that allows capacity events immediately
// per key, then refills to capacity again over window. A non-positive
// capacity becomes 1 and a non-positive window becomes one hour, for the
// same reason NewCounter refuses a zero limit.
func NewTokenBucket(capacity int, window time.Duration) *TokenBucket {
	if capacity <= 0 {
		capacity = 1
	}
	if window <= 0 {
		window = time.Hour
	}
	return &TokenBucket{
		capacity: float64(capacity),
		refill:   float64(capacity) / window.Seconds(),
		buckets:  make(map[string]*bucketState),
		now:      time.Now,
	}
}

// Allow reports whether key may proceed, and spends one token when it does.
// An empty key always proceeds, matching Counter.Allow.
func (b *TokenBucket) Allow(key string) bool {
	if key == "" {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	state, ok := b.buckets[key]
	if !ok {
		b.buckets[key] = &bucketState{tokens: b.capacity - 1, lastSeen: now}
		return true
	}
	elapsed := now.Sub(state.lastSeen).Seconds()
	if elapsed > 0 {
		state.tokens += elapsed * b.refill
		if state.tokens > b.capacity {
			state.tokens = b.capacity
		}
		state.lastSeen = now
	}
	if state.tokens < 1 {
		return false
	}
	state.tokens--
	return true
}

// RequestIdentity derives a stable rate-limit key for an inbound request: the
// session's CSRF token when a session is established, otherwise the caller's
// IP address. GoSX's session middleware and CSRF protection run before an
// Action, so a browser posting a real form always carries a session token by
// the time the action runs; the IP fallback only matters for a client that
// skips cookies entirely.
func RequestIdentity(r *http.Request) string {
	if r == nil {
		return ""
	}
	if token := strings.TrimSpace(session.Token(r)); token != "" {
		return "session:" + token
	}
	if ip := ClientIP(r); ip != "" {
		return "ip:" + ip
	}
	return ""
}

// ClientIP extracts the caller's address from r.RemoteAddr. It does not
// trust forwarded headers, which a client can set to any value, so it only
// reports what the network connection itself reveals.
func ClientIP(r *http.Request) string {
	if r == nil || r.RemoteAddr == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
