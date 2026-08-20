/*
 * NeurOps Agent — Logger
 * Copyright (c) NeurOps 2025. All rights reserved.
 *
 * Structured, levelled logger.  Writes JSON-formatted lines to a rotating
 * daily log file AND to stdout.  Each log record carries a timestamp,
 * level, component name, and free-form message.
 *
 * Log levels (numeric, matches agent.json):
 *   0 = TRACE  1 = DEBUG  2 = INFO  3 = WARN  4 = ERROR  5 = FATAL
 */

package logger

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Level is the numeric log severity.
type Level int

const (
	TRACE Level = iota
	DEBUG
	INFO
	WARN
	ERROR
	FATAL
)

var levelNames = map[Level]string{
	TRACE: "TRACE",
	DEBUG: "DEBUG",
	INFO:  "INFO",
	WARN:  "WARN",
	ERROR: "ERROR",
	FATAL: "FATAL",
}

// record is the JSON shape written to disk / stdout.
type record struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Component string `json:"component"`
	Message   string `json:"message"`
	Caller    string `json:"caller,omitempty"`
}

// Logger is a thread-safe, levelled, rolling-file logger.
type Logger struct {
	mu        sync.Mutex
	component string
	level     Level
	writer    io.Writer
	logDir    string
	dateKey   string // "2006-01-02"
	file      *os.File
}

// New creates a Logger that writes to logs/<component>-<date>.log.
// If the logs directory does not exist it is created.
func New(component string, level Level) *Logger {
	l := &Logger{component: component, level: level, logDir: "logs"}
	_ = os.MkdirAll(l.logDir, 0o755)
	l.rotate()
	return l
}

// SetLevel changes the minimum level at runtime (hot-reload support).
func (l *Logger) SetLevel(level Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// ── Public log methods ────────────────────────────────────────────────────────

func (l *Logger) Trace(msg string)  { l.log(TRACE, msg) }
func (l *Logger) Debug(msg string)  { l.log(DEBUG, msg) }
func (l *Logger) Info(msg string)   { l.log(INFO, msg) }
func (l *Logger) Warn(msg string)   { l.log(WARN, msg) }
func (l *Logger) Error(msg string)  { l.log(ERROR, msg) }
func (l *Logger) Fatal(msg string)  { l.log(FATAL, msg) }

// Tracef / Debugf / Infof / Warnf / Errorf / Fatalf — formatted variants.
func (l *Logger) Tracef(f string, a ...any) { l.log(TRACE, fmt.Sprintf(f, a...)) }
func (l *Logger) Debugf(f string, a ...any) { l.log(DEBUG, fmt.Sprintf(f, a...)) }
func (l *Logger) Infof(f string, a ...any)  { l.log(INFO, fmt.Sprintf(f, a...)) }
func (l *Logger) Warnf(f string, a ...any)  { l.log(WARN, fmt.Sprintf(f, a...)) }
func (l *Logger) Errorf(f string, a ...any) { l.log(ERROR, fmt.Sprintf(f, a...)) }
func (l *Logger) Fatalf(f string, a ...any) { l.log(FATAL, fmt.Sprintf(f, a...)) }

// WithComponent returns a child logger sharing the parent's file but with
// a different component label (useful for subsystems).
func (l *Logger) WithComponent(name string) *Logger {
	l.mu.Lock()
	defer l.mu.Unlock()
	return &Logger{
		component: name,
		level:     l.level,
		writer:    l.writer,
		logDir:    l.logDir,
		dateKey:   l.dateKey,
		file:      l.file,
	}
}

// ── Internal ──────────────────────────────────────────────────────────────────

func (l *Logger) log(level Level, msg string) {
	if level < l.level {
		return
	}

	now := time.Now()
	today := now.Format("2006-01-02")

	l.mu.Lock()
	defer l.mu.Unlock()

	// Roll file if day changed.
	if today != l.dateKey {
		l.rotate()
	}

	// Capture caller (skip log→log call chain: 3 frames up).
	_, file, line, ok := runtime.Caller(2)
	caller := ""
	if ok {
		caller = fmt.Sprintf("%s:%d", filepath.Base(file), line)
	}

	r := record{
		Timestamp: now.Format("2006-01-02 15:04:05.000"),
		Level:     levelNames[level],
		Component: l.component,
		Message:   msg,
		Caller:    caller,
	}

	data, _ := json.Marshal(r)
	line2 := string(data) + "\n"

	if l.writer != nil {
		_, _ = fmt.Fprint(l.writer, line2)
	}
	_, _ = fmt.Fprint(os.Stdout, line2)
}

func (l *Logger) rotate() {
	today := time.Now().Format("2006-01-02")
	if today == l.dateKey {
		return
	}
	if l.file != nil {
		_ = l.file.Close()
	}
	name := filepath.Join(l.logDir, fmt.Sprintf("%s-%s.log", strings.ReplaceAll(l.component, "/", "-"), today))
	f, err := os.OpenFile(name, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err == nil {
		l.file = f
		l.writer = f
	}
	l.dateKey = today
}
