# go-concurrency

[![codecov](https://codecov.io/gh/iglin/go-concurrency/branch/main/graph/badge.svg?token=T8JP58VM91)](https://codecov.io/gh/iglin/go-concurrency)

Concurrency utils for goroutines synchronization in golang.

# Usage

## IdLocker

Below is an example of using IdLocker for synchronizing concurrent modifications of metrics in `resources` map.

See IdLocker API godoc for more information.

```go
package main

import (
	"fmt"
	"github.com/iglin/go-concurrency"
	"sync"
)

var (
	resources = map[string]int{
		"metric1": 1,
		"metric2": 1000,
	}

	locker = concurrency.NewIdLocker()
)

func incrementMetric(metricId string, waitGroup *sync.WaitGroup) {
	locker.Lock(metricId)
	defer locker.Unlock(metricId)

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
	"github.com/iglin/go-concurrency"
	"sync"
)

var (
	resources = map[string]int{
		"metric1": 1,
		"metric2": 1000,
	}

	ctx, cancel = context.WithCancel(context.Background())

	locker = concurrency.NewIdRWLocker(concurrency.LockerSettings{
		MaxSize:             10,
		StatsEnabled:        true,
		CollectorEnabled:    true,
		CollectorContext:    ctx,
		CollectorFirePeriod: 1 * time.Minute,
		LockMaxLifetime:     10 * time.Minute,
	})
)

func incrementMetric(metricId string, waitGroup *sync.WaitGroup) {
	locker.Lock(metricId)
	defer locker.Unlock(metricId)

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