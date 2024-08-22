package idlock

import (
	"context"
	"fmt"
	"github.com/puzpuzpuz/xsync/v3"
	"sync"
	"time"
)

func prepareSettings(cachingEnabledByDefault bool, userSettings ...LockerSettings) LockerSettings {
	settings := LockerSettings{}
	if len(userSettings) > 0 {
		settings = userSettings[0]
	} else {
		settings.CacheLocks = cachingEnabledByDefault
	}

	if settings.CacheLocks {
		// configure stats collection
		settings.StatsEnabled = settings.StatsEnabled || settings.CollectorEnabled || settings.MaxSize > 0

		// configure old locks collector
		if settings.CollectorEnabled {
			if settings.CollectorContext == nil {
				settings.CollectorContext = context.Background()
			}
			if settings.CollectorFirePeriod == 0 {
				settings.CollectorFirePeriod = DefaultCollectorFirePeriod
			}
		}
	}

	return settings
}

type lockerWithCollector[K comparable, V sync.Locker] struct {
	locks    *xsync.MapOf[K, V]
	stats    *xsync.MapOf[K, *stat]
	settings LockerSettings
}

type stat struct {
	mutex    sync.RWMutex
	held     bool
	queue    int
	lastUsed time.Time
}

func newLockerWithCollector[K comparable, V sync.Locker](cachingEnabledByDefault bool, settings ...LockerSettings) *lockerWithCollector[K, V] {
	locker := lockerWithCollector[K, V]{
		locks:    xsync.NewMapOf[K, V](),
		settings: prepareSettings(cachingEnabledByDefault, settings...),
	}
	if locker.settings.CacheLocks {
		if locker.settings.StatsEnabled {
			locker.stats = xsync.NewMapOf[K, *stat]()
		}
		if locker.settings.CollectorEnabled {
			go locker.runCollector()
		}
	}
	return &locker
}

func (l *lockerWithCollector[K, V]) runCollector() {
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

func (l *lockerWithCollector[K, V]) collectOldLocks() {
	now := time.Now()
	threshold := l.settings.LockMaxLifetime
	l.stats.Range(func(resourceId K, stat *stat) bool {
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

func (l *lockerWithCollector[K, V]) removeOldest() bool {
	oldestTime := time.Now()
	var oldestResourceId K
	found := false
	l.stats.Range(func(resourceId K, stat *stat) bool {
		stat.mutex.RLock()

		if !stat.held && stat.lastUsed.Before(oldestTime) {
			found = true
			oldestResourceId = resourceId
			oldestTime = stat.lastUsed
		}
		stat.mutex.RUnlock()
		return true
	})
	if found {
		l.removeLock(oldestResourceId)
		return true
	}
	return false
}

func (l *lockerWithCollector[K, V]) removeLock(resourceId K) {
	if mutex, ok := l.locks.Load(resourceId); ok {
		mutex.Lock()
		defer mutex.Unlock()

		l.removeStat(resourceId)
		l.locks.Delete(resourceId)
	}
}

func (l *lockerWithCollector[K, V]) removeStat(resourceId K) {
	if stat, ok := l.stats.Load(resourceId); ok {
		stat.mutex.Lock()
		defer stat.mutex.Unlock()
		l.stats.Delete(resourceId)
	}
}

func (l *lockerWithCollector[K, V]) addToQueue(resourceId K) {
	stat, loaded := l.stats.LoadOrStore(resourceId, &stat{
		held:     false,
		queue:    1,
		lastUsed: time.Now(),
	})
	if loaded {
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

func (l *lockerWithCollector[K, V]) updateStat(resourceId K, held bool) {
	stat, loaded := l.stats.LoadOrStore(resourceId, &stat{
		held:     held,
		queue:    0,
		lastUsed: time.Now(),
	})
	if loaded {
		stat.mutex.Lock()
		stat.held = held
		if held {
			stat.queue--
		}
		stat.lastUsed = time.Now()
		stat.mutex.Unlock()
	}
}

func (l *lockerWithCollector[K, V]) GetCacheSize() int {
	return l.locks.Size()
}

func (l *lockerWithCollector[K, V]) GetStats() map[K]Stat {
	result := make(map[K]Stat)
	if l.settings.StatsEnabled {
		l.stats.Range(func(resourceId K, statInternal *stat) bool {
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

func (l *lockerWithCollector[T, V]) Delete(resourceId T) {
	if !l.settings.CacheLocks {
		return
	}
	l.removeLock(resourceId)
}

type lock[T comparable, V sync.Locker] struct {
	idLocker        *lockerWithCollector[T, V]
	resourceId      T
	alreadyUnlocked bool
	unlock          func()
}

func (l *lock[K, V]) Unlock() {
	if l.alreadyUnlocked {
		panic(fmt.Errorf("idlock: lock with id %v was already released", l.resourceId))
	}

	defer l.unlock()

	l.alreadyUnlocked = true

	if l.idLocker.settings.CacheLocks {
		if l.idLocker.settings.StatsEnabled {
			l.idLocker.updateStat(l.resourceId, false)
		}
	} else {
		l.idLocker.locks.Delete(l.resourceId)
	}
}
