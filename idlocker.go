package concurrency

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type IdLocker interface {
	Lock(resourceId any)
	Unlock(resourceId any)
}

func NewIdLocker() IdLocker {
	return &idLocker{}
}

type IdRWLocker interface {
	Lock(resourceId any)
	Unlock(resourceId any)
	RLock(resourceId any)
	RUnlock(resourceId any)

	GetCacheSize() int
	GetStats() map[any]Stat
}

type LockerSettings struct {
	MaxSize             int
	StatsEnabled        bool
	CollectorEnabled    bool
	CollectorContext    context.Context
	CollectorFirePeriod time.Duration
	LockMaxLifetime     time.Duration
}

type Stat struct {
	Held     bool
	Queue    int
	LastUsed time.Time
}

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
	if locker.settings.StatsEnabled {
		locker.statsEnabled = true
	} else {
		locker.statsEnabled = locker.settings.CollectorEnabled || locker.settings.MaxSize > 0
	}

	// configure old locks collector
	if locker.settings.CollectorEnabled {
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

func (l *idRWLocker) removeOldest() {
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
	}
}

func (l *idRWLocker) removeLock(resourceId any) {
	mutexAny, _ := l.locks.LoadOrStore(resourceId, &sync.RWMutex{})
	mutex := mutexAny.(*sync.RWMutex)
	mutex.Lock()
	defer mutex.Unlock()
	l.locks.Delete(resourceId)

	statMutexAny, _ := l.stats.LoadOrStore(resourceId, &stat{held: true})
	statMutex := statMutexAny.(*sync.RWMutex)
	statMutex.Lock()
	defer statMutex.Unlock()
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
		for l.GetCacheSize() > maxSize {
			l.removeOldest()
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
