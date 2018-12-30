# codel

[![Build Status](https://travis-ci.org/joshbohde/codel.svg?branch=master)](https://travis-ci.org/joshbohde/codel)
[![GoDoc](https://godoc.org/github.com/joshbohde/codel?status.svg)](https://godoc.org/github.com/joshbohde/codel)

`codel` implements the [Controlled Delay](https://queue.acm.org/detail.cfm?id=2209336) algorithm for overload detection, providing a mechanism to shed load when overloaded. It optimizes for throughput, even when downstream rates dynamically change, while keeping delays low when not overloaded.

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
	// This needs to be called in order to release resources.
	defer c.Close()

	// Attempt to acquire the lock.
	err := c.Acquire(context.Background())

	// if err is not nil, acquisition failed.
	if err != nil {
		return
	}

	// If acquisition succeeded, we need to release it.
	defer c.Release()
}

```

## Benchmarks

The `Lock` serializes access, introducing latency overhead. When not overloaded, this overhead should be under 1us.

```
BenchmarkLock-4                  2000000               740 ns/op              96 B/op          1 allocs/op
BenchmarkLockOverloaded-4        1000000              4256 ns/op             676 B/op          3 allocs/op
```
