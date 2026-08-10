package ratelimit

import (
	"net/http"
	"net/http/httptest"
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
