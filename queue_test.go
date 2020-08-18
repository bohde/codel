package codel

import (
	"sync"
	"testing"
	"time"

	"github.com/flyingmutant/rapid"
)

type queueMachine struct {
	q *priorityQueue // queue being tested
	n int            // maximum queue size
}

// Init is an action for initializing  a queueMachine instance.
func (m *queueMachine) Init(t *rapid.T) {
	n := rapid.IntRange(1, 3).Draw(t, "n").(int)
	q := newQueue(n)
	m.q = &q
	m.n = n
}

// Model of Push
func (m *queueMachine) Push(t *rapid.T) {
	r := prendezvouz{
		priority:     rapid.Int().Draw(t, "priority").(int),
		enqueuedTime: time.Now(),
		errChan:      make(chan error, 1),
	}

	m.q.Push(&r)
}

// Model of Remove
func (m *queueMachine) Remove(t *rapid.T) {
	if m.q.Empty() {
		t.Skip("empty")
	}

	r := (*m.q)[rapid.IntRange(0, m.q.Len()-1).Draw(t, "i").(int)]
	m.q.Remove(r)
}

// Model of Drop
func (m *queueMachine) Drop(t *rapid.T) {
	if m.q.Empty() {
		t.Skip("empty")
	}

	r := (*m.q)[rapid.IntRange(0, m.q.Len()-1).Draw(t, "i").(int)]
	r.Drop()
}

// Model of Signal
func (m *queueMachine) Pop(t *rapid.T) {
	if m.q.Empty() {
		t.Skip("empty")
	}

	r := m.q.Pop()
	r.Signal()
}

// validate that invariants hold
func (m *queueMachine) Check(t *rapid.T) {
	if m.q.Len() > m.q.Cap() {
		t.Fatalf("queue over capacity: %v vs expected %v", m.q.Len(), m.q.Cap())
	}

	for i, r := range *m.q {
		if r.index != i {
			t.Fatalf("illegal index: expected %d, got %+v ", i, r)
		}
	}

}

func TestPriorityQueue(t *testing.T) {
	t.Run("It should meet invariants", func(t *testing.T) {
		rapid.Check(t, rapid.Run(&queueMachine{}))
	})

	t.Run("It should not panic", func(t *testing.T) {
		q := newQueue(3)

		mu := sync.Mutex{}

		wg := sync.WaitGroup{}

		for i := 0; i < 4; i++ {
			wg.Add(1)
			priority := i
			go func() {
				defer wg.Done()
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("Failed with panic %+v", r)
					}
				}()

				for i := 0; i < 1000; i++ {
					r := prendezvouz{
						priority:     priority,
						enqueuedTime: time.Now(),
						errChan:      make(chan error, 1),
					}

					mu.Lock()
					pushed := q.Push(&r)
					mu.Unlock()

					if !pushed {
						continue
					}

					mu.Lock()
					if r.index >= 0 {
						q.Remove(&r)
					}
					mu.Unlock()
				}

			}()
		}

		wg.Wait()

	})

}
