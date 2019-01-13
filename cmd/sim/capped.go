package main

import (
	"errors"
	"sync"
)

type Capped struct {
	mu  sync.Mutex
	cur int64
	cap int64
}

func (s *Capped) Lock() error {
	s.mu.Lock()
	if s.cur >= s.cap {
		s.mu.Unlock()
		return errors.New("too many threads")
	}

	s.cur++
	s.mu.Unlock()

	return nil
}

func (s *Capped) Unlock() {
	s.mu.Lock()
	s.cur--
	s.mu.Unlock()
}
