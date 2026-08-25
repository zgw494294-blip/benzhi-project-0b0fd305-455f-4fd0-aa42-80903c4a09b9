package application

import (
	"context"
	"sync"
)

type actorTask struct {
	ctx  context.Context
	fn   func() (MutationResult, error)
	done chan actorResult
}
type actorResult struct {
	value MutationResult
	err   error
}
type actor struct{ queue chan actorTask }
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
	a := &actor{queue: make(chan actorTask, r.capacity)}
	r.actors[id] = a
	go func() {
		for task := range a.queue {
			if err := task.ctx.Err(); err != nil {
				task.done <- actorResult{err: err}
				continue
			}
			v, err := task.fn()
			task.done <- actorResult{value: v, err: err}
		}
	}()
	return a
}
func (r *actorRegistry) do(ctx context.Context, id string, fn func() (MutationResult, error)) (MutationResult, error) {
	task := actorTask{ctx: ctx, fn: fn, done: make(chan actorResult, 1)}
	a := r.get(id)
	select {
	case a.queue <- task:
	case <-ctx.Done():
		return MutationResult{}, ctx.Err()
	}
	select {
	case result := <-task.done:
		return result.value, result.err
	case <-ctx.Done():
		return MutationResult{}, ctx.Err()
	}
}
