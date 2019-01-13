package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
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
	Deadline     time.Duration
	InputPerSec  int64
	OutputPerSec int64
	Completed    uint64
	Rejected     uint64
	Started      int64
	Stats        stats.Stats
	mu           sync.Mutex
}

func (sim *Simulation) Process() {
	sim.mu.Lock()
	time.Sleep(msToWait(sim.OutputPerSec))
	sim.mu.Unlock()
}

func (sim *Simulation) String() string {
	successPercentage := float64(atomic.LoadUint64(&sim.Completed)) / float64(sim.Started)
	rejectedPercentage := float64(atomic.LoadUint64(&sim.Rejected)) / float64(sim.Started)

	return fmt.Sprintf("method=%s duration=%s deadline=%s input=%d output=%d throughput=%.2f completed=%.4f rejected=%.4f p50=%s p95=%s p99=%s ",
		sim.Method, sim.TimeToRun, sim.Deadline,
		sim.InputPerSec, sim.OutputPerSec,
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
			ctx, cancel := context.WithTimeout(context.Background(), sim.Deadline)

			timer := sim.Stats.Time()

			err := lock.Acquire(ctx)
			cancel()

			if err == nil {
				sim.Process()

				timer.Mark()
				atomic.AddUint64(&sim.Completed, 1)
				lock.Release()

			} else {
				atomic.AddUint64(&sim.Rejected, 1)
			}

			wg.Done()
		}()

	}
}

func Overloaded(runtime time.Duration, deadline time.Duration, opts codel.Options) []*Simulation {
	wg := sync.WaitGroup{}

	runs := []*Simulation{}

	run := func(in, out int64) {
		wg.Add(2)

		codelRun := Simulation{
			Method:       "codel",
			Deadline:     deadline,
			InputPerSec:  in,
			OutputPerSec: out,
			TimeToRun:    runtime,
			Stats:        stats.New(),
		}

		queueRun := Simulation{
			Method:       "queue",
			Deadline:     deadline,
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
	run(990, 1000)
	run(950, 1000)
	run(900, 1000)

	run(1000, 900)
	run(1000, 750)
	run(1000, 500)
	run(1000, 250)
	run(1000, 100)

	wg.Wait()
	return runs
}

func main() {
	log.SetOutput(os.Stdout)

	runtime := flag.Duration("simulation-time", 5*time.Second, "Time to run each simulation")
	targetLatency := flag.Duration("target-latency", 5*time.Millisecond, "Target latency")
	deadline := flag.Duration("deadline", 1*time.Second, "Hard deadline to remain in the queue")

	flag.Parse()

	opts := codel.Options{
		MaxPending:     1000,
		MaxOutstanding: 10,
		TargetLatency:  *targetLatency,
	}

	runs := Overloaded(*runtime, *deadline, opts)
	for _, r := range runs {
		log.Printf("%s", r.String())
	}

}
