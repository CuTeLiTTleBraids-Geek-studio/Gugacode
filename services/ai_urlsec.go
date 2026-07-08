package services

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// ValidateBaseURL validates a user-supplied API base URL to prevent API key
// leakage to malicious endpoints (N-73).
//
// Rules:
//  1. Must parse as a valid URL with a non-empty host.
//  2. Scheme must be http or https only (rejects file:, data:, ftp:, gopher:,
//     javascript:, etc. which could exfiltrate the API key or read local files).
//  3. Must not contain embedded userinfo (e.g. "http://user:pass@host") — this
//     is a credential-leakage vector and is never needed for AI providers.
//  4. For non-loopback hosts, scheme MUST be https. Loopback hosts
//     (localhost, 127.0.0.1, ::1, *.localhost) are allowed over plain http to
//     support local LLM servers (Ollama, LM Studio, llama.cpp).
//
// The check is intentionally NOT an allowlist of specific provider hosts —
// users need to add custom OpenAI-compatible endpoints. The scheme + loopback
// enforcement blocks the main exfiltration vectors while preserving flexibility.
func ValidateBaseURL(baseURL string) error {
	if baseURL == "" {
		return fmt.Errorf("base URL is required")
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("invalid base URL: %w", err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("base URL scheme must be http or https, got %q", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("base URL must have a host")
	}
	if u.User != nil {
		return fmt.Errorf("base URL must not contain embedded credentials")
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("base URL must have a host")
	}
	if !isLoopbackHost(host) && scheme != "https" {
		return fmt.Errorf("base URL for non-loopback host %q must use https", host)
	}
	return nil
}

// isLoopbackHost reports whether host is a loopback address. It accepts:
//   - "localhost"
//   - "*.localhost" (e.g. "ollama.localhost")
//   - IPv4 loopback "127.0.0.1" (and any 127.x.x.x)
//   - IPv6 loopback "::1"
//
// For IP literals, net.ParseIP is used and the result is checked with
// net.IP.IsLoopback(), which correctly handles the full 127.0.0.0/8 range
// and the IPv6 ::1.
func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	if strings.HasSuffix(host, ".localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}
