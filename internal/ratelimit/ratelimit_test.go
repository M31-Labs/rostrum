package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCounterAllowsUpToLimitThenRejects(t *testing.T) {
	counter := NewCounter(5)
	for i := 0; i < 5; i++ {
		if !counter.Allow("speaker") {
			t.Fatalf("submission %d: expected Allow to succeed under the cap", i+1)
		}
	}
	if counter.Allow("speaker") {
		t.Fatal("expected the 6th submission to be rejected")
	}
	if got := counter.Count("speaker"); got != 5 {
		t.Fatalf("expected the recorded count to stay at 5, got %d", got)
	}
}

func TestCounterKeysAreIndependent(t *testing.T) {
	counter := NewCounter(1)
	if !counter.Allow("a") {
		t.Fatal("expected the first key's first event to be allowed")
	}
	if !counter.Allow("b") {
		t.Fatal("expected a different key to have its own budget")
	}
	if counter.Allow("a") {
		t.Fatal("expected key a's second event to be rejected")
	}
}

func TestCounterEmptyKeyAlwaysAllowed(t *testing.T) {
	counter := NewCounter(1)
	counter.Allow("")
	if !counter.Allow("") {
		t.Fatal("expected an empty key to never be capped")
	}
}

func TestTokenBucketRefillsOverTime(t *testing.T) {
	bucket := NewTokenBucket(2, time.Hour)
	fixed := time.Now()
	bucket.now = func() time.Time { return fixed }
	if !bucket.Allow("ip") {
		t.Fatal("expected the first request to be allowed")
	}
	if !bucket.Allow("ip") {
		t.Fatal("expected the second request to be allowed")
	}
	if bucket.Allow("ip") {
		t.Fatal("expected the third request to be rejected before any refill")
	}
	fixed = fixed.Add(31 * time.Minute)
	bucket.now = func() time.Time { return fixed }
	if !bucket.Allow("ip") {
		t.Fatal("expected a partial refill after half the window to allow one more request")
	}
}

func TestRequestIdentityFallsBackToIP(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/submit", nil)
	request.RemoteAddr = "203.0.113.5:54321"
	if got := RequestIdentity(request); got != "ip:203.0.113.5" {
		t.Fatalf("expected the IP fallback identity, got %q", got)
	}
}

func TestClientIPHandlesMissingPort(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/submit", nil)
	request.RemoteAddr = "203.0.113.5"
	if got := ClientIP(request); got != "203.0.113.5" {
		t.Fatalf("expected the raw remote address when it has no port, got %q", got)
	}
}

func TestClientIPIgnoresForwardingFromUntrustedPeer(t *testing.T) {
	t.Setenv("TRUSTED_PROXY_CIDRS", "127.0.0.1/32")
	request := httptest.NewRequest(http.MethodPost, "/submit", nil)
	request.RemoteAddr = "203.0.113.5:54321"
	request.Header.Set("X-Forwarded-For", "198.51.100.8")
	if got := ClientIP(request); got != "203.0.113.5" {
		t.Fatalf("untrusted forwarding identity = %q, want direct peer", got)
	}
}

func TestClientIPWalksTrustedForwardingChain(t *testing.T) {
	t.Setenv("TRUSTED_PROXY_CIDRS", "127.0.0.0/8, 10.42.0.0/16")
	request := httptest.NewRequest(http.MethodPost, "/submit", nil)
	request.RemoteAddr = "127.0.0.1:54321"
	request.Header.Set("X-Forwarded-For", "192.0.2.99, 203.0.113.8, 10.42.1.7")
	if got := ClientIP(request); got != "203.0.113.8" {
		t.Fatalf("trusted forwarding identity = %q, want closest untrusted hop", got)
	}
}

func TestClientIPIgnoresMalformedAttackerControlledPrefix(t *testing.T) {
	t.Setenv("TRUSTED_PROXY_CIDRS", "127.0.0.1/32")
	request := httptest.NewRequest(http.MethodPost, "/submit", nil)
	request.RemoteAddr = "127.0.0.1:54321"
	request.Header.Set("X-Forwarded-For", "definitely-not-an-ip, 198.51.100.8")
	if got := ClientIP(request); got != "198.51.100.8" {
		t.Fatalf("forwarding identity = %q, want appended observed client", got)
	}
}

func TestClientIPBoundsForwardedHopWalk(t *testing.T) {
	t.Setenv("TRUSTED_PROXY_CIDRS", "127.0.0.0/8")
	request := httptest.NewRequest(http.MethodPost, "/submit", nil)
	request.RemoteAddr = "127.0.0.1:54321"
	request.Header.Set("X-Forwarded-For", strings.Repeat("127.0.0.2,", 64)+"127.0.0.3")
	if got := ClientIP(request); got != "127.0.0.2" {
		t.Fatalf("bounded trusted chain identity = %q, want last inspected hop", got)
	}
}

func TestValidateTrustedProxyCIDRs(t *testing.T) {
	for _, value := range []string{"", "127.0.0.1/32", "10.42.0.0/16, fd00::/8"} {
		if err := ValidateTrustedProxyCIDRs(value); err != nil {
			t.Fatalf("valid trusted proxy value %q rejected: %v", value, err)
		}
	}
	if err := ValidateTrustedProxyCIDRs("127.0.0.1"); err == nil {
		t.Fatal("address without an explicit prefix was accepted")
	}
}
