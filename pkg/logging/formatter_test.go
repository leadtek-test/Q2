package logging_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"q2/pkg/logging"
)

func TestTextFormatter(t *testing.T) {
	t.Parallel()

	formatter := logging.TextFormatter{}
	entry := logging.LogEntry{
		Timestamp: time.Date(2026, 3, 14, 8, 0, 0, 0, time.UTC),
		Level:     logging.LevelInfo,
		Message:   "service started",
	}

	line, err := formatter.Format(entry)
	if err != nil {
		t.Fatalf("Format() error = %v", err)
	}
	if line != "2026-03-14T08:00:00Z INFO service started" {
		t.Fatalf("Format() = %q, want expected text format", line)
	}
}

func TestJsonFormatter(t *testing.T) {
	t.Parallel()

	formatter := logging.JsonFormatter{}
	entry := logging.LogEntry{
		ID:        "abc",
		Timestamp: time.Date(2026, 3, 14, 8, 0, 0, 0, time.UTC),
		Level:     logging.LevelError,
		Message:   "job failed",
		Source:    "unit-test",
	}

	line, err := formatter.Format(entry)
	if err != nil {
		t.Fatalf("Format() error = %v", err)
	}
	if !strings.Contains(line, "\"message\":\"job failed\"") {
		t.Fatalf("json line = %q, want message field", line)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(line), &payload); err != nil {
		t.Fatalf("json unmarshal error = %v", err)
	}
	if payload["source"] != "unit-test" {
		t.Fatalf("payload[source] = %v, want unit-test", payload["source"])
	}
}
