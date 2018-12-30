package stats

import (
	"time"

	"github.com/bmizerany/perks/quantile"
)

type Stats struct {
	stream  *quantile.Stream
	samples chan float64
	done    chan struct{}
}

type Timer struct {
	start   time.Time
	samples chan float64
}

func (t Timer) Mark() {
	elapsed := time.Since(t.start)
	t.samples <- float64(elapsed.Nanoseconds())
}

func New() Stats {
	samples := make(chan float64, 10)
	stream := quantile.NewTargeted(0.5, 0.95, 0.99)
	done := make(chan struct{})

	go func() {
		for s := range samples {
			stream.Insert(s)
		}
		done <- struct{}{}
	}()

	return Stats{
		samples: samples,
		stream:  stream,
		done:    done,
	}
}

func (s Stats) Time() Timer {
	return Timer{
		start:   time.Now(),
		samples: s.samples,
	}
}

func (s Stats) Query(quantile float64) time.Duration {
	return time.Duration(s.stream.Query(quantile))
}

func (s Stats) Close() {
	close(s.samples)
	<-s.done
}
