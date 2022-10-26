package lru

import (
	"fmt"
	"sync"

	"github.com/hashicorp/golang-lru/simplelru"
)

const (
	// Default2QRecentRatioG is the ratio of the 2Q cache dedicated
	// to recently added entries that have only been accessed once.
	Default2QRecentRatioG = 0.25

	// Default2QGhostEntriesG is the default ratio of ghost
	// entries kept to track entries recently evicted
	Default2QGhostEntriesG = 0.50
)

// TwoQueueCacheG is a thread-safe fixed size 2Q cache.
// 2Q is an enhancement over the standard LRU cache
// in that it tracks both frequently and recently used
// entries separately. This avoids a burst in access to new
// entries from evicting frequently used entries. It adds some
// additional tracking overhead to the standard LRU cache, and is
// computationally about 2x the cost, and adds some metadata over
// head. The ARCCache is similar, but does not require setting any
// parameters.
type TwoQueueCacheG[K comparable, V any] struct {
	size       int
	recentSize int

	recent      simplelru.LRUCacheG[K, V]
	frequent    simplelru.LRUCacheG[K, V]
	recentEvict simplelru.LRUCacheG[K, V]
	lock        sync.RWMutex
}

// New2QG creates a new TwoQueueCacheG using the default
// values for the parameters.
func New2QG[K comparable, V any](size int) (*TwoQueueCacheG[K, V], error) {
	return New2QParamsG[K, V](size, Default2QRecentRatioG, Default2QGhostEntriesG)
}

// New2QParamsG creates a new TwoQueueCacheG using the provided
// parameter values.
func New2QParamsG[K comparable, V any](size int, recentRatio, ghostRatio float64) (*TwoQueueCacheG[K, V], error) {
	if size <= 0 {
		return nil, fmt.Errorf("invalid size")
	}
	if recentRatio < 0.0 || recentRatio > 1.0 {
		return nil, fmt.Errorf("invalid recent ratio")
	}
	if ghostRatio < 0.0 || ghostRatio > 1.0 {
		return nil, fmt.Errorf("invalid ghost ratio")
	}

	// Determine the sub-sizes
	recentSize := int(float64(size) * recentRatio)
	evictSize := int(float64(size) * ghostRatio)

	// Allocate the LRUs
	recent, err := simplelru.NewLRUG[K, V](size, nil)
	if err != nil {
		return nil, err
	}
	frequent, err := simplelru.NewLRUG[K, V](size, nil)
	if err != nil {
		return nil, err
	}
	recentEvict, err := simplelru.NewLRUG[K, V](evictSize, nil)
	if err != nil {
		return nil, err
	}

	// Initialize the cache
	c := &TwoQueueCacheG[K, V]{
		size:        size,
		recentSize:  recentSize,
		recent:      recent,
		frequent:    frequent,
		recentEvict: recentEvict,
	}
	return c, nil
}

// Get looks up a key's value from the cache.
func (c *TwoQueueCacheG[K, V]) Get(key K) (value V, ok bool) {
	c.lock.Lock()
	defer c.lock.Unlock()

	// Check if this is a frequent value
	if val, ok := c.frequent.Get(key); ok {
		return val, ok
	}

	// If the value is contained in recent, then we
	// promote it to frequent
	if val, ok := c.recent.Peek(key); ok {
		c.recent.Remove(key)
		c.frequent.Add(key, val)
		return val, ok
	}

	// No hit
	return
}

// Add adds a value to the cache.
func (c *TwoQueueCacheG[K, V]) Add(key K, value V) {
	c.lock.Lock()
	defer c.lock.Unlock()

	// Check if the value is frequently used already,
	// and just update the value
	if c.frequent.Contains(key) {
		c.frequent.Add(key, value)
		return
	}

	// Check if the value is recently used, and promote
	// the value into the frequent list
	if c.recent.Contains(key) {
		c.recent.Remove(key)
		c.frequent.Add(key, value)
		return
	}

	// If the value was recently evicted, add it to the
	// frequently used list
	if c.recentEvict.Contains(key) {
		c.ensureSpace(true)
		c.recentEvict.Remove(key)
		c.frequent.Add(key, value)
		return
	}

	// Add to the recently seen list
	c.ensureSpace(false)
	c.recent.Add(key, value)
}

// ensureSpace is used to ensure we have space in the cache
func (c *TwoQueueCacheG[K, V]) ensureSpace(recentEvict bool) {
	// If we have space, nothing to do
	recentLen := c.recent.Len()
	freqLen := c.frequent.Len()
	if recentLen+freqLen < c.size {
		return
	}

	// If the recent buffer is larger than
	// the target, evict from there
	if recentLen > 0 && (recentLen > c.recentSize || (recentLen == c.recentSize && !recentEvict)) {
		k, _, _ := c.recent.RemoveOldest()
		var empty V
		c.recentEvict.Add(k, empty)
		return
	}

	// Remove from the frequent list otherwise
	c.frequent.RemoveOldest()
}

// Len returns the number of items in the cache.
func (c *TwoQueueCacheG[K, V]) Len() int {
	c.lock.RLock()
	defer c.lock.RUnlock()
	return c.recent.Len() + c.frequent.Len()
}

// Keys returns a slice of the keys in the cache.
// The frequently used keys are first in the returned slice.
func (c *TwoQueueCacheG[K, V]) Keys() []K {
	c.lock.RLock()
	defer c.lock.RUnlock()
	k1 := c.frequent.Keys()
	k2 := c.recent.Keys()
	return append(k1, k2...)
}

// Remove removes the provided key from the cache.
func (c *TwoQueueCacheG[K, V]) Remove(key K) {
	c.lock.Lock()
	defer c.lock.Unlock()
	if c.frequent.Remove(key) {
		return
	}
	if c.recent.Remove(key) {
		return
	}
	if c.recentEvict.Remove(key) {
		return
	}
}

// Purge is used to completely clear the cache.
func (c *TwoQueueCacheG[K, V]) Purge() {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.recent.Purge()
	c.frequent.Purge()
	c.recentEvict.Purge()
}

// Contains is used to check if the cache contains a key
// without updating recency or frequency.
func (c *TwoQueueCacheG[K, V]) Contains(key K) bool {
	c.lock.RLock()
	defer c.lock.RUnlock()
	return c.frequent.Contains(key) || c.recent.Contains(key)
}

// Peek is used to inspect the cache value of a key
// without updating recency or frequency.
func (c *TwoQueueCacheG[K, V]) Peek(key K) (value V, ok bool) {
	c.lock.RLock()
	defer c.lock.RUnlock()
	if val, ok := c.frequent.Peek(key); ok {
		return val, ok
	}
	return c.recent.Peek(key)
}
