package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/joshbohde/codel"
	"github.com/joshbohde/codel/stats"
)

func msToWait(perSec int64) time.Duration {
	ms := rand.ExpFloat64() / (float64(perSec) / 1000)
	return time.Duration(ms * float64(time.Millisecond))
}

type Simulation struct {
	Method       string
	TimeToRun    time.Duration
	InputPerSec  int64
	OutputPerSec int64
	Completed    uint64
	Rejected     uint64
	Started      int64
	Stats        stats.Stats
	Limit        int64
	mu           sync.Mutex
}

func (sim *Simulation) Process() {
	sim.mu.Lock()
	time.Sleep(msToWait(sim.OutputPerSec))
	sim.mu.Unlock()
}

func (sim *Simulation) String() string {
	successPercentage := float64(sim.Completed) / float64(sim.Started)
	rejectedPercentage := float64(sim.Rejected) / float64(sim.Started)

	return fmt.Sprintf("method=%s duration=%s input=%d output=%d limit=%d throughput=%.2f completed=%.4f rejected=%.4f p50=%s p95=%s p99=%s ",
		sim.Method, sim.TimeToRun,
		sim.InputPerSec, sim.OutputPerSec, sim.Limit,
		float64(sim.InputPerSec)*successPercentage, successPercentage, rejectedPercentage,
		sim.Stats.Query(0.5), sim.Stats.Query(0.95), sim.Stats.Query(0.99))

}

// Model input & output as random processes with average throughput.
func (sim *Simulation) Run(lock Locker) {
	start := time.Now()

	wg := sync.WaitGroup{}

	for {
		time.Sleep(msToWait(sim.InputPerSec))

		if time.Since(start) > sim.TimeToRun {
			break
		}

		sim.Started++
		wg.Add(1)

		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)

			timer := sim.Stats.Time()

			err := lock.Acquire(ctx)
			cancel()

			if err == nil {
				sim.Process()

				lock.Release()
				timer.Mark()
				atomic.AddUint64(&sim.Completed, 1)
			} else {
				atomic.AddUint64(&sim.Rejected, 1)
			}

			wg.Done()
		}()

	}

	sim.Limit = lock.Limit()
}

func Overloaded(runtime time.Duration, opts codel.Options) []*Simulation {
	wg := sync.WaitGroup{}

	runs := []*Simulation{}

	run := func(in, out int64) {
		wg.Add(2)

		codelRun := Simulation{
			Method:       "codel",
			InputPerSec:  in,
			OutputPerSec: out,
			TimeToRun:    runtime,
			Stats:        stats.New(),
		}

		queueRun := Simulation{
			Method:       "queue",
			InputPerSec:  in,
			OutputPerSec: out,
			TimeToRun:    runtime,
			Stats:        stats.New(),
		}

		runs = append(runs, &codelRun, &queueRun)

		go func() {
			codelRun.Run(codel.New(opts))
			wg.Done()
		}()
		go func() {
			queueRun.Run(NewSemaphore(opts))
			wg.Done()
		}()
	}

	run(1000, 1000)
	run(1000, 900)
	run(1000, 750)
	run(1000, 500)
	run(1000, 250)
	run(1000, 100)

	wg.Wait()
	return runs
}

func main() {
	runtime := *flag.Duration("simulation-time", 5*time.Second, "Time to run each simulation")
	targetLatency := *flag.Duration("target-latency", 5*time.Millisecond, "Time to run each simulation")

	flag.Parse()

	opts := codel.Options{
		MaxPending:         1000,
		InitialOutstanding: 10,
		MaxOutstanding:     10,
		TargetLatency:      targetLatency,
	}

	runs := Overloaded(runtime, opts)
	for _, r := range runs {
		log.Printf("%s", r.String())
	}

}
