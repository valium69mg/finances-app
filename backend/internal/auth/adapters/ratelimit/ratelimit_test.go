package ratelimit_test

import (
	"testing"
	"time"

	"github.com/valium69mg/finances-app/backend/internal/auth/adapters/ratelimit"
)

func TestAllow(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
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
