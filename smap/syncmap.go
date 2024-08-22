package smap

import "sync"

// TypedSyncMap has the same interface as golang sync.Map, but extends it with generic types for keys and values.
// Any type can be used as key or value type, including references.
//
// New instance can be created by constructor and specifying types for key and value, e.g.
//
// typedSyncMap := NewTypedSyncMap[int, *resource]()
type TypedSyncMap[K any, V any] struct {
	m sync.Map
}

// NewTypedSyncMap constructor creates new TypedSyncMap instance and returns reference on it.
func NewTypedSyncMap[K any, V any]() *TypedSyncMap[K, V] {
	return &TypedSyncMap[K, V]{}
}

// Load returns the value stored in the map for a key, or nil if no
// value is present.
// The ok result indicates whether value was found in the map.
func (t *TypedSyncMap[K, V]) Load(key K) (value V, ok bool) {
	if val, ok := t.m.Load(key); ok {
		return val.(V), true
	} else {
		return value, false
	}
}

// Store sets the value for a key.
func (t *TypedSyncMap[K, V]) Store(key K, value V) {
	t.m.Store(key, value)
}

// LoadOrStore returns the existing value for the key if present.
// Otherwise, it stores and returns the given value.
// The loaded result is true if the value was loaded, false if stored.
func (t *TypedSyncMap[K, V]) LoadOrStore(key K, value V) (actual V, loaded bool) {
	if loadedVal, ok := t.m.LoadOrStore(key, value); ok {
		return loadedVal.(V), true
	} else {
		return value, false
	}
}

// LoadAndDelete deletes the value for a key, returning the previous value if any.
// The loaded result reports whether the key was present.
func (t *TypedSyncMap[K, V]) LoadAndDelete(key K) (value V, loaded bool) {
	if loadedVal, ok := t.m.LoadAndDelete(key); ok {
		return loadedVal.(V), true
	} else {
		return value, false
	}
}

// Delete deletes the value for a key.
func (t *TypedSyncMap[K, V]) Delete(key K) {
	t.m.Delete(key)
}

// Range calls f sequentially for each key and value present in the map.
// If f returns false, range stops the iteration.
//
// Range does not necessarily correspond to any consistent snapshot of the Map's
// contents: no key will be visited more than once, but if the value for any key
// is stored or deleted concurrently (including by f), Range may reflect any
// mapping for that key from any point during the Range call. Range does not
// block other methods on the receiver; even f itself may call any method on m.
//
// Range may be O(N) with the number of elements in the map even if f returns
// false after a constant number of calls.
func (t *TypedSyncMap[K, V]) Range(f func(key K, value V) bool) {
	t.m.Range(func(key, value any) bool {
		return f(key.(K), value.(V))
	})
}
