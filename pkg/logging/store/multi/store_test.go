package multi_test

import (
	"errors"
	"testing"
	"time"

	"q2/pkg/logging"
	"q2/pkg/logging/store/memory"
	"q2/pkg/logging/store/multi"
)

func TestStoreWriteReadAndClear(t *testing.T) {
	t.Parallel()

	storeA := memory.NewStore()
	storeB := memory.NewStore()
	store, err := multi.NewStore([]logging.LogStore{storeA, storeB})
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	now := time.Now().UTC()
	entry := logging.LogEntry{
		ID:        "id-1",
		Timestamp: now,
		Level:     logging.LevelInfo,
		Message:   "service booted",
	}

	if err := store.Write(entry); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	entries, err := store.Read(logging.LevelInfo, logging.LogFilter{})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("Read() len = %d, want 1", len(entries))
	}

	if err := store.Clear(now.Add(time.Second)); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}

	afterClear, err := store.Read(logging.LevelAll, logging.LogFilter{})
	if err != nil {
		t.Fatalf("Read() after clear error = %v", err)
	}
	if len(afterClear) != 0 {
		t.Fatalf("Read() after clear len = %d, want 0", len(afterClear))
	}
}

func TestStoreReadAggregatesErrors(t *testing.T) {
	t.Parallel()

	storeA := memory.NewStore()
	store, err := multi.NewStore([]logging.LogStore{storeA, errStore{err: errors.New("boom")}})
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	writeErr := store.Write(logging.LogEntry{
		ID:        "id-1",
		Timestamp: time.Now().UTC(),
		Level:     logging.LevelInfo,
		Message:   "ok",
	})
	if writeErr == nil {
		t.Fatalf("Write() error should not be nil")
	}

	entries, readErr := store.Read(logging.LevelInfo, logging.LogFilter{})
	if len(entries) != 1 {
		t.Fatalf("Read() entries len = %d, want 1", len(entries))
	}
	if readErr == nil {
		t.Fatalf("Read() error should not be nil")
	}
}

type errStore struct {
	err error
}

func (s errStore) Write(entry logging.LogEntry) error {
	return s.err
}

func (s errStore) Read(level logging.Level, filter logging.LogFilter) ([]logging.LogEntry, error) {
	return nil, s.err
}

func (s errStore) Clear(before time.Time) error {
	return s.err
}
