package webpush

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	wp "github.com/SherClockHolmes/webpush-go"
)

func TestStorePersistsDeduplicatesAndReloads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", subscriptionsFileName)
	store, err := NewStoreAt(path)
	if err != nil {
		t.Fatal(err)
	}
	first := &wp.Subscription{Endpoint: "https://push.example/a"}
	if err := store.Add(first); err != nil {
		t.Fatal(err)
	}
	updated := &wp.Subscription{Endpoint: first.Endpoint}
	updated.Keys.Auth = "new-auth"
	if err := store.Add(updated); err != nil {
		t.Fatal(err)
	}
	if store.Count() != 1 {
		t.Fatalf("count = %d", store.Count())
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
	reloaded, err := NewStoreAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.All(); len(got) != 1 || got[0].Keys.Auth != "new-auth" {
		t.Fatalf("reloaded = %+v", got)
	}
	if err := reloaded.Remove(first.Endpoint); err != nil {
		t.Fatal(err)
	}
	empty, err := NewStoreAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if empty.Count() != 0 {
		t.Fatalf("count after unsubscribe = %d", empty.Count())
	}
}

func TestStoreWriteFailurePreservesPreviousFileAndMemory(t *testing.T) {
	path := filepath.Join(t.TempDir(), subscriptionsFileName)
	store, err := NewStoreAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Add(&wp.Subscription{Endpoint: "https://push.example/old"}); err != nil {
		t.Fatal(err)
	}
	store.write = func(string, map[string]*wp.Subscription) error {
		return errors.New("injected write failure")
	}
	if err := store.Add(&wp.Subscription{Endpoint: "https://push.example/new"}); err == nil {
		t.Fatal("expected write failure")
	}
	if store.Count() != 1 || store.All()[0].Endpoint != "https://push.example/old" {
		t.Fatalf("memory changed after failed write: %+v", store.All())
	}
	reloaded, err := NewStoreAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Count() != 1 || reloaded.All()[0].Endpoint != "https://push.example/old" {
		t.Fatalf("file changed after failed write: %+v", reloaded.All())
	}
}

func TestStoreRejectsMalformedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), subscriptionsFileName)
	if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStoreAt(path); err == nil {
		t.Fatal("expected malformed store to fail closed")
	}
}
