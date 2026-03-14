package logging

import (
	"encoding/json"
	"fmt"
	"time"
)

type Formatter interface {
	Format(entry LogEntry) (string, error)
}

type TextFormatter struct{}

func (TextFormatter) Format(entry LogEntry) (string, error) {
	return fmt.Sprintf("%s %s %s", entry.Timestamp.Format(time.RFC3339), entry.Level.String(), entry.Message), nil
}

type JsonFormatter struct{}

func (JsonFormatter) Format(entry LogEntry) (string, error) {
	payload, err := json.Marshal(entry)
	if err != nil {
		return "", err
	}

	return string(payload), nil
}
