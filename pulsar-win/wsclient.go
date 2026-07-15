// WebSocket client to the coordinator (spec 6.2 item 4, ch. 8), a port of
// the macOS CoordinatorClient: register on connect, protocol ping every 10 s
// (clock sync 8.5), state heartbeat every 5 s, reconnect with exponential
// backoff 1..60 s, incoming envelopes delivered to a handler. gorilla/websocket
// — the same library the coordinator uses.
package main

import (
	"crypto/rand"
	"encoding/json"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	protocol "relux.works/duet/pulsar-win/wire"
)

// Identity is what register carries; LibrespotVersion is read at each
// (re)connect so a daemon that came up later still reports its version.
type Identity struct {
	NodeID           string
	Token            string
	AppVersion       string
	LibrespotVersion func() string
	HandshakeTimeout time.Duration
	Capabilities     []string
}

type WSClient struct {
	url      string
	identity Identity
	log      *slog.Logger
	clock    *ClockSync

	// Set by the owner before Start.
	OnMessage     func(env protocol.Envelope, payload any)
	OnConnected   func()
	StateProvider func() protocol.StatePayload

	// Intervals are variables so tests can shrink them.
	PingInterval      time.Duration // spec 8.5: every 10 s
	HeartbeatInterval time.Duration // spec 8.4: every 5 s
	MinBackoff        time.Duration
	MaxBackoff        time.Duration

	mu      sync.Mutex
	conn    *websocket.Conn
	backoff time.Duration
	stopped bool
	healthy bool // welcome exchanged and link up (tray status)
	stop    chan struct{}
}

func NewWSClient(url string, identity Identity, log *slog.Logger) *WSClient {
	if identity.LibrespotVersion == nil {
		identity.LibrespotVersion = func() string { return "unknown" }
	}
	if identity.HandshakeTimeout == 0 {
		identity.HandshakeTimeout = 10 * time.Second
	}
	if len(identity.Capabilities) == 0 {
		identity.Capabilities = []string{protocol.CapabilitySeamlessAdoption}
	} else {
		identity.Capabilities = canonicalNodeCapabilities(identity.Capabilities)
	}
	return &WSClient{
		url:               url,
		identity:          identity,
		log:               log,
		clock:             NewClockSync(),
		PingInterval:      10 * time.Second,
		HeartbeatInterval: 5 * time.Second,
		MinBackoff:        time.Second,
		MaxBackoff:        60 * time.Second,
		stop:              make(chan struct{}),
	}
}

func (c *WSClient) Clock() *ClockSync { return c.clock }

func (c *WSClient) Start() {
	c.mu.Lock()
	c.backoff = c.MinBackoff
	c.mu.Unlock()
	go c.run()
}

func (c *WSClient) Stop() {
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

// MarkHealthy resets the backoff after a confirmed healthy exchange
// (welcome received) — mirror of the macOS client.
func (c *WSClient) MarkHealthy() {
	c.mu.Lock()
	c.backoff = c.MinBackoff
	c.healthy = true
	c.mu.Unlock()
}

// Healthy reports whether the coordinator link is up (welcome exchanged and
// not since torn down). Read by the tray status line.
func (c *WSClient) Healthy() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.healthy
}

// Send wraps a typed payload into an envelope and writes it. Safe from any
// goroutine; drops with a log line when no connection is up (spec 8.6: no
// delivery guarantee at this layer).
func (c *WSClient) Send(msgType string, payload any) {
	env, err := protocol.NewEnvelope(newMessageID(time.Now()), nowMS(), msgType, payload)
	if err != nil {
		c.log.Error("encode failed", "type", msgType, "err", err)
		return
	}
	c.mu.Lock()
	conn := c.conn
	if conn != nil {
		conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		err = conn.WriteJSON(env)
	}
	c.mu.Unlock()
	if conn == nil {
		c.log.Debug("send skipped, not connected", "type", msgType)
		return
	}
	if err != nil {
		c.log.Warn("ws send failed", "type", msgType, "err", err)
	}
}

func (c *WSClient) run() {
	for {
		select {
		case <-c.stop:
			return
		default:
		}

		dialer := websocket.Dialer{HandshakeTimeout: c.identity.HandshakeTimeout}
		conn, _, err := dialer.Dial(c.url, nil)
		if err != nil {
			c.log.Warn("connect failed", "url", c.url, "err", err)
			if !c.sleepBackoff() {
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

		c.log.Info("connected to coordinator", "url", c.url)
		c.Send(protocol.TypeRegister, &protocol.RegisterPayload{
			NodeID:           c.identity.NodeID,
			Token:            c.identity.Token,
			AppVersion:       c.identity.AppVersion,
			LibrespotVersion: c.identity.LibrespotVersion(),
			Capabilities:     append([]string(nil), c.identity.Capabilities...),
		})
		if c.OnConnected != nil {
			c.OnConnected()
		}

		done := make(chan struct{})
		go c.timers(done)
		c.readLoop(conn) // blocks until the connection dies
		close(done)

		c.mu.Lock()
		if c.conn == conn {
			c.conn = nil
		}
		c.mu.Unlock()
		conn.Close()

		if !c.sleepBackoff() {
			return
		}
	}
}

func canonicalNodeCapabilities(values []string) []string {
	unique := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value != "" {
			unique[value] = struct{}{}
		}
	}
	result := make([]string, 0, len(unique))
	for value := range unique {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

// timers drives the protocol ping (clock sync) and the state heartbeat for
// the lifetime of one connection.
func (c *WSClient) timers(done <-chan struct{}) {
	ping := time.NewTicker(c.PingInterval)
	heartbeat := time.NewTicker(c.HeartbeatInterval)
	defer ping.Stop()
	defer heartbeat.Stop()
	for {
		select {
		case <-done:
			return
		case <-c.stop:
			return
		case <-ping.C:
			c.Send(protocol.TypePing, &protocol.PingPayload{T1: nowMS()})
		case <-heartbeat.C:
			if c.StateProvider != nil {
				state := c.StateProvider()
				c.Send(protocol.TypeState, &state)
			}
		}
	}
}

func (c *WSClient) readLoop(conn *websocket.Conn) {
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			select {
			case <-c.stop:
			default:
				c.log.Warn("ws receive failed, reconnecting", "err", err)
			}
			return
		}
		t4 := nowMS()

		var env protocol.Envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			c.log.Warn("bad frame", "err", err)
			continue
		}
		if env.V != protocol.Version {
			c.log.Warn("protocol version mismatch; reconnecting", "got", env.V,
				"want", protocol.Version)
			return
		}
		if !protocol.KnownType(env.Type) {
			c.log.Warn("unknown message type ignored", "type", env.Type) // spec 8.6
			continue
		}
		payload, err := protocol.DecodePayload(env)
		if err != nil {
			c.log.Warn("bad payload", "type", env.Type, "err", err)
			continue
		}

		if pong, ok := payload.(*protocol.PongPayload); ok {
			accepted := c.clock.AddSample(pong.T1, pong.T2, pong.T3, t4)
			offset, _ := c.clock.OffsetMS()
			c.log.Debug("clock sample",
				"rtt_ms", c.clock.LastRTTMS(), "offset_ms", offset, "accepted", accepted)
			continue
		}
		if c.OnMessage != nil {
			c.OnMessage(env, payload)
		}
	}
}

// sleepBackoff waits the current backoff (doubling it, capped at MaxBackoff)
// and reports false when the client was stopped meanwhile.
func (c *WSClient) sleepBackoff() bool {
	c.mu.Lock()
	c.healthy = false // disconnected — reconnecting
	delay := c.backoff
	if delay < c.MinBackoff {
		delay = c.MinBackoff
	}
	c.backoff = delay * 2
	if c.backoff > c.MaxBackoff {
		c.backoff = c.MaxBackoff
	}
	c.mu.Unlock()
	c.log.Info("reconnect scheduled", "after", delay)
	select {
	case <-c.stop:
		return false
	case <-time.After(delay):
		return true
	}
}

// --- message ids ---
// ULID (48-bit ms timestamp + 80-bit random, Crockford base32) matching the
// coordinator's internal/ulid encoder; ids stay lexicographically sortable.

const ulidAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

func newULID(t time.Time) string {
	var b [16]byte
	ms := uint64(t.UnixMilli())
	b[0] = byte(ms >> 40)
	b[1] = byte(ms >> 32)
	b[2] = byte(ms >> 24)
	b[3] = byte(ms >> 16)
	b[4] = byte(ms >> 8)
	b[5] = byte(ms)
	if _, err := rand.Read(b[6:]); err != nil {
		panic(err) // crypto/rand failure is unrecoverable
	}
	// 26 chars, MSB-first: 128 bits left-padded to 130 so 26*5 aligns.
	dst := make([]byte, 26)
	acc := uint32(0)
	bits := uint32(2)
	out := 0
	for _, by := range b {
		acc = acc<<8 | uint32(by)
		bits += 8
		for bits >= 5 {
			bits -= 5
			dst[out] = ulidAlphabet[(acc>>bits)&31]
			out++
		}
	}
	return string(dst)
}

func newMessageID(t time.Time) string { return "msg_" + newULID(t) }
