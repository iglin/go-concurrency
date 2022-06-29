package go_concurrency

import (
	"fmt"
	"sync"
	"time"
)

type IdRWLocker struct {
	locks sync.Map
	stats sync.Map
}

type stat struct {
	mutex    sync.RWMutex
	held     bool
	queue    int
	lastUsed time.Time
}

func (l *IdRWLocker) Lock(resourceId any) {
	l.addToQueue(resourceId)
	val, _ := l.locks.LoadOrStore(resourceId, &sync.RWMutex{})
	mutex := val.(*sync.RWMutex)
	mutex.Lock()
	l.updateStat(resourceId, true)
}

func (l *IdRWLocker) RLock(resourceId any) {
	l.addToQueue(resourceId)
	val, _ := l.locks.LoadOrStore(resourceId, &sync.RWMutex{})
	mutex := val.(*sync.RWMutex)
	mutex.RLock()
	l.updateStat(resourceId, true)
}

func (l *IdRWLocker) Unlock(resourceId any) {
	l.addToQueue(resourceId)
	val, ok := l.locks.Load(resourceId)
	if ok {
		l.updateStat(resourceId, false)
		mutex := val.(*sync.RWMutex)
		mutex.Unlock()
	} else {
		panic(fmt.Sprintf("memory: lock not found for resource id '%v'", resourceId))
	}
}

func (l *IdRWLocker) RUnlock(resourceId any) {
	l.addToQueue(resourceId)
	val, ok := l.locks.Load(resourceId)
	if ok {
		l.updateStat(resourceId, false)
		mutex := val.(*sync.RWMutex)
		mutex.RUnlock()
	} else {
		panic(fmt.Sprintf("memory: lock not found for resource id '%v'", resourceId))
	}
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
