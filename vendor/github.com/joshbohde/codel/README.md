# codel

[![Build Status](https://travis-ci.org/joshbohde/codel.svg?branch=master)](https://travis-ci.org/joshbohde/codel)
[![GoDoc](https://godoc.org/github.com/joshbohde/codel?status.svg)](https://godoc.org/github.com/joshbohde/codel)

`codel` implements the [Controlled Delay](https://queue.acm.org/detail.cfm?id=2209336) algorithm for overload detection, providing a mechanism to shed load when overloaded. It optimizes for latency while keeping throughput high, even when downstream rates dynamically change.

`codel` keeps latency low when even severely overloaded, by preemptively shedding some load when wait latency is long. It is comparable to using a queue to handle bursts of load, but improves upon this technique by avoiding the latency required to handle all previous entries in the queue.

In a simulation of 1000 reqs/sec incoming, 500 reqs/sec outgoing averages for 10 seconds, here's the corresponding throughput and latency profile of both a queue and `codel`. Throughput is slightly higher than the average due to randomness in the simulation.

| method | throughput | p50          | p95          | p99          |
|--------|------------|--------------|--------------|--------------|
| queue  | 507.41     | 963.604953ms | 1.024595796s | 1.041455537s |
| codel  | 513.17     | 27.718827ms  | 44.085795ms  | 62.756499ms  |


Source code for the simulations are included in the [sim directory](cmd/sim).

## Installation

```
$ go get github.com/joshbohde/codel
```

## Example

```
import (
    "context"
    "github.com/joshbohde/codel"
)

func Example() {
	c := codel.New(codel.Options{
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

```

## Benchmarks

The `Lock` serializes access, introducing latency overhead. When not overloaded, this overhead should be under 1us.

```
BenchmarkLockUnblocked-4        20000000                73.1 ns/op             0 B/op          0 allocs/op
BenchmarkLockBlocked-4           2000000               665 ns/op             176 B/op          2 allocs/op
```
