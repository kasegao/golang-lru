package simplelru

import (
	"errors"

	omap "github.com/kasegao/go-orderedmap"
)

// EvictCallbackG is used to get a callback when a cache entry is evicted
type EvictCallbackG[K comparable, V any] func(key K, value V)

// LRUG implements a non-thread safe fixed size LRU cache
type LRUG[K comparable, V any] struct {
	size    int
	items   *omap.OrderedMap[K, V]
	onEvict EvictCallbackG[K, V]
}

// NewLRUG constructs an LRU of the given size
func NewLRUG[K comparable, V any](size int, onEvict EvictCallbackG[K, V]) (*LRUG[K, V], error) {
	if size <= 0 {
		return nil, errors.New("must provide a positive size")
	}

	c := &LRUG[K, V]{
		size:    size,
		items:   omap.New[K, V](),
		onEvict: onEvict,
	}
	return c, nil
}

// Purge is used to completely clear the cache.
func (c *LRUG[K, V]) Purge() {
	items := c.items.Items()
	for _, item := range items {
		if c.onEvict != nil {
			c.onEvict(item.Key, item.Value)
		}
		c.items.Delete(item.Key)
	}
}

// Add adds a value to the cache. Returns true if an eviction occurred.
func (c *LRUG[K, V]) Add(key K, value V) (evicted bool) {
	if _, ok := c.items.Get(key); ok {
		c.items.ToTail(key)
	}

	c.items.Set(key, value)
	evict := c.items.Len() > c.size
	if evict {
		c.removeOldest()
	}
	return evict
}

// Get looks up a key's value from the cache.
func (c *LRUG[K, V]) Get(key K) (value V, ok bool) {
	v, ok := c.Peek(key)
	if ok {
		c.items.ToTail(key)
	}
	return v, ok
}

// Contains checks if a key is in the cache, without updating the recent-ness or deleting it for being stale.
func (c *LRUG[K, V]) Contains(key K) (ok bool) {
	_, ok = c.items.Get(key)
	return ok
}

// Peek returns the key value (or undefined if not found) without updating the "recently used"-ness of the key.
func (c *LRUG[K, V]) Peek(key K) (value V, ok bool) {
	v, ok := c.items.Get(key)
	return v, ok
}

// Remove removes the provided key from the cache, returning if the key was contained.
func (c *LRUG[K, V]) Remove(key K) (present bool) {
	_, ok := c.Peek(key)
	if ok {
		c.removeElement(key)
	}
	return ok
}

// RemoveOldest removes the oldest item from the cache.
func (c *LRUG[K, V]) RemoveOldest() (key K, value V, ok bool) {
	e, ok := c.items.GetAt(0)
	if ok {
		c.removeElement(e.Key)
		return e.Key, e.Value, ok
	}
	return
}

// GetOldest returns the oldest entry without updating the "recently used"-ness of the key.
func (c *LRUG[K, V]) GetOldest() (key K, value V, ok bool) {
	e, ok := c.items.GetAt(0)
	if ok {
		return e.Key, e.Value, ok
	}
	return
}

// Keys returns a slice of the keys in the cache, from oldest to newest.
func (c *LRUG[K, V]) Keys() []K {
	return c.items.Keys()
}

// Len returns the number of items in the cache.
func (c *LRUG[K, V]) Len() int {
	return c.items.Len()
}

// Resize changes the cache size.
func (c *LRUG[K, V]) Resize(size int) (evicted int) {
	diff := c.Len() - size
	if diff < 0 {
		diff = 0
	}
	for i := 0; i < diff; i++ {
		c.removeOldest()
	}
	c.size = size
	return diff
}

// removeOldest removes the oldest item from the cache.
func (c *LRUG[K, V]) removeOldest() {
	e, ok := c.items.GetAt(0)
	if ok {
		c.removeElement(e.Key)
	}
}

// removeElement is used to remove a given list element from the cache
func (c *LRUG[K, V]) removeElement(key K) {
	e, ok := c.items.Pop(key)
	if ok && c.onEvict != nil {
		c.onEvict(e.Key, e.Value)
	}
}
