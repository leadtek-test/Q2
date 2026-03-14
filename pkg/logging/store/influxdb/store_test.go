package influxdb

import (
	"os"
	"testing"
	"time"

	"q2/pkg/logging"
)

func TestNormalizeConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   Config
		wantErr bool
	}{
		{
			name: "ok",
			input: Config{
				URL:    "http://localhost:8086",
				Token:  "token",
				Org:    "q2",
				Bucket: "logging",
			},
		},
		{name: "missing url", input: Config{Token: "token", Org: "q2", Bucket: "logging"}, wantErr: true},
		{name: "missing token", input: Config{URL: "http://localhost:8086", Org: "q2", Bucket: "logging"}, wantErr: true},
		{name: "missing org", input: Config{URL: "http://localhost:8086", Token: "token", Bucket: "logging"}, wantErr: true},
		{name: "missing bucket", input: Config{URL: "http://localhost:8086", Token: "token", Org: "q2"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeConfig(tt.input)
			if tt.wantErr && err == nil {
				t.Fatalf("normalizeConfig() expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("normalizeConfig() error = %v", err)
			}
			if !tt.wantErr && got.Measurement == "" {
				t.Fatalf("normalizeConfig() measurement should not be empty")
			}
		})
	}
}

func TestStoreIntegration(t *testing.T) {
	url := os.Getenv("INFLUX_TEST_URL")
	token := os.Getenv("INFLUX_TEST_TOKEN")
	org := os.Getenv("INFLUX_TEST_ORG")
	bucket := os.Getenv("INFLUX_TEST_BUCKET")
	if url == "" || token == "" || org == "" || bucket == "" {
		t.Skip("set INFLUX_TEST_URL/INFLUX_TEST_TOKEN/INFLUX_TEST_ORG/INFLUX_TEST_BUCKET to run integration test")
	}

	measurement := "logs_test_" + time.Now().UTC().Format("150405")
	store, err := NewStore(t.Context(), Config{
		URL:         url,
		Token:       token,
		Org:         org,
		Bucket:      bucket,
		Measurement: measurement,
	})
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	base := time.Now().UTC().Add(-time.Second)
	entry := logging.LogEntry{
		ID:        "influx-1",
		Timestamp: base,
		Level:     logging.LevelInfo,
		Message:   "service booted",
		Source:    "integration",
		Attrs: map[string]any{
			"trace_id": "trace-1",
		},
	}
	if err := store.Write(entry); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	var entries []logging.LogEntry
	deadline := time.Now().Add(5 * time.Second)
	for {
		entries, err = store.Read(logging.LevelInfo, logging.LogFilter{
			Start:    base.Add(-time.Second),
			End:      time.Now().UTC().Add(time.Second),
			Contains: "booted",
			Limit:    10,
		})
		if err != nil {
			t.Fatalf("Read() error = %v", err)
		}
		if len(entries) != 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	if len(entries) == 0 {
		t.Fatalf("Read() returned no entries")
	}

	if err := store.Clear(time.Now().UTC().Add(time.Second)); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}
}
