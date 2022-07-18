package concurrency

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestTypedSyncMap_Store(t *testing.T) {
	m := NewTypedSyncMap[int, string]()
	m.Store(1, "test1")
	m.Store(2, "test2")

	val, ok := m.Load(1)
	assert.True(t, ok)
	assert.Equal(t, "test1", val)

	val, ok = m.Load(2)
	assert.True(t, ok)
	assert.Equal(t, "test2", val)

	val, ok = m.Load(3)
	assert.False(t, ok)
	assert.Empty(t, val)
}

func TestTypedSyncMap_Delete(t *testing.T) {
	m := NewTypedSyncMap[int, string]()
	m.Store(1, "test1")
	m.Store(2, "test2")

	m.Delete(1)

	val, ok := m.Load(1)
	assert.False(t, ok)
	assert.Empty(t, val)

	val, ok = m.Load(2)
	assert.True(t, ok)
	assert.Equal(t, "test2", val)

	m.Delete(2)

	val, ok = m.Load(2)
	assert.False(t, ok)
	assert.Empty(t, val)

	m.Delete(3)
}

func TestTypedSyncMap_LoadAndDelete(t *testing.T) {
	m := NewTypedSyncMap[int, string]()
	m.Store(1, "test1")
	m.Store(2, "test2")

	val, ok := m.LoadAndDelete(1)
	assert.True(t, ok)
	assert.Equal(t, "test1", val)

	val, ok = m.Load(1)
	assert.False(t, ok)
	assert.Empty(t, val)

	val, ok = m.Load(2)
	assert.True(t, ok)
	assert.Equal(t, "test2", val)

	val, ok = m.LoadAndDelete(2)
	assert.True(t, ok)
	assert.Equal(t, "test2", val)

	val, ok = m.Load(2)
	assert.False(t, ok)
	assert.Empty(t, val)

	val, ok = m.LoadAndDelete(3)
	assert.False(t, ok)
	assert.Empty(t, val)
}
