package store

import (
	"context"
)

// Metadata represents the lightweight state of a key
type Metadata struct {
	Timestamp int64
	Deleted   bool
}

// Value represents the stored entry in the KV store
type Value struct {
	Data      []byte         `json:"data"`
	Timestamp int64          `json:"timestamp"`
	Version   map[string]int `json:"version"` // Vector clock or similar placeholder
	Deleted   bool           `json:"deleted"` // Tombstone for distributed deletion
}

// GetTimestamp implements vclock.ResolvableValue
func (v *Value) GetTimestamp() int64 { return v.Timestamp }

// GetVersion implements vclock.ResolvableValue
func (v *Value) GetVersion() map[string]int { return v.Version }

// Store defines the interface for the storage layer
type Store interface {
	Get(ctx context.Context, key string) (*Value, error)
	Put(ctx context.Context, key string, val *Value) error
	// PutReplicated applies a value received from a peer node.
	// Unlike Put, it does NOT increment the local vector clock — causal history
	// belongs to the originating node. It only notifies watchers if the value
	// is actually newer than what is currently stored.
	PutReplicated(ctx context.Context, key string, val *Value) error
	Delete(ctx context.Context, key string) error
	Watch(ctx context.Context, key string) (<-chan *Value, func(), error)
	GetAllMetadata(ctx context.Context) (map[string]Metadata, error)
	Close() error
}
