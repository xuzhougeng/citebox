package integration

import (
	"bytes"
	"testing"
	"time"
)

func TestAssetStorePutGet(t *testing.T) {
	store := NewAssetStore()

	data := []byte("asset-bytes")
	id, expiresAt := store.Put(data, "image/png", "fig.png", 0)
	if id == "" {
		t.Fatal("Put() returned empty id")
	}
	if time.Until(expiresAt) <= 0 || time.Until(expiresAt) > DefaultAssetTTL+time.Second {
		t.Fatalf("Put() expiresAt = %v, want ~%v from now", expiresAt, DefaultAssetTTL)
	}

	got, mediaType, filename, ok := store.Get(id)
	if !ok {
		t.Fatal("Get() miss for stored asset")
	}
	if !bytes.Equal(got, data) || mediaType != "image/png" || filename != "fig.png" {
		t.Fatalf("Get() = %q/%q/%q", got, mediaType, filename)
	}

	if _, _, _, ok := store.Get("missing-id"); ok {
		t.Fatal("Get(unknown) = true, want false")
	}
}

func TestAssetStoreExpiry(t *testing.T) {
	store := NewAssetStore()

	id, _ := store.Put([]byte("short-lived"), "text/plain", "a.txt", time.Millisecond)
	time.Sleep(5 * time.Millisecond)
	if _, _, _, ok := store.Get(id); ok {
		t.Fatal("Get() after TTL = true, want false（惰性过期）")
	}

	// 过期清理不影响其他资产
	liveID, _ := store.Put([]byte("live"), "text/plain", "b.txt", time.Hour)
	if _, _, _, ok := store.Get(liveID); !ok {
		t.Fatal("Get() live asset miss after expiring another")
	}
}
