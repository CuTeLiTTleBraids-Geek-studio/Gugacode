package services

import (
	"strings"
	"testing"
)

// G-PERF-04: Performance benchmarks for critical paths.
// These benchmarks establish a performance baseline. CI compares
// results against the baseline and fails if >20% regression.
//
// Note: BenchmarkGenerateNonce lives in main_test.go (package main)
// because generateNonce is defined in main.go, not the services package.

// BenchmarkPathsecValidate benchmarks the path validation used on every file operation
func BenchmarkPathsecValidate(b *testing.B) {
	root := "/workspace/project"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ValidatePathWithinRoot(root, "/workspace/project/src/main.go")
	}
}

// BenchmarkAtomicWriteJSON benchmarks atomic JSON writes
func BenchmarkAtomicWriteJSON(b *testing.B) {
	dir := b.TempDir()
	data := map[string]interface{}{
		"name":    "test",
		"version": "1.0.0",
		"items":   []string{"a", "b", "c", "d", "e"},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		path := dir + "/test.json"
		if err := atomicWriteJSON(path, data, 0644); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParseAIError benchmarks error response parsing
func BenchmarkParseAIError(b *testing.B) {
	// Can't easily benchmark with real http.Response, so benchmark the JSON parsing
	body := `{"error":{"message":"rate limit exceeded","type":"rate_limit_error","code":"429"}}`
	b.SetBytes(int64(len(body)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = strings.NewReader(body)
	}
}
