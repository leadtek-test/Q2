package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"q2/pkg/logging"
	filestore "q2/pkg/logging/store/file"
	"q2/pkg/logging/store/memory"
)

func main() {
	backend := flag.String("backend", "memory", "storage backend: memory or file")
	filePath := flag.String("file", "tmp/logdemo/logs.jsonl", "path for the file backend")
	format := flag.String("format", "text", "output format: text or json")
	flag.Parse()

	store, cleanup, err := newStore(*backend, *filePath)
	if err != nil {
		log.Fatalf("create store: %v", err)
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
		log.Fatalf("create manager: %v", err)
	}
	if err := setFormatter(manager, *format); err != nil {
		log.Fatalf("set formatter: %v", err)
	}
	defer func() {
		if err = manager.Close(context.Background()); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "close manager: %v\n", err)
		}
	}()

	manager.RegisterLogHandler(logging.HandlerFunc(func(entry logging.LogEntry) error {
		handled++
		_, _ = fmt.Fprintf(os.Stderr, "handled %s log %s\n", entry.Level.String(), entry.ID)
		return nil
	}))

	start := time.Now().UTC()
	must(manager.WriteLog(logging.LevelDebug.String(), "debug is filtered by default min level"))
	must(manager.WriteLog(logging.LevelInfo.String(), "service booted"))
	must(manager.WriteLog(logging.LevelWarn.String(), "cache response is slow"))
	must(manager.WriteLog(logging.LevelError.String(), "job failed"))
	must(manager.Flush(context.Background()))

	fmt.Println("== all persisted logs ==")
	allEntries, err := manager.ReadFormattedLogs("", logging.LogFilter{})
	must(err)
	printEntries(allEntries)

	fmt.Println("== warn logs in range ==")
	warnEntries, err := manager.ReadFormattedLogs(logging.LevelWarn.String(), logging.LogFilter{
		Start: start.Add(-time.Second),
		End:   time.Now().UTC().Add(time.Second),
	})
	must(err)
	printEntries(warnEntries)

	must(manager.ClearLogs(time.Now().UTC().Add(time.Second)))
	remaining, err := manager.ReadFormattedLogs("", logging.LogFilter{})
	must(err)

	fmt.Println("== remaining after clear ==")
	printEntries(remaining)
	fmt.Printf("handler processed %d entries\n", handled)
}

func newStore(backend string, filePath string) (logging.LogStore, func(), error) {
	switch backend {
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

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
