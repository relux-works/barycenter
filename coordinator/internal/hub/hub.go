// Package hub is the WebSocket server for the two nodes (spec 7.1 ws-hub):
// token auth on register, connection registry with last-write-wins, protocol
// ping->pong timestamping (spec 8.5), heartbeat-based offline detection.
package hub

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"relux.works/duet/coordinator/internal/protocol"
	"relux.works/duet/coordinator/internal/ulid"
)

const (
	registerDeadline        = 5 * time.Second
	writeDeadline           = 10 * time.Second
	maxPendingRegistrations = 256
	closeInvalidAuth        = 4401 // spec 8.2
	closeRevokedAuth        = 4403
)

type Event any

// NodeKey identifies a connected pulsar: orbit + slot (v2.1 multi-tenant).
type NodeKey struct {
	Orbit int64
	Slot  protocol.NodeID
}

// NodeSnapshot is a defensive, authorization-free projection for application
// services that must make a point-in-time delivery decision. The store still
// resolves the authoritative actor and binding generation transactionally.
type NodeSnapshot struct {
	Connected      bool
	LastSeenAt     int64
	Capabilities   protocol.CapabilitySet
	PlaybackState  string
	OutputDegraded bool
	// CaptureQuality is an ephemeral, content-free heartbeat snapshot. Nil is
	// the honest legacy/mixed-version state; callers receive a defensive copy.
	CaptureQuality *protocol.CaptureQualityState
	// RTT is tied to the authenticated socket generation. Reconnect clears the
	// sample so a scheduler cannot arm media from a predecessor connection's
	// clock evidence.
	RTTMS        int64
	RTTSampledAt int64
	// CredentialTokenHash is a transient high-entropy witness for matching the
	// current authenticated socket to the authoritative slot generation. It
	// must never be logged, serialized to clients or persisted in receipts.
	CredentialTokenHash string
}

func presencePlaybackState(playback string) string {
	switch playback {
	case "stopped", "paused", "wait":
		return "idle"
	case "loading", "playing":
		return "main"
	case "voice":
		return "interrupt"
	default:
		return "unknown"
	}
}

type (
	EvRegistered struct {
		Key              NodeKey
		AppVersion       string
		LibrespotVersion string
		Capabilities     protocol.CapabilitySet
	}
	EvOnline  struct{ Key NodeKey }
	EvOffline struct{ Key NodeKey }
	EvMessage struct {
		Key                 NodeKey
		CredentialTokenHash string
		Env                 protocol.Envelope
		Payload             any
	}
	// EvBinary carries only a validated bounded live frame. Raw bytes are kept
	// in memory until fanout and are never formatted into logs or persisted.
	EvBinary struct {
		Key                 NodeKey
		CredentialTokenHash string
		ReceivedAtMS        int64
		Frame               protocol.LivePTTBinaryFrame
		Raw                 []byte
	}
)

// TokenLookup resolves a node token to its orbit and slot (store-backed).
type TokenLookup func(token string) (orbitID int64, slot string, ok bool)

type conn struct {
	ws                  *websocket.Conn
	send                chan outboundFrame
	stop                chan struct{}
	once                sync.Once
	credentialTokenHash string
	capabilities        protocol.CapabilitySet
	captureQualityGuard protocol.CaptureQualityGenerationGuard
}

type outboundFrame struct {
	envelope *protocol.Envelope
	binary   []byte
}

func (c *conn) close() {
	c.once.Do(func() {
		close(c.stop)
		c.ws.Close()
	})
}

type Hub struct {
	log          *slog.Logger
	lookup       TokenLookup
	offlineAfter time.Duration

	Events chan Event
	// registerSlots bounds unauthenticated upgraded sockets waiting to present
	// their first register frame. Authenticated sockets leave this pool before
	// entering the connection registry.
	registerSlots chan struct{}

	mu             sync.Mutex
	conns          map[NodeKey]*conn
	lastSeen       map[NodeKey]time.Time
	online         map[NodeKey]bool
	capabilities   map[NodeKey]protocol.CapabilitySet
	rttMS          map[NodeKey]int64
	rttSampledAt   map[NodeKey]time.Time
	playbackState  map[NodeKey]string
	outputDegraded map[NodeKey]bool
	captureQuality map[NodeKey]*protocol.CaptureQualityState
}

func New(log *slog.Logger, lookup TokenLookup, offlineAfter time.Duration) *Hub {
	return &Hub{
		log:          log,
		lookup:       lookup,
		offlineAfter: offlineAfter,
		// Liveness (EvOnline/EvOffline) must never be dropped (bugs #3): the
		// buffer absorbs bursts and emit() blocks rather than drops when full.
		Events:         make(chan Event, 256),
		registerSlots:  make(chan struct{}, maxPendingRegistrations),
		conns:          map[NodeKey]*conn{},
		lastSeen:       map[NodeKey]time.Time{},
		online:         map[NodeKey]bool{},
		capabilities:   map[NodeKey]protocol.CapabilitySet{},
		rttMS:          map[NodeKey]int64{},
		rttSampledAt:   map[NodeKey]time.Time{},
		playbackState:  map[NodeKey]string{},
		outputDegraded: map[NodeKey]bool{},
		captureQuality: map[NodeKey]*protocol.CaptureQualityState{},
	}
}

// Run drives offline detection until stop is closed.
func (h *Hub) Run(stop <-chan struct{}) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case now := <-t.C:
			h.sweepOffline(now, stop)
		}
	}
}

// sweepOffline demotes nodes past the offline deadline and emits EvOffline
// reliably. The state flip happens under the lock; the emit is a BLOCKING send
// OUTSIDE the lock (bugs #3): a full channel must back-pressure, never drop a
// liveness edge — a dropped EvOffline strands a dead node "online" forever.
// Emitting outside the lock keeps the consuming loop free to call back into the
// hub (Send/Online take the same mutex) without deadlocking.
func (h *Hub) sweepOffline(now time.Time, stop <-chan struct{}) {
	var gone []NodeKey
	h.mu.Lock()
	for key, seen := range h.lastSeen {
		if h.online[key] && now.Sub(seen) > h.offlineAfter {
			h.online[key] = false
			gone = append(gone, key)
		}
	}
	h.mu.Unlock()
	for _, key := range gone {
		h.emit(EvOffline{Key: key}, stop)
	}
}

// emit delivers a liveness event, blocking until the loop drains it or the
// escape channel (hub shutdown / connection close) fires.
func (h *Hub) emit(ev Event, escape <-chan struct{}) {
	select {
	case h.Events <- ev:
	case <-escape:
	}
}

// Online reports slot liveness for one orbit (for /status texts).
func (h *Hub) Online(orbit int64) map[protocol.NodeID]bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := map[protocol.NodeID]bool{}
	for key, on := range h.online {
		if key.Orbit == orbit {
			out[key.Slot] = on
		}
	}
	return out
}

// Stats: totals for /healthz.
func (h *Hub) Stats() (connected int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, on := range h.online {
		if on {
			connected++
		}
	}
	return
}

// NodeSnapshots returns exact current connection capability sets and liveness
// timestamps without exposing mutable hub maps. Unknown future capability
// names remain present in the immutable CapabilitySet value.
func (h *Hub) NodeSnapshots() map[NodeKey]NodeSnapshot {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make(map[NodeKey]NodeSnapshot, len(h.lastSeen))
	for key, seen := range h.lastSeen {
		connection, connected := h.conns[key]
		credentialTokenHash := ""
		if connection != nil {
			credentialTokenHash = connection.credentialTokenHash
		}
		rttSampledAt := int64(0)
		if sampledAt := h.rttSampledAt[key]; !sampledAt.IsZero() {
			rttSampledAt = sampledAt.UnixMilli()
		}
		out[key] = NodeSnapshot{
			Connected:           connected,
			LastSeenAt:          seen.UnixMilli(),
			Capabilities:        h.capabilities[key],
			PlaybackState:       h.playbackState[key],
			OutputDegraded:      h.outputDegraded[key],
			CaptureQuality:      protocol.CloneCaptureQualityState(h.captureQuality[key]),
			CredentialTokenHash: credentialTokenHash,
			RTTMS:               h.rttMS[key],
			RTTSampledAt:        rttSampledAt,
		}
	}
	return out
}

// Send wraps payload into an envelope and queues it to the node.
// Returns false if the node has no live connection (spec 8.6: no delivery
// guarantee; confirmation loops live in the session layer).
func (h *Hub) Send(key NodeKey, msgType string, payload any) bool {
	env, err := protocol.NewEnvelope(ulid.NewMessageID(time.Now()), time.Now().UnixMilli(), msgType, payload)
	if err != nil {
		h.log.Error("encode outgoing", "type", msgType, "err", err)
		return false
	}
	h.mu.Lock()
	c := h.conns[key]
	h.mu.Unlock()
	if c == nil {
		return false
	}
	select {
	case c.send <- outboundFrame{envelope: &env}:
		return true
	case <-c.stop:
		return false
	}
}

// TrySendBinary queues one already-validated live frame without blocking.
// Each connection has an isolated fixed-capacity queue; a slow receiver can
// therefore be failed independently instead of retaining audio or delaying
// healthy peers.
func (h *Hub) TrySendBinary(key NodeKey, raw []byte) bool {
	if len(raw) < protocol.LivePTTFrameHeaderBytes || len(raw) > protocol.LivePTTMaxMessageBytes {
		return false
	}
	if _, err := protocol.DecodeLivePTTBinaryFrame(raw); err != nil {
		return false
	}
	h.mu.Lock()
	c := h.conns[key]
	h.mu.Unlock()
	if c == nil {
		return false
	}
	frame := outboundFrame{binary: append([]byte(nil), raw...)}
	select {
	case c.send <- frame:
		return true
	case <-c.stop:
		return false
	default:
		return false
	}
}

// Disconnect closes the exact live node generation after a canonical
// credential revocation. It atomically removes the generation from snapshots,
// emits the offline edge, and is idempotent for repeated enforcement.
func (h *Hub) Disconnect(key NodeKey) bool {
	h.mu.Lock()
	c := h.conns[key]
	if c != nil {
		delete(h.conns, key)
		h.online[key] = false
		delete(h.capabilities, key)
		delete(h.rttMS, key)
		delete(h.rttSampledAt, key)
		delete(h.captureQuality, key)
	}
	h.mu.Unlock()
	if c == nil {
		return false
	}
	h.emit(EvOffline{Key: key}, c.stop)
	_ = c.ws.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(closeRevokedAuth, "credential revoked"),
		time.Now().Add(time.Second),
	)
	c.close()
	return true
}

var upgrader = websocket.Upgrader{
	// Pulsar is a native client; Apple and Windows WebSocket stacks may attach
	// an Origin even though no cookie authority exists. The bearer is presented
	// only in the bounded first protocol frame, never ambient HTTP state.
	CheckOrigin: func(*http.Request) bool { return true },
}

// HandleWS is the /ws endpoint.
func (h *Hub) HandleWS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || r.URL.Path != "/ws" || r.URL.RawQuery != "" ||
		r.ContentLength != 0 || len(r.TransferEncoding) != 0 ||
		len(r.Header.Values("Authorization")) != 0 {
		http.Error(w, "invalid websocket request", http.StatusBadRequest)
		return
	}
	select {
	case h.registerSlots <- struct{}{}:
	case <-r.Context().Done():
		return
	default:
		http.Error(w, "registration capacity unavailable", http.StatusServiceUnavailable)
		return
	}
	releaseRegistration := func() { <-h.registerSlots }
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		releaseRegistration()
		h.log.Warn("ws upgrade failed", "err", err)
		return
	}
	// L9: the endpoint is public in prod and gorilla defaults to UNLIMITED
	// frame size — an unauthenticated client could post a multi-GB frame
	// before token validation. Node messages are tiny; 64 KiB is generous.
	ws.SetReadLimit(64 << 10)

	key, reg, capabilities, ok := h.awaitRegister(ws)
	releaseRegistration()
	if !ok {
		return // awaitRegister closed the socket
	}

	tokenDigest := sha256.Sum256([]byte(reg.Token))
	c := &conn{
		ws: ws, send: make(chan outboundFrame, 32), stop: make(chan struct{}),
		credentialTokenHash: hex.EncodeToString(tokenDigest[:]),
		capabilities:        capabilities,
	}

	h.mu.Lock()
	if old := h.conns[key]; old != nil {
		h.log.Info("replacing connection (last-write-wins)", "orbit", key.Orbit, "slot", key.Slot)
		old.close()
	}
	h.conns[key] = c
	h.lastSeen[key] = time.Now()
	h.capabilities[key] = capabilities
	delete(h.rttMS, key)
	delete(h.rttSampledAt, key)
	delete(h.playbackState, key)
	delete(h.outputDegraded, key)
	delete(h.captureQuality, key)
	wasOnline := h.online[key]
	h.online[key] = true
	h.mu.Unlock()

	h.Events <- EvRegistered{
		Key: key, AppVersion: reg.AppVersion, LibrespotVersion: reg.LibrespotVersion,
		Capabilities: capabilities,
	}
	if !wasOnline {
		h.Events <- EvOnline{Key: key}
	}

	go h.writer(key, c)
	h.reader(key, c)
}

func (h *Hub) awaitRegister(ws *websocket.Conn) (NodeKey, *protocol.RegisterPayload, protocol.CapabilitySet, bool) {
	ws.SetReadDeadline(time.Now().Add(registerDeadline))
	messageType, raw, err := ws.ReadMessage()
	if err != nil || messageType != websocket.TextMessage {
		ws.Close()
		return NodeKey{}, nil, protocol.CapabilitySet{}, false
	}
	var env protocol.Envelope
	if err := json.Unmarshal(raw, &env); err != nil || env.Type != protocol.TypeRegister {
		h.closeWithCode(ws, closeInvalidAuth, "first message must be register")
		return NodeKey{}, nil, protocol.CapabilitySet{}, false
	}
	payload, err := protocol.DecodePayload(env)
	if err != nil {
		h.closeWithCode(ws, closeInvalidAuth, "malformed register")
		return NodeKey{}, nil, protocol.CapabilitySet{}, false
	}
	reg := payload.(*protocol.RegisterPayload)
	capabilities, err := protocol.ParseCapabilitySet(reg.Capabilities)
	if err != nil {
		h.closeWithCode(ws, closeInvalidAuth, "malformed register")
		return NodeKey{}, nil, protocol.CapabilitySet{}, false
	}
	orbitID, slot, ok := h.lookup(reg.Token)
	if !ok {
		h.log.Warn("register rejected", "claimed_slot", reg.NodeID)
		h.closeWithCode(ws, closeInvalidAuth, "invalid token")
		return NodeKey{}, nil, protocol.CapabilitySet{}, false
	}
	if reg.NodeID != slot {
		// The token decides; a stale config claiming another slot is noted.
		h.log.Warn("register slot mismatch, token wins", "claimed", reg.NodeID, "actual", slot)
	}
	ws.SetReadDeadline(time.Time{})
	return NodeKey{Orbit: orbitID, Slot: protocol.NodeID(slot)}, reg, capabilities, true
}

func (h *Hub) closeWithCode(ws *websocket.Conn, code int, text string) {
	msg := websocket.FormatCloseMessage(code, text)
	ws.WriteControl(websocket.CloseMessage, msg, time.Now().Add(time.Second))
	ws.Close()
}

func (h *Hub) writer(key NodeKey, c *conn) {
	for {
		select {
		case <-c.stop:
			return
		case frame := <-c.send:
			c.ws.SetWriteDeadline(time.Now().Add(writeDeadline))
			var err error
			if frame.envelope != nil {
				err = c.ws.WriteJSON(*frame.envelope)
			} else {
				err = c.ws.WriteMessage(websocket.BinaryMessage, frame.binary)
			}
			if err != nil {
				h.log.Warn("write failed", "orbit", key.Orbit, "slot", key.Slot, "err", err)
				c.close()
				return
			}
			if frame.envelope != nil {
				h.log.Debug("sent", "orbit", key.Orbit, "slot", key.Slot,
					"type", frame.envelope.Type, "id", frame.envelope.ID)
			}
		}
	}
}

func (h *Hub) reader(key NodeKey, c *conn) {
	defer func() {
		c.close()
		h.mu.Lock()
		if h.conns[key] == c {
			delete(h.conns, key)
		}
		h.mu.Unlock()
	}()
	for {
		messageType, raw, err := c.ws.ReadMessage()
		if err != nil {
			select {
			case <-c.stop: // replaced by a newer connection: not a liveness signal
			default:
				h.log.Info("connection lost", "orbit", key.Orbit, "slot", key.Slot, "err", err)
			}
			return
		}
		t2 := time.Now().UnixMilli()
		if messageType == websocket.BinaryMessage {
			frame, decodeErr := protocol.DecodeLivePTTBinaryFrame(raw)
			if decodeErr != nil {
				h.log.Warn("rejected bounded binary frame", "reason", "invalid_frame",
					"bytes", len(raw))
				continue
			}
			h.mu.Lock()
			if h.conns[key] != c {
				h.mu.Unlock()
				return
			}
			h.lastSeen[key] = time.Now()
			cameBack := !h.online[key]
			h.online[key] = true
			h.mu.Unlock()
			if cameBack {
				h.emit(EvOnline{Key: key}, c.stop)
			}
			select {
			case h.Events <- EvBinary{Key: key, CredentialTokenHash: c.credentialTokenHash,
				ReceivedAtMS: t2, Frame: frame, Raw: append([]byte(nil), raw...)}:
			case <-c.stop:
				return
			}
			continue
		}
		if messageType != websocket.TextMessage {
			continue
		}

		var env protocol.Envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			h.log.Warn("bad frame", "slot", key.Slot, "err", err)
			continue
		}
		if env.V != protocol.Version {
			h.log.Warn("protocol version mismatch; disconnecting", "slot", key.Slot,
				"got", env.V, "want", protocol.Version)
			return
		}
		if !protocol.KnownType(env.Type) {
			h.log.Warn("unknown message type ignored", "slot", key.Slot, "type", env.Type) // spec 8.6
			continue
		}
		payload, err := protocol.DecodePayload(env)
		if err != nil {
			h.log.Warn("bad payload", "slot", key.Slot, "type", env.Type, "err", err)
			continue
		}
		if state, ok := payload.(*protocol.StatePayload); ok && state.CaptureQuality != nil {
			if !c.capabilities.Supports(protocol.CapabilityCaptureQuality) {
				h.log.Warn("capture quality state rejected", "reason", "capability_missing")
				continue
			}
			result := c.captureQualityGuard.Accept(
				state.CaptureQuality.Generation, state.CaptureQuality.UpdatedMonotonicMS)
			if result == protocol.CaptureQualityStale || result == protocol.CaptureQualityInvalid {
				h.log.Warn("capture quality state rejected", "reason", result)
				continue
			}
		}

		h.mu.Lock()
		if h.conns[key] != c {
			// A last-write-wins reconnect may race a frame already read from the
			// predecessor socket. That frame must not repopulate liveness or RTT
			// evidence after the new authenticated generation cleared it.
			h.mu.Unlock()
			return
		}
		receivedAt := time.Now()
		h.lastSeen[key] = receivedAt
		if state, ok := payload.(*protocol.StatePayload); ok {
			h.rttMS[key] = state.RTTMS
			h.rttSampledAt[key] = receivedAt
			h.playbackState[key] = presencePlaybackState(state.Playback)
			h.outputDegraded[key] = state.Degraded
			h.captureQuality[key] = protocol.CloneCaptureQualityState(state.CaptureQuality)
		}
		cameBack := !h.online[key]
		h.online[key] = true
		h.mu.Unlock()
		if state, ok := payload.(*protocol.StatePayload); ok && state.CaptureQuality != nil &&
			(state.CaptureQuality.Quality != protocol.CaptureQualityAccepted ||
				state.CaptureQuality.InputHealth != protocol.CaptureHealthOK) {
			h.log.Info("capture quality diagnostic",
				"workflow", state.CaptureQuality.Workflow,
				"quality", state.CaptureQuality.Quality,
				"reason", state.CaptureQuality.Reason,
				"input_health", state.CaptureQuality.InputHealth)
		}
		if cameBack {
			// Reliable, outside the lock (bugs #3): the old best-effort drop
			// could strand a live node "offline" in the FSM after a burst.
			h.emit(EvOnline{Key: key}, c.stop)
		}

		// Protocol clock-sync ping is answered inline: t2/t3 accuracy matters
		// (spec 8.5), the session loop must not delay it.
		if ping, isPing := payload.(*protocol.PingPayload); isPing {
			pong := protocol.PongPayload{T1: ping.T1, T2: t2, T3: time.Now().UnixMilli()}
			env, err := protocol.NewEnvelope(ulid.NewMessageID(time.Now()), pong.T3, protocol.TypePong, &pong)
			if err == nil {
				select {
				case c.send <- outboundFrame{envelope: &env}:
				case <-c.stop:
					return
				}
			}
			continue
		}

		h.log.Debug("received", "orbit", key.Orbit, "slot", key.Slot, "type", env.Type, "id", env.ID)
		select {
		case h.Events <- EvMessage{
			Key: key, CredentialTokenHash: c.credentialTokenHash,
			Env: env, Payload: payload,
		}:
		case <-c.stop:
			return
		}
	}
}
