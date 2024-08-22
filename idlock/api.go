package idlock

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const DefaultCollectorFirePeriod = 1 * time.Minute

// IdLocker can be used to synchronize goroutines on any resource id.
//
// E.g. you have some shared resources with comparable identifier, so you may synchronize access to these resources
// on this identifier.
//
// You can call lock := IdLocker.Lock(identifier) to acquire lock,
// and then release lock by calling lock.Unlock().
//
// Instance of this interface is to be created with NewIdLocker() constructor. You should pass this instance reference to any
// code that will use it for synchronization.
//
// By default, locks caching disabled for IdLocker.
type IdLocker[T comparable] interface {
	// Lock blocks goroutine until lock on the requested resourceId is obtained.
	// The lock must be released by calling IdLocker.Unlock(resourceId).
	Lock(resourceId T) Lock

	// Delete removes lock from cache. Waits until another goroutine releases lock in case lock is currently held.
	// This function is no-op in case locks caching is disabled.
	Delete(resourceId T)

	// GetCacheSize returns current number of locks cached by the IdLocker.
	GetCacheSize() int

	// GetStats returns cached locks statistics for the moment this function was called.
	// Returns empty map if statistics collection is disabled.
	GetStats() map[T]Stat
}

// IdRWLocker can be used to synchronize goroutines concurrent writes and reads on any resource by the resource id.
// E.g. you got some structure with UUID field 'Id', and you want to synchronize its usage
// according to the sync.RWMutex pattern.
//
// You can call IdRWLocker.Lock(struct.Id) before doing any modifications of the structure,
// and then release lock by calling IdRWLocker.Unlock(struct.Id).
//
// You can call lock := IdRWLocker.Lock(identifier) to acquire write lock or lock := IdRWLocker.RLock(identifier) to acquire read lock,
// and then release lock by calling lock.Unlock().
//
// Instance of this interface is to be created with NewIdRWLocker() constructor. You should pass this instance reference to any
// code that will use it for synchronization.
//
// IdRWLocker supports statistics collection, setting max number of locks to be cached in memory
// and collecting old locks in background. See LockerSettings for all the settings available.
//
// Please note, that locks caching is always enabled for IdRWLocker. By default, number of cached locks is unlimited and background collector is disabled.
type IdRWLocker[T comparable] interface {
	IdLocker[T]

	// RLock blocks goroutine until read lock on the requested resourceId is obtained.
	// The lock must be released by calling IdRWLocker.RUnlock(resourceId).
	//
	// If LockerSettings.MaxSize is greater than 0, then IdRWLocker.RLock(resourceId) call for the new resourceId
	// will block until the number of other locks being held is LockerSettings.MaxSize - 1.
	RLock(resourceId T) Lock
}

// LockerSettings is to be used for providing settings for IdRWLocker via NewIdRWLocker(settings) constructor.
//
// It allows to limit max number of locks cached in memory and to tune background old locks collector job.
type LockerSettings struct {
	// CacheLocks enables locks caching. IdRWLocker requires caching to be enabled.
	//
	// By default, for IdLocker locks caching disabled
	// and locks will be removed from cache on any Unlock function call;
	// for IdRWLocker locks caching enabled.
	CacheLocks bool

	// MaxSize sets the max number of locks that can be cached in memory.
	// Zero value disables max cached locks number limitation.
	// If MaxSize is greater than 0, then Lock and RLock functions call for the new resourceId
	// will block until the number of other locks being held is MaxSize - 1.
	// Then the lock that was not used for the longest time will be removed from cache.
	//
	// By default, MaxSize is 0.
	MaxSize int

	// StatsEnabled sets whether the statistics should be collected for the locks being cached.
	//
	// Since limiting MaxSize and background old locks collecting require statistics,
	// each of these two features will set StatsEnabled to true.
	// When CacheLocks is not set, stats are not being collected.
	// Otherwise (locks are being cached, but there is no automatic cleanup for them), passed value will be respected.
	// Default value for StatsEnabled is false.
	StatsEnabled bool

	// CollectorEnabled enables background job for collecting cached locks which were not be used for longer then
	// the LockerSettings.LockMaxLifetime.
	//
	// By default, locks collector job is disabled.
	CollectorEnabled bool

	// CollectorContext can be used to provide context.Context in which cached lock collector job is ran.
	// Cancelling this context will cause background collector job to finish and no further created locks will be collected.
	//
	// By default, cached locks collector job is ran in context.Background.
	CollectorContext context.Context

	// CollectorFirePeriod sets the period in which job collecting old cached locks is fired.
	//
	// By default, CollectorFirePeriod equals to DefaultCollectorFirePeriod.
	CollectorFirePeriod time.Duration

	// LockMaxLifetime sets the amount of time, after which unused cached lock
	// should be collected by background collector job.
	// Zero means that unused cached lock should be collected on the next collector job fire.
	//
	// By default, LockMaxLifetime is zero.
	LockMaxLifetime time.Duration
}

type Lock interface {
	Unlock()
}

// Stat provides statistics for the specific resource id lock cached in IdLocker or IdRWLocker:
// was the lock held, the size of the queue of goroutines waiting to lock this id and the last usage time of this id lock.
//
// Stat contains statistics for the moment statistics was requested by calling IdLocker.GetStats().
type Stat struct {
	// Indicates whether the lock on the resourceId was held for the moment statistics were requested.
	Held bool

	// Queue is the number of goroutines waiting for obtaining the lock on the resourceId.
	Queue int

	// LastUsed is the last usage time of this resourceId lock.
	LastUsed time.Time
}

// NewIdLocker constructor creates new IdLocker instance. You should pass this instance reference to any
// code that will use it for synchronization.
func NewIdLocker[K comparable](settings ...LockerSettings) IdLocker[K] {
	return &locker[K]{lockerWithCollector: newLockerWithCollector[K, *sync.Mutex](false, settings...)}
}

// NewIdRWLocker constructor creates new IdRWLocker instance. You should pass this instance reference to any
// code that will use it for synchronization.
func NewIdRWLocker[K comparable](settings ...LockerSettings) IdRWLocker[K] {
	l := &rwLocker[K]{lockerWithCollector: newLockerWithCollector[K, *sync.RWMutex](true, settings...)}
	if !l.settings.CacheLocks {
		panic(fmt.Errorf("idlocker: IdRWLocker should always be created with LockerSettings.CacheLocks = true"))
	}
	return l
}
