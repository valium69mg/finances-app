package ratelimit_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/valium69mg/finances-app/backend/internal/auth/adapters/ratelimit"
)

var start = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func TestAllow(t *testing.T) {
	now := start
	l := ratelimit.New(func() time.Time { return now })
	const limit, span = 2, time.Minute

	steps := []struct {
		name    string
		advance time.Duration
		key     string
		want    bool
	}{
		{"first", 0, "a", true},
		{"second", time.Second, "a", true},
		{"third is blocked", time.Second, "a", false},
		{"other key unaffected", 0, "b", true},
		{"still blocked inside window", 30 * time.Second, "a", false},
		{"new window resets", 30 * time.Second, "a", true},
	}
	for _, s := range steps {
		now = now.Add(s.advance)
		if got := l.Allow(s.key, limit, span); got != s.want {
			t.Fatalf("%s: got %v, want %v", s.name, got, s.want)
		}
	}
}

func TestAllowRejectedEventsAreNotCounted(t *testing.T) {
	now := start
	l := ratelimit.New(func() time.Time { return now })
	l.Allow("k", 1, time.Minute)
	for range 10 {
		if l.Allow("k", 1, time.Minute) {
			t.Fatal("must stay blocked")
		}
	}
	now = now.Add(time.Minute)
	if !l.Allow("k", 1, time.Minute) {
		t.Fatal("window must reset regardless of rejected attempts")
	}
}

func TestAllowWindowBoundary(t *testing.T) {
	now := start
	l := ratelimit.New(func() time.Time { return now })
	l.Allow("k", 1, time.Minute)
	now = start.Add(time.Minute - time.Nanosecond)
	if l.Allow("k", 1, time.Minute) {
		t.Fatal("still inside the window")
	}
	now = start.Add(time.Minute)
	if !l.Allow("k", 1, time.Minute) {
		t.Fatal("window must be over exactly at start+span")
	}
}

func TestPeekDoesNotRecord(t *testing.T) {
	now := start
	l := ratelimit.New(func() time.Time { return now })
	for range 100 {
		if !l.Peek("k", 1, time.Minute) {
			t.Fatal("peek must not consume the budget")
		}
	}
	if !l.Allow("k", 1, time.Minute) {
		t.Fatal("allow after peeks must succeed")
	}
	if l.Peek("k", 1, time.Minute) {
		t.Fatal("peek must report an exhausted budget")
	}
}

func TestPeekZeroLimit(t *testing.T) {
	l := ratelimit.New(func() time.Time { return start })
	if l.Peek("k", 0, time.Minute) {
		t.Fatal("zero limit never fits")
	}
}

func TestRecordAndPeek(t *testing.T) {
	now := start
	l := ratelimit.New(func() time.Time { return now })
	const limit, span = 3, time.Minute

	for i := 1; i <= limit; i++ {
		if !l.Peek("k", limit, span) {
			t.Fatalf("peek before record %d must pass", i)
		}
		l.Record("k", span)
	}
	if l.Peek("k", limit, span) {
		t.Fatal("budget must be exhausted after limit records")
	}
	l.Record("k", span) // over the limit stays exhausted
	if l.Peek("k", limit, span) {
		t.Fatal("extra record must not reopen the budget")
	}
	now = now.Add(span)
	if !l.Peek("k", limit, span) {
		t.Fatal("budget must reset after the window")
	}
}

func TestRecordStartsFreshWindowAfterExpiry(t *testing.T) {
	now := start
	l := ratelimit.New(func() time.Time { return now })
	l.Record("k", time.Minute)
	now = now.Add(2 * time.Minute)
	l.Record("k", time.Minute)
	if !l.Peek("k", 2, time.Minute) {
		t.Fatal("only the record in the current window counts")
	}
	l.Record("k", time.Minute)
	if l.Peek("k", 2, time.Minute) {
		t.Fatal("two records in the current window exhaust a limit of 2")
	}
}

func TestKeysAreIsolatedAcrossMethods(t *testing.T) {
	l := ratelimit.New(func() time.Time { return start })
	l.Record("a", time.Minute)
	l.Record("a", time.Minute)
	if l.Peek("a", 2, time.Minute) {
		t.Fatal("a must be exhausted")
	}
	if !l.Peek("b", 2, time.Minute) || !l.Allow("b", 2, time.Minute) {
		t.Fatal("b must be unaffected by a")
	}
}

func TestAllowAndRecordShareCounters(t *testing.T) {
	l := ratelimit.New(func() time.Time { return start })
	l.Record("k", time.Minute)
	if !l.Allow("k", 2, time.Minute) {
		t.Fatal("second event fits")
	}
	if l.Allow("k", 2, time.Minute) {
		t.Fatal("third event must not fit")
	}
}

// A long window must survive the pruning triggered by calls with short windows.
func TestPruneKeepsLongerWindows(t *testing.T) {
	now := start
	l := ratelimit.New(func() time.Time { return now })
	l.Allow("hour", 1, time.Hour)
	l.Allow("short", 1, time.Minute)

	now = now.Add(5 * time.Minute)
	l.Allow("trigger", 1, time.Minute) // triggers a prune with a short window
	if l.Allow("hour", 1, time.Hour) {
		t.Fatal("hourly window was pruned by a short-window call")
	}
	if !l.Allow("short", 1, time.Minute) {
		t.Fatal("expired short window must have reset")
	}
}

func TestConcurrentAllowNeverExceedsLimit(t *testing.T) {
	l := ratelimit.New(func() time.Time { return start })
	const limit, workers = 50, 200

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		allowed int
	)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if l.Allow("shared", limit, time.Hour) {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if allowed != limit {
		t.Fatalf("allowed = %d, want exactly %d", allowed, limit)
	}
}

func TestConcurrentMixedOperationsAreRaceFree(t *testing.T) {
	now := start
	var nowMu sync.Mutex
	l := ratelimit.New(func() time.Time {
		nowMu.Lock()
		defer nowMu.Unlock()
		now = now.Add(time.Second)
		return now
	})

	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			key := fmt.Sprintf("k%d", i%5)
			l.Peek(key, 3, time.Minute)
			l.Record(key, time.Minute)
			l.Allow(key, 3, time.Minute)
		}()
	}
	wg.Wait()
}

func TestConcurrentRecordsAreAllCounted(t *testing.T) {
	l := ratelimit.New(func() time.Time { return start })
	const n = 64
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l.Record("k", time.Hour)
		}()
	}
	wg.Wait()
	if !l.Peek("k", n+1, time.Hour) {
		t.Fatalf("limit %d must still have room after %d records", n+1, n)
	}
	if l.Peek("k", n, time.Hour) {
		t.Fatalf("limit %d must be exhausted after %d records", n, n)
	}
}
