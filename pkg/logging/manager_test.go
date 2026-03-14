package logging_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"q2/pkg/logging"
	"q2/pkg/logging/store/memory"
)

func TestManagerWriteFlushAndRead(t *testing.T) {
	t.Parallel()

	store := memory.NewStore()
	manager := newTestManager(t, store)

	if err := manager.WriteLog("INFO", "service started"); err != nil {
		t.Fatalf("WriteLog() error = %v", err)
	}
	if err := manager.Flush(context.Background()); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	entries, err := manager.ReadLogs("INFO", logging.LogFilter{})
	if err != nil {
		t.Fatalf("ReadLogs() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("ReadLogs() len = %d, want 1", len(entries))
	}
	if entries[0].Source != "test-suite" {
		t.Fatalf("entry source = %q, want test-suite", entries[0].Source)
	}
}

func TestManagerRejectsInvalidLevel(t *testing.T) {
	t.Parallel()

	store := memory.NewStore()
	manager := newTestManager(t, store)

	if err := manager.WriteLog("TRACE", "nope"); !errors.Is(err, logging.ErrInvalidLevel) {
		t.Fatalf("WriteLog() error = %v, want %v", err, logging.ErrInvalidLevel)
	}
}

func TestManagerQueueFull(t *testing.T) {
	store := &blockingStore{release: make(chan struct{})}
	manager := newTestManager(t, store, logging.WithQueueSize(1))

	if err := manager.WriteLog("INFO", "first"); err != nil {
		t.Fatalf("first WriteLog() error = %v", err)
	}

	waitForCondition(t, time.Second, func() bool {
		return store.started.Load() == 1
	})

	if err := manager.WriteLog("INFO", "second"); err != nil {
		t.Fatalf("second WriteLog() error = %v", err)
	}
	if err := manager.WriteLog("INFO", "third"); !errors.Is(err, logging.ErrQueueFull) {
		t.Fatalf("third WriteLog() error = %v, want %v", err, logging.ErrQueueFull)
	}

	close(store.release)
	if err := manager.Flush(context.Background()); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
}

func TestManagerCloseRejectsFurtherWrites(t *testing.T) {
	t.Parallel()

	store := memory.NewStore()
	manager := newTestManager(t, store)

	if err := manager.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := manager.WriteLog("INFO", "later"); !errors.Is(err, logging.ErrManagerClosed) {
		t.Fatalf("WriteLog() after Close error = %v, want %v", err, logging.ErrManagerClosed)
	}
}

func TestManagerHandlerErrorDoesNotRollbackWrite(t *testing.T) {
	t.Parallel()

	store := memory.NewStore()
	errorCh := make(chan error, 1)
	manager := newTestManager(t, store, logging.WithErrorHandler(func(err error) {
		errorCh <- err
	}))

	var handled atomic.Int32
	manager.RegisterLogHandler(logging.HandlerFunc(func(entry logging.LogEntry) error {
		handled.Add(1)
		return nil
	}))
	manager.RegisterLogHandler(logging.HandlerFunc(func(entry logging.LogEntry) error {
		return fmt.Errorf("handler failed for %s", entry.ID)
	}))

	if err := manager.WriteLog("ERROR", "job failed"); err != nil {
		t.Fatalf("WriteLog() error = %v", err)
	}
	if err := manager.Flush(context.Background()); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	select {
	case err := <-errorCh:
		if err == nil {
			t.Fatal("error callback received nil error")
		}
	case <-time.After(time.Second):
		t.Fatal("expected handler error callback")
	}

	if handled.Load() != 1 {
		t.Fatalf("successful handler count = %d, want 1", handled.Load())
	}

	entries, err := manager.ReadLogs("ERROR", logging.LogFilter{})
	if err != nil {
		t.Fatalf("ReadLogs() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("ReadLogs() len = %d, want 1", len(entries))
	}
}

func TestManagerConcurrentWrites(t *testing.T) {
	t.Parallel()

	store := memory.NewStore()
	manager := newTestManager(t, store, logging.WithQueueSize(512))

	const total = 100
	var wg sync.WaitGroup
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			if err := manager.WriteLog("INFO", fmt.Sprintf("message-%d", index)); err != nil {
				t.Errorf("WriteLog() error = %v", err)
			}
		}(i)
	}
	wg.Wait()

	if err := manager.Flush(context.Background()); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	entries, err := manager.ReadLogs("INFO", logging.LogFilter{})
	if err != nil {
		t.Fatalf("ReadLogs() error = %v", err)
	}
	if len(entries) != total {
		t.Fatalf("ReadLogs() len = %d, want %d", len(entries), total)
	}
}

func TestManagerReadFormattedLogsDefaultsToTextFormatter(t *testing.T) {
	t.Parallel()

	store := memory.NewStore()
	manager := newTestManager(t, store)

	if err := manager.WriteLog("INFO", "service started"); err != nil {
		t.Fatalf("WriteLog() error = %v", err)
	}
	if err := manager.Flush(context.Background()); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	lines, err := manager.ReadFormattedLogs("INFO", logging.LogFilter{})
	if err != nil {
		t.Fatalf("ReadFormattedLogs() error = %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("ReadFormattedLogs() len = %d, want 1", len(lines))
	}
	if !strings.Contains(lines[0], " INFO service started") {
		t.Fatalf("formatted line = %q, want text formatter output", lines[0])
	}
}

func TestManagerSetFormatterToJSON(t *testing.T) {
	t.Parallel()

	store := memory.NewStore()
	manager := newTestManager(t, store)
	manager.SetFormatter(logging.JsonFormatter{})

	if err := manager.WriteLog("WARN", "cache slow"); err != nil {
		t.Fatalf("WriteLog() error = %v", err)
	}
	if err := manager.Flush(context.Background()); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	lines, err := manager.ReadFormattedLogs("WARN", logging.LogFilter{})
	if err != nil {
		t.Fatalf("ReadFormattedLogs() error = %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("ReadFormattedLogs() len = %d, want 1", len(lines))
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &payload); err != nil {
		t.Fatalf("json line unmarshal error = %v, line = %q", err, lines[0])
	}
	if payload["message"] != "cache slow" {
		t.Fatalf("payload[message] = %v, want cache slow", payload["message"])
	}
}

func TestManagerWriteLogWithAttrsClonesInput(t *testing.T) {
	t.Parallel()

	store := memory.NewStore()
	manager := newTestManager(t, store)

	attrs := map[string]any{
		"trace_id": "trace-1",
		"service":  "api",
	}
	if err := manager.WriteLogWithAttrs("INFO", "request finished", attrs); err != nil {
		t.Fatalf("WriteLogWithAttrs() error = %v", err)
	}

	attrs["trace_id"] = "mutated"

	if err := manager.Flush(context.Background()); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	entries, err := manager.ReadLogs("INFO", logging.LogFilter{})
	if err != nil {
		t.Fatalf("ReadLogs() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("ReadLogs() len = %d, want 1", len(entries))
	}
	if got := entries[0].Attrs["trace_id"]; got != "trace-1" {
		t.Fatalf("entry attrs trace_id = %v, want trace-1", got)
	}
}

func TestManagerFlushCallsStoreFlush(t *testing.T) {
	t.Parallel()

	store := &flushAwareStore{}
	manager := newTestManager(t, store)

	if err := manager.WriteLog("INFO", "service started"); err != nil {
		t.Fatalf("WriteLog() error = %v", err)
	}
	if err := manager.Flush(context.Background()); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	entries, err := manager.ReadLogs("INFO", logging.LogFilter{})
	if err != nil {
		t.Fatalf("ReadLogs() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("ReadLogs() len = %d, want 1", len(entries))
	}
	if got := store.flushCount.Load(); got == 0 {
		t.Fatalf("flush count = %d, want >= 1", got)
	}
}

type blockingStore struct {
	started atomic.Int32
	release chan struct{}
	mu      sync.Mutex
	entries []logging.LogEntry
}

func (s *blockingStore) Write(entry logging.LogEntry) error {
	s.started.Add(1)
	<-s.release

	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, entry)
	return nil
}

func (s *blockingStore) Read(level logging.Level, filter logging.LogFilter) ([]logging.LogEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]logging.LogEntry, 0, len(s.entries))
	for _, entry := range s.entries {
		if logging.MatchEntry(entry, level, filter) {
			out = append(out, entry)
		}
	}
	return out, nil
}

func (s *blockingStore) Clear(before time.Time) error {
	return nil
}

type flushAwareStore struct {
	mu         sync.Mutex
	pending    []logging.LogEntry
	committed  []logging.LogEntry
	flushCount atomic.Int32
}

func (s *flushAwareStore) Write(entry logging.LogEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending = append(s.pending, entry)
	return nil
}

func (s *flushAwareStore) Read(level logging.Level, filter logging.LogFilter) ([]logging.LogEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]logging.LogEntry, 0, len(s.committed))
	for _, entry := range s.committed {
		if logging.MatchEntry(entry, level, filter) {
			out = append(out, entry)
		}
	}
	return out, nil
}

func (s *flushAwareStore) Clear(before time.Time) error {
	return nil
}

func (s *flushAwareStore) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.pending) != 0 {
		s.committed = append(s.committed, s.pending...)
		s.pending = s.pending[:0]
	}
	s.flushCount.Add(1)
	return nil
}

func newTestManager(t *testing.T, store logging.LogStore, opts ...logging.Option) *logging.Manager {
	t.Helper()

	options := append([]logging.Option{
		logging.WithSource("test-suite"),
	}, opts...)

	manager, err := logging.NewManager(store, options...)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	t.Cleanup(func() {
		if err := manager.Close(context.Background()); err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("Close() error = %v", err)
		}
	})

	return manager
}

func waitForCondition(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("condition not met before timeout")
}
