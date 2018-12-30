# codel

`codel` implements the [Controlled Delay](https://queue.acm.org/detail.cfm?id=2209336) algorithm as a lock. It implements overload detection based upon latency, while attempting to maximize throughput.

## Installation

`go get github.com/joshbohde/codel`

## Example

```
import (
    "context"
    "github.com/joshbohde/codel"
)

func Example() {
	c := codel.New(codel.Options{
		MaxPending:     100,
		MaxOutstanding: 10,
		TargetLatency:  5 * time.Millisecond,
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

The `Lock` does serialize access, introducing latency. If not overloaded, it generally presents <1us overhead.

```
BenchmarkLock-4                  2000000               740 ns/op              96 B/op          1 allocs/op
BenchmarkLockOverloaded-4        1000000              4256 ns/op             676 B/op          3 allocs/op
```
