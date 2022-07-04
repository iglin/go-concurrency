package concurrency

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const DefaultCollectorFirePeriod = 1 * time.Minute

// IdLocker can be used to synchronize goroutines on any resource id.
// E.g. you got some structure with UUID field 'Id', and you do not want to allow concurrent modification of this
// structure.
//
// You can call IdLocker.Lock(struct.Id) before doing any modifications of the structure,
// and then release lock by calling IdLocker.Unlock(struct.Id).
//
// Instance of this interface is to be created with NewIdLocker() constructor. You should pass this instance reference to any
// code that will use it for synchronization.
type IdLocker interface {
	// Lock blocks goroutine until lock on the requested resourceId is obtained.
	// The lock must be released by calling IdLocker.Unlock(resourceId).
	Lock(resourceId any)

	// Unlock releases the lock on specified resourceId.
	Unlock(resourceId any)
}

// NewIdLocker constructor creates new IdLocker instance. You should pass this instance reference to any
// code that will use it for synchronization.
func NewIdLocker() IdLocker {
	return &idLocker{}
}

// IdRWLocker can be used to synchronize goroutines concurrent writes and reads on any resource by the resource id.
// E.g. you got some structure with UUID field 'Id', and you want to synchronize its usage
// according to the sync.RWMutex pattern.
//
// You can call IdRWLocker.Lock(struct.Id) before doing any modifications of the structure,
// and then release lock by calling IdRWLocker.Unlock(struct.Id).
//
// You can use IdRWLocker.RLock(struct.Id) when reading the structure,
// and then release lock by calling IdRWLocker.RUnlock(struct.Id).
//
// Instance of this interface is to be created with NewIdRWLocker() constructor. You should pass this instance reference to any
// code that will use it for synchronization.
//
// IdRWLocker supports statistics collection, setting max number of locks to be cached in memory
// and collecting old locks in background. See LockerSettings for all the settings available.
//
// By default, number of cached locks is unlimited and background collector is disabled.
type IdRWLocker interface {
	// Lock blocks goroutine until write lock on the requested resourceId is obtained.
	// The lock must be released by calling IdRWLocker.Unlock(resourceId).
	//
	// If LockerSettings.MaxSize is greater than 0, then IdRWLocker.Lock(resourceId) call for the new resourceId
	// will block until the number of other locks being held is LockerSettings.MaxSize - 1.
	Lock(resourceId any)

	// Unlock releases write lock on specified resourceId.
	Unlock(resourceId any)

	// RLock blocks goroutine until read lock on the requested resourceId is obtained.
	// The lock must be released by calling IdRWLocker.RUnlock(resourceId).
	//
	// If LockerSettings.MaxSize is greater than 0, then IdRWLocker.RLock(resourceId) call for the new resourceId
	// will block until the number of other locks being held is LockerSettings.MaxSize - 1.
	RLock(resourceId any)
	// RUnlock releases read lock on specified resourceId.
	RUnlock(resourceId any)

	// GetCacheSize returns current number of locks cached by the IdRWLocker.
	GetCacheSize() int

	// GetStats returns cached locks statistics for the moment this function was called.
	// Returns empty map if statistics collection is disabled.
	GetStats() map[any]Stat
}

// LockerSettings is to be used for providing settings for IdRWLocker via NewIdRWLocker(settings) constructor.
//
// It allows to limit max number of locks cached in memory and to tune background old locks collector job.
type LockerSettings struct {
	// MaxSize sets the max number of locks that can be cached in memory.
	// Value less or equal to zero disables max cached locks number limitation.
	// If MaxSize is greater than 0, then IdRWLocker.Lock and IdRWLocker.RLock functions call for the new resourceId
	// will block until the number of other locks being held is MaxSize - 1.
	//
	// By default, MaxSize is 0.
	MaxSize int

	// StatsEnabled sets whether the statistics should be collected for the locks being cached.
	//
	// Since limiting MaxSize and background old locks collecting require statistics,
	// each of these two feature will set StatsEnabled to true. Otherwise, default value for StatsEnabled is false.
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

// Stat provides statistics for the specific resource id lock cached in IdRWLocker:
// was the lock held, the size of the queue of goroutines waiting to lock this id and the last usage time of this id lock.
//
// Stat contains statistics for the moment statistics was requested by calling IdRWLocker.GetStats().
type Stat struct {
	// Indicates whether the lock on the resourceId was held for the moment statistics were requested.
	Held bool

	// Queue is the number of goroutines waiting for obtaining the lock on the resourceId.
	Queue int

	// LastUsed is the last usage time of this resourceId lock.
	LastUsed time.Time
}

// NewIdRWLocker constructor creates new IdRWLocker instance. You should pass this instance reference to any
// code that will use it for synchronization.
//
// Optionally, you can provide LockerSettings to limit max number of cached locks and to set up
// background job for collecting old locks. By default, these features are disabled: number of cached
// locks is unlimited, and old locks are not collected in background.
func NewIdRWLocker(settings ...LockerSettings) IdRWLocker {
	locker := idRWLocker{}
	if len(settings) == 0 {
		locker.settings = LockerSettings{
			MaxSize:          -1,
			StatsEnabled:     false,
			CollectorEnabled: false,
		}
	} else {
		locker.settings = settings[0]
		if locker.settings.CollectorEnabled && locker.settings.CollectorContext == nil {
			locker.settings.CollectorContext = context.Background()
		}
	}

	// configure stats collection
	locker.statsEnabled = locker.settings.StatsEnabled ||
		locker.settings.CollectorEnabled ||
		locker.settings.MaxSize > 0
	if locker.statsEnabled {
		locker.settings.StatsEnabled = true
	}

	// configure old locks collector
	if locker.settings.CollectorEnabled {
		if locker.settings.CollectorFirePeriod == 0 {
			locker.settings.CollectorFirePeriod = DefaultCollectorFirePeriod
		}
		go locker.runCollector()
	}
	return &locker
}

type idRWLocker struct {
	locks        sync.Map
	stats        sync.Map
	statsEnabled bool
	settings     LockerSettings
}

type stat struct {
	mutex    sync.RWMutex
	held     bool
	queue    int
	lastUsed time.Time
}

func (l *idRWLocker) runCollector() {
	ctx := l.settings.CollectorContext
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		select {
		case <-time.After(l.settings.CollectorFirePeriod):
			l.collectOldLocks()
		case <-ctx.Done():
			return
		}
	}
}

func (l *idRWLocker) collectOldLocks() {
	now := time.Now()
	threshold := l.settings.LockMaxLifetime
	l.stats.Range(func(resourceId, statAny any) bool {
		stat := statAny.(*stat)
		stat.mutex.RLock()
		if !stat.held && now.Sub(stat.lastUsed) > threshold {
			stat.mutex.RUnlock()
			l.removeLock(resourceId)
		} else {
			stat.mutex.RUnlock()
		}
		return true
	})
}

func (l *idRWLocker) removeOldest() bool {
	oldestTime := time.Now()
	var oldestResourceId any
	l.stats.Range(func(resourceId, statAny any) bool {
		stat := statAny.(*stat)
		stat.mutex.RLock()

		if !stat.held && stat.lastUsed.Before(oldestTime) {
			oldestResourceId = resourceId
			oldestTime = stat.lastUsed
		}
		stat.mutex.RUnlock()
		return true
	})
	if oldestResourceId != nil {
		l.removeLock(oldestResourceId)
		return true
	}
	return false
}

func (l *idRWLocker) removeLock(resourceId any) {
	mutexAny, _ := l.locks.LoadOrStore(resourceId, &sync.RWMutex{})
	mutex := mutexAny.(*sync.RWMutex)
	mutex.Lock()
	defer mutex.Unlock()
	l.locks.Delete(resourceId)

	statAny, _ := l.stats.LoadOrStore(resourceId, &stat{held: true})
	stat := statAny.(*stat)
	stat.mutex.Lock()
	defer stat.mutex.Unlock()
	l.stats.Delete(resourceId)
}

func (l *idRWLocker) Lock(resourceId any) {
	if l.statsEnabled {
		l.addToQueue(resourceId)
	}
	val, _ := l.locks.LoadOrStore(resourceId, &sync.RWMutex{})
	mutex := val.(*sync.RWMutex)
	mutex.Lock()
	if l.statsEnabled {
		l.updateStat(resourceId, true)
	}
}

func (l *idRWLocker) RLock(resourceId any) {
	if l.statsEnabled {
		l.addToQueue(resourceId)
	}
	val, _ := l.locks.LoadOrStore(resourceId, &sync.RWMutex{})
	mutex := val.(*sync.RWMutex)
	mutex.RLock()
	if l.statsEnabled {
		l.updateStat(resourceId, true)
	}
}

func (l *idRWLocker) Unlock(resourceId any) {
	val, ok := l.locks.Load(resourceId)
	if ok {
		if l.statsEnabled {
			l.updateStat(resourceId, false)
		}
		mutex := val.(*sync.RWMutex)
		mutex.Unlock()
	} else {
		panic(fmt.Sprintf("memory: lock not found for resource id '%v'", resourceId))
	}
}

func (l *idRWLocker) RUnlock(resourceId any) {
	val, ok := l.locks.Load(resourceId)
	if ok {
		if l.statsEnabled {
			l.updateStat(resourceId, false)
		}
		mutex := val.(*sync.RWMutex)
		mutex.RUnlock()
	} else {
		panic(fmt.Sprintf("memory: lock not found for resource id '%v'", resourceId))
	}
}

func (l *idRWLocker) GetCacheSize() int {
	size := 0
	l.locks.Range(func(_, _ any) bool {
		size++
		return true
	})
	return size
}

func (l *idRWLocker) GetStats() map[any]Stat {
	result := make(map[any]Stat)
	if l.statsEnabled {
		l.stats.Range(func(resourceId, statAny any) bool {
			statInternal := statAny.(*stat)
			statInternal.mutex.RLock()
			result[resourceId] = Stat{
				Held:     statInternal.held,
				Queue:    statInternal.queue,
				LastUsed: statInternal.lastUsed,
			}
			statInternal.mutex.RUnlock()
			return true
		})
	}
	return result
}

func (l *idRWLocker) addToQueue(resourceId any) {
	statAny, loaded := l.stats.LoadOrStore(resourceId, &stat{
		held:     false,
		queue:    1,
		lastUsed: time.Now(),
	})
	if loaded {
		stat := statAny.(*stat)
		stat.mutex.Lock()
		stat.queue++
		stat.lastUsed = time.Now()
		stat.mutex.Unlock()
	}
	maxSize := l.settings.MaxSize
	if maxSize > 0 {
		for l.GetCacheSize() >= maxSize {
			if l.removeOldest() {
				continue
			} else {
				time.Sleep(100 * time.Millisecond)
			}
		}
	}
}

func (l *idRWLocker) updateStat(resourceId any, held bool) {
	statAny, loaded := l.stats.LoadOrStore(resourceId, &stat{
		held:     held,
		queue:    0,
		lastUsed: time.Now(),
	})
	if loaded {
		stat := statAny.(*stat)
		stat.mutex.Lock()
		stat.held = held
		if held {
			stat.queue--
		}
		stat.lastUsed = time.Now()
		stat.mutex.Unlock()
	}
}

type idLocker struct {
	locks sync.Map
}

func (l *idLocker) Lock(resourceId any) {
	for { // sometimes mutex created by another goroutine will be loaded from the locks map, so might need to try several times
		if lockObtained := l.lockInternal(resourceId); lockObtained {
			return
		}
	}
}

func (l *idLocker) lockInternal(resourceId any) bool {
	val, loaded := l.locks.LoadOrStore(resourceId, &sync.Mutex{})
	mutex := val.(*sync.Mutex)
	if loaded { // another goroutine already uses this resource
		mutex.Lock() // wait for another goroutine to remove the lock from the map
		defer mutex.Unlock()
		return false // signal that lock is still not obtained and need to try again
	} else {
		mutex.Lock()
		return true
	}
}

func (l *idLocker) Unlock(resourceId any) {
	val, ok := l.locks.LoadAndDelete(resourceId)
	if ok {
		mutex := val.(*sync.Mutex)
		mutex.Unlock()
	} else {
		panic(fmt.Sprintf("no lock found for resourceId '%v'", resourceId))
	}
}
