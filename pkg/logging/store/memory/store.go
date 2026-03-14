package memory

import (
	"sync"
	"time"

	"q2/pkg/logging"
)

type Store struct {
	mu      sync.RWMutex
	entries []logging.LogEntry
}

func NewStore() *Store {
	return &Store{}
}

func (s *Store) Write(entry logging.LogEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.entries = append(s.entries, cloneEntry(entry))
	return nil
}

func (s *Store) Read(level logging.Level, filter logging.LogFilter) ([]logging.LogEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries := make([]logging.LogEntry, 0, len(s.entries))
	for _, entry := range s.entries {
		if !logging.MatchEntry(entry, level, filter) {
			continue
		}

		entries = append(entries, cloneEntry(entry))
		if filter.Limit > 0 && len(entries) >= filter.Limit {
			break
		}
	}

	return entries, nil
}

func (s *Store) Clear(before time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	kept := s.entries[:0]
	for _, entry := range s.entries {
		if entry.Timestamp.Before(before) {
			continue
		}
		kept = append(kept, cloneEntry(entry))
	}

	s.entries = kept
	return nil
}

func cloneEntry(entry logging.LogEntry) logging.LogEntry {
	entry.Attrs = cloneAttrs(entry.Attrs)
	return entry
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
