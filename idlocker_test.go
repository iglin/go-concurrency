package concurrency

import (
	"github.com/stretchr/testify/assert"
	"sync"
	"testing"
)

const scale = 1000

func TestIdLocker(t *testing.T) {
	locker := NewIdLocker()

	resourceId := 1
	sharedCounter := 0

	wg := sync.WaitGroup{}
	wg.Add(scale)
	for i := 0; i < scale; i++ {
		go func() {
			locker.Lock(resourceId)
			defer locker.Unlock(resourceId)
			sharedCounter++
			wg.Done()
		}()
	}
	wg.Wait()
	assert.Equal(t, scale, sharedCounter)
}

func TestIdRWLockerWithStats(t *testing.T) {
	testIdRWLockerInternal(t, true)
}

func TestIdRWLockerWithoutStats(t *testing.T) {
	testIdRWLockerInternal(t, false)
}

func testIdRWLockerInternal(t *testing.T, statsEnabled bool) {
	locker := NewIdRWLocker(LockerSettings{
		StatsEnabled: statsEnabled,
	})

	resourceId := 1
	sharedCounter := 0

	wg := sync.WaitGroup{}
	wg.Add(2 * scale)
	for i := 1; i <= 2*scale; i++ {
		go func() {
			locker.RLock(resourceId)
			assert.LessOrEqual(t, sharedCounter, scale)
			locker.RUnlock(resourceId)

			locker.Lock(resourceId)
			defer locker.Unlock(resourceId)
			if sharedCounter < scale {
				sharedCounter++
			}
			wg.Done()
		}()
	}
	wg.Wait()
	assert.Equal(t, scale, sharedCounter)
	if statsEnabled {
		assert.Equal(t, 1, len(locker.GetStats()))
		stat := locker.GetStats()[resourceId]
		assert.Equal(t, 0, stat.Queue)
		assert.False(t, stat.Held)
	} else {
		assert.Empty(t, locker.GetStats())
	}
	assert.Equal(t, 1, locker.GetCacheSize())
}
