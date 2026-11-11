package ratelimit

import (
	"sync"
	"testing"
	"time"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newTest(max int, window time.Duration) (*Limiter, *clock) {
	c := &clock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	l := New(max, window)
	l.now = c.now
	return l, c
}

func TestTakeLimitsAndResets(t *testing.T) {
	l, c := newTest(3, time.Minute)
	for i := 0; i < 3; i++ {
		if ok, _ := l.Take("alice"); !ok {
			t.Fatalf("attempt %d refused, want allowed", i+1)
		}
	}
	c.advance(20 * time.Second)
	ok, wait := l.Take("alice")
	if ok {
		t.Fatal("4th attempt allowed, want refused")
	}
	if wait != 40*time.Second {
		t.Errorf("retry after = %v, want 40s", wait)
	}
	if ok, _ := l.Take("bob"); !ok {
		t.Error("other key refused, want allowed")
	}

	c.advance(40 * time.Second)
	if ok, _ := l.Take("alice"); !ok {
		t.Error("attempt after the window refused, want allowed")
	}
}

func TestRefund(t *testing.T) {
	l, _ := newTest(1, time.Minute)
	l.Take("alice")
	l.Refund("alice")
	if ok, _ := l.Take("alice"); !ok {
		t.Error("refunded attempt still counted")
	}
	l.Refund("nobody") // must not panic or create an entry
	if _, ok := l.entries["nobody"]; ok {
		t.Error("refund created an entry")
	}
}

func TestNilLimiterAllowsAll(t *testing.T) {
	l := New(0, time.Minute)
	if l != nil {
		t.Fatal("New(0, ...) should return nil")
	}
	for i := 0; i < 100; i++ {
		if ok, _ := l.Take("alice"); !ok {
			t.Fatal("nil limiter refused an attempt")
		}
	}
	l.Refund("alice")
}

func TestSweepDropsExpired(t *testing.T) {
	l, c := newTest(1, time.Minute)
	l.Take("alice")
	l.Take("bob")
	c.advance(2 * time.Minute)
	l.Take("carol")
	if len(l.entries) != 1 {
		t.Errorf("entries = %d after sweep, want 1", len(l.entries))
	}
}

func TestConcurrentTakes(t *testing.T) {
	l := New(5, time.Minute)
	var wg sync.WaitGroup
	var mu sync.Mutex
	allowed := 0
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ok, _ := l.Take("alice"); ok {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if allowed != 5 {
		t.Errorf("allowed = %d of 50 parallel attempts, want 5", allowed)
	}
}
