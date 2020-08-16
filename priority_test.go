package codel

import (
	"context"
	"flag"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var sim = flag.Bool("sim", false, "run simulation test")

func msToWait(perSec int64) time.Duration {
	ms := rand.ExpFloat64() / (float64(perSec) / 1000)
	return time.Duration(ms * float64(time.Millisecond))
}

func TestPriority(t *testing.T) {
	t.Run("It should drop if zero enqueued", func(t *testing.T) {
		limiter := NewPriority(Options{
			MaxPending:     0,
			MaxOutstanding: 1,
			TargetLatency:  10 * time.Millisecond,
		})

		err := limiter.Acquire(context.Background(), 0)
		if err != nil {
			t.Errorf("Expected nil err: %s", err)
			return
		}

		err = limiter.Acquire(context.Background(), 0)
		if err == nil {
			t.Errorf("Expected non-nil err: %s", err)
		}

	})

}

// Simulate 3 priorities of 1000 reqs/second each, fighting for a
// process that can process 15 concurrent at 100 reqs/second. This
// should be enough capacity for the highest priority, The middle
// priority seeing partial unavailability, and the lowest priority
// seeing near full unavailability
func TestConcurrentSimulation(t *testing.T) {
	if !(*sim) {
		t.Log("Skipping sim since -sim not passed")
		t.Skip()
	}

	wg := sync.WaitGroup{}

	limiter := NewPriority(Options{
		MaxPending:     100,
		MaxOutstanding: 15,
		TargetLatency:  10 * time.Millisecond,
	})

	for i := 0; i < 3; i++ {
		wg.Add(1)

		priority := 10 * i
		go func() {
			defer wg.Done()

			inner := sync.WaitGroup{}

			success := int64(0)
			error := int64(0)

			for i := 0; i < 1000; i++ {
				inner.Add(1)
				time.Sleep(msToWait(1000))

				go func() {
					defer inner.Done()

					err := limiter.Acquire(context.Background(), priority)
					if err != nil {
						atomic.AddInt64(&error, 1)
						return
					}
					defer limiter.Release()

					time.Sleep(msToWait(100))
					atomic.AddInt64(&success, 1)

				}()
			}

			inner.Wait()
			t.Logf("priority=%d success=%f dropped=%f", priority, (float64(success) / 1000), float64(error)/1000)

		}()
	}

	wg.Wait()

}

func BenchmarkPLockUnblocked(b *testing.B) {
	c := NewPriority(Options{
		MaxPending:     1,
		MaxOutstanding: 1,
		TargetLatency:  5 * time.Millisecond,
	})

	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		err := c.Acquire(ctx, i)

		if err != nil {
			b.Log("Got an error:", err)
			return
		}
		c.Release()
	}
	b.StopTimer()
}

func BenchmarkPLockBlocked(b *testing.B) {
	const concurrent = 4

	c := NewPriority(Options{
		MaxPending:     1,
		MaxOutstanding: concurrent,
		TargetLatency:  5 * time.Millisecond,
	})

	ctx := context.Background()

	// Acquire maximum outstanding to avoid fast path
	for i := 0; i < concurrent; i++ {
		err := c.Acquire(ctx, i)
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
		err := c.Acquire(ctx, i)

		if err != nil {
			b.Log("Got an error:", err)
			return
		}
	}
}
