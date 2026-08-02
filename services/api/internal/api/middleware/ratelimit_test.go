package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func serve(t *testing.T, h http.Handler, remoteAddr string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	req.RemoteAddr = remoteAddr
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
}

func TestRateLimitBlocksAfterLimit(t *testing.T) {
	h := RateLimit(3, time.Minute)(okHandler())

	for i := 1; i <= 3; i++ {
		if code := serve(t, h, "1.2.3.4:1111"); code != http.StatusOK {
			t.Fatalf("request %d: got %d, want 200", i, code)
		}
	}
	if code := serve(t, h, "1.2.3.4:1111"); code != http.StatusTooManyRequests {
		t.Fatalf("request over the limit: got %d, want 429", code)
	}
}

func TestRateLimitIsPerClient(t *testing.T) {
	h := RateLimit(1, time.Minute)(okHandler())

	if code := serve(t, h, "1.2.3.4:1111"); code != http.StatusOK {
		t.Fatalf("first client: got %d, want 200", code)
	}
	if code := serve(t, h, "1.2.3.4:1111"); code != http.StatusTooManyRequests {
		t.Fatalf("first client over limit: got %d, want 429", code)
	}
	// A different client must not inherit the first one's exhausted window.
	if code := serve(t, h, "5.6.7.8:2222"); code != http.StatusOK {
		t.Fatalf("second client: got %d, want 200", code)
	}
}

func TestRateLimitWindowResets(t *testing.T) {
	h := RateLimit(1, 50*time.Millisecond)(okHandler())

	if code := serve(t, h, "1.2.3.4:1111"); code != http.StatusOK {
		t.Fatalf("first request: got %d, want 200", code)
	}
	if code := serve(t, h, "1.2.3.4:1111"); code != http.StatusTooManyRequests {
		t.Fatalf("second request: got %d, want 429", code)
	}
	time.Sleep(60 * time.Millisecond)
	if code := serve(t, h, "1.2.3.4:1111"); code != http.StatusOK {
		t.Fatalf("after window reset: got %d, want 200", code)
	}
}

// A limit of 0 disables the middleware — used to turn limiting off via config.
func TestRateLimitDisabled(t *testing.T) {
	h := RateLimit(0, time.Minute)(okHandler())
	for i := 0; i < 50; i++ {
		if code := serve(t, h, "1.2.3.4:1111"); code != http.StatusOK {
			t.Fatalf("disabled limiter blocked request %d with %d", i, code)
		}
	}
}

// Real clients arrive on a fresh ephemeral port per TCP connection. Keying on
// the raw RemoteAddr therefore gives every request its own bucket and silently
// disables the limiter — this caught exactly that bug.
func TestRateLimitIgnoresSourcePort(t *testing.T) {
	h := RateLimit(2, time.Minute)(okHandler())

	if code := serve(t, h, "9.9.9.9:40001"); code != http.StatusOK {
		t.Fatalf("request 1: got %d, want 200", code)
	}
	if code := serve(t, h, "9.9.9.9:40002"); code != http.StatusOK {
		t.Fatalf("request 2: got %d, want 200", code)
	}
	if code := serve(t, h, "9.9.9.9:40003"); code != http.StatusTooManyRequests {
		t.Fatalf("same host on a new port must share the bucket: got %d, want 429", code)
	}
}

// Expired windows must be evicted so the map cannot grow without bound.
func TestRateLimitEvictsExpiredEntries(t *testing.T) {
	rl := &rateLimiter{hits: map[string]*window{}, limit: 1, window: 20 * time.Millisecond, lastGC: time.Now()}
	rl.allow("a")
	rl.allow("b")
	if len(rl.hits) != 2 {
		t.Fatalf("expected 2 tracked clients, got %d", len(rl.hits))
	}
	time.Sleep(30 * time.Millisecond)
	rl.allow("c") // triggers the sweep
	if len(rl.hits) != 1 {
		t.Fatalf("expected expired entries to be evicted, still tracking %d", len(rl.hits))
	}
}
