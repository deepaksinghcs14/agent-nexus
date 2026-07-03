package github

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

func TestRateLimitWait(t *testing.T) {
	reset := strconv.FormatInt(time.Now().Add(10*time.Minute).Unix(), 10)

	if w := rateLimitWait(resp(200, "4000", reset), 500); w != 0 {
		t.Errorf("plenty of quota should not wait, got %v", w)
	}
	if w := rateLimitWait(resp(200, "499", reset), 500); w < 9*time.Minute {
		t.Errorf("at/below floor must wait until reset, got %v", w)
	}
	if w := rateLimitWait(resp(403, "0", reset), 500); w < 9*time.Minute {
		t.Errorf("exhausted 403 must wait until reset, got %v", w)
	}
	if w := rateLimitWait(resp(403, "", ""), 500); w != time.Minute {
		t.Errorf("rejected without headers should back off a minute, got %v", w)
	}
	if w := rateLimitWait(resp(200, "", ""), 500); w != 0 {
		t.Errorf("no headers on success should proceed, got %v", w)
	}
}
