// Daemon /events WebSocket client — port of the LibrespotClient.swift events
// leg: connect to ws://127.0.0.1:<api_port>/events, parse the daemon event
// stream tolerantly (unknown types and fields ignored), reconnect after 1 s
// on any failure, and run the 10 s ping watchdog.
//
// The watchdog is a prod fix carried over from the mac node (R0): a
// half-dead socket (daemon died without a close frame) hangs the receive
// forever — the reconnect loop never fires and takeover detection goes
// blind. Ping every 10 s; a missed pong (enforced through the read deadline)
// kills the connection and forces a reconnect.
package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// LibrespotEvent is one parsed /events frame. Pointer fields mirror the
// Swift optionals: absent and zero are different answers.
type LibrespotEvent struct {
	Type     string
	URI      *string
	Name     *string
	Position *int64
	Duration *int64
	Value    *int // volume
	Max      *int // volume
}

// parseLibrespotEvent mirrors LibrespotClient.parseEvent: any JSON object
// with a "type" is an event; data fields are picked per type and everything
// else is ignored. ok=false only for frames without a usable type.
func parseLibrespotEvent(raw []byte) (LibrespotEvent, bool) {
	var frame struct {
		Type string `json:"type"`
		Data struct {
			URI      *string `json:"uri"`
			Name     *string `json:"name"`
			Position *int64  `json:"position"`
			Duration *int64  `json:"duration"`
			Value    *int    `json:"value"`
			Max      *int    `json:"max"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &frame); err != nil || frame.Type == "" {
		return LibrespotEvent{}, false
	}
	return LibrespotEvent{
		Type:     frame.Type,
		URI:      frame.Data.URI,
		Name:     frame.Data.Name,
		Position: frame.Data.Position,
		Duration: frame.Data.Duration,
		Value:    frame.Data.Value,
		Max:      frame.Data.Max,
	}, true
}

type EventsClient struct {
	url     string
	onEvent func(LibrespotEvent)
	log     *slog.Logger

	// Knobs are variables so tests can shrink them. Defaults mirror the mac
	// client: reconnect after 1 s, ping every 10 s.
	ReconnectDelay time.Duration
	PingInterval   time.Duration
	// PongWait is how long past a ping the socket may stay silent before the
	// read deadline declares it dead (the "failed pong" of the mac watchdog).
	PongWait time.Duration

	mu      sync.Mutex
	conn    *websocket.Conn
	stopped bool
	stop    chan struct{}
}

// NewEventsClient targets the local daemon's /events stream.
func NewEventsClient(apiPort int, onEvent func(LibrespotEvent), log *slog.Logger) *EventsClient {
	return newEventsClientURL(fmt.Sprintf("ws://127.0.0.1:%d/events", apiPort), onEvent, log)
}

func newEventsClientURL(url string, onEvent func(LibrespotEvent), log *slog.Logger) *EventsClient {
	return &EventsClient{
		url:            url,
		onEvent:        onEvent,
		log:            log,
		ReconnectDelay: time.Second,
		PingInterval:   10 * time.Second,
		PongWait:       5 * time.Second,
		stop:           make(chan struct{}),
	}
}

func (c *EventsClient) Start() { go c.run() }

func (c *EventsClient) Stop() {
	c.mu.Lock()
	if c.stopped {
		c.mu.Unlock()
		return
	}
	c.stopped = true
	close(c.stop)
	if c.conn != nil {
		c.conn.Close()
	}
	c.mu.Unlock()
}

func (c *EventsClient) run() {
	for {
		select {
		case <-c.stop:
			return
		default:
		}

		dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
		conn, _, err := dialer.Dial(c.url, nil)
		if err != nil {
			// Daemon restarting: retry until the supervisor brings it back.
			if !c.sleepReconnect() {
				return
			}
			continue
		}

		c.mu.Lock()
		if c.stopped {
			c.mu.Unlock()
			conn.Close()
			return
		}
		c.conn = conn
		c.mu.Unlock()
		c.log.Debug("librespot events stream connected", "url", c.url)

		c.readLoop(conn)

		c.mu.Lock()
		if c.conn == conn {
			c.conn = nil
		}
		c.mu.Unlock()
		conn.Close()

		if !c.sleepReconnect() {
			return
		}
	}
}

// readLoop pumps frames until the connection dies. Liveness contract: every
// received frame or pong extends the read deadline by PingInterval+PongWait;
// the pinger pokes the daemon every PingInterval. A half-dead socket answers
// nothing, the deadline expires, ReadMessage errors, and the caller
// reconnects — the gorilla shape of the mac sendPing watchdog.
func (c *EventsClient) readLoop(conn *websocket.Conn) {
	deadline := c.PingInterval + c.PongWait
	conn.SetReadDeadline(time.Now().Add(deadline))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(deadline))
	})

	pingerDone := make(chan struct{})
	defer close(pingerDone)
	go func() {
		ticker := time.NewTicker(c.PingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-pingerDone:
				return
			case <-c.stop:
				return
			case <-ticker.C:
				if err := conn.WriteControl(websocket.PingMessage, nil,
					time.Now().Add(5*time.Second)); err != nil {
					conn.Close() // wakes the blocked ReadMessage
					return
				}
			}
		}
	}()

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			select {
			case <-c.stop:
			default:
				c.log.Warn("librespot events stream dead, reconnecting", "err", err)
			}
			return
		}
		conn.SetReadDeadline(time.Now().Add(deadline))
		if ev, ok := parseLibrespotEvent(raw); ok && c.onEvent != nil {
			c.onEvent(ev)
		}
	}
}

func (c *EventsClient) sleepReconnect() bool {
	select {
	case <-c.stop:
		return false
	case <-time.After(c.ReconnectDelay):
		return true
	}
}
