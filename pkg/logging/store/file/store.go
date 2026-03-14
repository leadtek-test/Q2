package filestore

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"q2/pkg/logging"
)

const scannerBufferSize = 1024 * 1024

type Store struct {
	mu   sync.Mutex
	path string
	file *os.File
}

func NewStore(path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("file store path is required")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}

	return &Store{
		path: path,
		file: file,
	}, nil
}

func (s *Store) Write(entry logging.LogEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	encoder := json.NewEncoder(s.file)
	return encoder.Encode(entry)
}

func (s *Store) Read(level logging.Level, filter logging.LogFilter) ([]logging.LogEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), scannerBufferSize)

	entries := make([]logging.LogEntry, 0)
	for scanner.Scan() {
		var entry logging.LogEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			return nil, err
		}
		if !logging.MatchEntry(entry, level, filter) {
			continue
		}

		entries = append(entries, entry)
		if filter.Limit > 0 && len(entries) >= filter.Limit {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return entries, nil
}

func (s *Store) Clear(before time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	input, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer input.Close()

	tempPath := s.path + ".tmp"
	output, err := os.OpenFile(tempPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}

	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 0, 64*1024), scannerBufferSize)
	encoder := json.NewEncoder(output)

	for scanner.Scan() {
		var entry logging.LogEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			output.Close()
			return err
		}
		if entry.Timestamp.Before(before) {
			continue
		}
		if err := encoder.Encode(entry); err != nil {
			_ = output.Close()
			return err
		}
	}

	if err := scanner.Err(); err != nil {
		_ = output.Close()
		return err
	}

	if err := output.Close(); err != nil {
		return err
	}

	if err := s.file.Close(); err != nil {
		return err
	}

	if err := os.Rename(tempPath, s.path); err != nil {
		return err
	}

	file, err := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}

	s.file = file
	return nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.file == nil {
		return nil
	}
	err := s.file.Close()
	s.file = nil
	return err
}
