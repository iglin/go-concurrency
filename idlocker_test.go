package concurrency

import (
	"github.com/stretchr/testify/assert"
	"sync"
	"testing"
)

const scale = 1000

func TestIdLocker(t *testing.T) {
	locker := NewIdLocker()

	resourceId := 1
	sharedCounter := 0

	wg := sync.WaitGroup{}
	wg.Add(scale)
	for i := 0; i < scale; i++ {
		go func() {
			locker.Lock(resourceId)
			defer locker.Unlock(resourceId)
			sharedCounter++
			wg.Done()
		}()
	}
	wg.Wait()
	assert.Equal(t, scale, sharedCounter)
}
