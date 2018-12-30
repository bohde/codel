package codel

import (
	"context"
	"sync"
	"testing"
	"time"
)

func ExampleLock() {
	c := New(Options{
		maxPending:     100,
		maxOutstanding: 10,
		targetLatency:  5 * time.Millisecond,
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
		maxPending:     1,
		maxOutstanding: 1,
		targetLatency:  5 * time.Millisecond,
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
		maxPending:     1,
		maxOutstanding: 1,
		targetLatency:  5 * time.Millisecond,
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
		maxPending:     1,
		maxOutstanding: concurrent,
		targetLatency:  5 * time.Millisecond,
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
		maxPending:     1,
		maxOutstanding: 1,
		targetLatency:  5 * time.Millisecond,
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
		maxPending:     b.N,
		maxOutstanding: 10,
		targetLatency:  5 * time.Millisecond,
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
