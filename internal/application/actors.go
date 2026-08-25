package application

import (
	"context"
	"sync"
)

type actorTask struct {
	ctx context.Context
	fn  func() (MutationResult, error)
}
type actorResult struct {
	value MutationResult
	err   error
}
type actor struct {
	queue   chan actorTask
	results chan actorResult
}
type actorRegistry struct {
	mu       sync.Mutex
	capacity int
	actors   map[string]*actor
}

func newActorRegistry(capacity int) *actorRegistry {
	if capacity < 1 {
		capacity = 32
	}
	return &actorRegistry{capacity: capacity, actors: map[string]*actor{}}
}
func (r *actorRegistry) get(id string) *actor {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a := r.actors[id]; a != nil {
		return a
	}
	a := &actor{queue: make(chan actorTask, r.capacity), results: make(chan actorResult)}
	r.actors[id] = a
	go func() {
		for task := range a.queue {
			if err := task.ctx.Err(); err != nil {
				a.results <- actorResult{err: err}
				continue
			}
			v, err := task.fn()
			a.results <- actorResult{value: v, err: err}
		}
	}()
	return a
}
func (r *actorRegistry) do(ctx context.Context, id string, fn func() (MutationResult, error)) (MutationResult, error) {
	task := actorTask{ctx: ctx, fn: fn}
	a := r.get(id)
	select {
	case a.queue <- task:
	case <-ctx.Done():
		return MutationResult{}, ctx.Err()
	}
	select {
	case result := <-a.results:
		return result.value, result.err
	case <-ctx.Done():
		return MutationResult{}, ctx.Err()
	}
}
