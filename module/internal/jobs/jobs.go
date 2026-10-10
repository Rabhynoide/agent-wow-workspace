// Package jobs runs one asynchronous job at a time and exposes its protobuf status.
package jobs

import (
	"context"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
)

// Job is a running background task with a mutable status message.
type Job[T proto.Message] struct {
	Ctx    context.Context
	Start  time.Time
	cancel context.CancelFunc
	done   chan struct{}
	mu     sync.Mutex
	st     T
}

// Update mutates the status under the job lock.
func (j *Job[T]) Update(f func(T)) {
	j.mu.Lock()
	f(j.st)
	j.mu.Unlock()
}

// Snap returns a copy of the status.
func (j *Job[T]) Snap() T {
	j.mu.Lock()
	defer j.mu.Unlock()
	return proto.Clone(j.st).(T)
}

func (j *Job[T]) Done() <-chan struct{} { return j.done }

// Runner owns the current job of a module. Start and Stop are serialized, so two
// concurrent starts can never leave an orphan job running.
type Runner[T proto.Message] struct {
	ctl  sync.Mutex // serializes Start/Stop
	mu   sync.Mutex // guards cur/last
	cur  *Job[T]
	last T
	// Decorate adjusts a running job's snapshot (e.g. elapsed time); may be nil.
	Decorate func(j *Job[T], st T)
}

func NewRunner[T proto.Message](idle T) *Runner[T] { return &Runner[T]{last: idle} }

// Lock holds the start/stop lock; callers that inspect and then start a job use it with StartLocked.
func (r *Runner[T]) Lock()   { r.ctl.Lock() }
func (r *Runner[T]) Unlock() { r.ctl.Unlock() }

// Cur returns the running job or nil.
func (r *Runner[T]) Cur() *Job[T] {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cur
}

// Start stops the current job and runs body in the background with the initial status.
// body must leave the final status in the job (via Update) before returning.
func (r *Runner[T]) Start(initial T, body func(j *Job[T])) *Job[T] {
	r.ctl.Lock()
	defer r.ctl.Unlock()
	return r.StartLocked(initial, body)
}

// StartLocked is Start for callers already holding Lock.
func (r *Runner[T]) StartLocked(initial T, body func(j *Job[T])) *Job[T] {
	r.stopLocked()
	ctx, cancel := context.WithCancel(context.Background())
	j := &Job[T]{Ctx: ctx, Start: time.Now(), cancel: cancel, done: make(chan struct{}), st: initial}
	r.mu.Lock()
	r.cur = j
	r.mu.Unlock()
	go func() {
		defer cancel()
		body(j)
		st := j.Snap()
		r.mu.Lock()
		r.last, r.cur = st, nil
		r.mu.Unlock()
		close(j.done)
	}()
	return j
}

// Stop cancels the current job and waits for it to finish.
func (r *Runner[T]) Stop() {
	r.ctl.Lock()
	defer r.ctl.Unlock()
	r.stopLocked()
}

func (r *Runner[T]) stopLocked() {
	if j := r.Cur(); j != nil {
		j.cancel()
		<-j.done
	}
}

// Current returns the running job's status, or the last finished one.
func (r *Runner[T]) Current() T {
	r.mu.Lock()
	j, last := r.cur, r.last
	r.mu.Unlock()
	if j == nil {
		return proto.Clone(last).(T)
	}
	st := j.Snap()
	if r.Decorate != nil {
		r.Decorate(j, st)
	}
	return st
}

// Wait blocks until the current job ends, waitS seconds pass (def when 0, capped at max) or ctx ends.
func (r *Runner[T]) Wait(ctx context.Context, waitS, def, max uint32) T {
	if j := r.Cur(); j != nil {
		if waitS == 0 {
			waitS = def
		}
		select {
		case <-j.done:
		case <-time.After(time.Duration(min(waitS, max)) * time.Second):
		case <-ctx.Done():
		}
	}
	return r.Current()
}

// Sleep waits d or until ctx ends; it reports whether the full duration elapsed.
func Sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
