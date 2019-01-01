package codel

import (
	"context"
	"math/rand"
	"testing"
	"time"
)

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}

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

	// Attempt to acquire the lock.
	err := c.Acquire(context.Background())

	// if err is not nil, acquisition failed.
	if err != nil {
		return
	}

	// If acquisition succeeded, we need to release it.
	defer c.Release()

	// Do some process with external resources
}

func TestLock(t *testing.T) {
	c := New(Options{
		MaxPending:     1,
		MaxOutstanding: 1,
		TargetLatency:  5 * time.Millisecond,
	})

	err := c.Acquire(context.Background())
	if err != nil {
		t.Error("Got an error:", err)
	}

	c.Release()
}

func TestLockCanHaveMultiple(t *testing.T) {
	const concurrent = 4

	c := New(Options{
		MaxPending:     1,
		MaxOutstanding: concurrent,
		TargetLatency:  5 * time.Millisecond,
	})

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

func BenchmarkLockUnblocked(b *testing.B) {
	c := New(Options{
		MaxPending:     1,
		MaxOutstanding: 1,
		TargetLatency:  5 * time.Millisecond,
	})

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

func BenchmarkLockBlocked(b *testing.B) {
	const concurrent = 4

	c := New(Options{
		MaxPending:     1,
		MaxOutstanding: concurrent,
		TargetLatency:  5 * time.Millisecond,
	})

	ctx := context.Background()

	// Acquire maximum outstanding to avoid fast path
	for i := 0; i < concurrent; i++ {
		err := c.Acquire(ctx)
		if err != nil {
			b.Error("Got an error:", err)
			return
		}
	}

	b.ResetTimer()

	// Race the release and the acquire in order to benchmark slow path
	for i := 0; i < b.N; i++ {
		go func() {
			c.Release()

		}()
		err := c.Acquire(ctx)

		if err != nil {
			b.Log("Got an error:", err)
			return
		}
	}
}
