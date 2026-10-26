package main

import (
	"context"
	"sync"
	"time"
)

type Event struct {
	Kind   string  `json:"kind"`
	URL    string  `json:"url,omitempty"`
	Title  string  `json:"title,omitempty"`
	Type   string  `json:"type,omitempty"`
	Action string  `json:"action,omitempty"`
	Value  float64 `json:"value,omitempty"`
}

type queue struct {
	events []Event
	wake   chan struct{}
}

type Queues struct {
	mu    sync.Mutex
	byDev map[string]*queue
	limit int
}

func NewQueues() *Queues {
	return &Queues{byDev: map[string]*queue{}, limit: 16}
}

func (q *Queues) get(id string) *queue {
	d, ok := q.byDev[id]
	if !ok {
		d = &queue{wake: make(chan struct{})}
		q.byDev[id] = d
	}
	return d
}

func (q *Queues) Push(id string, e Event) {
	q.mu.Lock()
	defer q.mu.Unlock()
	d := q.get(id)
	if e.Kind == "cast" {
		kept := d.events[:0]
		for _, old := range d.events {
			if old.Kind != "control" {
				kept = append(kept, old)
			}
		}
		d.events = kept
	}
	d.events = append(d.events, e)
	if len(d.events) > q.limit {
		d.events = d.events[len(d.events)-q.limit:]
	}
	close(d.wake)
	d.wake = make(chan struct{})
}

func (q *Queues) Next(ctx context.Context, id string, wait time.Duration) (Event, bool) {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	for {
		q.mu.Lock()
		d := q.get(id)
		if len(d.events) > 0 {
			e := d.events[0]
			d.events = d.events[1:]
			q.mu.Unlock()
			return e, true
		}
		wake := d.wake
		q.mu.Unlock()
		select {
		case <-wake:
		case <-timer.C:
			return Event{}, false
		case <-ctx.Done():
			return Event{}, false
		}
	}
}

func (q *Queues) Drop(id string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.byDev, id)
}
