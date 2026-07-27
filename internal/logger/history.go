package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	maxHistoryEntries = 1000
	maxLogFileSize    = int64(10 * 1024 * 1024)
	maxLogFiles       = 5
)

var sensitiveLogPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(authorization\s*[:=]\s*)(?:bearer\s+)?([^\s]+)`),
	regexp.MustCompile(`(?i)(bearer\s+)([A-Za-z0-9._~+/=-]+)`),
	regexp.MustCompile(`(?i)((?:admin_?password|rest_?password|password|jwt|token)\s*[:=]\s*)([^\s,;]+)`),
}

type Entry struct {
	ID        int64     `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
}

type historyCore struct {
	mu      *sync.RWMutex
	entries *[]Entry
	nextID  *int64
	fields  []zapcore.Field
}

var (
	historyMu      sync.RWMutex
	historyEntries = make([]Entry, 0, maxHistoryEntries)
	historyNextID  int64
)

func newHistoryCore() zapcore.Core {
	return &historyCore{mu: &historyMu, entries: &historyEntries, nextID: &historyNextID}
}

func (core *historyCore) Enabled(level zapcore.Level) bool { return level >= zap.DebugLevel }

func (core *historyCore) With(fields []zapcore.Field) zapcore.Core {
	clone := *core
	clone.fields = append(append([]zapcore.Field{}, core.fields...), fields...)
	return &clone
}

func (core *historyCore) Check(entry zapcore.Entry, checked *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if core.Enabled(entry.Level) {
		return checked.AddCore(entry, core)
	}
	return checked
}

func (core *historyCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	message := redact(entry.Message)
	core.mu.Lock()
	*core.nextID++
	item := Entry{
		ID:        *core.nextID,
		Timestamp: entry.Time.UTC(),
		Level:     entry.Level.String(),
		Message:   message,
	}
	*core.entries = append(*core.entries, item)
	if len(*core.entries) > maxHistoryEntries {
		copy(*core.entries, (*core.entries)[len(*core.entries)-maxHistoryEntries:])
		*core.entries = (*core.entries)[:maxHistoryEntries]
	}
	core.mu.Unlock()
	return nil
}

func (core *historyCore) Sync() error { return nil }

func List(afterID int64, limit int, level string) []Entry {
	if limit < 1 {
		limit = 200
	}
	if limit > 500 {
		limit = 500
	}
	level = strings.ToLower(strings.TrimSpace(level))
	historyMu.RLock()
	defer historyMu.RUnlock()
	items := make([]Entry, 0, limit)
	for _, item := range historyEntries {
		if item.ID <= afterID || (level != "" && item.Level != level) {
			continue
		}
		items = append(items, item)
		if len(items) == limit {
			break
		}
	}
	return items
}

func redact(value string) string {
	for _, pattern := range sensitiveLogPatterns {
		value = pattern.ReplaceAllString(value, `${1}[已隐藏]`)
	}
	return strings.TrimRight(value, "\r\n")
}

type rotatingWriter struct {
	mu       sync.Mutex
	path     string
	maxBytes int64
	maxFiles int
	file     *os.File
	size     int64
}

func newRotatingWriter(path string, maxBytes int64, maxFiles int) (*rotatingWriter, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return nil, err
	}
	writer := &rotatingWriter{path: path, maxBytes: maxBytes, maxFiles: maxFiles}
	if err := writer.open(); err != nil {
		return nil, err
	}
	return writer, nil
}

func (writer *rotatingWriter) open() error {
	file, err := os.OpenFile(writer.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0640)
	if err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return err
	}
	writer.file = file
	writer.size = info.Size()
	return nil
}

func (writer *rotatingWriter) Write(data []byte) (int, error) {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	if writer.size+int64(len(data)) > writer.maxBytes {
		if err := writer.rotate(); err != nil {
			return 0, err
		}
	}
	data = []byte(redact(string(data)) + "\n")
	written, err := writer.file.Write(data)
	writer.size += int64(written)
	return written, err
}

func (writer *rotatingWriter) Sync() error {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	if writer.file == nil {
		return nil
	}
	return writer.file.Sync()
}

func (writer *rotatingWriter) rotate() error {
	if writer.file != nil {
		if err := writer.file.Close(); err != nil {
			return err
		}
	}
	for index := writer.maxFiles - 1; index >= 1; index-- {
		oldPath := fmt.Sprintf("%s.%d", writer.path, index)
		newPath := fmt.Sprintf("%s.%d", writer.path, index+1)
		if index == writer.maxFiles-1 {
			_ = os.Remove(newPath)
		}
		if _, err := os.Stat(oldPath); err == nil {
			if err := os.Rename(oldPath, newPath); err != nil {
				return err
			}
		}
	}
	if _, err := os.Stat(writer.path); err == nil {
		if err := os.Rename(writer.path, writer.path+".1"); err != nil {
			return err
		}
	}
	return writer.open()
}
