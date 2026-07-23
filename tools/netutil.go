package tools

import (
	"fmt"
	"net"
	"net/http"
	"syscall"
	"time"
)

// isDisallowedIP reports whether ip must never be dialed by outbound HTTP
// tools (web_fetch, vision): loopback, link-local (this also covers cloud
// metadata endpoints such as 169.254.169.254), private, unspecified, and
// multicast addresses.
//
// Extracted from web_fetch.go so that vision.go and any future outbound
// HTTP tool can share the same SSRF defense.
func isDisallowedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	return ip.IsLoopback() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() ||
		ip.IsMulticast() ||
		ip.IsPrivate()
}

// safeDialControl is invoked by net.Dialer right before connecting to the
// fully-resolved address (i.e. after DNS resolution), so it also protects
// against DNS-rebinding and blocks SSRF via HTTP redirects since the same
// Transport/Dialer is reused for redirected requests.
func safeDialControl(network, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("BLOCKED: refusing to dial %q: %w", address, err)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("BLOCKED: refusing to dial non-IP address %q", address)
	}
	if isDisallowedIP(ip) {
		return fmt.Errorf("BLOCKED: refusing to dial internal/private address %s", ip)
	}
	return nil
}

// newSSRFSafeTransport returns an http.Transport whose Dialer enforces the
// SSRF dial control. Use this for any HTTP client that fetches user/LLM
// supplied URLs to prevent access to internal/private network ranges.
func newSSRFSafeTransport() *http.Transport {
	return &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: 10 * time.Second,
			Control: safeDialControl,
		}).DialContext,
	}
}

// newSSRFSafeHTTPClient returns an http.Client with SSRF protection enabled.
// All HTTP tools that download from user/LLM-supplied URLs should use this.
func newSSRFSafeHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: newSSRFSafeTransport(),
	}
}
