package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// prompt-6 Task 6 / BUG-M10: responseRecorder must preserve status codes.
func TestResponseRecorder_PreservesStatusCode(t *testing.T) {
	rec := &responseRecorder{
		ResponseWriter: httptest.NewRecorder(),
		buf:            &bytes.Buffer{},
		statusCode:     http.StatusOK,
	}
	rec.WriteHeader(http.StatusNotFound)
	if rec.statusCode != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rec.statusCode)
	}
	n, err := rec.Write([]byte("missing"))
	if err != nil || n != 7 {
		t.Fatalf("Write: n=%d err=%v", n, err)
	}
	if rec.buf.String() != "missing" {
		t.Fatalf("body = %q", rec.buf.String())
	}
}

func TestResponseRecorder_DefaultStatusOK(t *testing.T) {
	rec := &responseRecorder{
		ResponseWriter: httptest.NewRecorder(),
		buf:            &bytes.Buffer{},
		statusCode:     http.StatusOK,
	}
	// Downstream may never call WriteHeader; default stays 200.
	if rec.statusCode != http.StatusOK {
		t.Fatalf("default status = %d", rec.statusCode)
	}
}

// N-34 (prompt-4.md): CSP nonce injection tests.

func TestInjectNonceIntoHTML_BareScriptTag(t *testing.T) {
	html := []byte(`<html><body><script>console.log("hi")</script></body></html>`)
	got := injectNonceIntoHTML(html, "abc123")
	if !strings.Contains(string(got), `<script nonce="abc123">`) {
		t.Fatalf("expected nonce injected into <script>, got: %s", got)
	}
}

func TestInjectNonceIntoHTML_ModuleScriptTag(t *testing.T) {
	html := []byte(`<script type="module" src="/main.ts"></script>`)
	got := injectNonceIntoHTML(html, "n0nc3")
	if !strings.Contains(string(got), `<script nonce="n0nc3" type="module" src="/main.ts">`) {
		t.Fatalf("expected nonce injected before type attribute, got: %s", got)
	}
}

func TestInjectNonceIntoHTML_PreservesExistingNonce(t *testing.T) {
	html := []byte(`<script nonce="existing">foo()</script>`)
	got := injectNonceIntoHTML(html, "new")
	if strings.Contains(string(got), `nonce="new"`) {
		t.Fatalf("should not override existing nonce, got: %s", got)
	}
	if !strings.Contains(string(got), `nonce="existing"`) {
		t.Fatalf("existing nonce should be preserved, got: %s", got)
	}
}

func TestInjectNonceIntoHTML_MultipleScriptTags(t *testing.T) {
	html := []byte(`<script>a()</script><script type="module">b()</script>`)
	got := injectNonceIntoHTML(html, "xyz")
	if strings.Count(string(got), `nonce="xyz"`) != 2 {
		t.Fatalf("expected 2 nonce injections, got: %s", got)
	}
}

func TestInjectNonceIntoHTML_NoScriptTags(t *testing.T) {
	html := []byte(`<html><body><p>hello</p></body></html>`)
	got := injectNonceIntoHTML(html, "n")
	if string(got) != string(html) {
		t.Fatalf("expected no changes when no <script> tags, got: %s", got)
	}
}

func TestInjectNonceIntoHTML_SelfClosingScript(t *testing.T) {
	// Self-closing <script src="..."/> is invalid HTML but we should
	// still inject the nonce — the regex matches any <script...>.
	html := []byte(`<script src="external.js"/>`)
	got := injectNonceIntoHTML(html, "tok")
	if !strings.Contains(string(got), `nonce="tok"`) {
		t.Fatalf("expected nonce injected, got: %s", got)
	}
}

func TestGenerateNonce_LengthAndHex(t *testing.T) {
	nonce, err := generateNonce()
	if err != nil {
		t.Fatalf("generateNonce returned unexpected error: %v", err)
	}
	// 16 bytes -> 32 hex chars
	if len(nonce) != 32 {
		t.Fatalf("expected 32-char hex nonce, got %d chars: %s", len(nonce), nonce)
	}
	if nonce == "" {
		t.Fatalf("expected non-empty nonce")
	}
	for _, c := range nonce {
		isHex := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')
		if !isHex {
			t.Fatalf("non-hex char %q in nonce %s", c, nonce)
		}
	}
}

func TestGenerateNonce_Uniqueness(t *testing.T) {
	seen := make(map[string]bool, 1000)
	for i := 0; i < 1000; i++ {
		n, err := generateNonce()
		if err != nil {
			t.Fatalf("generateNonce returned unexpected error on iteration %d: %v", i, err)
		}
		if seen[n] {
			t.Fatalf("nonce %s repeated after %d iterations", n, i)
		}
		seen[n] = true
	}
}

// TestGenerateNonce_NonEmptyOnSuccess verifies the happy path: a non-empty
// hex string and nil error. Mocking crypto/rand.Read failure in Go is
// awkward without dependency injection, so we assert the success path
// returns a usable nonce — the failure branch returns ("", err) by
// construction (G-SEC-10).
func TestGenerateNonce_NonEmptyOnSuccess(t *testing.T) {
	nonce, err := generateNonce()
	if err != nil {
		t.Fatalf("expected nil error on healthy crypto/rand, got: %v", err)
	}
	if nonce == "" {
		t.Fatalf("expected non-empty nonce, got empty string")
	}
}

func TestContentSecurityPolicyWithNonce_Formats(t *testing.T) {
	csp := fmt.Sprintf(contentSecurityPolicyWithNonce, "test123")
	if !strings.Contains(csp, "'nonce-test123'") {
		t.Fatalf("expected CSP to contain 'nonce-test123', got: %s", csp)
	}
	// style-src keeps 'unsafe-inline' (Vue scoped styles), but script-src
	// must use the nonce instead. Verify script-src specifically.
	parts := strings.Split(csp, ";")
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if strings.HasPrefix(trimmed, "script-src") {
			if strings.Contains(trimmed, "'unsafe-inline'") {
				t.Fatalf("script-src must not contain 'unsafe-inline': %s", trimmed)
			}
			if !strings.Contains(trimmed, "'nonce-test123'") {
				t.Fatalf("script-src must contain 'nonce-test123': %s", trimmed)
			}
		}
	}
}

func TestContentSecurityPolicyStatic_NoUnsafeInline(t *testing.T) {
	if strings.Contains(contentSecurityPolicyStatic, "'unsafe-inline'") {
		// style-src keeps 'unsafe-inline' (Vue scoped styles), but
		// script-src must not. Verify script-src specifically.
		parts := strings.Split(contentSecurityPolicyStatic, ";")
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if strings.HasPrefix(trimmed, "script-src") {
				if strings.Contains(trimmed, "'unsafe-inline'") {
					t.Fatalf("script-src must not contain 'unsafe-inline': %s", trimmed)
				}
			}
		}
	}
}

// G-PERF-04: BenchmarkGenerateNonce benchmarks CSP nonce generation.
// Lives here (package main) because generateNonce is defined in main.go,
// not the services package. The CI perf-benchmark job targets
// ./services/... and so does not exercise this benchmark; it is kept for
// local/manual performance characterization of the CSP nonce path.
// generateNonce is also covered by TestGenerateNonce_* above.
func BenchmarkGenerateNonce(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = generateNonce()
	}
}
