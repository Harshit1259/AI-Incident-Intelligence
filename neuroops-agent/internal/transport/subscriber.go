/*
 * NeurOps Agent — Event Subscriber
 * Copyright (c) NeurOps 2025. All rights reserved.
 *
 * Wraps a ZeroMQ SUB socket.  The product pushes commands (poll triggers,
 * config updates, runbook executions, etc.) to subscribed agents.
 *
 * Message wire format (same as publisher, inverted direction):
 *   <topic><base64(json_payload)>
 *
 * Usage:
 *   sub, _ := transport.NewEventSubscriber("192.168.1.10", 9440, "AGENT.", handler, log)
 *   sub.Start()
 *   // ... later ...
 *   sub.Shutdown()
 */

package transport

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	zmq "github.com/pebbe/zmq4"

	"github.com/neuroops/agent/internal/logger"
)

// EventHandler is called for every inbound command from the product.
type EventHandler func(event map[string]any)

// EventSubscriber receives commands from the NeurOps product.
type EventSubscriber struct {
	socket   *zmq.Socket
	topic    string
	endpoint string
	handler  EventHandler
	log      *logger.Logger
	shutdown bool
	ctx      *zmq.Context
}

// NewEventSubscriber creates a SUB subscriber. Call Start() to connect.
func NewEventSubscriber(host string, port int, topic string, handler EventHandler, log *logger.Logger) (*EventSubscriber, error) {
	ctx, err := zmq.NewContext()
	if err != nil {
		return nil, fmt.Errorf("creating zmq context: %w", err)
	}

	sock, err := ctx.NewSocket(zmq.SUB)
	if err != nil {
		return nil, fmt.Errorf("creating zmq SUB socket: %w", err)
	}

	return &EventSubscriber{
		socket:   sock,
		topic:    topic,
		endpoint: fmt.Sprintf("tcp://%s:%d", host, port),
		handler:  handler,
		log:      log,
		ctx:      ctx,
	}, nil
}

// Start connects to the product and begins receiving events.
func (s *EventSubscriber) Start() error {
	if s.handler == nil {
		return fmt.Errorf("event handler must not be nil")
	}

	_ = s.socket.SetLinger(0 * time.Second)
	_ = s.socket.SetRcvhwm(5000)
	_ = s.socket.SetRcvtimeo(-1 * time.Second) // block until message arrives
	_ = s.socket.SetSubscribe(s.topic)

	if err := s.socket.Connect(s.endpoint); err != nil {
		return fmt.Errorf("connecting subscriber to %s: %w", s.endpoint, err)
	}

	go s.receive()
	s.log.Infof("Event subscriber connected ← %s (topic=%q)", s.endpoint, s.topic)
	return nil
}

// Shutdown disconnects the subscriber.
func (s *EventSubscriber) Shutdown() {
	if s.socket == nil {
		return
	}
	s.shutdown = true
	_ = s.ctx.Term() // unblocks the blocking Recv
	s.log.Info("Event subscriber shut down")
}

// ── Internal ──────────────────────────────────────────────────────────────────

func (s *EventSubscriber) receive() {
	for {
		if s.shutdown {
			return
		}

		raw, err := s.socket.Recv(0)
		if err != nil {
			if strings.Contains(err.Error(), "Context was terminated") {
				return
			}
			s.log.Warnf("Subscriber recv error: %v", err)
			continue
		}

		if len(raw) == 0 {
			continue
		}

		// Strip topic prefix, base64-decode, unmarshal.
		payload := strings.TrimPrefix(raw, s.topic)
		decoded, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			s.log.Warnf("Failed to decode inbound event: %v", err)
			continue
		}

		event := make(map[string]any)
		if err := json.Unmarshal(decoded, &event); err != nil {
			s.log.Warnf("Failed to parse inbound event JSON: %v", err)
			continue
		}

		s.log.Tracef("Received command: %v", event)
		s.handler(event)
	}
}
