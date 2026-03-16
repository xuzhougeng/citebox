package service

import (
	"testing"
	"time"
)

func TestSessionManagerCreateValidateDelete(t *testing.T) {
	manager := NewSessionManager(time.Hour)

	session, err := manager.Create("wanglab")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if session.ID == "" {
		t.Fatal("Create() returned empty session ID")
	}

	got, ok := manager.Validate(session.ID)
	if !ok {
		t.Fatal("Validate() = false, want true")
	}
	if got.Username != "wanglab" {
		t.Fatalf("Validate() username = %q, want %q", got.Username, "wanglab")
	}

	manager.Delete(session.ID)
	if _, ok := manager.Validate(session.ID); ok {
		t.Fatal("Validate() after Delete() = true, want false")
	}
}

func TestSessionManagerExpiresSessions(t *testing.T) {
	manager := NewSessionManager(time.Millisecond)

	session, err := manager.Create("wanglab")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	time.Sleep(5 * time.Millisecond)

	if _, ok := manager.Validate(session.ID); ok {
		t.Fatal("Validate() expired session = true, want false")
	}
}

func TestSessionManagerDeleteAll(t *testing.T) {
	manager := NewSessionManager(time.Hour)

	first, err := manager.Create("wanglab")
	if err != nil {
		t.Fatalf("Create(first) error = %v", err)
	}
	second, err := manager.Create("wanglab")
	if err != nil {
		t.Fatalf("Create(second) error = %v", err)
	}

	manager.DeleteAll()

	if _, ok := manager.Validate(first.ID); ok {
		t.Fatal("Validate(first) after DeleteAll() = true, want false")
	}
	if _, ok := manager.Validate(second.ID); ok {
		t.Fatal("Validate(second) after DeleteAll() = true, want false")
	}
}
