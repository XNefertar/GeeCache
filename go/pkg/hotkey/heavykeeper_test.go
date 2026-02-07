package hotkey

import (
	"strconv"
	"sync"
	"testing"
)

func TestQueryTopKDataRace(t *testing.T) {
	hk := NewHeavyKeeper(100, 10, 0.9)

	var wg sync.WaitGroup

	// Writer Goroutine: Concurrently call CheckAndAdd
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			hk.CheckAndAdd("key" + strconv.Itoa(i))
		}
	}()

	// Reader Goroutine: Concurrently call QueryTopK
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			hk.QueryTopK(true)
		}
	}()

	wg.Wait()
}
