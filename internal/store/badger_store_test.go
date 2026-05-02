package store

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestBadgerStore(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "badger-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	s, err := NewBadgerStore(tmpDir, 100)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	ctx := context.Background()
	key := "foo"
	val := &Value{
		Data:      []byte("bar"),
		Timestamp: time.Now().UnixNano(),
		Version:   map[string]int{"node-1": 1},
		Deleted:   bool(false),
	}

	// Test Put
	err = s.Put(ctx, key, val)
	if err != nil {
		t.Errorf("failed to put: %v", err)
	}

	// Test Get
	got, err := s.Get(ctx, key)
	if err != nil {
		t.Errorf("failed to get: %v", err)
	}
	if got == nil {
		t.Fatal("expected value, got nil")
	}
	if string(got.Data) != "bar" {
		t.Errorf("expected bar, got %s", string(got.Data))
	}
	if got.Version["node-1"] != 1 {
		t.Errorf("expected version 1, got %d", got.Version["node-1"])
	}

	// Test Delete
	err = s.Delete(ctx, key)
	if err != nil {
		t.Errorf("failed to delete: %v", err)
	}

	got, err = s.Get(ctx, key)
	if err != nil {
		t.Errorf("failed to get after delete: %v", err)
	}
	if got != nil {
		t.Error("expected nil after delete")
	}
}

func TestTombstone(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "badger-tombstone-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	s, err := NewBadgerStore(tmpDir, 100)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	ctx := context.Background()
	key := "soft-delete"
	val := &Value{
		Data:      []byte("important data"),
		Timestamp: time.Now().UnixNano(),
		Deleted:   true,
	}

	err = s.Put(ctx, key, val)
	if err != nil {
		t.Fatal(err)
	}

	got, err := s.Get(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || !got.Deleted {
		t.Error("expected value with deleted=true tombstone")
	}
}
