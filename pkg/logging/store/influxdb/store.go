package influxdb

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	api "github.com/influxdata/influxdb-client-go/v2/api"

	"q2/pkg/logging"
)

const defaultMeasurement = "logs"
const requestTimeout = 5 * time.Second

type Config struct {
	URL         string
	Token       string
	Org         string
	Bucket      string
	Measurement string
}

type Store struct {
	client      influxdb2.Client
	writeAPI    api.WriteAPIBlocking
	queryAPI    api.QueryAPI
	deleteAPI   api.DeleteAPI
	org         string
	bucket      string
	measurement string
}

func NewStore(ctx context.Context, cfg Config) (*Store, error) {
	normalized, err := normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}

	client := influxdb2.NewClient(normalized.URL, normalized.Token)
	health, err := client.Health(ctx)
	if err != nil {
		client.Close()
		return nil, err
	}
	if health != nil && strings.ToLower(string(health.Status)) != "pass" {
		client.Close()
		return nil, fmt.Errorf("influxdb health is %q", health.Status)
	}

	return &Store{
		client:      client,
		writeAPI:    client.WriteAPIBlocking(normalized.Org, normalized.Bucket),
		queryAPI:    client.QueryAPI(normalized.Org),
		deleteAPI:   client.DeleteAPI(),
		org:         normalized.Org,
		bucket:      normalized.Bucket,
		measurement: normalized.Measurement,
	}, nil
}

func (s *Store) Write(entry logging.LogEntry) error {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	tags := map[string]string{
		"level":  entry.Level.String(),
		"source": entry.Source,
	}

	fields := map[string]any{
		"id":      entry.ID,
		"message": entry.Message,
	}

	if len(entry.Attrs) != 0 {
		payload, err := json.Marshal(entry.Attrs)
		if err != nil {
			return err
		}
		fields["attrs"] = string(payload)
	}

	point := influxdb2.NewPoint(s.measurement, tags, fields, entry.Timestamp)
	return s.writeAPI.WritePoint(ctx, point)
}

func (s *Store) Read(level logging.Level, filter logging.LogFilter) ([]logging.LogEntry, error) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	start := fluxTime(filter.Start, "time(v: 0)")
	stop := fluxTime(filter.End, "now()")

	var query strings.Builder
	query.WriteString("import \"strings\"\n")
	query.WriteString(fmt.Sprintf("from(bucket: %s)", fluxString(s.bucket)))
	query.WriteString(fmt.Sprintf(" |> range(start: %s, stop: %s)", start, stop))
	query.WriteString(fmt.Sprintf(" |> filter(fn: (r) => r._measurement == %s)", fluxString(s.measurement)))
	query.WriteString(" |> filter(fn: (r) => r._field == \"id\" or r._field == \"message\" or r._field == \"attrs\")")
	if level != logging.LevelAll {
		query.WriteString(fmt.Sprintf(" |> filter(fn: (r) => r.level == %s)", fluxString(level.String())))
	}
	query.WriteString(" |> pivot(rowKey:[\"_time\",\"level\",\"source\"], columnKey:[\"_field\"], valueColumn:\"_value\")")
	if filter.Contains != "" {
		query.WriteString(fmt.Sprintf(" |> filter(fn: (r) => strings.containsStr(v: r.message, substr: %s))", fluxString(filter.Contains)))
	}
	query.WriteString(" |> sort(columns: [\"_time\"], desc: false)")
	if filter.Limit > 0 {
		query.WriteString(fmt.Sprintf(" |> limit(n: %d)", filter.Limit))
	}

	result, err := s.queryAPI.Query(ctx, query.String())
	if err != nil {
		return nil, err
	}

	entries := make([]logging.LogEntry, 0)
	for result.Next() {
		record := result.Record()

		levelRaw, ok := record.ValueByKey("level").(string)
		if !ok || levelRaw == "" {
			continue
		}
		parsedLevel, err := logging.ParseLevel(levelRaw)
		if err != nil || parsedLevel == logging.LevelAll {
			continue
		}

		id, _ := record.ValueByKey("id").(string)
		message, _ := record.ValueByKey("message").(string)
		source, _ := record.ValueByKey("source").(string)

		entry := logging.LogEntry{
			ID:        id,
			Timestamp: record.Time(),
			Level:     parsedLevel,
			Message:   message,
			Source:    source,
		}

		if attrsRaw, ok := record.ValueByKey("attrs").(string); ok && attrsRaw != "" {
			var attrs map[string]any
			if err := json.Unmarshal([]byte(attrsRaw), &attrs); err != nil {
				return nil, err
			}
			entry.Attrs = attrs
		}

		entries = append(entries, entry)
	}

	if result.Err() != nil {
		return nil, result.Err()
	}

	return entries, nil
}

func (s *Store) Clear(before time.Time) error {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	start := time.Unix(0, 0).UTC()
	predicate := fmt.Sprintf(`_measurement=%s`, fluxString(s.measurement))
	return s.deleteAPI.DeleteWithName(ctx, s.org, s.bucket, start, before, predicate)
}

func (s *Store) Flush() error {
	return nil
}

func (s *Store) Close() error {
	if s.client != nil {
		s.client.Close()
	}
	return nil
}

func normalizeConfig(cfg Config) (Config, error) {
	cfg.URL = strings.TrimSpace(cfg.URL)
	cfg.Token = strings.TrimSpace(cfg.Token)
	cfg.Org = strings.TrimSpace(cfg.Org)
	cfg.Bucket = strings.TrimSpace(cfg.Bucket)
	cfg.Measurement = strings.TrimSpace(cfg.Measurement)

	if cfg.URL == "" {
		return Config{}, fmt.Errorf("influx url is required")
	}
	if cfg.Token == "" {
		return Config{}, fmt.Errorf("influx token is required")
	}
	if cfg.Org == "" {
		return Config{}, fmt.Errorf("influx org is required")
	}
	if cfg.Bucket == "" {
		return Config{}, fmt.Errorf("influx bucket is required")
	}
	if cfg.Measurement == "" {
		cfg.Measurement = defaultMeasurement
	}

	return cfg, nil
}

func fluxString(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + replacer.Replace(value) + `"`
}

func fluxTime(value time.Time, fallback string) string {
	if value.IsZero() {
		return fallback
	}
	return fmt.Sprintf("time(v: %s)", fluxString(value.UTC().Format(time.RFC3339Nano)))
}
