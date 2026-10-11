package backupstatus_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/valium69mg/finances-app/backend/internal/reminders/adapters/backupstatus"
)

func write(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "last-success")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLastSuccess(t *testing.T) {
	at, found, err := backupstatus.New(write(t, "2026-10-10T09:00:12Z\n")).LastSuccess(context.Background())
	if err != nil || !found {
		t.Fatalf("found = %v, err = %v", found, err)
	}
	if want := time.Date(2026, 10, 10, 9, 0, 12, 0, time.UTC); !at.Equal(want) {
		t.Errorf("at = %v, want %v", at, want)
	}
}

func TestMissingFileIsNotAnError(t *testing.T) {
	_, found, err := backupstatus.New(filepath.Join(t.TempDir(), "absent")).LastSuccess(context.Background())
	if err != nil || found {
		t.Fatalf("found = %v, err = %v, want not found and no error", found, err)
	}
}

func TestInvalidContentIsAnError(t *testing.T) {
	for name, content := range map[string]string{
		"empty":   "",
		"garbage": "yesterday",
		"huge":    strings.Repeat("2026-10-10T09:00:12Z", 100),
	} {
		t.Run(name, func(t *testing.T) {
			if _, found, err := backupstatus.New(write(t, content)).LastSuccess(context.Background()); err == nil || found {
				t.Fatalf("found = %v, err = %v, want an error", found, err)
			}
		})
	}
}

func TestUnreadablePathIsAnError(t *testing.T) {
	// A directory cannot be read as a file.
	if _, found, err := backupstatus.New(t.TempDir()).LastSuccess(context.Background()); err == nil || found {
		t.Fatalf("found = %v, err = %v, want an error", found, err)
	}
}
