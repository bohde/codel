// Package codel implements the Controlled Delay
// (https://queue.acm.org/detail.cfm?id=2209336) algorithm for
// overload detection, providing a mechanism to shed load when
// overloaded. It optimizes for latency while keeping throughput high,
// even when downstream rates dynamically change.
// It keeps latency low when even severely overloaded, by preemptively
// shedding some load when wait latency is long. It is comparable to
// using a queue to handle bursts of load, but improves upon this
// technique by avoiding the latency required to handle all previous
// entries in the queue.
package codel

import (
	"container/list"
	"context"
	"errors"
	"math"
	"sync"
	"time"
)

// Dropped is the error that will be returned if this token is dropped
var Dropped = errors.New("dropped")

const (
	interval = 10 * time.Millisecond
)

// rendezvouz is for returning context to the calling goroutine
type rendezvouz struct {
	enqueuedTime time.Time
	errChan      chan error
}

func (r rendezvouz) Drop() {
	select {
	case r.errChan <- Dropped:
	default:
	}
}

func (r rendezvouz) Signal() {
	close(r.errChan)
}

// Options are options to configure a Lock.
type Options struct {
	MaxPending     int           // The maximum number of pending acquires
	MaxOutstanding int           // The maximum number of concurrent acquires
	TargetLatency  time.Duration // The target latency to wait for an acquire. Acquires that take longer than this can fail.
}

// Lock implements a FIFO lock with concurrency control, based upon the CoDel algorithm (https://queue.acm.org/detail.cfm?id=2209336).
type Lock struct {
	mu             sync.Mutex
	target         time.Duration
	firstAboveTime time.Time
	dropNext       time.Time

	droppedCount int64
	dropping     bool

	waiters    list.List
	maxPending int64

	outstanding    int64
	maxOutstanding int64
}

func New(opts Options) *Lock {
	q := Lock{
		target:         opts.TargetLatency,
		maxOutstanding: int64(opts.MaxOutstanding),
		maxPending:     int64(opts.MaxPending),
	}

	return &q
}

// Acquire a Lock with FIFO ordering, respecting the context. Returns an error it fails to acquire.
func (l *Lock) Acquire(ctx context.Context) error {
	l.mu.Lock()

	// Fast path if we are unblocked.
	if l.outstanding < l.maxOutstanding && l.waiters.Len() == 0 {
		l.outstanding++
		l.mu.Unlock()
		return nil
	}

	// If our queue is full, drop
	if int64(l.waiters.Len()) == l.maxPending {
		l.externalDrop()
		l.mu.Unlock()
		return Dropped
	}

	r := rendezvouz{
		enqueuedTime: time.Now(),
		errChan:      make(chan error),
	}

	elem := l.waiters.PushBack(r)
	l.mu.Unlock()

	select {

	case err := <-r.errChan:
		return err

	case <-ctx.Done():
		err := ctx.Err()

		l.mu.Lock()

		select {
		case err = <-r.errChan:
		default:
			l.waiters.Remove(elem)
			l.externalDrop()
		}

		l.mu.Unlock()

		return err
	}
}

// Release a previously acquired lock.
func (l *Lock) Release() {
	l.mu.Lock()
	l.outstanding--
	if l.outstanding < 0 {
		l.mu.Unlock()
		panic("lock: bad release")
	}

	l.deque()

	l.mu.Unlock()

}

// Adjust the time based upon interval / sqrt(droppedCount)
func (l *Lock) controlLaw(t time.Time) time.Time {
	return t.Add(time.Duration(float64(interval) / math.Sqrt(float64(l.droppedCount))))
}

// Pull a single instance off the queue. This should be
func (l *Lock) doDeque(now time.Time) (r rendezvouz, ok bool, okToDrop bool) {
	next := l.waiters.Front()

	if next == nil {
		return rendezvouz{}, false, false
	}

	l.waiters.Remove(next)

	r = next.Value.(rendezvouz)

	sojurnDuration := now.Sub(r.enqueuedTime)

	if sojurnDuration < l.target || l.waiters.Len() == 0 {
		l.firstAboveTime = time.Time{}
	} else if (l.firstAboveTime == time.Time{}) {
		l.firstAboveTime = now.Add(interval)
	} else if now.After(l.firstAboveTime) {
		okToDrop = true
	}

	return r, true, okToDrop

}

// Signal that we couldn't write to the queue
func (l *Lock) externalDrop() {
	l.dropping = true
	l.droppedCount++
	l.dropNext = l.controlLaw(l.dropNext)
}

// Pull instances off the queue until we no longer drop
func (l *Lock) deque() {
	now := time.Now()

	rendezvouz, ok, okToDrop := l.doDeque(now)

	// The queue has no entries, so return
	if !ok {
		return
	}

	if !okToDrop {
		l.dropping = false
		l.outstanding++
		rendezvouz.Signal()
		return
	}

	if l.dropping {
		for now.After(l.dropNext) && l.dropping {
			rendezvouz.Drop()
			rendezvouz, ok, okToDrop = l.doDeque(now)

			if !ok {
				return
			}

			l.droppedCount++

			if !okToDrop {
				l.dropping = false
			} else {
				l.dropNext = l.controlLaw(l.dropNext)
			}
		}
	} else if now.Sub(l.dropNext) < interval || now.Sub(l.firstAboveTime) >= interval {
		rendezvouz.Drop()
		rendezvouz, ok, _ = l.doDeque(now)

		if !ok {
			return
		}

		l.dropping = true

		if l.droppedCount > 2 {
			l.droppedCount -= 2
		} else {
			l.droppedCount = 1
		}

		l.dropNext = l.controlLaw(now)
	}

	l.outstanding++
	rendezvouz.Signal()
}
