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
	"sync/atomic"
	"time"
)

// Dropped is the error that will be returned if this token is dropped
var Dropped = errors.New("dropped")

const (
	interval            = 10 * time.Millisecond
	outstandingInterval = 1 * time.Second
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
	MaxPending         int           // The maximum number of pending acquires
	InitialOutstanding int           // The initial number of concurrent acquires
	MaxOutstanding     int           // The maximum number of concurrent acquires
	TargetLatency      time.Duration // The target latency to wait for an acquire. Acquires that take longer than this can fail.
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

	outstanding           int64
	outstandingLimit      int64
	maxOutstandingLimit   int64
	nextOutstandingAdjust time.Time
}

func New(opts Options) *Lock {
	q := Lock{
		target:              opts.TargetLatency,
		outstandingLimit:    int64(opts.InitialOutstanding),
		maxOutstandingLimit: int64(opts.MaxOutstanding),
		maxPending:          int64(opts.MaxPending),
	}

	return &q
}

// Limit atomically returns the current limit of the lock
func (l *Lock) Limit() int64 {
	return atomic.LoadInt64(&l.outstandingLimit)
}

// Acquire a Lock with FIFO ordering, respecting the context. Returns an error it fails to acquire.
func (l *Lock) Acquire(ctx context.Context) error {
	l.mu.Lock()

	// Fast path if we are unblocked.
	if l.outstanding < l.outstandingLimit && l.waiters.Len() == 0 {
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

	now := time.Now()

	// If we have enqueued acquires, and can scale outstandingLimit, increase by one
	if l.outstandingLimit < l.maxOutstandingLimit && l.waiters.Len() > 0 && l.nextOutstandingAdjust.Before(now) {
		l.outstandingLimit++
	}

	keepGoing := true
	for keepGoing && l.outstanding < l.outstandingLimit {
		keepGoing = l.deque(now)
	}

	l.mu.Unlock()
}

// Backoff reduces the amount of concurrent aqcuires
func (l *Lock) Backoff() {
	l.mu.Lock()

	now := time.Now()

	if l.nextOutstandingAdjust.Before(now) {
		// scale down 0.7 times, rounding up, ensuring we scale down
		before := l.outstanding

		if before > 1 {
			l.outstandingLimit = ((before * 7) + 9) / 10
			if l.outstandingLimit >= before {
				l.outstandingLimit--
			}
		}

	}

	l.nextOutstandingAdjust = l.nextOutstandingAdjust.Add(outstandingInterval)

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
func (l *Lock) deque(now time.Time) bool {
	rendezvouz, ok, okToDrop := l.doDeque(now)

	// The queue has no entries, so return
	if !ok {
		return false
	}

	if !okToDrop {
		l.dropping = false
		l.outstanding++
		rendezvouz.Signal()
		return true
	}

	if l.dropping {
		for now.After(l.dropNext) && l.dropping {
			rendezvouz.Drop()
			rendezvouz, ok, okToDrop = l.doDeque(now)

			if !ok {
				return false
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
			return false
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
	return true
}
