package store

import (
	"cmp"
	"slices"
)

// Bounded because a detail on a heavily reviewed pull request is megabytes.
const (
	detailCap = 25
	filesCap  = 25
	commitCap = 40
	jobCap    = 5
)

type cache[V any] struct {
	held  map[string]V
	seen  map[string]int
	limit int
}

// newCache builds the maps up front, since a first write made on a copy of the model would drop them.
func newCache[V any](limit int) cache[V] {
	return cache[V]{held: make(map[string]V), seen: make(map[string]int), limit: limit}
}

func (c cache[V]) get(key string) V { return c.held[key] }

func (c cache[V]) look(key string) (V, bool) {
	v, ok := c.held[key]
	return v, ok
}

func (c cache[V]) len() int { return len(c.held) }

func (c *cache[V]) put(key string, v V) {
	if c.held == nil {
		c.held, c.seen = make(map[string]V), make(map[string]int)
	}
	c.held[key] = v
	c.seen[key] = c.next()
}

func (c *cache[V]) touch(key string) {
	if _, ok := c.held[key]; !ok {
		return
	}
	c.seen[key] = c.next()
}

// next reads the highest stamp from the map rather than a counter, which a caller on a copy would lose.
func (c cache[V]) next() int {
	high := 0
	for _, at := range c.seen {
		high = max(high, at)
	}
	return high + 1
}

func (c *cache[V]) evict(wrote string, pinned func(string) bool) []string {
	if c.limit <= 0 || len(c.held) <= c.limit {
		return nil
	}

	spare := make([]string, 0, len(c.held))
	for key := range c.held {
		if key != wrote && !pinned(key) {
			spare = append(spare, key)
		}
	}
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
