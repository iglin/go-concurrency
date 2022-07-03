package concurrency

import (
	"context"
	"github.com/stretchr/testify/assert"
	"sync"
	"testing"
	"time"
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

func TestIdRWLockerDefaults(t *testing.T) {
	locker := NewIdRWLocker()
	idRwLocker := locker.(*idRWLocker)
	assert.False(t, idRwLocker.statsEnabled)
	assert.NotNil(t, idRwLocker.settings)
	assert.False(t, idRwLocker.settings.CollectorEnabled)
	assert.False(t, idRwLocker.settings.StatsEnabled)
	assert.GreaterOrEqual(t, 0, idRwLocker.settings.MaxSize)
}

func TestIdRWLockerSettings(t *testing.T) {
	ctx := context.Background()
	locker := NewIdRWLocker(LockerSettings{
		MaxSize:             10,
		StatsEnabled:        true,
		CollectorEnabled:    true,
		CollectorContext:    ctx,
		CollectorFirePeriod: 2 * time.Minute,
		LockMaxLifetime:     10 * time.Minute,
	})
	idRwLocker := locker.(*idRWLocker)
	assert.True(t, idRwLocker.statsEnabled)
	assert.NotNil(t, idRwLocker.settings)
	assert.True(t, idRwLocker.settings.CollectorEnabled)
	assert.True(t, idRwLocker.settings.StatsEnabled)
	assert.Equal(t, 10, idRwLocker.settings.MaxSize)
	assert.Equal(t, ctx, idRwLocker.settings.CollectorContext)
	assert.Equal(t, 2*time.Minute, idRwLocker.settings.CollectorFirePeriod)
	assert.Equal(t, 10*time.Minute, idRwLocker.settings.LockMaxLifetime)
	ctx.Done()
}

func TestIdRWLockerSettingsWithMaxSize(t *testing.T) {
	locker := NewIdRWLocker(LockerSettings{
		MaxSize:          10,
		StatsEnabled:     false,
		CollectorEnabled: false,
	})
	idRwLocker := locker.(*idRWLocker)
	assert.True(t, idRwLocker.statsEnabled)
	assert.NotNil(t, idRwLocker.settings)
	assert.False(t, idRwLocker.settings.CollectorEnabled)
	assert.True(t, idRwLocker.settings.StatsEnabled)
	assert.Equal(t, 10, idRwLocker.settings.MaxSize)
}

func TestIdRWLockerSettingsWithCollector(t *testing.T) {
	ctx := context.Background()
	locker := NewIdRWLocker(LockerSettings{
		MaxSize:          -1,
		StatsEnabled:     false,
		CollectorEnabled: true,
		CollectorContext: ctx,
	})
	idRwLocker := locker.(*idRWLocker)
	assert.True(t, idRwLocker.statsEnabled)
	assert.NotNil(t, idRwLocker.settings)
	assert.True(t, idRwLocker.settings.CollectorEnabled)
	assert.True(t, idRwLocker.settings.StatsEnabled)
	assert.Equal(t, -1, idRwLocker.settings.MaxSize)
	assert.Equal(t, DefaultCollectorFirePeriod, idRwLocker.settings.CollectorFirePeriod)
	assert.Equal(t, time.Duration(0), idRwLocker.settings.LockMaxLifetime)
	ctx.Done()
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
