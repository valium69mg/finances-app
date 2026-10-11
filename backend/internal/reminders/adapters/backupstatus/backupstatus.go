// Package backupstatus reads the timestamp of the last successful database
// backup. The backup job on the VM writes it (RFC 3339, one line) to a file
// that the API container mounts read-only.
package backupstatus

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
	"time"
)

// maxFileSize bounds the read: the file holds one timestamp.
const maxFileSize = 256

// Reader reads the status file at a path.
type Reader struct{ path string }

// New builds a Reader for the file at path.
func New(path string) *Reader { return &Reader{path: path} }

// LastSuccess returns when the last backup succeeded. found is false when the
// file does not exist (no backup was ever recorded). A file that cannot be read
// or does not hold an RFC 3339 timestamp is an error.
func (r *Reader) LastSuccess(context.Context) (time.Time, bool, error) {
	f, err := os.Open(r.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return time.Time{}, false, nil
		}
		return time.Time{}, false, fmt.Errorf("open backup status: %w", err)
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, maxFileSize))
	if err != nil {
		return time.Time{}, false, fmt.Errorf("read backup status: %w", err)
	}
	at, err := time.Parse(time.RFC3339, strings.TrimSpace(string(raw)))
	if err != nil {
		return time.Time{}, false, errors.New("backup status is not an RFC 3339 timestamp")
	}
	return at, true, nil
}
