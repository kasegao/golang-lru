package lru

import (
	"sync"

	"github.com/hashicorp/golang-lru/simplelru"
)

const (
	// DefaultEvictedBufferSizeG defines the default buffer size to store evicted key/val
	DefaultEvictedBufferSizeG = 16
)

// CacheG is a thread-safe fixed size LRU cache.
type CacheG[K comparable, V any] struct {
	lru         *simplelru.LRUG[K, V]
	evictedKeys []K
	evictedVals []V
	onEvictedCB func(k K, v V)
	lock        sync.RWMutex
}

// NewG creates an LRU of the given size.
func NewG[K comparable, V any](size int) (*CacheG[K, V], error) {
	return NewWithEvictG[K, V](size, nil)
}

// NewWithEvictG constructs a fixed size cache with the given eviction
// callback.
func NewWithEvictG[K comparable, V any](size int, onEvicted func(key K, value V)) (c *CacheG[K, V], err error) {
	// create a cache with default settings
	c = &CacheG[K, V]{
		onEvictedCB: onEvicted,
	}
	if onEvicted != nil {
		c.initEvictBuffers()
		onEvicted = c.onEvicted
	}
	c.lru, err = simplelru.NewLRUG(size, onEvicted)
	return
}

func (c *CacheG[K, V]) initEvictBuffers() {
	c.evictedKeys = make([]K, 0, DefaultEvictedBufferSizeG)
	c.evictedVals = make([]V, 0, DefaultEvictedBufferSizeG)
}

// onEvicted save evicted key/val and sent in externally registered callback
// outside of critical section
func (c *CacheG[K, V]) onEvicted(k K, v V) {
	c.evictedKeys = append(c.evictedKeys, k)
	c.evictedVals = append(c.evictedVals, v)
}

// Purge is used to completely clear the cache.
func (c *CacheG[K, V]) Purge() {
	var ks []K
	var vs []V
	c.lock.Lock()
	c.lru.Purge()
	if c.onEvictedCB != nil && len(c.evictedKeys) > 0 {
		ks, vs = c.evictedKeys, c.evictedVals
		c.initEvictBuffers()
	}
	c.lock.Unlock()
	// invoke callback outside of critical section
	if c.onEvictedCB != nil {
		for i := 0; i < len(ks); i++ {
			c.onEvictedCB(ks[i], vs[i])
		}
	}
}

// Add adds a value to the cache. Returns true if an eviction occurred.
func (c *CacheG[K, V]) Add(key K, value V) (evicted bool) {
	var k K
	var v V
	c.lock.Lock()
	evicted = c.lru.Add(key, value)
	if c.onEvictedCB != nil && evicted {
		k, v = c.evictedKeys[0], c.evictedVals[0]
		c.evictedKeys, c.evictedVals = c.evictedKeys[:0], c.evictedVals[:0]
	}
	c.lock.Unlock()
	if c.onEvictedCB != nil && evicted {
		c.onEvictedCB(k, v)
	}
	return
}

// Get looks up a key's value from the cache.
func (c *CacheG[K, V]) Get(key K) (value V, ok bool) {
	c.lock.Lock()
	value, ok = c.lru.Get(key)
	c.lock.Unlock()
	return value, ok
}

// Contains checks if a key is in the cache, without updating the
// recent-ness or deleting it for being stale.
func (c *CacheG[K, V]) Contains(key K) bool {
	c.lock.RLock()
	containKey := c.lru.Contains(key)
	c.lock.RUnlock()
	return containKey
}

// Peek returns the key value (or undefined if not found) without updating
// the "recently used"-ness of the key.
func (c *CacheG[K, V]) Peek(key K) (value V, ok bool) {
	c.lock.RLock()
	value, ok = c.lru.Peek(key)
	c.lock.RUnlock()
	return value, ok
}

// ContainsOrAdd checks if a key is in the cache without updating the
// recent-ness or deleting it for being stale, and if not, adds the value.
// Returns whether found and whether an eviction occurred.
func (c *CacheG[K, V]) ContainsOrAdd(key K, value V) (ok, evicted bool) {
	var k K
	var v V
	c.lock.Lock()
	if c.lru.Contains(key) {
		c.lock.Unlock()
		return true, false
	}
	evicted = c.lru.Add(key, value)
	if c.onEvictedCB != nil && evicted {
		k, v = c.evictedKeys[0], c.evictedVals[0]
		c.evictedKeys, c.evictedVals = c.evictedKeys[:0], c.evictedVals[:0]
	}
	c.lock.Unlock()
	if c.onEvictedCB != nil && evicted {
		c.onEvictedCB(k, v)
	}
	return false, evicted
}

// PeekOrAdd checks if a key is in the cache without updating the
// recent-ness or deleting it for being stale, and if not, adds the value.
// Returns whether found and whether an eviction occurred.
func (c *CacheG[K, V]) PeekOrAdd(key K, value V) (previous interface{}, ok, evicted bool) {
	var k K
	var v V
	c.lock.Lock()
	previous, ok = c.lru.Peek(key)
	if ok {
		c.lock.Unlock()
		return previous, true, false
	}
	evicted = c.lru.Add(key, value)
	if c.onEvictedCB != nil && evicted {
		k, v = c.evictedKeys[0], c.evictedVals[0]
		c.evictedKeys, c.evictedVals = c.evictedKeys[:0], c.evictedVals[:0]
	}
	c.lock.Unlock()
	if c.onEvictedCB != nil && evicted {
		c.onEvictedCB(k, v)
	}
	return nil, false, evicted
}

// Remove removes the provided key from the cache.
func (c *CacheG[K, V]) Remove(key K) (present bool) {
	var k K
	var v V
	c.lock.Lock()
	present = c.lru.Remove(key)
	if c.onEvictedCB != nil && present {
		k, v = c.evictedKeys[0], c.evictedVals[0]
		c.evictedKeys, c.evictedVals = c.evictedKeys[:0], c.evictedVals[:0]
	}
	c.lock.Unlock()
	if c.onEvictedCB != nil && present {
		c.onEvictedCB(k, v)
	}
	return
}

// Resize changes the cache size.
func (c *CacheG[K, V]) Resize(size int) (evicted int) {
	var ks []K
	var vs []V
	c.lock.Lock()
	evicted = c.lru.Resize(size)
	if c.onEvictedCB != nil && evicted > 0 {
		ks, vs = c.evictedKeys, c.evictedVals
		c.initEvictBuffers()
	}
	c.lock.Unlock()
	if c.onEvictedCB != nil && evicted > 0 {
		for i := 0; i < len(ks); i++ {
			c.onEvictedCB(ks[i], vs[i])
		}
	}
	return evicted
}

// RemoveOldest removes the oldest item from the cache.
func (c *CacheG[K, V]) RemoveOldest() (key K, value V, ok bool) {
	var k K
	var v V
	c.lock.Lock()
	key, value, ok = c.lru.RemoveOldest()
	if c.onEvictedCB != nil && ok {
		k, v = c.evictedKeys[0], c.evictedVals[0]
		c.evictedKeys, c.evictedVals = c.evictedKeys[:0], c.evictedVals[:0]
	}
	c.lock.Unlock()
	if c.onEvictedCB != nil && ok {
		c.onEvictedCB(k, v)
	}
	return
}

// GetOldest returns the oldest entry
func (c *CacheG[K, V]) GetOldest() (key K, value V, ok bool) {
	c.lock.RLock()
	key, value, ok = c.lru.GetOldest()
	c.lock.RUnlock()
	return
}

// Keys returns a slice of the keys in the cache, from oldest to newest.
func (c *CacheG[K, V]) Keys() []K {
	c.lock.RLock()
	keys := c.lru.Keys()
	c.lock.RUnlock()
	return keys
}

// Len returns the number of items in the cache.
func (c *CacheG[K, V]) Len() int {
	c.lock.RLock()
	length := c.lru.Len()
	c.lock.RUnlock()
	return length
}
