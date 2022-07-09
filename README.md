# go-concurrency

[![codecov](https://codecov.io/gh/iglin/go-concurrency/branch/main/graph/badge.svg?token=T8JP58VM91)](https://codecov.io/gh/iglin/go-concurrency)

Concurrency utils for goroutines synchronization in golang.

# Usage

Below is an example of using IdLocker for synchronizing concurrent modifications of metrics in `resources` map. 

```go
package main

import (
	"fmt"
	concurrency "github.com/iglin/go-concurrency"
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