package codel

import (
	"context"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joshbohde/codel/stats"
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

// Model input & output as random processes with average throughput.
func throughputTemplate(inputPerSec, outputPerSec int, timeout time.Duration) func(b *testing.B) {
	msToWait := func(perSec int) time.Duration {
		ms := rand.ExpFloat64() / (float64(perSec) / 1000)
		return time.Duration(ms * float64(time.Millisecond))
	}

	return func(b *testing.B) {

		lock := New(Options{
			MaxPending:     1000,
			MaxOutstanding: 10,
			TargetLatency:  time.Millisecond,
		})
		defer lock.Close()

		wg := sync.WaitGroup{}
		wg.Add(b.N)
		dropped := uint64(0)

		stat := stats.New()

		mutex := sync.Mutex{}

		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			time.Sleep(msToWait(inputPerSec))

			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), timeout)

				timer := stat.Time()

				err := lock.Acquire(ctx)
				cancel()

				if err != nil {
					atomic.AddUint64(&dropped, 1)
				} else {
					// Simulate a single threaded server
					mutex.Lock()
					time.Sleep(msToWait(outputPerSec))
					mutex.Unlock()

					lock.Release()
					timer.Mark()
				}
				wg.Done()
			}()
		}

		wg.Wait()
		stat.Close()

		b.StopTimer()
		b.Logf("n=%d input=%d output=%d dropped=%.4f p50=%s p95=%s p99=%s ", b.N,
			inputPerSec, outputPerSec,
			float64(dropped)/float64(b.N), stat.Query(0.5), stat.Query(0.95), stat.Query(0.99))

	}
}

func BenchmarkLockThroughput(b *testing.B) {
	b.Run("input=1000 output=2000", throughputTemplate(1000, 2000, 1*time.Second))
	b.Run("input=1000 output=1000", throughputTemplate(1000, 1000, 1*time.Second))
	b.Run("input=1000 output=900", throughputTemplate(1000, 900, 1*time.Second))
	b.Run("input=1000 output=750", throughputTemplate(1000, 750, 1*time.Second))
	b.Run("input=1000 output=500", throughputTemplate(1000, 500, 1*time.Second))
	b.Run("input=1000 output=250", throughputTemplate(1000, 250, 1*time.Second))
	b.Run("input=1000 output=100", throughputTemplate(1000, 100, 1*time.Second))
	b.Run("input=1000 output=100 timeout=100ms", throughputTemplate(1000, 100, 1*time.Millisecond))
}
