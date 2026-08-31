// Package log provides zerolog logging adapters implementing vuhive.Logger and vuhive.LogEvent.
package log

import (
	"io"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/diode"
)

// ZerologLogger is a zerolog-backed implementation of Logger.
type ZerologLogger struct {
	zlog   zerolog.Logger
	closer io.Closer
}

// New creates a new ZerologLogger writing JSON logs synchronously to w at the specified zerolog Level.
func New(w io.Writer, level zerolog.Level) *ZerologLogger {
	zlog := zerolog.New(w).Level(level).With().Timestamp().Logger()
	return &ZerologLogger{zlog: zlog}
}

// NewWithFormat creates a ZerologLogger that uses either human-readable console output ("pretty")
// or structured JSON output ("json"). Defaults to JSON for unrecognized formats.
func NewWithFormat(w io.Writer, level zerolog.Level, format string) *ZerologLogger {
	out := w
	if format == "pretty" {
		out = zerolog.ConsoleWriter{Out: w, TimeFormat: time.RFC3339}
	}
	zlog := zerolog.New(out).Level(level).With().Timestamp().Logger()

	return &ZerologLogger{zlog: zlog}
}

// NewAsync creates a ZerologLogger writing JSON logs asynchronously via a lock-free ring-buffer diode.
// This prevents lock contention and I/O blocking during high-concurrency load tests.
func NewAsync(w io.Writer, level zerolog.Level) *ZerologLogger {
	if w == io.Discard {
		return New(w, level)
	}
	dw := diode.NewWriter(struct{ io.Writer }{w}, 10000, 10*time.Millisecond, nil)
	zlog := zerolog.New(dw).Level(level).With().Timestamp().Logger()
	return &ZerologLogger{zlog: zlog, closer: dw}
}

// NewAsyncWithFormat creates an asynchronous ZerologLogger using either pretty console or JSON output,
// backed by a lock-free diode ring-buffer.
func NewAsyncWithFormat(w io.Writer, level zerolog.Level, format string) *ZerologLogger {
	if w == io.Discard {
		return NewWithFormat(w, level, format)
	}
	out := w
	if format == "pretty" {
		out = zerolog.ConsoleWriter{Out: w, TimeFormat: time.RFC3339}
	}
	dw := diode.NewWriter(struct{ io.Writer }{out}, 10000, 10*time.Millisecond, nil)
	zlog := zerolog.New(dw).Level(level).With().Timestamp().Logger()

	return &ZerologLogger{zlog: zlog, closer: dw}
}

// Close flushes and cleans up the underlying diode writer if present.
func (l *ZerologLogger) Close() error {
	if l.closer != nil {
		return l.closer.Close()
	}
	return nil
}

// NewWithZerolog wraps an existing zerolog.Logger into a ZerologLogger.
func NewWithZerolog(zlog zerolog.Logger) *ZerologLogger {
	return &ZerologLogger{zlog: zlog}
}

// WithScenario returns a child ZerologLogger with the "scenario" field bound.
func (l *ZerologLogger) WithScenario(scenario string) *ZerologLogger {
	return &ZerologLogger{zlog: l.zlog.With().Str("scenario", scenario).Logger(), closer: l.closer}
}

// WithVU returns a child ZerologLogger with the "vu_id" field bound.
func (l *ZerologLogger) WithVU(vuID int) *ZerologLogger {
	return &ZerologLogger{zlog: l.zlog.With().Int("vu_id", vuID).Logger(), closer: l.closer}
}

// WithIteration returns a child ZerologLogger with the "iteration" field bound.
func (l *ZerologLogger) WithIteration(iter int64) *ZerologLogger {
	return &ZerologLogger{zlog: l.zlog.With().Int64("iteration", iter).Logger(), closer: l.closer}
}

// WithFields returns a child ZerologLogger with arbitrary key-value context fields.
func (l *ZerologLogger) WithFields(fields map[string]any) *ZerologLogger {
	ctx := l.zlog.With()
	for k, v := range fields {
		ctx = ctx.Interface(k, v)
	}
	return &ZerologLogger{zlog: ctx.Logger(), closer: l.closer}
}

// Zerolog returns the underlying zerolog.Logger instance.
func (l *ZerologLogger) Zerolog() zerolog.Logger {
	return l.zlog
}

// Debug starts a new debug log event.
func (l *ZerologLogger) Debug() LogEvent {
	return &logEvent{event: l.zlog.Debug()}
}

// Info starts a new info log event.
func (l *ZerologLogger) Info() LogEvent {
	return &logEvent{event: l.zlog.Info()}
}

// Warn starts a new warning log event.
func (l *ZerologLogger) Warn() LogEvent {
	return &logEvent{event: l.zlog.Warn()}
}

// Error starts a new error log event.
func (l *ZerologLogger) Error() LogEvent {
	return &logEvent{event: l.zlog.Error()}
}

// logEvent implements vuhive.LogEvent by wrapping a zerolog.Event.
type logEvent struct {
	event *zerolog.Event
}

func (e *logEvent) Str(key, val string) LogEvent {
	if e.event != nil {
		e.event.Str(key, val)
	}
	return e
}

func (e *logEvent) Int(key string, val int) LogEvent {
	if e.event != nil {
		e.event.Int(key, val)
	}
	return e
}

func (e *logEvent) Int64(key string, val int64) LogEvent {
	if e.event != nil {
		e.event.Int64(key, val)
	}
	return e
}

func (e *logEvent) Float64(key string, val float64) LogEvent {
	if e.event != nil {
		e.event.Float64(key, val)
	}
	return e
}

func (e *logEvent) Bool(key string, val bool) LogEvent {
	if e.event != nil {
		e.event.Bool(key, val)
	}
	return e
}

func (e *logEvent) Dur(key string, val time.Duration) LogEvent {
	if e.event != nil {
		e.event.Dur(key, val)
	}
	return e
}

func (e *logEvent) Err(err error) LogEvent {
	if e.event != nil {
		e.event.Err(err)
	}
	return e
}

func (e *logEvent) Msg(msg string) {
	if e.event != nil {
		e.event.Msg(msg)
	}
}

// Compile-time interface satisfaction checks.
var (
	_ Logger   = (*ZerologLogger)(nil)
	_ LogEvent = (*logEvent)(nil)
)
