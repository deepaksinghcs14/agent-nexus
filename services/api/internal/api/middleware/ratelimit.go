package middleware

import (
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/deepaksingh/agent-nexus/services/api/pkg/errs"
)

// Fixed-window per-client rate limiting for the endpoints reachable without
// credentials (login/register/refresh, OAuth callbacks, webhook + gateway
// ingress). Those are the only places an anonymous caller can spend our
// resources, so they are the only places that need a limiter.
//
// The client key comes from chi's ClientIPFromXFFTrustedProxies, NOT from a raw
// X-Forwarded-For read: a spoofable key is worse than no limiter at all,
// because an attacker rotating the header gets both unlimited attempts and
// unbounded map growth.
//
// ponytail: in-memory, so the effective ceiling is limit × replicas. Matches the
// existing single-replica assumption (see the runner's "ONE replica" note); swap
// for a Postgres or Redis counter only if the API is genuinely scaled out.

type rateLimiter struct {
	mu     sync.Mutex
	hits   map[string]*window
	limit  int
	window time.Duration
	lastGC time.Time
}

type window struct {
	count int
	start time.Time
}

// RateLimit allows `limit` requests per `per` interval per client. A limit <= 0
// disables the middleware entirely.
func RateLimit(limit int, per time.Duration) func(http.Handler) http.Handler {
	rl := &rateLimiter{
		hits:   make(map[string]*window),
		limit:  limit,
		window: per,
		lastGC: time.Now(),
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if limit <= 0 {
				next.ServeHTTP(w, r)
				return
			}
			key := clientKey(r)
			if !rl.allow(key) {
				w.Header().Set("Retry-After", strconv.Itoa(int(per.Seconds())))
				errs.Write(w, errs.TooManyRequests("too many requests — slow down and try again shortly"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// clientKey identifies the caller for bucketing. RemoteAddr is only a fallback
// and must be stripped of its port: it is a fresh ephemeral port per TCP
// connection, so keying on the raw value gives every request its own bucket and
// silently disables the limiter.
func clientKey(r *http.Request) string {
	if ip := middleware.GetClientIP(r.Context()); ip != "" {
		return ip
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

func (rl *rateLimiter) allow(key string) bool {
	now := time.Now()
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Evict expired windows opportunistically so the map can't grow without
	// bound across long-lived processes.
	if now.Sub(rl.lastGC) > rl.window {
		for k, v := range rl.hits {
			if now.Sub(v.start) >= rl.window {
				delete(rl.hits, k)
			}
		}
		rl.lastGC = now
	}

	w, ok := rl.hits[key]
	if !ok || now.Sub(w.start) >= rl.window {
		rl.hits[key] = &window{count: 1, start: now}
		return true
	}
	w.count++
	return w.count <= rl.limit
}
