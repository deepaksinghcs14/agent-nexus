package native

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"syscall"
	"time"
)

// SSRF guard for agent-driven outbound HTTP.
//
// The check lives in the dialer's Control hook rather than on the URL string
// because Control runs once per connection attempt, AFTER DNS resolution and
// again for every redirect hop. That single placement closes three holes a
// URL-level check cannot: a hostname that resolves to a private address, a
// public URL that 302s to one, and DNS rebinding between the check and the
// dial. The address Control receives is the one actually being connected to.
//
// Self-hosted installs sometimes need agents to reach internal services on
// purpose, so HTTP_TOOL_ALLOW_HOSTS lists hostnames exempt from the check.

// blockedIP reports whether connecting to ip should be refused.
func blockedIP(ip net.IP) bool {
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified() ||
		// Carrier-grade NAT (100.64.0.0/10) — not covered by IsPrivate, and it
		// is where several cloud platforms put internal endpoints.
		(ip.To4() != nil && ip.To4()[0] == 100 && ip.To4()[1] >= 64 && ip.To4()[1] <= 127)
}

// safeControl returns a dialer Control func that refuses blocked destinations.
func safeControl(network, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("blocked: unparseable address %q", address)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		// Control always receives a resolved literal; anything else is unexpected.
		return fmt.Errorf("blocked: unresolved address %q", host)
	}
	if blockedIP(ip) {
		return fmt.Errorf("blocked: %s is a private, loopback, or link-local address "+
			"(set HTTP_TOOL_ALLOW_HOSTS to allow specific internal hosts)", ip)
	}
	return nil
}

// SafeHTTPClient builds an SSRF-guarded client: the dialer's Control hook
// (safeControl) blocks private/internal addresses post-DNS, catching redirect
// hops and DNS rebinding a URL-string check would miss. Hosts listed in
// allowHosts bypass the IP check entirely and get a plain dialer. Used by
// native_http_request; any other code path that fetches a caller-supplied URL
// should reuse this rather than a bare http.Client.
func SafeHTTPClient(allowHosts []string, timeout time.Duration) *http.Client {
	allowed := make(map[string]bool, len(allowHosts))
	for _, h := range allowHosts {
		if h = strings.ToLower(strings.TrimSpace(h)); h != "" {
			allowed[h] = true
		}
	}

	guarded := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second, Control: safeControl}
	plain := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		// addr is still the pre-resolution host:port here, so the allowlist is
		// matched on the hostname the caller actually asked for.
		if host, _, err := net.SplitHostPort(addr); err == nil && allowed[strings.ToLower(host)] {
			return plain.DialContext(ctx, network, addr)
		}
		return guarded.DialContext(ctx, network, addr)
	}
	return &http.Client{Timeout: timeout, Transport: transport}
}
