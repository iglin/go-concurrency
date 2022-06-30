package go_concurrency

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type IdRWLocker struct {
	locks    sync.Map
	stats    sync.Map
	settings LockerSettings
}

func NewIdRWLocker(settings ...LockerSettings) *IdRWLocker {
	locker := IdRWLocker{}
	if len(settings) == 0 {
		locker.settings = LockerSettings{
			MaxSize:          -1,
			CollectorEnabled: false,
		}
	} else {
		locker.settings = settings[0]
		if locker.settings.CollectorEnabled && locker.settings.CollectorContext == nil {
			locker.settings.CollectorContext = context.Background()
		}
	}
	if locker.settings.CollectorEnabled {
		go locker.runCollector()
	}
	return &locker
}

func (l *IdRWLocker) runCollector() {
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

func (l *IdRWLocker) collectOldLocks() {
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

type LockerSettings struct {
	MaxSize             int
	CollectorEnabled    bool
	CollectorContext    context.Context
	CollectorFirePeriod time.Duration
	LockMaxLifetime     time.Duration
}

type stat struct {
	mutex    sync.RWMutex
	held     bool
	queue    int
	lastUsed time.Time
}

func (l *IdRWLocker) removeOldest() {
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

func (l *IdRWLocker) removeLock(resourceId any) {
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

func (l *IdRWLocker) Lock(resourceId any) {
	needStats := l.needStats()
	if needStats {
		l.addToQueue(resourceId)
	}
	val, _ := l.locks.LoadOrStore(resourceId, &sync.RWMutex{})
	mutex := val.(*sync.RWMutex)
	mutex.Lock()
	if needStats {
		l.updateStat(resourceId, true)
	}
}

func (l *IdRWLocker) RLock(resourceId any) {
	needStats := l.needStats()
	if needStats {
		l.addToQueue(resourceId)
	}
	val, _ := l.locks.LoadOrStore(resourceId, &sync.RWMutex{})
	mutex := val.(*sync.RWMutex)
	mutex.RLock()
	if needStats {
		l.updateStat(resourceId, true)
	}
}

func (l *IdRWLocker) Unlock(resourceId any) {
	val, ok := l.locks.Load(resourceId)
	if ok {
		if l.needStats() {
			l.updateStat(resourceId, false)
		}
		mutex := val.(*sync.RWMutex)
		mutex.Unlock()
	} else {
		panic(fmt.Sprintf("memory: lock not found for resource id '%v'", resourceId))
	}
}

func (l *IdRWLocker) RUnlock(resourceId any) {
	val, ok := l.locks.Load(resourceId)
	if ok {
		if l.needStats() {
			l.updateStat(resourceId, false)
		}
		mutex := val.(*sync.RWMutex)
		mutex.RUnlock()
	} else {
		panic(fmt.Sprintf("memory: lock not found for resource id '%v'", resourceId))
	}
}

func (l *IdRWLocker) needStats() bool {
	return l.settings.MaxSize > 0 || l.settings.CollectorEnabled
}

func (l *IdRWLocker) addToQueue(resourceId any) {
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
		for l.getSize() > maxSize {
			l.removeOldest()
		}
	}
}

func (l *IdRWLocker) getSize() int {
	size := 0
	l.locks.Range(func(_, _ any) bool {
		size++
		return true
	})
	return size
}

func (l *IdRWLocker) updateStat(resourceId any, held bool) {
	statAny, loaded := l.stats.LoadOrStore(resourceId, &stat{
		held:     held,
		queue:    0,
		lastUsed: time.Now(),
	})
	if loaded {
		stat := statAny.(*stat)
		stat.mutex.Lock()
		stat.held = true
		if held {
			stat.queue--
		}
		stat.lastUsed = time.Now()
		stat.mutex.Unlock()
	}
}

type IdLocker struct {
	locks sync.Map
}

func (l *IdLocker) Lock(resourceId any) {
	for { // sometimes mutex created by another goroutine will be loaded from the locks map, so might need to try several times
		if lockObtained := l.lockInternal(resourceId); lockObtained {
			return
		}
	}
}

func (l *IdLocker) lockInternal(resourceId any) bool {
	val, loaded := l.locks.LoadOrStore(resourceId, &sync.Mutex{})
	mutex := val.(*sync.Mutex)
	if loaded { // another goroutine already uses this resource
		mutex.Lock() // wait for another goroutine to remove the lock from the map
		return false // signal that lock is still not obtained and need to try again
	} else {
		mutex.Lock()
		return true
	}
}

func (l *IdLocker) Unlock(resourceId any) {
	val, ok := l.locks.LoadAndDelete(resourceId)
	if ok {
		mutex := val.(*sync.Mutex)
		mutex.Unlock()
	} else {
		panic(fmt.Sprintf("no lock found for resourceId '%v'", resourceId))
	}
}
