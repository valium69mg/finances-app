// Package backupstatus reads the timestamp of the last successful database
// backup. The backup job on the VM writes it to a file that the API container
// mounts read-only: line 1 is the RFC 3339 timestamp, an optional line 2 is
// "size_bytes=N" (the dump size). Older files hold only the timestamp.
package backupstatus

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/valium69mg/finances-app/backend/internal/reminders/app"
)

// maxFileSize bounds the read: the file holds a timestamp and a size.
const maxFileSize = 256

const sizeKey = "size_bytes="

// Reader reads the status file at a path.
type Reader struct{ path string }

// New builds a Reader for the file at path.
func New(path string) *Reader { return &Reader{path: path} }

// LastSuccess returns when the last backup succeeded. found is false when the
// file does not exist (no backup was ever recorded). A file that cannot be read
// or whose first line is not an RFC 3339 timestamp is an error.
func (r *Reader) LastSuccess(ctx context.Context) (time.Time, bool, error) {
	b, found, err := r.LastBackup(ctx)
	return b.At, found, err
}

// LastBackup is LastSuccess plus the dump size when the file carries it. A
// missing or malformed size line is ignored: only the timestamp is required.
func (r *Reader) LastBackup(context.Context) (app.BackupInfo, bool, error) {
	f, err := os.Open(r.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return app.BackupInfo{}, false, nil
		}
		return app.BackupInfo{}, false, fmt.Errorf("open backup status: %w", err)
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, maxFileSize))
	if err != nil {
		return app.BackupInfo{}, false, fmt.Errorf("read backup status: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	at, err := time.Parse(time.RFC3339, strings.TrimSpace(lines[0]))
	if err != nil {
		return app.BackupInfo{}, false, errors.New("backup status is not an RFC 3339 timestamp")
	}
	b := app.BackupInfo{At: at}
	for _, line := range lines[1:] {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), sizeKey); ok {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
				b.SizeBytes = n
			}
		}
	}
	return b, true, nil
}
