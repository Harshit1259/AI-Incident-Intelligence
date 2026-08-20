/*
 * NeurOps Agent — Multi-Publisher
 * Copyright (c) NeurOps 2025. All rights reserved.
 *
 * Wraps both the ZMQ EventPublisher and the HTTP forwarder behind a
 * single Publish() method.  Collectors are unaware of how many
 * downstream transports exist — they call Publish() and both ZMQ and
 * HTTP receive the event.
 *
 * ZMQ is treated as primary (its return value is propagated).
 * HTTP is best-effort — failures are logged but never block the caller.
 */

package transport

// MultiPublisher fans out events to both ZMQ and HTTP transports.
type MultiPublisher struct {
	primary   *EventPublisher // ZMQ (original)
	secondary *HTTPForwarder  // HTTP (new)
}

// NewMultiPublisher creates a publisher that sends to both transports.
// Either parameter may be nil if that transport is not configured.
func NewMultiPublisher(zmqPub *EventPublisher, httpFwd *HTTPForwarder) *MultiPublisher {
	return &MultiPublisher{
		primary:   zmqPub,
		secondary: httpFwd,
	}
}

// Publish sends the event to both transports.  The return value reflects
// the ZMQ publisher's result; HTTP is fire-and-forget.
func (m *MultiPublisher) Publish(event map[string]any) bool {
	zmqOk := true
	if m.primary != nil {
		zmqOk = m.primary.Publish(event)
	}
	if m.secondary != nil {
		m.secondary.Publish(event) // best-effort, don't block on failure
	}
	return zmqOk
}

// QueueDepth returns the ZMQ publisher's queue depth (primary metric).
func (m *MultiPublisher) QueueDepth() int {
	if m.primary != nil {
		return m.primary.QueueDepth()
	}
	return 0
}

// Shutdown stops both transports gracefully.
func (m *MultiPublisher) Shutdown() {
	if m.primary != nil {
		m.primary.Shutdown()
	}
	if m.secondary != nil {
		m.secondary.Shutdown()
	}
}

// Ensure MultiPublisher satisfies the Publisher interface used by collectors.
var _ interface {
	Publish(event map[string]any) bool
} = (*MultiPublisher)(nil)
