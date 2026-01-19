package logtail

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/appgram/logdump/internal/config"
)

type LogEntry struct {
	Timestamp  time.Time
	Source     string
	Content    string
	Tags       []string
	Filtered   bool
	LineNumber int
}

type Stream struct {
	Config     config.StreamConfig
	File       *os.File
	Reader     *bufio.Reader
	LineNumber int
	Done       chan struct{}
}

type Manager struct {
	streams  map[string]*Stream
	entries  chan LogEntry
	buffer   []LogEntry
	bufferMu sync.RWMutex
	mu       sync.RWMutex
	ctx      context.Context
	cancel   context.CancelFunc
	tailOnly bool // skip history, only show new logs
}

func NewManager() *Manager {
	return NewManagerWithOptions(false)
}

func NewManagerWithOptions(tailOnly bool) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		streams:  make(map[string]*Stream),
		entries:  make(chan LogEntry, 10000),
		buffer:   make([]LogEntry, 0, 1000),
		ctx:      ctx,
		cancel:   cancel,
		tailOnly: tailOnly,
	}
}

func (m *Manager) Tail(cfg config.StreamConfig) error {
	matches, err := filepath.Glob(filepath.Join(cfg.Path, "*"))
	if err != nil {
		return err
	}

	for _, match := range matches {
		if !cfg.Matches(match) {
			continue
		}
		if err := m.addFile(cfg, match); err != nil {
			return err
		}
	}

	if len(matches) == 0 {
		m.watchDirectory(cfg)
	}

	return nil
}

func (m *Manager) addFile(cfg config.StreamConfig, path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.streams[path]; ok {
		return nil
	}

	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", path, err)
	}

	stream := &Stream{
		Config:     cfg,
		File:       file,
		Reader:     bufio.NewReader(file),
		LineNumber: 0,
		Done:       make(chan struct{}),
	}

	m.streams[path] = stream

	go stream.read(m.ctx, m.entries, m.tailOnly)

	return nil
}

func (m *Manager) watchDirectory(cfg config.StreamConfig) {
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-m.ctx.Done():
				return
			case <-ticker.C:
				matches, _ := filepath.Glob(filepath.Join(cfg.Path, "*"))
				for _, match := range matches {
					if cfg.Matches(match) {
						_ = m.addFile(cfg, match)
					}
				}
			}
		}
	}()
}

func (s *Stream) read(ctx context.Context, entries chan<- LogEntry, tailOnly bool) {
	defer s.File.Close()
	defer close(s.Done)

	var offset int64 = 0

	// If tailOnly, start at end of file (skip history)
	if tailOnly {
		var err error
		offset, err = s.File.Seek(0, io.SeekEnd)
		if err != nil {
			return
		}
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
			fileSize, err := s.File.Seek(0, io.SeekEnd)
			if err != nil {
				return
			}

			if offset < fileSize {
				if _, err := s.File.Seek(offset, io.SeekStart); err != nil {
					return
				}
				reader := bufio.NewReader(s.File)
				for {
					line, err := reader.ReadString('\n')
					if err != nil {
						if err == io.EOF {
							break
						}
						return
					}

					s.LineNumber++
					entry := LogEntry{
						Timestamp:  time.Now(),
						Source:     s.Config.Name,
						Content:    strings.TrimSuffix(line, "\n"),
						Tags:       s.Config.Tags,
						LineNumber: s.LineNumber,
					}

					select {
					case entries <- entry:
					case <-ctx.Done():
						return
					default:
						go func(e LogEntry) {
							entries <- e
						}(entry)
					}
				}
				newOffset, err := s.File.Seek(0, io.SeekCurrent)
				if err != nil {
					return
				}
				offset = newOffset
			}
		}

		time.Sleep(100 * time.Millisecond)
	}
}

func (m *Manager) Entries() <-chan LogEntry {
	return m.entries
}

func (m *Manager) GetStreams() map[string]*Stream {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*Stream)
	for k, v := range m.streams {
		result[k] = v
	}
	return result
}

func (m *Manager) Close() {
	m.cancel()
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, stream := range m.streams {
		if stream.File != nil {
			stream.File.Close()
		}
	}
}

func (m *Manager) AddEntry(entry LogEntry) {
	m.bufferMu.Lock()
	defer m.bufferMu.Unlock()

	m.buffer = append(m.buffer, entry)
	if len(m.buffer) > 1000 {
		m.buffer = m.buffer[len(m.buffer)-1000:]
	}
}

func (m *Manager) Search(ctx context.Context, pattern string, source string) (<-chan LogEntry, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid pattern: %w", err)
	}

	results := make(chan LogEntry, 100)

	go func() {
		defer close(results)

		m.bufferMu.RLock()
		defer m.bufferMu.RUnlock()

		for _, entry := range m.buffer {
			if source == "" || entry.Source == source {
				if re.MatchString(entry.Content) {
					select {
					case results <- entry:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()

	return results, nil
}

func (m *Manager) GetEntries(source string, limit int) []LogEntry {
	m.bufferMu.RLock()
	defer m.bufferMu.RUnlock()

	var entries []LogEntry
	for _, entry := range m.buffer {
		if source == "" || entry.Source == source {
			entries = append(entries, entry)
		}
	}

	if limit > 0 && len(entries) > limit {
		entries = entries[len(entries)-limit:]
	}

	return entries
}

func (m *Manager) GetBuffer() []LogEntry {
	m.bufferMu.RLock()
	defer m.bufferMu.RUnlock()

	result := make([]LogEntry, len(m.buffer))
	copy(result, m.buffer)
	return result
}

func (m *Manager) StartBuffering() {
	go func() {
		for entry := range m.entries {
			m.AddEntry(entry)
		}
	}()
}
