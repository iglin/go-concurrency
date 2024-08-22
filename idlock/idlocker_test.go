package idlock

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/assert"
	"sync"
	"testing"
	"time"
)

const scale = 1000

func TestIdLocker(t *testing.T) {
	locker := NewIdLocker[int]()

	resourceId := 1
	sharedCounter := 0

	wg := &sync.WaitGroup{}
	wg.Add(scale)
	for i := 0; i < scale; i++ {
		go incrementCounter(locker, resourceId, wg, &sharedCounter)
	}
	fmt.Println("Waiting all goroutines to finish")
	wg.Wait()
	assert.Equal(t, scale, sharedCounter)
}

func incrementCounter(locker IdLocker[int], resourceId int, wg *sync.WaitGroup, sharedCounter *int) {
	l := locker.Lock(resourceId)
	defer l.Unlock()
	fmt.Println("Incrementing", *sharedCounter)
	*sharedCounter++
	wg.Done()
}

func TestIdRWLockerDefaults(t *testing.T) {
	locker := NewIdRWLocker[int]()
	idRwLocker := locker.(*rwLocker[int])
	assert.NotNil(t, idRwLocker.settings)
	assert.True(t, idRwLocker.settings.CacheLocks)
	assert.False(t, idRwLocker.settings.StatsEnabled)
	assert.False(t, idRwLocker.settings.CollectorEnabled)
	assert.False(t, idRwLocker.settings.StatsEnabled)
	assert.GreaterOrEqual(t, 0, idRwLocker.settings.MaxSize)
}

func TestIdRWLockerSettings(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	locker := NewIdRWLocker[int](LockerSettings{
		CacheLocks:          true,
		MaxSize:             10,
		StatsEnabled:        true,
		CollectorEnabled:    true,
		CollectorContext:    ctx,
		CollectorFirePeriod: 2 * time.Minute,
		LockMaxLifetime:     10 * time.Minute,
	})
	idRwLocker := locker.(*rwLocker[int])
	assert.NotNil(t, idRwLocker.settings)
	assert.True(t, idRwLocker.settings.CacheLocks)
	assert.True(t, idRwLocker.settings.StatsEnabled)
	assert.True(t, idRwLocker.settings.CollectorEnabled)
	assert.True(t, idRwLocker.settings.StatsEnabled)
	assert.Equal(t, 10, idRwLocker.settings.MaxSize)
	assert.Equal(t, ctx, idRwLocker.settings.CollectorContext)
	assert.Equal(t, 2*time.Minute, idRwLocker.settings.CollectorFirePeriod)
	assert.Equal(t, 10*time.Minute, idRwLocker.settings.LockMaxLifetime)
	cancel()
}

func TestIdRWLockerSettingsPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("The code did not panic")
		}
	}()
	_ = NewIdRWLocker[int](LockerSettings{
		MaxSize: 10,
	})
}

func TestIdRWLockerSettingsWithMaxSize(t *testing.T) {
	locker := NewIdRWLocker[int](LockerSettings{
		CacheLocks:       true,
		MaxSize:          10,
		StatsEnabled:     false,
		CollectorEnabled: false,
	})
	idRwLocker := locker.(*rwLocker[int])
	assert.NotNil(t, idRwLocker.settings)
	assert.True(t, idRwLocker.settings.CacheLocks)
	assert.True(t, idRwLocker.settings.StatsEnabled)
	assert.False(t, idRwLocker.settings.CollectorEnabled)
	assert.True(t, idRwLocker.settings.StatsEnabled)
	assert.Equal(t, 10, idRwLocker.settings.MaxSize)
}

func TestIdRWLockerSettingsWithCollector(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	locker := NewIdRWLocker[int](LockerSettings{
		CacheLocks:       true,
		MaxSize:          0,
		StatsEnabled:     false,
		CollectorEnabled: true,
		CollectorContext: ctx,
	})
	idRwLocker := locker.(*rwLocker[int])
	assert.NotNil(t, idRwLocker.settings)
	assert.True(t, idRwLocker.settings.CacheLocks)
	assert.True(t, idRwLocker.settings.StatsEnabled)
	assert.True(t, idRwLocker.settings.CollectorEnabled)
	assert.True(t, idRwLocker.settings.StatsEnabled)
	assert.Equal(t, 0, idRwLocker.settings.MaxSize)
	assert.Equal(t, DefaultCollectorFirePeriod, idRwLocker.settings.CollectorFirePeriod)
	assert.Equal(t, time.Duration(0), idRwLocker.settings.LockMaxLifetime)
	cancel()
}

func TestIdRwLockerMaxSize(t *testing.T) {
	locker := NewIdRWLocker[int](LockerSettings{CacheLocks: true, MaxSize: 5})
	for i := 0; i < 10; i++ {
		lock := locker.Lock(i)
		lock.Unlock()
	}
	assert.Equal(t, 5, locker.GetCacheSize())
}

func TestIdRwLockerCollector(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	locker := NewIdRWLocker[int](LockerSettings{
		CacheLocks:          true,
		CollectorEnabled:    true,
		CollectorContext:    ctx,
		CollectorFirePeriod: 200 * time.Millisecond,
		LockMaxLifetime:     1 * time.Second,
	})
	lock := locker.Lock(1)
	lock.Unlock()

	lock = locker.Lock(2)
	defer lock.Unlock()

	time.Sleep(1200 * time.Millisecond)
	anotherLock := locker.Lock(3)
	anotherLock.Unlock()

	cancel()

	assert.Equal(t, 2, locker.GetCacheSize())

	time.Sleep(1200 * time.Millisecond)
	assert.Equal(t, 2, locker.GetCacheSize())
}

func TestIdRWLockerWithStats(t *testing.T) {
	testIdRWLockerInternal(t, true)
}

func TestIdRWLockerWithoutStats(t *testing.T) {
	testIdRWLockerInternal(t, false)
}

func testIdRWLockerInternal(t *testing.T, statsEnabled bool) {
	locker := NewIdRWLocker[int](LockerSettings{
		CacheLocks:   true,
		StatsEnabled: statsEnabled,
	})

	resourceId := 1
	sharedCounter := 0

	wg := sync.WaitGroup{}
	wg.Add(2 * scale)
	for i := 1; i <= 2*scale; i++ {
		go func() {
			rLock := locker.RLock(resourceId)
			assert.LessOrEqual(t, sharedCounter, scale)
			rLock.Unlock()

			lock := locker.Lock(resourceId)
			defer lock.Unlock()
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
