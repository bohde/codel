package codel

import (
	"context"
	"math"
	"sync"
	"time"
)

const maxInt = int((^uint(0)) >> 1)

// prendezvouz is for returning context to the calling goroutine
type prendezvouz struct {
	priority     int
	index        int
	enqueuedTime time.Time
	errChan      chan error
}

func (r prendezvouz) Drop() {
	select {
	case r.errChan <- Dropped:
	default:
	}
}

func (r prendezvouz) Signal() {
	close(r.errChan)
}

// PLock implements a FIFO lock with concurrency control and priority, based upon the CoDel algorithm (https://queue.acm.org/detail.cfm?id=2209336).
type PLock struct {
	mu             sync.Mutex
	target         time.Duration
	firstAboveTime time.Time
	dropNext       time.Time

	droppedCount int64
	dropping     bool

	waiters    priorityQueue
	maxPending int64

	outstanding    int64
	maxOutstanding int64
}

func NewPriority(opts Options) *PLock {
	q := PLock{
		target:         opts.TargetLatency,
		maxOutstanding: int64(opts.MaxOutstanding),
		maxPending:     int64(opts.MaxPending),
		waiters:        newQueue(opts.MaxPending),
	}

	return &q
}

// Acquire a PLock with FIFO ordering, respecting the context. Returns an error it fails to acquire.
func (l *PLock) Acquire(ctx context.Context, priority int) error {
	l.mu.Lock()

	// Fast path if we are unblocked.
	if l.outstanding < l.maxOutstanding && l.waiters.Len() == 0 {
		l.outstanding++
		l.mu.Unlock()
		return nil
	}

	r := prendezvouz{
		priority:     priority,
		enqueuedTime: time.Now(),
		errChan:      make(chan error, 1),
	}

	pushed := l.waiters.Push(&r)

	if !pushed {
		l.externalDrop()
		l.mu.Unlock()
		return Dropped
	}

	l.mu.Unlock()

	select {

	case err := <-r.errChan:
		return err

	case <-ctx.Done():
		err := ctx.Err()

		l.mu.Lock()
		defer l.mu.Unlock()

		select {
		case err = <-r.errChan:
		default:
			l.waiters.Remove(&r)
			l.externalDrop()
		}

		return err
	}
}

// Release a previously acquired lock.
func (l *PLock) Release() {
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
func (l *PLock) controlLaw(t time.Time) time.Time {
	return t.Add(time.Duration(float64(interval) / math.Sqrt(float64(l.droppedCount))))
}

// Pull a single instance off the queue. This should be
func (l *PLock) doDeque(now time.Time) (r prendezvouz, ok bool, okToDrop bool) {
	if l.waiters.Empty() {
		return r, false, false
	}

	r = l.waiters.Pop()

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
func (l *PLock) externalDrop() {
	l.dropping = true
	l.droppedCount++
	l.dropNext = l.controlLaw(l.dropNext)
}

// Pull instances off the queue until we no longer drop
func (l *PLock) deque() {
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
