package native

import (
	"net/http"
	"strconv"
	"testing"
	"time"
)

func resp(status int, remaining, reset string) *http.Response {
	h := http.Header{}
	if remaining != "" {
		h.Set("X-RateLimit-Remaining", remaining)
	}
	if reset != "" {
		h.Set("X-RateLimit-Reset", reset)
	}
	return &http.Response{StatusCode: status, Header: h}
}

func TestRateLimited(t *testing.T) {
	if !rateLimited(resp(403, "0", ""), nil) {
		t.Error("403 with remaining=0 must be rate-limited")
	}
	if !rateLimited(resp(429, "", ""), []byte(`{"message":"API rate limit exceeded for user ID 1"}`)) {
		t.Error("429 with rate-limit message must be rate-limited")
	}
	if rateLimited(resp(403, "4999", ""), []byte(`{"message":"Resource not accessible"}`)) {
		t.Error("permission 403 must NOT be treated as rate limit")
	}
	if rateLimited(resp(404, "0", ""), nil) {
		t.Error("non-403/429 statuses are never rate limits")
	}
}

func TestRateLimitReset(t *testing.T) {
	epoch := time.Now().Add(20 * time.Minute).Unix()
	got := rateLimitReset(resp(403, "0", strconv.FormatInt(epoch, 10)))
	if got.Unix() != epoch {
		t.Errorf("reset = %v, want epoch %d", got, epoch)
	}
	// Missing header falls back to roughly an hour out.
	fallback := rateLimitReset(resp(403, "0", ""))
	if d := time.Until(fallback); d < 55*time.Minute || d > 65*time.Minute {
		t.Errorf("fallback reset should be ~1h out, got %v", d)
	}
}
