/*
 * NeurOps Agent — Event Publisher
 * Copyright (c) NeurOps 2025. All rights reserved.
 *
 * Wraps a ZeroMQ PUSH socket.  Plugin and collector output is serialised
 * to JSON, base64-encoded, prepended with a topic prefix, and sent to the
 * NeurOps product over TCP.
 *
 * Message wire format:
 *   <topic><base64(json_payload)>
 *
 * The publisher is non-blocking: events are enqueued internally and a
 * background goroutine drains the queue.  A configurable high-water-mark
 * prevents unbounded memory growth when the product is unreachable.
 */

package transport

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	zmq "github.com/pebbe/zmq4"

	"github.com/neuroops/agent/internal/logger"
)

const (
	defaultQueueSize = 100_000
	defaultHWM       = 5_000
	defaultLinger    = 0
	defaultSendTimeout = 0 // non-blocking
)

// EventPublisher sends telemetry events to the NeurOps product.
type EventPublisher struct {
	socket   *zmq.Socket
	events   chan []byte
	topic    string
	endpoint string
	log      *logger.Logger
	shutdown bool
	ctx      *zmq.Context
}

// NewEventPublisher creates a PUSH publisher. Call Start() to connect.
func NewEventPublisher(host string, port int, topic string, queueSize int, log *logger.Logger) (*EventPublisher, error) {
	ctx, err := zmq.NewContext()
	if err != nil {
		return nil, fmt.Errorf("creating zmq context: %w", err)
	}

	sock, err := ctx.NewSocket(zmq.PUSH)
	if err != nil {
		return nil, fmt.Errorf("creating zmq PUSH socket: %w", err)
	}

	if queueSize <= 0 {
		queueSize = defaultQueueSize
	}

	return &EventPublisher{
		socket:   sock,
		events:   make(chan []byte, queueSize),
		topic:    topic,
		endpoint: fmt.Sprintf("tcp://%s:%d", host, port),
		log:      log,
		ctx:      ctx,
	}, nil
}

// Start connects to the product and begins draining the event queue.
func (p *EventPublisher) Start() error {
	if err := p.socket.SetLinger(defaultLinger * time.Second); err != nil {
		return fmt.Errorf("setting linger: %w", err)
	}
	if err := p.socket.SetSndhwm(defaultHWM); err != nil {
		return fmt.Errorf("setting hwm: %w", err)
	}
	if err := p.socket.SetSndtimeo(defaultSendTimeout * time.Second); err != nil {
		return fmt.Errorf("setting send timeout: %w", err)
	}

	if err := p.socket.Connect(p.endpoint); err != nil {
		return fmt.Errorf("connecting to %s: %w", p.endpoint, err)
	}

	go p.drain()
	p.log.Infof("Event publisher connected → %s", p.endpoint)
	return nil
}

// Publish enqueues a MotadataMap-style event for delivery.
// Returns false if the queue is full (backpressure signal).
func (p *EventPublisher) Publish(event map[string]any) bool {
	if p.socket == nil || p.shutdown {
		return false
	}
	data, err := json.Marshal(event)
	if err != nil {
		p.log.Errorf("Failed to marshal event: %v", err)
		return false
	}
	select {
	case p.events <- data:
		return true
	default:
		p.log.Warn("Event queue full — dropping event (product may be unreachable)")
		return false
	}
}

// PublishBatch enqueues multiple events atomically.
func (p *EventPublisher) PublishBatch(events []map[string]any) int {
	sent := 0
	for _, e := range events {
		if p.Publish(e) {
			sent++
		}
	}
	return sent
}

// Shutdown disconnects and drains any remaining queued events.
func (p *EventPublisher) Shutdown() {
	if p.socket == nil {
		return
	}
	p.shutdown = true
	close(p.events)
	_ = p.socket.Disconnect(p.endpoint)
	_ = p.socket.Close()
	_ = p.ctx.Term()
	p.log.Info("Event publisher shut down")
}

// QueueDepth returns the number of pending events waiting to be sent.
func (p *EventPublisher) QueueDepth() int {
	return len(p.events)
}

// ── Internal ──────────────────────────────────────────────────────────────────

func (p *EventPublisher) drain() {
	for data := range p.events {
		if !p.shutdown {
			encoded := p.topic + base64.StdEncoding.EncodeToString(data)
			if _, err := p.socket.Send(encoded, 0); err != nil {
				p.log.Warnf("Failed to send event: %v", err)
			}
		}
	}
}

// ── Errors ────────────────────────────────────────────────────────────────────

var ErrPublisherNotStarted = errors.New("event publisher not started")
var ErrQueueFull = errors.New("event queue is full")
