package ring

import (
	"fmt"
	"hash/crc32"
	"sort"
	"sync"
)

// ConsistentHash represents a consistent hashing ring.
type ConsistentHash struct {
	mu       sync.RWMutex
	replicas int
	keys     []uint32
	hashMap  map[uint32]string
}

// New creates a new ConsistentHash ring.
// replicas controls how many virtual nodes are created per physical node.
func New(replicas int) *ConsistentHash {
	return &ConsistentHash{
		replicas: replicas,
		hashMap:  make(map[uint32]string),
	}
}

// Add inserts physical nodes into the ring.
func (c *ConsistentHash) Add(nodes ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, node := range nodes {
		for i := 0; i < c.replicas; i++ {
			// Create a virtual node key
			vNodeKey := []byte(fmt.Sprintf("%d", i) + node)
			hash := crc32.ChecksumIEEE(vNodeKey)

			c.keys = append(c.keys, hash)
			c.hashMap[hash] = node
		}
	}

	sort.Slice(c.keys, func(i, j int) bool {
		return c.keys[i] < c.keys[j]
	})
}

// Get returns the closest node in the ring for the given key.
func (c *ConsistentHash) Get(key string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.keys) == 0 {
		return ""
	}

	hash := crc32.ChecksumIEEE([]byte(key))

	// Binary search for appropriate virtual node
	idx := sort.Search(len(c.keys), func(i int) bool {
		return c.keys[i] >= hash
	})

	// Wrap around if we hit the end of the ring
	if idx == len(c.keys) {
		idx = 0
	}

	return c.hashMap[c.keys[idx]]
}

// Remove deletes a physical node from the ring.
func (c *ConsistentHash) Remove(node string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i := 0; i < c.replicas; i++ {
		vNodeKey := []byte(fmt.Sprintf("%d", i) + node)
		hash := crc32.ChecksumIEEE(vNodeKey)
		
		delete(c.hashMap, hash)
		
		// Remove from keys slice
		for j, k := range c.keys {
			if k == hash {
				c.keys = append(c.keys[:j], c.keys[j+1:]...)
				break
			}
		}
	}
}
