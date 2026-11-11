// Package ratelimit counts failed attempts per key over a fixed window.
package ratelimit

import (
	"sync"
	"time"
)

// Limiter allows at most Max attempts per key in each Window. The window
// starts at a key's first counted attempt.
//
// Take reserves an attempt before the work starts, and Refund hands it back
// if the attempt shouldn't count. Reserving first stops parallel requests
// from all passing the check before any failure is recorded.
type Limiter struct {
	max    int
	window time.Duration
	now    func() time.Time

	mu        sync.Mutex
	entries   map[string]*entry
	lastSweep time.Time
}

type entry struct {
	count int
	reset time.Time
}

// New returns a limiter, or nil if max is 0 or less. A nil limiter allows
// everything.
func New(max int, window time.Duration) *Limiter {
	if max <= 0 || window <= 0 {
		return nil
	}
	return &Limiter{max: max, window: window, now: time.Now, entries: map[string]*entry{}}
}

// Take reserves an attempt for key. If the key has used its attempts, it
// returns false and how long until the window ends.
func (l *Limiter) Take(key string) (bool, time.Duration) {
	if l == nil {
		return true, 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.sweep(now)
	e, ok := l.entries[key]
	if !ok || !now.Before(e.reset) {
		e = &entry{reset: now.Add(l.window)}
		l.entries[key] = e
	}
	if e.count >= l.max {
		return false, e.reset.Sub(now)
	}
	e.count++
	return true, 0
}

// Refund returns an attempt reserved with Take, so it doesn't count.
func (l *Limiter) Refund(key string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if e, ok := l.entries[key]; ok && e.count > 0 {
		e.count--
	}
}

// sweep drops expired entries, at most once per window, so memory doesn't
// grow with every key ever seen.
func (l *Limiter) sweep(now time.Time) {
	if now.Sub(l.lastSweep) < l.window {
		return
	}
	for k, e := range l.entries {
		if !now.Before(e.reset) {
			delete(l.entries, k)
		}
	}
	l.lastSweep = now
}
