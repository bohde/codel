package codel

import (
	"context"
	"sync"
	"testing"
	"time"
)

func Example() {
	c := New(Options{
		// The maximum number of pending acquires
		MaxPending: 100,
		// The maximum number of concurrent acquires
		MaxOutstanding: 10,
		// The target latency to wait for an acquire.
		// Acquires that take longer than this can fail.
		TargetLatency: 5 * time.Millisecond,
	})

	// This needs to be called in order to release resources.
	defer c.Close()

	err := c.Acquire(context.Background())
	if err != nil {
		return
	}

	defer c.Release()

}

func TestLock(t *testing.T) {
	c := New(Options{
		MaxPending:     1,
		MaxOutstanding: 1,
		TargetLatency:  5 * time.Millisecond,
	})
	defer c.Close()

	err := c.Acquire(context.Background())
	if err != nil {
		t.Error("Got an error:", err)
	}

	c.Release()
}

func TestAcquireFailsForCanceledContext(t *testing.T) {
	c := New(Options{
		MaxPending:     1,
		MaxOutstanding: 1,
		TargetLatency:  5 * time.Millisecond,
	})
	defer c.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := c.Acquire(ctx)
	if err == nil {
		t.Error("Expected an error:", err)
		c.Release()
	}

}

func TestLockCanHaveMultiple(t *testing.T) {
	const concurrent = 4

	c := New(Options{
		MaxPending:     1,
		MaxOutstanding: concurrent,
		TargetLatency:  5 * time.Millisecond,
	})
	defer c.Close()

	ctx := context.Background()

	for i := 0; i < concurrent; i++ {
		err := c.Acquire(ctx)
		if err != nil {
			t.Error("Got an error:", err)
			return
		}
	}

	for i := 0; i < concurrent; i++ {
		c.Release()
	}
}

func BenchmarkLock(b *testing.B) {
	c := New(Options{
		MaxPending:     1,
		MaxOutstanding: 1,
		TargetLatency:  5 * time.Millisecond,
	})
	defer c.Close()

	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		err := c.Acquire(ctx)

		if err != nil {
			b.Log("Got an error:", err)
			return
		}
		c.Release()
	}
	b.StopTimer()
}

func BenchmarkLockOverloaded(b *testing.B) {
	c := New(Options{
		MaxPending:     b.N,
		MaxOutstanding: 10,
		TargetLatency:  5 * time.Millisecond,
	})
	defer c.Close()

	wg := sync.WaitGroup{}
	wg.Add(b.N)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		go func() {
			defer wg.Done()
			err := c.Acquire(context.Background())

			if err != nil {
				return
			}
			c.Release()
		}()
	}
	wg.Wait()

	b.StopTimer()

}
