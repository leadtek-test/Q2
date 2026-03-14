package memory_test

import (
	"testing"
	"time"

	"q2/pkg/logging"
	"q2/pkg/logging/store/memory"
)

func TestStoreReadWithFiltersAndClear(t *testing.T) {
	t.Parallel()

	store := memory.NewStore()
	base := time.Date(2026, 3, 13, 10, 0, 0, 0, time.UTC)

	entries := []logging.LogEntry{
		{ID: "1", Timestamp: base.Add(-2 * time.Hour), Level: logging.LevelInfo, Message: "booted"},
		{ID: "2", Timestamp: base.Add(-time.Hour), Level: logging.LevelWarn, Message: "slow cache"},
		{ID: "3", Timestamp: base, Level: logging.LevelError, Message: "job failed"},
	}
	for _, entry := range entries {
		if err := store.Write(entry); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}

	got, err := store.Read(logging.LevelAll, logging.LogFilter{
		Start:    base.Add(-90 * time.Minute),
		End:      base,
		Contains: "cache",
		Limit:    1,
	})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != "2" {
		t.Fatalf("Read() = %+v, want entry 2", got)
	}

	if err := store.Clear(base.Add(-30 * time.Minute)); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}

	remaining, err := store.Read(logging.LevelAll, logging.LogFilter{})
	if err != nil {
		t.Fatalf("Read() after Clear error = %v", err)
	}
	if len(remaining) != 1 || remaining[0].ID != "3" {
		t.Fatalf("remaining = %+v, want only entry 3", remaining)
	}
}
