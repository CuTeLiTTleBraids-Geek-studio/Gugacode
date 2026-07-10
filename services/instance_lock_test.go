package services

import (
	"testing"
)

func TestInstanceLock_AcquireRelease(t *testing.T) {
	dir := t.TempDir()
	lock := NewInstanceLock(dir)
	if err := lock.Acquire(); err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if err := lock.Release(); err != nil {
		t.Fatalf("release: %v", err)
	}
}

func TestInstanceLock_SecondAcquireFails(t *testing.T) {
	dir := t.TempDir()
	lock1 := NewInstanceLock(dir)
	lock2 := NewInstanceLock(dir)
	if err := lock1.Acquire(); err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer lock1.Release()
	if err := lock2.Acquire(); err == nil {
		t.Error("second acquire should have failed")
	}
}

func TestInstanceLock_CanReacquireAfterRelease(t *testing.T) {
	dir := t.TempDir()
	lock := NewInstanceLock(dir)
	if err := lock.Acquire(); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if err := lock.Release(); err != nil {
		t.Fatalf("release: %v", err)
	}
	if err := lock.Acquire(); err != nil {
		t.Fatalf("reacquire after release: %v", err)
	}
	lock.Release()
}

func TestInstanceLock_ReleaseIdempotent(t *testing.T) {
	dir := t.TempDir()
	lock := NewInstanceLock(dir)
	if err := lock.Acquire(); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if err := lock.Release(); err != nil {
		t.Fatalf("first release: %v", err)
	}
	if err := lock.Release(); err != nil {
		t.Fatalf("second release should be idempotent: %v", err)
	}
}
