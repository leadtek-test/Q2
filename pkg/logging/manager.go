package logging

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

const defaultQueueSize = 128

type ErrorHandler func(error)

type Option func(*Manager)

func WithMinLevel(level Level) Option {
	return func(m *Manager) {
		m.minLevel = level
	}
}

func WithQueueSize(size int) Option {
	return func(m *Manager) {
		m.queueSize = size
	}
}

func WithSource(source string) Option {
	return func(m *Manager) {
		m.source = source
	}
}

func WithErrorHandler(handler ErrorHandler) Option {
	return func(m *Manager) {
		m.onError = handler
	}
}

func WithFormatter(formatter Formatter) Option {
	return func(m *Manager) {
		m.formatter = normalizeFormatter(formatter)
	}
}

type Manager struct {
	store     LogStore
	minLevel  Level
	source    string
	queueSize int
	onError   ErrorHandler

	queue      chan queueItem
	workerDone chan struct{}

	enqueueMu   sync.Mutex
	accepting   bool
	queueClosed bool

	handlersMu sync.RWMutex
	handlers   []LogHandler

	formatterMu sync.RWMutex
	formatter   Formatter

	sequence atomic.Uint64
}

type queueItem struct {
	entry    LogEntry
	hasEntry bool
	barrier  chan struct{}
}

func NewManager(store LogStore, opts ...Option) (*Manager, error) {
	if store == nil {
		return nil, errors.New("log store is required")
	}

	manager := &Manager{
		store:      store,
		minLevel:   LevelInfo,
		queueSize:  defaultQueueSize,
		accepting:  true,
		onError:    func(error) {},
		formatter:  TextFormatter{},
		workerDone: make(chan struct{}),
	}

	for _, opt := range opts {
		opt(manager)
	}

	if manager.queueSize <= 0 {
		return nil, fmt.Errorf("queue size must be positive: %d", manager.queueSize)
	}
	if manager.minLevel == LevelAll {
		manager.minLevel = LevelDebug
	}

	manager.queue = make(chan queueItem, manager.queueSize)
	go manager.run()

	return manager, nil
}

func (m *Manager) RegisterLogHandler(handler LogHandler) {
	if handler == nil {
		return
	}

	m.handlersMu.Lock()
	defer m.handlersMu.Unlock()

	m.handlers = append(m.handlers, handler)
}

func (m *Manager) SetFormatter(formatter Formatter) {
	m.formatterMu.Lock()
	defer m.formatterMu.Unlock()

	m.formatter = normalizeFormatter(formatter)
}

func (m *Manager) WriteLog(level string, message string) error {
	return m.writeLog(level, message, nil)
}

func (m *Manager) WriteLogWithAttrs(level string, message string, attrs map[string]any) error {
	return m.writeLog(level, message, attrs)
}

func (m *Manager) writeLog(level string, message string, attrs map[string]any) error {
	parsedLevel, err := ParseLevel(level)
	if err != nil {
		return err
	}
	if parsedLevel == LevelAll {
		return fmt.Errorf("%w: write requires a concrete level", ErrInvalidLevel)
	}
	if parsedLevel < m.minLevel {
		return nil
	}

	entry := LogEntry{
		ID:        m.nextID(),
		Timestamp: time.Now().UTC(),
		Level:     parsedLevel,
		Message:   message,
		Source:    m.source,
		Attrs:     cloneAttrs(attrs),
	}

	return m.enqueue(queueItem{entry: entry, hasEntry: true})
}

func (m *Manager) ReadLogs(level string, filter LogFilter) ([]LogEntry, error) {
	parsedLevel, err := ParseLevel(level)
	if err != nil {
		return nil, err
	}

	return m.store.Read(parsedLevel, filter)
}

func (m *Manager) ClearLogs(before time.Time) error {
	if err := m.Flush(context.Background()); err != nil {
		return err
	}

	return m.store.Clear(before)
}

func (m *Manager) ReadFormattedLogs(level string, filter LogFilter) ([]string, error) {
	entries, err := m.ReadLogs(level, filter)
	if err != nil {
		return nil, err
	}

	formatter := m.snapshotFormatter()
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		line, err := formatter.Format(entry)
		if err != nil {
			return nil, err
		}
		lines = append(lines, line)
	}

	return lines, nil
}

func (m *Manager) Flush(ctx context.Context) error {
	m.enqueueMu.Lock()
	if err := m.flushLocked(ctx); err != nil {
		return err
	}

	return m.flushStore()
}

func (m *Manager) Close(ctx context.Context) error {
	m.enqueueMu.Lock()
	if m.queueClosed {
		m.enqueueMu.Unlock()
		return nil
	}

	m.accepting = false
	if err := m.flushLocked(ctx); err != nil {
		return err
	}

	m.enqueueMu.Lock()
	if !m.queueClosed {
		close(m.queue)
		m.queueClosed = true
	}
	m.enqueueMu.Unlock()

	select {
	case <-m.workerDone:
		return m.flushStore()
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *Manager) enqueue(item queueItem) error {
	m.enqueueMu.Lock()
	defer m.enqueueMu.Unlock()

	if !m.accepting {
		return ErrManagerClosed
	}

	select {
	case m.queue <- item:
		return nil
	default:
		return ErrQueueFull
	}
}

func (m *Manager) flushLocked(ctx context.Context) error {
	if m.queueClosed {
		m.enqueueMu.Unlock()
		return nil
	}

	barrier := make(chan struct{})
	select {
	case m.queue <- queueItem{barrier: barrier}:
		m.enqueueMu.Unlock()
	case <-ctx.Done():
		m.enqueueMu.Unlock()
		return ctx.Err()
	}

	select {
	case <-barrier:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *Manager) nextID() string {
	sequence := m.sequence.Add(1)
	var buffer [48]byte
	out := buffer[:0]
	out = strconv.AppendInt(out, time.Now().UTC().UnixNano(), 10)
	out = append(out, '-')
	out = strconv.AppendUint(out, sequence, 10)
	return string(out)
}

func (m *Manager) run() {
	defer close(m.workerDone)

	for item := range m.queue {
		if item.barrier != nil {
			close(item.barrier)
			continue
		}

		if !item.hasEntry {
			continue
		}

		entry := cloneEntry(item.entry)
		if err := m.store.Write(entry); err != nil {
			m.onError(err)
			continue
		}

		for _, handler := range m.snapshotHandlers() {
			if err := handler.Handle(cloneEntry(entry)); err != nil {
				m.onError(err)
			}
		}
	}
}

func (m *Manager) snapshotHandlers() []LogHandler {
	m.handlersMu.RLock()
	defer m.handlersMu.RUnlock()

	handlers := make([]LogHandler, len(m.handlers))
	copy(handlers, m.handlers)
	return handlers
}

func (m *Manager) snapshotFormatter() Formatter {
	m.formatterMu.RLock()
	defer m.formatterMu.RUnlock()

	return normalizeFormatter(m.formatter)
}

func normalizeFormatter(formatter Formatter) Formatter {
	if formatter == nil {
		return TextFormatter{}
	}

	return formatter
}

func (m *Manager) flushStore() error {
	flusher, ok := m.store.(LogStoreFlusher)
	if !ok {
		return nil
	}

	if err := flusher.Flush(); err != nil {
		m.onError(err)
		return err
	}

	return nil
}
