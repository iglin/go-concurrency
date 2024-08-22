package idlock

import (
	"sync"
)

type locker[K comparable] struct {
	*lockerWithCollector[K, *sync.Mutex]
}

func (l *locker[K]) Lock(resourceId K) Lock {
	var mutex *sync.Mutex
	if l.settings.CacheLocks {
		if l.settings.StatsEnabled {
			l.addToQueue(resourceId)
		}
		mutex, _ = l.locks.LoadOrCompute(resourceId, func() *sync.Mutex { return &sync.Mutex{} })
		mutex.Lock()
		if l.settings.StatsEnabled {
			l.updateStat(resourceId, true)
		}
	} else {
		var lockObtained bool
		// sometimes mutex created by another goroutine will be loaded from the locks map, so might need to retry several times
		for mutex, lockObtained = l.lockInternal(resourceId); !lockObtained; mutex, lockObtained = l.lockInternal(resourceId) {
		}
	}

	return &lock[K, *sync.Mutex]{
		idLocker:   l.lockerWithCollector,
		resourceId: resourceId,
		unlock:     func() { mutex.Unlock() },
	}
}

func (l *locker[K]) lockInternal(resourceId K) (*sync.Mutex, bool) {
	mutex, loaded := l.locks.LoadOrCompute(resourceId, func() *sync.Mutex { return &sync.Mutex{} })
	if loaded { // another goroutine already uses this resource
		mutex.Lock() // wait for another goroutine to release lock
		mutex.Unlock()
		return nil, false // signal that lock is still not obtained and need to try again
	} else {
		mutex.Lock()
		return mutex, true
	}
}

type rwLocker[K comparable] struct {
	*lockerWithCollector[K, *sync.RWMutex]
}

func (l *rwLocker[K]) Lock(resourceId K) Lock {
	if l.settings.StatsEnabled {
		l.addToQueue(resourceId)
	}
	mutex, _ := l.locks.LoadOrCompute(resourceId, func() *sync.RWMutex { return &sync.RWMutex{} })
	mutex.Lock()
	if l.settings.StatsEnabled {
		l.updateStat(resourceId, true)
	}

	return &lock[K, *sync.RWMutex]{
		idLocker:   l.lockerWithCollector,
		resourceId: resourceId,
		unlock:     func() { mutex.Unlock() },
	}
}

func (l *rwLocker[K]) RLock(resourceId K) Lock {
	if l.settings.StatsEnabled {
		l.addToQueue(resourceId)
	}
	mutex, _ := l.locks.LoadOrCompute(resourceId, func() *sync.RWMutex { return &sync.RWMutex{} })
	mutex.RLock()
	if l.settings.StatsEnabled {
		l.updateStat(resourceId, true)
	}

	return &lock[K, *sync.RWMutex]{
		idLocker:   l.lockerWithCollector,
		resourceId: resourceId,
		unlock:     func() { mutex.RUnlock() },
	}
}
