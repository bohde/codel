package main

import (
	"context"
	"flag"
	"log"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/joshbohde/codel"
	"github.com/joshbohde/codel/stats"
)

func msToWait(perSec int) time.Duration {
	ms := rand.ExpFloat64() / (float64(perSec) / 1000)
	return time.Duration(ms * float64(time.Millisecond))
}

func emit(perSec int, timeToRun time.Duration, action func()) int64 {
	start := time.Now()

	wg := sync.WaitGroup{}
	started := int64(0)

	for {
		time.Sleep(msToWait(perSec))

		if time.Now().Sub(start) > timeToRun {
			break
		}

		started++
		wg.Add(1)

		go func() {
			action()
			wg.Done()
		}()

	}
	return started
}

type fakeServer struct {
	mu     sync.Mutex
	perSec int
}

// Simulate a single threaded server
func (s *fakeServer) Process() {
	s.mu.Lock()
	time.Sleep(msToWait(s.perSec))
	s.mu.Unlock()
}

// Model input & output as random processes with average throughput.
func Simulate(method string, lock Locker, inputPerSec, outputPerSec int, timeToRun time.Duration) {
	stat := stats.New()
	server := fakeServer{perSec: outputPerSec}

	completed := uint64(0)

	started := emit(inputPerSec, timeToRun, func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)

		timer := stat.Time()

		err := lock.Acquire(ctx)
		cancel()

		if err == nil {
			server.Process()
			lock.Release()
			timer.Mark()
			atomic.AddUint64(&completed, 1)
		}
	})

	actualCompleted := atomic.LoadUint64(&completed)

	successPercentage := float64(actualCompleted) / float64(started)

	log.Printf("method=%s duration=%s input=%d output=%d throughput=%.2f completed=%.4f p50=%s p95=%s p99=%s ",
		method, timeToRun,
		inputPerSec, outputPerSec, float64(inputPerSec)*successPercentage, successPercentage,
		stat.Query(0.5), stat.Query(0.95), stat.Query(0.99))
}

func main() {
	runtime := flag.Duration("simulation-time", 5*time.Second, "Time to run each simulation")
	flag.Parse()

	wg := sync.WaitGroup{}

	opts := codel.Options{
		MaxPending:     1000,
		MaxOutstanding: 10,
		TargetLatency:  time.Millisecond,
	}

	run := func(in, out int) {
		wg.Add(2)
		go func() {
			Simulate("codel", codel.New(opts), in, out, *runtime)
			wg.Done()
		}()
		go func() {
			Simulate("queue", NewSemaphore(opts), in, out, *runtime)
			wg.Done()
		}()
	}

	run(1000, 2000)
	run(1000, 1000)
	run(1000, 900)
	run(1000, 750)
	run(1000, 500)
	run(1000, 250)
	run(1000, 100)

	wg.Wait()
}
