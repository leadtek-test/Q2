package multi

import (
	"errors"
	"sort"
	"time"

	"q2/pkg/logging"
)

type Store struct {
	stores []logging.LogStore
}

func NewStore(stores []logging.LogStore) (*Store, error) {
	if len(stores) == 0 {
		return nil, errors.New("at least one store is required")
	}

	cloned := make([]logging.LogStore, 0, len(stores))
	for _, store := range stores {
		if store == nil {
			continue
		}
		cloned = append(cloned, store)
	}
	if len(cloned) == 0 {
		return nil, errors.New("at least one non-nil store is required")
	}

	return &Store{stores: cloned}, nil
}

func (s *Store) Write(entry logging.LogEntry) error {
	var errs []error
	for _, store := range s.stores {
		if err := store.Write(cloneEntry(entry)); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func (s *Store) Read(level logging.Level, filter logging.LogFilter) ([]logging.LogEntry, error) {
	merged := make([]logging.LogEntry, 0)
	seen := make(map[string]struct{})
	partialFilter := filter
	partialFilter.Limit = 0

	var errs []error
	for _, store := range s.stores {
		entries, err := store.Read(level, partialFilter)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		for _, entry := range entries {
			key := entry.ID
			if key == "" {
				key = fallbackKey(entry)
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			merged = append(merged, cloneEntry(entry))
		}
	}

	sort.Slice(merged, func(i, j int) bool {
		if !merged[i].Timestamp.Equal(merged[j].Timestamp) {
			return merged[i].Timestamp.Before(merged[j].Timestamp)
		}
		return merged[i].ID < merged[j].ID
	})

	if filter.Limit > 0 && len(merged) > filter.Limit {
		merged = merged[:filter.Limit]
	}

	return merged, errors.Join(errs...)
}

func (s *Store) Clear(before time.Time) error {
	var errs []error
	for _, store := range s.stores {
		if err := store.Clear(before); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func (s *Store) Flush() error {
	var errs []error
	for _, store := range s.stores {
		flusher, ok := store.(logging.LogStoreFlusher)
		if !ok {
			continue
		}
		if err := flusher.Flush(); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func (s *Store) Close() error {
	var errs []error
	for _, store := range s.stores {
		closer, ok := store.(interface{ Close() error })
		if !ok {
			continue
		}
		if err := closer.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func fallbackKey(entry logging.LogEntry) string {
	return entry.Timestamp.UTC().Format(time.RFC3339Nano) + "|" + entry.Level.String() + "|" + entry.Source + "|" + entry.Message
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
