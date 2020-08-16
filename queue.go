package codel

import (
	"container/heap"
)

type queue []*prendezvouz

func (pq queue) Len() int { return len(pq) }

func (pq queue) Less(i, j int) bool {
	// We want Pop to give us the highest, not lowest, priority so we use greater than here.
	return pq[i].priority > pq[j].priority
}

func (pq queue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *queue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*prendezvouz)
	item.index = n
	*pq = append(*pq, item)
}

func (pq *queue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	item.index = -1 // for safety
	*pq = old[0 : n-1]
	return item
}

func (pq *queue) lowestIndex() int {
	old := *pq
	n := len(old)
	index := n / 2

	lowestIndex := index
	priority := maxInt

	for i := index; i < n; i++ {
		if old[i].priority < priority {
			lowestIndex = i
			priority = old[i].priority
		}
	}

	return lowestIndex
}

type priorityQueue queue

func newQueue(capacity int) priorityQueue {
	return priorityQueue(make([]*prendezvouz, 0, capacity))
}

func (pq *priorityQueue) Len() int {
	return len(*pq)
}

func (pq *priorityQueue) Cap() int {
	return cap(*pq)
}

func (pq *priorityQueue) push(r *prendezvouz) {
	heap.Push((*queue)(pq), r)
}

func (pq *priorityQueue) Push(r *prendezvouz) bool {
	// If we're under capacity, push it to the queue
	if pq.Len() < pq.Cap() {
		pq.push(r)
		return true
	}

	if pq.Cap() == 0 {
		return false
	}

	// otherwise, we need to check if this takes priority over the lowest element
	lowestIndex := ((*queue)(pq)).lowestIndex()
	last := (*pq)[lowestIndex]
	if last.priority < r.priority {
		(*pq)[lowestIndex] = r
		r.index = lowestIndex
		heap.Fix((*queue)(pq), lowestIndex)

		last.Drop()

		return true
	}

	return false

}

func (pq *priorityQueue) Empty() bool {
	return len(*pq) <= 0
}

func (pq *priorityQueue) Pop() prendezvouz {
	r := heap.Pop((*queue)(pq)).(*prendezvouz)
	return *r
}

func (pq *priorityQueue) Remove(r *prendezvouz) {
	heap.Remove((*queue)(pq), r.index)
}
