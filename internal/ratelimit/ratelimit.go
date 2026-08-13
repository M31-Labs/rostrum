// Package ratelimit provides small in-memory guards for public intake
// endpoints. Every limiter here is process-local: state resets when the
// process restarts, which is enough for a single-process deployment to
// stop one session or one IP address from growing the store without bound
// (SE-3b in the security-hardening spec).
package ratelimit

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"os"
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

// ValidateTrustedProxyCIDRs validates TRUSTED_PROXY_CIDRS before the server
// starts. An empty value keeps the safest default: ignore all forwarding
// headers and rate-limit by the direct network peer.
func ValidateTrustedProxyCIDRs(raw string) error {
	_, err := trustedProxyPrefixes(raw)
	return err
}

// ClientIP returns the closest untrusted address in the request path. It
// ignores X-Forwarded-For unless the direct peer belongs to an explicitly
// configured TRUSTED_PROXY_CIDRS network. Starting at the trusted peer and
// walking the chain right-to-left prevents a public client from choosing its
// own rate-limit identity by prepending a forged header value.
func ClientIP(r *http.Request) string {
	if r == nil || r.RemoteAddr == "" {
		return ""
	}
	peer, ok := parseRemoteAddr(r.RemoteAddr)
	if !ok {
		return strings.TrimSpace(r.RemoteAddr)
	}
	prefixes, err := trustedProxyPrefixes(os.Getenv("TRUSTED_PROXY_CIDRS"))
	if err != nil || !addressTrusted(peer, prefixes) {
		return peer.String()
	}

	forwarded := strings.Join(r.Header.Values("X-Forwarded-For"), ",")
	if strings.TrimSpace(forwarded) == "" {
		return peer.String()
	}
	// Work from the right because a conforming trusted proxy appends the
	// address it observed. Keep the amount of header text and the number of
	// proxy hops bounded; the far-left side is client-controlled and does not
	// need to be parsed once the closest untrusted address is known.
	const maxForwardedBytes = 8 << 10
	if len(forwarded) > maxForwardedBytes {
		forwarded = forwarded[len(forwarded)-maxForwardedBytes:]
		comma := strings.IndexByte(forwarded, ',')
		if comma < 0 {
			return peer.String()
		}
		forwarded = forwarded[comma+1:]
	}
	current := peer
	const maxForwardedHops = 32
	for hop := 0; hop < maxForwardedHops; hop++ {
		if !addressTrusted(current, prefixes) {
			return current.String()
		}
		comma := strings.LastIndexByte(forwarded, ',')
		raw := forwarded
		if comma >= 0 {
			raw = forwarded[comma+1:]
			forwarded = forwarded[:comma]
		} else {
			forwarded = ""
		}
		address, err := netip.ParseAddr(strings.TrimSpace(raw))
		if err != nil {
			// Only a malformed value inside the still-trusted suffix is
			// ambiguous. A malformed, attacker-controlled prefix is never read
			// after the observed client address stops the walk.
			return current.String()
		}
		current = address.Unmap()
		if forwarded == "" {
			return current.String()
		}
	}
	return current.String()
}

func parseRemoteAddr(value string) (netip.Addr, bool) {
	if addressPort, err := netip.ParseAddrPort(strings.TrimSpace(value)); err == nil {
		return addressPort.Addr().Unmap(), true
	}
	address, err := netip.ParseAddr(strings.TrimSpace(value))
	if err != nil {
		// Preserve compatibility with unusual net/http transports that expose
		// an unbracketed host:port value.
		host, _, splitErr := net.SplitHostPort(value)
		if splitErr != nil {
			return netip.Addr{}, false
		}
		address, err = netip.ParseAddr(host)
		if err != nil {
			return netip.Addr{}, false
		}
	}
	return address.Unmap(), true
}

func trustedProxyPrefixes(raw string) ([]netip.Prefix, error) {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
	prefixes := make([]netip.Prefix, 0, len(fields))
	for _, field := range fields {
		prefix, err := netip.ParsePrefix(field)
		if err != nil {
			return nil, fmt.Errorf("TRUSTED_PROXY_CIDRS contains invalid network %q", field)
		}
		prefixes = append(prefixes, prefix.Masked())
	}
	return prefixes, nil
}

func addressTrusted(address netip.Addr, prefixes []netip.Prefix) bool {
	for _, prefix := range prefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}
