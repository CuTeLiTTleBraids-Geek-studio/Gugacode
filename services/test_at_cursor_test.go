package services

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindTestNameAtLine_Go(t *testing.T) {
	src := `package p

func helper() {}

func TestAlpha(t *testing.T) {
	_ = 1
}

func TestBeta(t *testing.T) {
	t.Run("case1", func(t *testing.T) {
		_ = 2
	})
}
`
	// line inside TestAlpha body (0-based)
	name := findTestNameAtLine("go", src, 5)
	if name != "TestAlpha" {
		t.Fatalf("got %q want TestAlpha", name)
	}
	// inside t.Run body
	name = findTestNameAtLine("go", src, 10)
	if name != "TestBeta/case1" {
		t.Fatalf("got %q want TestBeta/case1", name)
	}
}

func TestFindTestNameAtLine_JS(t *testing.T) {
	src := `import { it, expect } from 'vitest'

it('hello world', () => {
  expect(1).toBe(1)
})
`
	name := findTestNameAtLine("typescript", src, 3)
	if name != "hello world" {
		t.Fatalf("got %q", name)
	}
}

func TestFindTestNameAtLine_JSEach(t *testing.T) {
	src := `import { test, expect } from 'vitest'

test.each([1, 2])('adds %i', (n) => {
  expect(n).toBeTruthy()
})
`
	name := findTestNameAtLine("typescript", src, 3)
	if name != "adds %i" {
		t.Fatalf("got %q want adds %%i", name)
	}
}

func TestCoverageService_ParseCoverProfile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "cover.out")
	body := "mode: set\nfoo/bar.go:10.2,12.3 2 1\nfoo/bar.go:20.1,21.2 1 0\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := NewCoverageService()
	hits, err := svc.ParseCoverProfile(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) < 5 {
		t.Fatalf("expected >=5 line hits, got %d", len(hits))
	}
	if !hits[0].Covered || hits[0].Line != 10 {
		t.Errorf("first hit: line=%d covered=%v want line 10 covered", hits[0].Line, hits[0].Covered)
	}
	last := hits[len(hits)-1]
	if last.Covered || last.Line != 21 {
		t.Errorf("last hit: line=%d covered=%v want line 21 uncovered", last.Line, last.Covered)
	}
	var foundUncovered bool
	for _, h := range hits {
		if h.Line == 20 && !h.Covered {
			foundUncovered = true
			break
		}
	}
	if !foundUncovered {
		t.Error("expected line 20 to be uncovered")
	}
}

func TestDebugService_StatusMessage(t *testing.T) {
	d := NewDebugService()
	msg := d.StatusMessage()
	if msg == "" {
		t.Fatal("expected non-empty status")
	}
}
