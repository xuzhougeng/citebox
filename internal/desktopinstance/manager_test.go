package desktopinstance

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestAcquireSignalsExistingInstance(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	manager, err := Acquire("CiteBoxSingleInstanceTest")
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	t.Cleanup(func() {
		if err := manager.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	activated := make(chan struct{}, 1)
	manager.SetActivateHandler(func() {
		select {
		case activated <- struct{}{}:
		default:
		}
	})

	secondManager, err := Acquire("CiteBoxSingleInstanceTest")
	if secondManager != nil {
		t.Fatal("Acquire() returned unexpected secondary manager")
	}
	if !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("Acquire() error = %v, want ErrAlreadyRunning", err)
	}

	select {
	case <-activated:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for primary instance activation")
	}
}

func TestSetActivateHandlerReplaysPendingActivation(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	manager, err := Acquire("CiteBoxPendingActivationTest")
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	t.Cleanup(func() {
		if err := manager.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	secondManager, err := Acquire("CiteBoxPendingActivationTest")
	if secondManager != nil {
		t.Fatal("Acquire() returned unexpected secondary manager")
	}
	if !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("Acquire() error = %v, want ErrAlreadyRunning", err)
	}

	var calls atomic.Int32
	manager.SetActivateHandler(func() {
		calls.Add(1)
	})

	if calls.Load() != 1 {
		t.Fatalf("SetActivateHandler() replayed %d activations, want 1", calls.Load())
	}
}
