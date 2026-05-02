package store

import (
	"fmt"
	"sync"

	"github.com/shivang/gossipdb/internal/logger"
	"go.uber.org/zap"
)

// WatchManager handles real-time subscriptions for keys.
type WatchManager struct {
	mu             sync.RWMutex
	subscribers    map[string]map[chan *Value]struct{}
	totalWatchers  int
	maxWatchers    int
}

// NewWatchManager creates a new WatchManager.
func NewWatchManager(maxWatchers int) *WatchManager {
	return &WatchManager{
		subscribers: make(map[string]map[chan *Value]struct{}),
		maxWatchers: maxWatchers,
	}
}

// Subscribe returns a channel that will receive updates for the given key.
func (wm *WatchManager) Subscribe(key string) (chan *Value, error) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if wm.totalWatchers >= wm.maxWatchers {
		return nil, fmt.Errorf("max watchers limit reached (%d)", wm.maxWatchers)
	}

	if _, exists := wm.subscribers[key]; !exists {
		wm.subscribers[key] = make(map[chan *Value]struct{})
	}

	// Use a buffered channel to handle basic backpressure
	ch := make(chan *Value, 100)
	wm.subscribers[key][ch] = struct{}{}
	wm.totalWatchers++

	return ch, nil
}

// Unsubscribe removes a channel from the subscribers list.
func (wm *WatchManager) Unsubscribe(key string, ch chan *Value) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if subs, exists := wm.subscribers[key]; exists {
		if _, ok := subs[ch]; ok {
			delete(subs, ch)
			close(ch)
			wm.totalWatchers--
			if len(subs) == 0 {
				delete(wm.subscribers, key)
			}
		}
	}
}

// Notify broadcasts a value change to all subscribers of the key.
func (wm *WatchManager) Notify(key string, val *Value) {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	subs, exists := wm.subscribers[key]
	if !exists {
		return
	}

	for ch := range subs {
		select {
		case ch <- val:
			// Sent successfully
		default:
			// Non-blocking notification: if the channel buffer is full, we drop the message.
			// This prevents slow consumers from blocking the store operations.
			logger.Get().Warn("Dropped watch event due to slow consumer", zap.String("key", key))
		}
	}
}
