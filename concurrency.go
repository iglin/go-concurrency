package go_concurrency

import (
	"fmt"
	"sync"
)

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
