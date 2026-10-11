package diagnostics

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Level is a log severity.
type Level string

const (
	LevelInfo  Level = "INFO"
	LevelWarn  Level = "WARN"
	LevelError Level = "ERROR"
)

// Event is a single structured log record. It is written as one JSON
// object per line (JSON Lines) to the installer log file and, in a compact
// human-readable form, to stdout. Events never contain secrets.
type Event struct {
	Timestamp   string `json:"timestamp"`
	Level       string `json:"level"`
	TxID        string `json:"txid,omitempty"`
	Step        string `json:"step"`
	Event       string `json:"event"`
	Code        string `json:"code,omitempty"`
	Message     string `json:"message,omitempty"`
	Detail      string `json:"detail,omitempty"`
	Recoverable bool   `json:"recoverable,omitempty"`
	DurationMS  int64  `json:"duration_ms,omitempty"`
}

// Logger writes JSON Lines to a file and human-readable lines to a stdout
// writer. A nil *Logger is a valid no-op logger.
type Logger struct {
	mu      sync.Mutex
	f       *os.File
	txid    string
	verbose bool
	out     io.Writer
}

// OpenLogger creates the log directory and opens the installer.jsonl file
// in append mode. txid is stamped on every event for correlation.
func OpenLogger(dir, txid string, verbose bool, out io.Writer) (*Logger, error) {
	if out == nil {
		out = io.Discard
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "installer.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	return &Logger{f: f, txid: txid, verbose: verbose, out: out}, nil
}

// NewDiscardLogger returns a logger that writes nowhere.
func NewDiscardLogger() *Logger { return &Logger{out: io.Discard} }

// Close flushes and closes the underlying file.
func (l *Logger) Close() error {
	if l == nil || l.f == nil {
		return nil
	}
	return l.f.Close()
}

// Info logs at INFO level.
func (l *Logger) Info(step, event string) { l.emit(LevelInfo, step, event, nil, 0) }

// Infof logs an INFO event with a message.
func (l *Logger) Infof(step, event, format string, args ...any) {
	l.emit(LevelInfo, step, event, &Event{Message: fmt.Sprintf(format, args...)}, 0)
}

// Warn logs a WARN event with an optional code.
func (l *Logger) Warn(step, event, code, message string) {
	l.emit(LevelWarn, step, event, &Event{Code: code, Message: message}, 0)
}

// Error logs an ERROR event derived from a structured error.
func (l *Logger) Error(step, event string, err error) {
	var code, message, detail string
	recoverable := false
	if de, ok := err.(*Error); ok {
		code, message, detail, recoverable = de.Code, de.Message, de.Detail, de.Recoverable
	} else if err != nil {
		message = err.Error()
	}
	l.emit(LevelError, step, event, &Event{Code: code, Message: message, Detail: detail, Recoverable: recoverable}, 0)
}

// Duration logs an INFO event with a duration_ms field.
func (l *Logger) Duration(step, event string, d time.Duration) {
	l.emit(LevelInfo, step, event, &Event{DurationMS: d.Milliseconds()}, 0)
}

func (l *Logger) emit(level Level, step, event string, extra *Event, _ int) {
	if l == nil {
		return
	}
	e := Event{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Level:     string(level),
		TxID:      l.txid,
		Step:      step,
		Event:     event,
	}
	if extra != nil {
		e.Code = extra.Code
		e.Message = extra.Message
		e.Detail = extra.Detail
		e.Recoverable = extra.Recoverable
		e.DurationMS = extra.DurationMS
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.f != nil {
		if b, err := json.Marshal(e); err == nil {
			_, _ = l.f.Write(append(b, '\n'))
		}
	}

	// Human-readable stdout line. Only verbose mode surfaces detail/message
	// for non-error events, to keep the default install output clean.
	line := fmt.Sprintf("[%s] %s: %s", level, step, event)
	if e.Code != "" {
		line += fmt.Sprintf(" (%s)", e.Code)
	}
	if (level == LevelError || l.verbose) && e.Message != "" {
		line += ": " + e.Message
	}
	if (level == LevelError || l.verbose) && e.Detail != "" && e.Detail != e.Message {
		line += " — " + e.Detail
	}
	fmt.Fprintln(l.out, line)
}

// Redact replaces every occurrence of any secret with "<redacted>". It is
// applied to any string that could conceivably contain credential material
// before it is logged or printed.
func Redact(s string, secrets ...string) string {
	if s == "" {
		return s
	}
	for _, sec := range secrets {
		if sec == "" {
			continue
		}
		s = strings.ReplaceAll(s, sec, "<redacted>")
	}
	return s
}
