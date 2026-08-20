/*
 * NeurOps Agent — Log Collector
 * Copyright (c) NeurOps 2025. All rights reserved.
 *
 * Watches log files and directories for new content, parses each line,
 * applies optional filters/escape transforms, and publishes structured
 * log events to the NeurOps product.
 *
 * Features:
 *   • Tail with position tracking (survives restarts without re-reading)
 *   • inotify/fsnotify-based watch (immediate delivery, not just polling)
 *   • Multi-line log assembly (stack traces, XML blocks, etc.)
 *   • Configurable worker pool for parallel file processing
 *   • Log-parser plugin hook (ship to pluginengine for grok/regex parsing)
 */

package log

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/neuroops/agent/internal/config"
	"github.com/neuroops/agent/internal/logger"
)

// Publisher minimal interface needed by the log collector.
type Publisher interface {
	Publish(event map[string]any) bool
}

// ── Position cache ────────────────────────────────────────────────────────────

// positions tracks byte offsets per file so restarts resume correctly.
type positions struct {
	mu   sync.Mutex
	data map[string]int64
	file string
}

func loadPositions(path string) *positions {
	p := &positions{data: make(map[string]int64), file: path}
	raw, err := os.ReadFile(path)
	if err == nil {
		_ = json.Unmarshal(raw, &p.data)
	}
	return p
}

func (p *positions) get(path string) int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.data[path]
}

func (p *positions) set(path string, offset int64) {
	p.mu.Lock()
	p.data[path] = offset
	p.mu.Unlock()
}

func (p *positions) flush() {
	p.mu.Lock()
	defer p.mu.Unlock()
	raw, _ := json.MarshalIndent(p.data, "", "  ")
	_ = os.WriteFile(p.file, raw, 0o644)
}

// ── Collector ─────────────────────────────────────────────────────────────────

// Collector watches files and ships log records.
type Collector struct {
	cfg      *config.LogAgentConfig
	agentCfg *config.AgentConfig
	pub      Publisher
	log      *logger.Logger
	stopCh   chan struct{}
	wg       sync.WaitGroup
	watcher  *fsnotify.Watcher
	pos      *positions
	jobs     chan logJob
	filters  []*regexp.Regexp
}

type logJob struct {
	filePath string
	tag      string
}

// New creates a log Collector.
func New(cfg *config.LogAgentConfig, agentCfg *config.AgentConfig, pub Publisher, log *logger.Logger) (*Collector, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("creating file watcher: %w", err)
	}

	var filters []*regexp.Regexp
	for _, pat := range cfg.LogFilters {
		if re, err := regexp.Compile(pat); err == nil {
			filters = append(filters, re)
		}
	}

	return &Collector{
		cfg:      cfg,
		agentCfg: agentCfg,
		pub:      pub,
		log:      log,
		stopCh:   make(chan struct{}),
		watcher:  watcher,
		pos:      loadPositions("config/cache-position.json"),
		jobs:     make(chan logJob, 1000),
		filters:  filters,
	}, nil
}

// Start begins watching all configured log directories and files.
func (c *Collector) Start() error {
	if len(c.cfg.LogDirectories) == 0 {
		c.log.Info("Log collector: no log.dirs configured — log agent idle")
		return nil
	}

	c.log.Info("Log collector starting")

	// Launch worker pool
	workers := c.cfg.MaxWorkers
	if workers <= 0 {
		workers = 2
	}
	for i := 0; i < workers; i++ {
		c.wg.Add(1)
		go c.worker(i)
	}

	// Watch each configured directory
	for _, dir := range c.cfg.LogDirectories {
		if err := c.watchDirectory(dir); err != nil {
			c.log.Warnf("Failed to watch %s: %v", dir.Path, err)
		}
	}

	// Background: process fsnotify events
	c.wg.Add(1)
	go c.watchLoop()

	// Background: flush positions periodically
	c.wg.Add(1)
	go c.positionFlusher()

	c.log.Info("Log collector started")
	return nil
}

// Stop halts the log collector cleanly.
func (c *Collector) Stop() {
	close(c.stopCh)
	_ = c.watcher.Close()
	close(c.jobs)
	c.wg.Wait()
	c.pos.flush() // final position save
	c.log.Info("Log collector stopped")
}

// ── Directory watching ────────────────────────────────────────────────────────

func (c *Collector) watchDirectory(dir config.LogDirectory) error {
	info, err := os.Stat(dir.Path)
	if err != nil {
		if c.cfg.IgnoreInvalidLogFile {
			c.log.Warnf("Log directory not found (ignored): %s", dir.Path)
			return nil
		}
		return err
	}

	if info.IsDir() {
		// Watch the directory for new files
		if err := c.watcher.Add(dir.Path); err != nil {
			return fmt.Errorf("watching directory %s: %w", dir.Path, err)
		}
		// Tail existing files
		pattern := dir.Pattern
		if pattern == "" {
			pattern = "*.log"
		}
		matches, _ := filepath.Glob(filepath.Join(dir.Path, pattern))
		for _, m := range matches {
			c.enqueueFile(m, dir.Tag)
		}
	} else {
		// Single file watch
		if err := c.watcher.Add(dir.Path); err != nil {
			return fmt.Errorf("watching file %s: %w", dir.Path, err)
		}
		c.enqueueFile(dir.Path, dir.Tag)
	}

	c.log.Infof("Watching: %s (tag=%s)", dir.Path, dir.Tag)
	return nil
}

func (c *Collector) enqueueFile(path, tag string) {
	select {
	case c.jobs <- logJob{filePath: path, tag: tag}:
	default:
		c.log.Warnf("Log job queue full — skipping %s", path)
	}
}

// ── Watch loop ────────────────────────────────────────────────────────────────

func (c *Collector) watchLoop() {
	defer c.wg.Done()

	for {
		select {
		case <-c.stopCh:
			return
		case event, ok := <-c.watcher.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				tag := c.tagForPath(event.Name)
				c.enqueueFile(event.Name, tag)
			}
		case err, ok := <-c.watcher.Errors:
			if !ok {
				return
			}
			c.log.Warnf("File watcher error: %v", err)
		}
	}
}

// ── Worker pool ───────────────────────────────────────────────────────────────

func (c *Collector) worker(id int) {
	defer c.wg.Done()

	for job := range c.jobs {
		c.processFile(job)
	}
}

func (c *Collector) processFile(job logJob) {
	f, err := os.Open(job.filePath)
	if err != nil {
		if !c.cfg.IgnoreInvalidLogFile {
			c.log.Warnf("Cannot open log file %s: %v", job.filePath, err)
		}
		return
	}
	defer f.Close()

	offset := c.pos.get(job.filePath)

	// If file was rotated (smaller than saved offset), restart from 0
	info, _ := f.Stat()
	if info != nil && info.Size() < offset {
		offset = 0
	}

	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			c.log.Warnf("Seek error in %s: %v", job.filePath, err)
			return
		}
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, c.cfg.WorkerMaxPageSizeBytes), c.cfg.WorkerMaxPageSizeBytes)

	var multiBuffer strings.Builder
	var lineCount int
	startPattern := c.multilinePattern(job.filePath)

	for scanner.Scan() {
		line := scanner.Text()
		line = c.escape(line)

		if c.isFiltered(line) {
			continue
		}

		if startPattern != nil {
			if startPattern.MatchString(line) {
				// Flush previous multiline block
				if multiBuffer.Len() > 0 {
					c.shipLogLine(multiBuffer.String(), job)
					multiBuffer.Reset()
				}
				multiBuffer.WriteString(line)
			} else {
				if multiBuffer.Len() > 0 {
					multiBuffer.WriteString(" ")
					multiBuffer.WriteString(line)
				} else {
					c.shipLogLine(line, job)
				}
			}
		} else {
			c.shipLogLine(line, job)
		}
		lineCount++
	}

	// Flush remaining multiline buffer
	if multiBuffer.Len() > 0 {
		c.shipLogLine(multiBuffer.String(), job)
	}

	// Save new position
	newOffset, _ := f.Seek(0, io.SeekCurrent)
	c.pos.set(job.filePath, newOffset)
}

// ── Event publishing ──────────────────────────────────────────────────────────

func (c *Collector) shipLogLine(line string, job logJob) {
	if strings.TrimSpace(line) == "" {
		return
	}
	event := map[string]any{
		"event.type":     "log",
		"event.source":   c.agentCfg.ProductHost,
		"event.category": categorizeLogLine(line),
		"log.source":     job.filePath,
		"log.tag":        job.tag,
		"log.message":    line,
		"agent.id":       c.agentCfg.AgentID,
		"object.ip":      c.agentCfg.ProductHost,
		"timestamp":      time.Now().Unix(),
		"timestamp.ms":   time.Now().UnixMilli(),
	}
	c.pub.Publish(event)
}

// categorizeLogLine inspects the log message text and returns an event
// category: "error", "warn", or "info".
func categorizeLogLine(line string) string {
	lower := strings.ToLower(line)
	switch {
	case strings.Contains(lower, "error") ||
		strings.Contains(lower, "fail") ||
		strings.Contains(lower, "exception"):
		return "error"
	case strings.Contains(lower, "warn") ||
		strings.Contains(lower, "warning"):
		return "warn"
	default:
		return "info"
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (c *Collector) positionFlusher() {
	defer c.wg.Done()
	t := time.NewTicker(time.Duration(c.cfg.PositionWriteSeconds) * time.Second)
	defer t.Stop()
	for {
		select {
		case <-c.stopCh:
			return
		case <-t.C:
			c.pos.flush()
		}
	}
}

func (c *Collector) escape(line string) string {
	for from, to := range c.cfg.EscapeCharacters {
		line = strings.ReplaceAll(line, from, to)
	}
	return line
}

func (c *Collector) isFiltered(line string) bool {
	for _, re := range c.filters {
		if re.MatchString(line) {
			return true
		}
	}
	return false
}

func (c *Collector) multilinePattern(path string) *regexp.Regexp {
	for _, mf := range c.cfg.MultilineFiles {
		if mf.Path == path && mf.StartPattern != "" {
			if re, err := regexp.Compile(mf.StartPattern); err == nil {
				return re
			}
		}
	}
	return nil
}

func (c *Collector) tagForPath(path string) string {
	for _, dir := range c.cfg.LogDirectories {
		if strings.HasPrefix(path, dir.Path) {
			return dir.Tag
		}
	}
	return filepath.Base(filepath.Dir(path))
}
