package core

import (
	"log"
	"sync"
)

// IncomingQueue is a useful tool to help managing incoming requests to a
// component
type IncomingQueue struct {
	sync.Mutex
	Capacity int
	queue    []Req
}

// Append adds a req to the queue
func (q *IncomingQueue) Append(req Req) {
	q.Lock()
	defer q.Unlock()

	if len(q.queue) >= q.Capacity {
		log.Panic("queue is full")
	}

	q.queue = append(q.queue, req)
}

// Peek returns the first element in the queue
func (q *IncomingQueue) Peek() Req {
	q.Lock()
	defer q.Unlock()

	return q.queue[0]
}

// Pop returns and removes the first element in the queue
func (q *IncomingQueue) Pop() Req {
	q.Lock()
	defer q.Unlock()

	req := q.queue[0]
	q.queue = q.queue[1:]
	return req
}

// IsFull checks if the queue is full
func (q *IncomingQueue) IsFull() bool {
	q.Lock()
	defer q.Unlock()

	return len(q.queue) >= q.Capacity
}

// Queue returns the slice that contains all the reqs
func (q *IncomingQueue) Queue() []Req {
	return q.queue
}

// SetQueue replaces the internal queue with the provided queue
func (q *IncomingQueue) SetQueue(queue []Req) {
	q.queue = queue
}

func NewIncomingQueue() *IncomingQueue {
	q := new(IncomingQueue)
	q.Capacity = 1024
	q.queue = make([]Req, 0)
	return q
}
