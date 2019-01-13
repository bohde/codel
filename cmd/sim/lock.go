package main

import (
	"context"
	"errors"
	"sync"

	"github.com/joshbohde/codel"
	"golang.org/x/sync/semaphore"
)

type Locker interface {
	Acquire(ctx context.Context) error
	Release()
	Limit() int64
	Backoff()
}

type Semaphore struct {
	mu    sync.Mutex
	cur   int64
	cap   int64
	limit int64
	sem   *semaphore.Weighted
}

func NewSemaphore(opts codel.Options) *Semaphore {
	s := semaphore.NewWeighted(int64(opts.MaxOutstanding))
	return &Semaphore{
		cap:   int64(opts.MaxPending) + int64(opts.MaxOutstanding),
		limit: int64(opts.MaxOutstanding),
		sem:   s,
	}
}

func (s *Semaphore) Limit() int64 {
	return s.limit
}

func (s *Semaphore) Backoff() {
}

func (s *Semaphore) Acquire(ctx context.Context) error {
	s.mu.Lock()
	// Drop if queue is full
	if s.cur >= s.cap {
		s.mu.Unlock()
		return errors.New("dropped")
	}

	s.cur++
	s.mu.Unlock()

	return s.sem.Acquire(ctx, 1)
}

func (s *Semaphore) Release() {
	s.mu.Lock()
	s.cur--
	s.mu.Unlock()

	s.sem.Release(1)
}
