// Package codel implements the Controlled Delay algorithm
// (https://queue.acm.org/detail.cfm?id=2209336) for overload
// detection, providing a mechanism to shed load when overloaded. It
// optimizes for throughput, even when downstream rates dynamically
// change, while keeping delays low when not overloaded.
package codel

import (
	"context"
	"errors"
	"time"
)

// Dropped is the error that will be returned if this token is dropped
var Dropped = errors.New("dropped")

const (
	interval = 100 * time.Millisecond
)

// rendezvouz is for returning context to the calling goroutine
type rendezvouz struct {
	enqueuedTime time.Time
	errChan      chan error
	ctx          context.Context
}

func (r rendezvouz) Drop() {
	r.errChan <- Dropped
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
	target         time.Duration
	firstAboveTime time.Time
	dropNext       time.Time
	count          uint
	dropping       bool
	incoming       chan rendezvouz
	outstanding    chan struct{}
	done           chan struct{}
}

func New(opts Options) *Lock {
	q := Lock{
		target:         opts.TargetLatency,
		firstAboveTime: time.Time{},
		dropNext:       time.Time{},
		count:          0,
		dropping:       false,
		incoming:       make(chan rendezvouz, opts.MaxPending),
		outstanding:    make(chan struct{}, opts.MaxOutstanding),
		done:           make(chan struct{}),
	}

	for i := 0; i < opts.MaxOutstanding; i++ {
		q.outstanding <- struct{}{}
	}

	go func() {
		ok := true
		for ok {
			ok = q.step()
		}

		q.drain()
		q.done <- struct{}{}

	}()

	return &q
}

// Acquire a Lock with FIFO ordering, respecting the context. Returns an error it fails to acquire.
func (l *Lock) Acquire(ctx context.Context) error {
	r := rendezvouz{
		enqueuedTime: time.Now(),
		errChan:      make(chan error),
		ctx:          ctx,
	}

	select {
	case l.incoming <- r:
	case <-ctx.Done():
		return ctx.Err()
	}

	select {
	case err := <-r.errChan:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Release a previously acquired lock.
func (l *Lock) Release() {
	l.outstanding <- struct{}{}
}

// Close the lock and wait for the background job to finish.
func (l *Lock) Close() {
	close(l.incoming)
	<-l.done
}

// Adjust the time based upon interval / sqrt(count)
func (l *Lock) controlLaw(t time.Time) time.Time {
	return t.Add(interval / time.Duration(sqrt(l.count)))
}

// Pull a single instance off the queue
func (l *Lock) doDeque(now time.Time) (r rendezvouz, ok bool, okToDrop bool) {
	r, ok = <-l.incoming
	sojurnDuration := now.Sub(r.enqueuedTime)

	if sojurnDuration < l.target {
		l.firstAboveTime = time.Time{}
	} else if (l.firstAboveTime == time.Time{}) {
		l.firstAboveTime = now.Add(interval)
	} else if now.After(l.firstAboveTime) {
		okToDrop = true
	}

	return

}

// Pull instances off the queue until we no longer drop
func (l *Lock) deque() (rendezvouz rendezvouz, ok bool) {
	now := time.Now()

	rendezvouz, ok, okToDrop := l.doDeque(now)

	// The queue has no more entries, so return
	if !ok {
		return
	}

	if !okToDrop {
		l.dropping = false
		return
	}

	if l.dropping {
		for now.After(l.dropNext) && l.dropping {
			l.count++
			rendezvouz.Drop()
			rendezvouz, ok, okToDrop = l.doDeque(now)

			if !ok {
				return
			}

			if !okToDrop {
				l.dropping = false
			} else {
				l.dropNext = l.controlLaw(l.dropNext)
			}
		}
	} else if okToDrop && now.Sub(l.dropNext) < interval || now.Sub(l.firstAboveTime) >= interval {
		rendezvouz.Drop()
		rendezvouz, ok, _ = l.doDeque(now)

		if !ok {
			return
		}

		l.dropping = true

		if l.count > 2 {
			l.count -= 2
		} else {
			l.count = 1
		}

		l.dropNext = l.controlLaw(now)
	}

	return
}

// Signal a single rendezvouz
func (l *Lock) step() (ok bool) {
	// grab a lock
	<-l.outstanding
	for {
		r, ok := l.deque()
		if !ok {
			return ok
		}
		select {
		case r.errChan <- nil:
			// from here the acquirer needs to release
			return ok
		case <-r.ctx.Done():
			// otherwise, the acquirer is canceled
		}
	}
}

// Drain the oustanding queue
func (l *Lock) drain() {
	// we need receive cap - 1, since we receive one for the closed incoming
	for i := 0; i < cap(l.outstanding)-1; i++ {
		<-l.outstanding
	}
}
