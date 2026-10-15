package catalog

import (
	"context"
	"sync"
)

type memoKey struct {
	locale  Locale
	version int64
}

// memo holds one assembled list per (locale, content version). Storing a
// newer version evicts the older ones, so it holds at most one list per
// locale once an import has settled. Failed loads are not stored.
type memo[T any] struct {
	mu      sync.Mutex
	entries map[memoKey]T
}

func newMemo[T any]() *memo[T] {
	return &memo[T]{entries: map[memoKey]T{}}
}

// get returns the memoised value for key, or calls load and stores its
// result. Concurrent misses may load twice; the results are equal.
func (m *memo[T]) get(ctx context.Context, key memoKey, load func(context.Context, Locale) (T, error)) (T, error) {
	if v, ok := m.lookup(key); ok {
		return v, nil
	}
	v, err := load(ctx, key.locale)
	if err != nil {
		var zero T
		return zero, err
	}
	m.store(key, v)
	return v, nil
}

func (m *memo[T]) lookup(key memoKey) (T, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.entries[key]
	return v, ok
}

// store keeps v unless an entry for a newer version of the locale is held;
// a slow load that read an older version must not evict it.
func (m *memo[T]) store(key memoKey, v T) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.holdsNewer(key) {
		return
	}
	for k := range m.entries {
		if k.version < key.version {
			delete(m.entries, k)
		}
	}
	m.entries[key] = v
}

func (m *memo[T]) holdsNewer(key memoKey) bool {
	for k := range m.entries {
		if k.locale == key.locale && k.version > key.version {
			return true
		}
	}
	return false
}
