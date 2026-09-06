package middleware

import "net/http"

// SecurityHeaders sets baseline defensive headers on every response. This is
// a JSON API (no HTML templates render server-side), so there's no CSP to
// author here — the frontend's next.config.js owns that. These are the
// headers that matter regardless: don't let a browser sniff a JSON response
// into executing as something else, don't let this API be framed, and don't
// leak the full request URL to a cross-origin Referer target.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}
