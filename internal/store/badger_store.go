// Package store provides the storage engine abstractions for GossipDB.
// Implements persistence using BadgerDB with real-time SSE watch support.
package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/shivang/gossipdb/internal/metrics"
	"github.com/shivang/gossipdb/pkg/vclock"
)

type BadgerStore struct {
	db     *badger.DB
	wm     *WatchManager
	nodeID string
}

func NewBadgerStore(path string, maxWatchers int) (*BadgerStore, error) {
	return NewBadgerStoreWithID(path, "local", maxWatchers)
}

func NewBadgerStoreWithID(path, nodeID string, maxWatchers int) (*BadgerStore, error) {
	opts := badger.DefaultOptions(path)
	// Suppress badger's own logging for cleaner output in production
	opts.Logger = nil

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open badger db: %w", err)
	}

	return &BadgerStore{
		db:     db,
		wm:     NewWatchManager(maxWatchers),
		nodeID: nodeID,
	}, nil
}

func (s *BadgerStore) Get(ctx context.Context, key string) (*Value, error) {
	var val *Value

	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err != nil {
			return err
		}

		return item.Value(func(v []byte) error {
			val = &Value{}
			return json.Unmarshal(v, val)
		})
	})

	if err != nil {
		if err == badger.ErrKeyNotFound {
			return nil, nil
		}
		return nil, err
	}

	return val, nil
}

func (s *BadgerStore) Put(ctx context.Context, key string, val *Value) error {
	// Merge and increment vector clock: val.Version is the incoming clock
	// (from API caller or gossip). We merge with existing then increment local node.
	existing, _ := s.Get(ctx, key)

	if val.Version == nil {
		val.Version = make(map[string]int)
	}

	if existing != nil && existing.Version != nil {
		// Merge incoming clock with local clock
		for node, ver := range existing.Version {
			if v, ok := val.Version[node]; !ok || v < ver {
				val.Version[node] = ver
			}
		}
	}

	// Increment this node's entry to mark this write originated here
	val.Version[s.nodeID]++

	data, err := json.Marshal(val)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	err = s.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(key), data)
	})

	if err == nil {
		metrics.WritesTotal.WithLabelValues("local").Inc()
		s.wm.Notify(key, val)
	}

	return err
}

// PutReplicated stores a value coming from a peer node via gossip.
// Key differences from Put:
//   1. Does NOT increment the local node's vector clock — causal ownership stays with the origin.
//   2. Uses vector-clock reconciliation to guard the write — only applies if the incoming value
//      is strictly newer (or concurrent with LWW resolution favouring it).
//   3. Calls wm.Notify ONLY when the write was actually applied, preventing duplicate notifications.
func (s *BadgerStore) PutReplicated(ctx context.Context, key string, val *Value) error {
	existing, err := s.Get(ctx, key)
	if err != nil {
		return err
	}

	if val.Version == nil {
		val.Version = make(map[string]int)
	}

	if existing != nil {
		// Gate the write: only proceed if incoming is actually winning
		winner, _ := vclock.ReconcileVC(existing, val, vclock.LWWGeneric[*Value]{})
		if winner != val {
			// Existing value is same or newer — skip entirely, no notification
			return nil
		}
		// Merge clocks so we preserve full causal history
		merged := vclock.VClock(existing.Version).Merge(vclock.VClock(val.Version))
		val.Version = map[string]int(merged)
	}

	data, err := json.Marshal(val)
	if err != nil {
		return fmt.Errorf("failed to marshal replicated value: %w", err)
	}

	err = s.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(key), data)
	})

	// Only notify watchers if the write actually succeeded — no duplicate events
	if err == nil {
		metrics.WritesTotal.WithLabelValues("replicated").Inc()
		
		// Track replication lag
		lag := float64(time.Now().UnixNano()-val.Timestamp) / 1e9
		if lag > 0 {
			metrics.ReplicationLagSeconds.Observe(lag)
		}

		s.wm.Notify(key, val)
	}

	return err
}


func (s *BadgerStore) Delete(ctx context.Context, key string) error {
	// In a distributed KV store, deletion often involves a tombstone.
	// However, we still provide a hard delete method or handle tombstones via Put.
	// For now, we'll implement hard delete but recommend using Put with Deleted=true.
	err := s.db.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(key))
	})

	if err == nil {
		// Create a tombstone value for notification
		tombstone := &Value{Deleted: true}
		s.wm.Notify(key, tombstone)
	}

	return err
}

func (s *BadgerStore) Watch(ctx context.Context, key string) (<-chan *Value, func(), error) {
	ch, err := s.wm.Subscribe(key)
	if err != nil {
		return nil, nil, err
	}
	return ch, func() { s.wm.Unsubscribe(key, ch) }, nil
}

func (s *BadgerStore) GetAllMetadata(ctx context.Context) (map[string]Metadata, error) {
	meta := make(map[string]Metadata)
	
	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = true
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			key := string(item.Key())
			
			err := item.Value(func(v []byte) error {
				var val Value
				if err := json.Unmarshal(v, &val); err != nil {
					return err
				}
				meta[key] = Metadata{
					Timestamp: val.Timestamp,
					Deleted:   val.Deleted,
				}
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})

	return meta, err
}

func (s *BadgerStore) Close() error {
	return s.db.Close()
}
