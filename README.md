# go-concurrency

[![codecov](https://codecov.io/gh/iglin/go-concurrency/branch/main/graph/badge.svg?token=T8JP58VM91)](https://codecov.io/gh/iglin/go-concurrency)

Concurrency utils for goroutines synchronization in golang.

# Usage

* [IdLocker](#idlocker)
* [IdRWLocker](#idrwlocker)
* [TypedSyncMap](#typedsyncmap)

## IdLocker

Below is an example of using IdLocker for synchronizing concurrent modifications of metrics in `resources` map.

See IdLocker API godoc for more information.

```go
package main

import (
	"fmt"
	"github.com/iglin/go-concurrency/idlock"
	"sync"
)

var (
	resources = map[string]int{
		"metric1": 1,
		"metric2": 1000,
	}

	locker = idlock.NewIdLocker[string]()
)

func incrementMetric(metricId string, waitGroup *sync.WaitGroup) {
	lock := locker.Lock(metricId)
	defer lock.Unlock()

	resources[metricId]++
	waitGroup.Done()
}

func main() {
	// wait group to wait all goroutines to finish
	waitGroup := sync.WaitGroup{}
	waitGroup.Add(3)

	// run concurrent increments for metric1
	go incrementMetric("metric1", &waitGroup)
	go incrementMetric("metric1", &waitGroup)
	go incrementMetric("metric1", &waitGroup)

	waitGroup.Wait()
	fmt.Println("metric1 value is ", resources["metric1"])
	// output: "metric1 value is 4
}
```


## IdRWLocker

Below is an example of using IdRWLocker for synchronizing concurrent writes and reads of metrics in `resources` map.

See IdRWLocker API godoc for more information. 

```go
package main

import (
	"fmt"
	"github.com/iglin/go-concurrency/idlock"
	"sync"
)

var (
	resources = map[string]int{
		"metric1": 1,
		"metric2": 1000,
	}

	ctx, cancel = context.WithCancel(context.Background())

	locker = idlock.NewIdRWLocker[string](idlock.LockerSettings{
		MaxSize:             10,
		StatsEnabled:        true,
		CollectorEnabled:    true,
		CollectorContext:    ctx,
		CollectorFirePeriod: 1 * time.Minute,
		LockMaxLifetime:     10 * time.Minute,
	})
)

func incrementMetric(metricId string, waitGroup *sync.WaitGroup) {
	lock := locker.Lock(metricId)
	defer lock.Unlock()

	resources[metricId]++
	waitGroup.Done()
}

func main() {
	// cancel old lock collector job context after all work is done
	cancel()

	// wait group to wait all goroutines to finish
	waitGroup := sync.WaitGroup{}
	waitGroup.Add(3)

	// run concurrent increments for metric1
	go incrementMetric("metric1", &waitGroup)
	go incrementMetric("metric1", &waitGroup)
	go incrementMetric("metric1", &waitGroup)

	waitGroup.Wait()
	fmt.Println("metric1 value is ", resources["metric1"])
	// output: "metric1 value is 4
}
```

## TypedSyncMap

TypedSyncMap has the same interface as golang sync.Map, but extends it with generic types for keys and values. 
Any type can be used as key or value type, including references. 

Below is an example of using TypedSyncMap as synchronized map of resources mapped by integer ids. 

See TypedSyncMap API godoc for more information. 

```go
package main

import (
	"fmt"
	"github.com/iglin/go-concurrency/smap"
)

type resource struct {
	name string
}

func main() {
	m := smap.NewTypedSyncMap[int, *resource]()

	m.Store(1, &resource{"test1"})

	res, deleted := m.LoadAndDelete(1)
	if deleted {
		fmt.Println("Resource deleted for id 1: ", *res)
	}
	// output: Resource deleted for id 1:  {test1}
```