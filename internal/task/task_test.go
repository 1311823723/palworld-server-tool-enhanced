package task

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSavSyncOnceSerializesConcurrentDecodes(t *testing.T) {
	var active int32
	var maxActive int32

	decode := func() error {
		now := atomic.AddInt32(&active, 1)
		defer atomic.AddInt32(&active, -1)
		for {
			current := atomic.LoadInt32(&maxActive)
			if now <= current || atomic.CompareAndSwapInt32(&maxActive, current, now) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
		return nil
	}

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			savSyncOnce(decode)
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&maxActive); got != 1 {
		t.Fatalf("max concurrent decodes = %d, want 1", got)
	}
}
