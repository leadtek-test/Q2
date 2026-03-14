package logging

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrInvalidLevel  = errors.New("invalid log level")
	ErrQueueFull     = errors.New("log queue is full")
	ErrManagerClosed = errors.New("log manager is closed")
)

type Level int

const (
	LevelAll Level = iota
	LevelDebug
	LevelInfo
	LevelWarn
	LevelError
)

func (l Level) String() string {
	switch l {
	case LevelAll:
		return "ALL"
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

func ParseLevel(raw string) (Level, error) {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "", "*", "ALL":
		return LevelAll, nil
	case "DEBUG":
		return LevelDebug, nil
	case "INFO":
		return LevelInfo, nil
	case "WARN", "WARNING":
		return LevelWarn, nil
	case "ERROR":
		return LevelError, nil
	default:
		return LevelAll, fmt.Errorf("%w: %q", ErrInvalidLevel, raw)
	}
}

type LogEntry struct {
	ID        string         `json:"id"`
	Timestamp time.Time      `json:"timestamp"`
	Level     Level          `json:"level"`
	Message   string         `json:"message"`
	Source    string         `json:"source,omitempty"`
	Attrs     map[string]any `json:"attrs,omitempty"`
}

type LogFilter struct {
	Start    time.Time
	End      time.Time
	Contains string
	Limit    int
}

type LogWriter interface {
	Write(entry LogEntry) error
}

type LogReader interface {
	Read(level Level, filter LogFilter) ([]LogEntry, error)
	Clear(before time.Time) error
}

type LogStore interface {
	LogWriter
	LogReader
}

type LogStoreFlusher interface {
	Flush() error
}

type LogHandler interface {
	Handle(entry LogEntry) error
}

type HandlerFunc func(entry LogEntry) error

func (f HandlerFunc) Handle(entry LogEntry) error {
	return f(entry)
}

func MatchEntry(entry LogEntry, level Level, filter LogFilter) bool {
	if level != LevelAll && entry.Level != level {
		return false
	}
	if !filter.Start.IsZero() && entry.Timestamp.Before(filter.Start) {
		return false
	}
	if !filter.End.IsZero() && entry.Timestamp.After(filter.End) {
		return false
	}
	if filter.Contains != "" && !strings.Contains(entry.Message, filter.Contains) {
		return false
	}
	return true
}

func cloneAttrs(attrs map[string]any) map[string]any {
	if len(attrs) == 0 {
		return nil
	}

	cloned := make(map[string]any, len(attrs))
	for key, value := range attrs {
		cloned[key] = value
	}

	return cloned
}

func cloneEntry(entry LogEntry) LogEntry {
	entry.Attrs = cloneAttrs(entry.Attrs)
	return entry
}
