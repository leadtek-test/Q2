package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"q2/pkg/logging"
	filestore "q2/pkg/logging/store/file"
	influxstore "q2/pkg/logging/store/influxdb"
	"q2/pkg/logging/store/memory"
)

type demoOptions struct {
	backend           string
	filePath          string
	format            string
	clear             bool
	influxURL         string
	influxToken       string
	influxOrg         string
	influxBucket      string
	influxMeasurement string
}

func main() {
	cmd := newRootCmd()
	cmd.SetArgs(normalizeLegacyLongFlags(os.Args[1:]))

	if err := cmd.Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	opts := demoOptions{}

	cmd := &cobra.Command{
		Use:          "logdemo",
		Short:        "Run logging system demo",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return fmt.Errorf("unexpected arguments: %v", args)
			}
			return runDemo(cmd.Context(), opts)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&opts.backend, "backend", "memory", "storage backend: memory, file, or influx")
	flags.StringVar(&opts.filePath, "file", "tmp/logdemo/logs.jsonl", "path for the file backend")
	flags.StringVar(&opts.format, "format", "text", "output format: text or json")
	flags.BoolVar(&opts.clear, "clear", false, "clear logs at the end of demo")
	flags.StringVar(&opts.influxURL, "influx-url", "http://localhost:8086", "InfluxDB URL")
	flags.StringVar(&opts.influxToken, "influx-token", "q2-dev-token", "InfluxDB token")
	flags.StringVar(&opts.influxOrg, "influx-org", "q2", "InfluxDB organization")
	flags.StringVar(&opts.influxBucket, "influx-bucket", "logging", "InfluxDB bucket")
	flags.StringVar(&opts.influxMeasurement, "influx-measurement", "logs", "InfluxDB measurement")

	return cmd
}

func runDemo(ctx context.Context, opts demoOptions) error {
	store, cleanup, err := newStore(ctx, opts.backend, opts.filePath, opts)
	if err != nil {
		return fmt.Errorf("create store: %w", err)
	}
	defer cleanup()

	var handled int
	manager, err := logging.NewManager(
		store,
		logging.WithSource("logdemo"),
		logging.WithMinLevel(logging.LevelInfo),
		logging.WithErrorHandler(func(err error) {
			_, _ = fmt.Fprintf(os.Stderr, "logging error: %v\n", err)
		}),
	)
	if err != nil {
		return fmt.Errorf("create manager: %w", err)
	}
	if err := setFormatter(manager, opts.format); err != nil {
		return fmt.Errorf("set formatter: %w", err)
	}
	defer func() {
		if closeErr := manager.Close(ctx); closeErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "close manager: %v\n", closeErr)
		}
	}()

	manager.RegisterLogHandler(logging.HandlerFunc(func(entry logging.LogEntry) error {
		handled++
		_, _ = fmt.Fprintf(os.Stderr, "handled %s log %s\n", entry.Level.String(), entry.ID)
		return nil
	}))

	start := time.Now().UTC()
	if err := manager.WriteLog(logging.LevelDebug.String(), "debug is filtered by default min level"); err != nil {
		return err
	}
	if err := manager.WriteLog(logging.LevelInfo.String(), "service booted"); err != nil {
		return err
	}
	if err := manager.WriteLog(logging.LevelWarn.String(), "cache response is slow"); err != nil {
		return err
	}
	if err := manager.WriteLog(logging.LevelError.String(), "job failed"); err != nil {
		return err
	}
	if err := manager.Flush(ctx); err != nil {
		return err
	}

	fmt.Println("== all persisted logs ==")
	allEntries, err := manager.ReadFormattedLogs("", logging.LogFilter{})
	if err != nil {
		return err
	}
	printEntries(allEntries)

	fmt.Println("== warn logs in range ==")
	warnEntries, err := manager.ReadFormattedLogs(logging.LevelWarn.String(), logging.LogFilter{
		Start: start.Add(-time.Second),
		End:   time.Now().UTC().Add(time.Second),
	})
	if err != nil {
		return err
	}
	printEntries(warnEntries)

	if opts.clear {
		if err := manager.ClearLogs(time.Now().UTC().Add(time.Second)); err != nil {
			return err
		}
	}
	remaining, err := manager.ReadFormattedLogs("", logging.LogFilter{})
	if err != nil {
		return err
	}

	fmt.Println("== remaining logs ==")
	printEntries(remaining)
	fmt.Printf("handler processed %d entries\n", handled)

	return nil
}

func newStore(ctx context.Context, backend string, filePath string, opts demoOptions) (logging.LogStore, func(), error) {
	switch strings.ToLower(strings.TrimSpace(backend)) {
	case "memory":
		store := memory.NewStore()
		return store, func() {}, nil
	case "file":
		store, err := filestore.NewStore(filePath)
		if err != nil {
			return nil, nil, err
		}
		return store, func() {
			_ = store.Close()
		}, nil
	case "influx", "influxdb", "tsdb":
		store, err := influxstore.NewStore(ctx, influxstore.Config{
			URL:         opts.influxURL,
			Token:       opts.influxToken,
			Org:         opts.influxOrg,
			Bucket:      opts.influxBucket,
			Measurement: opts.influxMeasurement,
		})
		if err != nil {
			return nil, nil, err
		}
		return store, func() {
			_ = store.Close()
		}, nil
	default:
		return nil, nil, fmt.Errorf("unsupported backend %q", backend)
	}
}

func setFormatter(manager *logging.Manager, format string) error {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "text":
		manager.SetFormatter(logging.TextFormatter{})
		return nil
	case "json":
		manager.SetFormatter(logging.JsonFormatter{})
		return nil
	default:
		return fmt.Errorf("unsupported formatter %q", format)
	}
}

func printEntries(entries []string) {
	for _, entry := range entries {
		fmt.Println(entry)
	}
}

func normalizeLegacyLongFlags(args []string) []string {
	normalized := make([]string, 0, len(args))
	for _, arg := range args {
		if isLegacyLongFlag(arg) {
			normalized = append(normalized, "-"+arg)
			continue
		}

		normalized = append(normalized, arg)
	}

	return normalized
}

func isLegacyLongFlag(arg string) bool {
	if !strings.HasPrefix(arg, "-") || strings.HasPrefix(arg, "--") {
		return false
	}
	if len(arg) <= 2 {
		return false
	}

	return arg[1] >= 'a' && arg[1] <= 'z'
}
