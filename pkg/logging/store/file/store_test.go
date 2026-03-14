package filestore_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"q2/pkg/logging"
	filestore "q2/pkg/logging/store/file"
)

func TestStoreWriteReadReopenAndClear(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "logs.jsonl")
	store, err := filestore.NewStore(path)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	base := time.Date(2026, 3, 13, 10, 0, 0, 0, time.UTC)
	input := []logging.LogEntry{
		{ID: "1", Timestamp: base.Add(-2 * time.Hour), Level: logging.LevelInfo, Message: "booted"},
		{ID: "2", Timestamp: base.Add(-time.Hour), Level: logging.LevelWarn, Message: "cache slow"},
		{ID: "3", Timestamp: base, Level: logging.LevelWarn, Message: "cache warm"},
	}

	for _, entry := range input {
		if err := store.Write(entry); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}

	_ = store.Close()
	store, err = filestore.NewStore(path)
	if err != nil {
		t.Fatalf("reopen NewStore() error = %v", err)
	}

	filtered, err := store.Read(logging.LevelWarn, logging.LogFilter{
		Contains: "cache",
		Limit:    1,
	})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if len(filtered) != 1 || filtered[0].ID != "2" {
		t.Fatalf("Read() = %+v, want first warn entry", filtered)
	}

	if err := store.Clear(base.Add(-30 * time.Minute)); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}

	remaining, err := store.Read(logging.LevelAll, logging.LogFilter{})
	if err != nil {
		t.Fatalf("Read() after Clear error = %v", err)
	}
	if len(remaining) != 1 || remaining[0].ID != "3" {
		t.Fatalf("remaining = %+v, want only latest entry", remaining)
	}
}

func TestStoreFlushPersistsBufferedWrites(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "buffered.jsonl")
	store, err := filestore.NewStore(path)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	entry := logging.LogEntry{
		ID:        "buffered-1",
		Timestamp: time.Date(2026, 3, 14, 10, 0, 0, 0, time.UTC),
		Level:     logging.LevelInfo,
		Message:   "buffered write",
	}
	if err := store.Write(entry); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := store.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(data), "\"buffered write\"") {
		t.Fatalf("file content = %q, want buffered entry", string(data))
	}
}
