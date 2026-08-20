package store

import (
	"cmp"
	"slices"
)

// The caps on what this package keeps. A detail on a heavily reviewed pull
// request is megabytes, and without a bound a day's reading holds every one.
const (
	detailCap = 25
	filesCap  = 25
	commitCap = 40
	jobCap    = 5
)

// cache is a bounded map of fetched values, ordered by when each was last read.
// The zero value is usable and unbounded: put builds the maps.
type cache[V any] struct {
	held  map[string]V
	seen  map[string]int
	limit int
}

// newCache builds one up front. Left to put, the maps are built by whichever
// writer went first, and half of them run on a copy of the model.
func newCache[V any](limit int) cache[V] {
	return cache[V]{held: make(map[string]V), seen: make(map[string]int), limit: limit}
}

// get is the value held for a key, or the zero value where nothing is. Every
// caller already reads a key never fetched that way.
func (c cache[V]) get(key string) V { return c.held[key] }

// look is get for a caller that has to tell a zero value from nothing at all.
func (c cache[V]) look(key string) (V, bool) {
	v, ok := c.held[key]
	return v, ok
}

func (c cache[V]) len() int { return len(c.held) }

// put writes a value and stamps it as the most recently read.
func (c *cache[V]) put(key string, v V) {
	if c.held == nil {
		c.held, c.seen = make(map[string]V), make(map[string]int)
	}
	c.held[key] = v
	c.seen[key] = c.next()
}

// touch restamps a value already held, for a read that refetches nothing. It
// records use and never creates it: a key held by nobody is not put here.
func (c *cache[V]) touch(key string) {
	if _, ok := c.held[key]; !ok {
		return
	}
	c.seen[key] = c.next()
}

// next is one past the highest stamp, read from the map rather than from a
// counter: an int field is the one write here a caller on a copy can lose.
func (c cache[V]) next() int {
	high := 0
	for _, at := range c.seen {
		high = max(high, at)
	}
	return high + 1
}

// evict drops the least recently read entries until the cache is inside its
// limit, answering with the keys it dropped so a caller can clear what sat with them.
func (c *cache[V]) evict(wrote string, pinned func(string) bool) []string {
	// No limit is unbounded, which is what a cache built without newCache is.
	if c.limit <= 0 || len(c.held) <= c.limit {
		return nil
	}

	// Never the key just written: it is the most recently read thing there is,
	// and where the rest are pinned it is the only candidate left.
	spare := make([]string, 0, len(c.held))
	for key := range c.held {
		if key != wrote && !pinned(key) {
			spare = append(spare, key)
		}
	}
	// Oldest first, which is the order they go in. A cache of nothing but pinned
	// entries goes over its limit: a pin outranks it, and a drop cannot be undone.
	slices.SortFunc(spare, func(a, b string) int { return cmp.Compare(c.seen[a], c.seen[b]) })

	var dropped []string
	for _, key := range spare {
		if len(c.held) <= c.limit {
			break
		}
		delete(c.held, key)
		delete(c.seen, key)
		dropped = append(dropped, key)
	}
	return dropped
}
