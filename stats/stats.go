package stats

import (
	"sync"
	"time"

	"github.com/bmizerany/perks/quantile"
)

type Stats struct {
	stream quantile.Stream
	lock   sync.Mutex
}

type Timer struct {
	start time.Time
	stats *Stats
}

func (t *Timer) Mark() {
	elapsed := time.Since(t.start)
	t.stats.Insert(float64(elapsed.Nanoseconds()))
}

func New() Stats {
	stream := quantile.NewTargeted(0.5, 0.95, 0.99)

	return Stats{
		stream: *stream,
	}
}

func (s *Stats) Insert(val float64) {
	s.lock.Lock()
	s.stream.Insert(val)
	s.lock.Unlock()
}

func (s *Stats) Time() Timer {
	return Timer{
		start: time.Now(),
		stats: s,
	}
}

func (s *Stats) Query(quantile float64) time.Duration {
	s.lock.Lock()
	val := s.stream.Query(quantile)
	s.lock.Unlock()
	return time.Duration(val)
}
