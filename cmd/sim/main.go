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

// Model input & output as random processes with average throughput.
func HTTPServerSim(inputPerSec, outputPerSec int, timeout time.Duration, timeToRun time.Duration) {
	start := time.Now()

	msToWait := func(perSec int) time.Duration {
		ms := rand.ExpFloat64() / (float64(perSec) / 1000)
		return time.Duration(ms * float64(time.Millisecond))
	}

	lock := codel.New(codel.Options{
		MaxPending:     1000,
		MaxOutstanding: 10,
		TargetLatency:  time.Millisecond,
	})
	defer lock.Close()

	wg := sync.WaitGroup{}
	started := uint64(0)
	dropped := uint64(0)

	stat := stats.New()

	mutex := sync.Mutex{}

	for {
		if time.Now().Sub(start) > timeToRun {
			break
		}

		started++

		time.Sleep(msToWait(inputPerSec))
		wg.Add(1)

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

	log.Printf("duration=%s input=%d output=%d timeout=%s dropped=%.4f p50=%s p95=%s p99=%s ", timeToRun,
		inputPerSec, outputPerSec, timeout,
		float64(dropped)/float64(started), stat.Query(0.5), stat.Query(0.95), stat.Query(0.99))
}

func main() {
	runtime := flag.Duration("simulation-time", 5*time.Second, "Time to run each simulation")
	flag.Parse()

	wg := sync.WaitGroup{}

	run := func(in, out int, timeout time.Duration) {
		wg.Add(1)
		go func() {
			HTTPServerSim(in, out, timeout, *runtime)
			wg.Done()
		}()
	}

	run(1000, 2000, 1*time.Second)
	run(1000, 1000, 1*time.Second)
	run(1000, 900, 1*time.Second)
	run(1000, 750, 1*time.Second)
	run(1000, 500, 1*time.Second)
	run(1000, 250, 1*time.Second)
	run(1000, 100, 1*time.Second)
	run(1000, 100, 100*time.Millisecond)

	wg.Wait()
}
