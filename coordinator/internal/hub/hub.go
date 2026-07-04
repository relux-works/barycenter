// Package hub is the WebSocket server for the two nodes (spec 7.1 ws-hub):
// token auth on register, connection registry with last-write-wins, protocol
// ping->pong timestamping (spec 8.5), heartbeat-based offline detection.
package hub

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"relux.works/duet/coordinator/internal/protocol"
	"relux.works/duet/coordinator/internal/ulid"
)

const (
	registerDeadline = 5 * time.Second
	writeDeadline    = 10 * time.Second
	closeInvalidAuth = 4401 // spec 8.2
)

type Event any

type (
	EvRegistered struct {
		Node             protocol.NodeID
		AppVersion       string
		LibrespotVersion string
	}
	EvOnline  struct{ Node protocol.NodeID }
	EvOffline struct{ Node protocol.NodeID }
	EvMessage struct {
		Node    protocol.NodeID
		Env     protocol.Envelope
		Payload any
	}
)

type conn struct {
	ws   *websocket.Conn
	send chan protocol.Envelope
	stop chan struct{}
	once sync.Once
}

func (c *conn) close() {
	c.once.Do(func() {
		close(c.stop)
		c.ws.Close()
	})
}

type Hub struct {
	log          *slog.Logger
	tokens       map[protocol.NodeID]string
	offlineAfter time.Duration

	Events chan Event

	mu       sync.Mutex
	conns    map[protocol.NodeID]*conn
	lastSeen map[protocol.NodeID]time.Time
	online   map[protocol.NodeID]bool
}

func New(log *slog.Logger, tokens map[protocol.NodeID]string, offlineAfter time.Duration) *Hub {
	return &Hub{
		log:          log,
		tokens:       tokens,
		offlineAfter: offlineAfter,
		Events:       make(chan Event, 64),
		conns:        map[protocol.NodeID]*conn{},
		lastSeen:     map[protocol.NodeID]time.Time{},
		online:       map[protocol.NodeID]bool{},
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
			h.mu.Lock()
			for node, seen := range h.lastSeen {
				if h.online[node] && now.Sub(seen) > h.offlineAfter {
					h.online[node] = false
					h.emitLocked(EvOffline{Node: node})
				}
			}
			h.mu.Unlock()
		}
	}
}

func (h *Hub) emitLocked(ev Event) {
	select {
	case h.Events <- ev:
	default:
		h.log.Warn("event channel full, dropping", "event", fmt.Sprintf("%T", ev))
	}
}

// Online reports current node liveness (for /healthz and /status).
func (h *Hub) Online() map[protocol.NodeID]bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := map[protocol.NodeID]bool{}
	for n := range h.tokens {
		out[n] = h.online[n]
	}
	return out
}

// Send wraps payload into an envelope and queues it to the node.
// Returns false if the node has no live connection (spec 8.6: no delivery
// guarantee; confirmation loops live in the session layer).
func (h *Hub) Send(node protocol.NodeID, msgType string, payload any) bool {
	env, err := protocol.NewEnvelope(ulid.NewMessageID(time.Now()), time.Now().UnixMilli(), msgType, payload)
	if err != nil {
		h.log.Error("encode outgoing", "type", msgType, "err", err)
		return false
	}
	h.mu.Lock()
	c := h.conns[node]
	h.mu.Unlock()
	if c == nil {
		return false
	}
	select {
	case c.send <- env:
		return true
	case <-c.stop:
		return false
	}
}

var upgrader = websocket.Upgrader{
	// The listener binds to the tailnet address only (spec 17); origin checks
	// are meaningless for node clients.
	CheckOrigin: func(*http.Request) bool { return true },
}

// HandleWS is the /ws endpoint.
func (h *Hub) HandleWS(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.log.Warn("ws upgrade failed", "err", err)
		return
	}

	node, reg, ok := h.awaitRegister(ws)
	if !ok {
		return // awaitRegister closed the socket
	}

	c := &conn{ws: ws, send: make(chan protocol.Envelope, 32), stop: make(chan struct{})}

	h.mu.Lock()
	if old := h.conns[node]; old != nil {
		h.log.Info("replacing connection (last-write-wins)", "node", node)
		old.close()
	}
	h.conns[node] = c
	h.lastSeen[node] = time.Now()
	wasOnline := h.online[node]
	h.online[node] = true
	h.mu.Unlock()

	h.Events <- EvRegistered{Node: node, AppVersion: reg.AppVersion, LibrespotVersion: reg.LibrespotVersion}
	if !wasOnline {
		h.Events <- EvOnline{Node: node}
	}

	go h.writer(node, c)
	h.reader(node, c)
}

func (h *Hub) awaitRegister(ws *websocket.Conn) (protocol.NodeID, *protocol.RegisterPayload, bool) {
	ws.SetReadDeadline(time.Now().Add(registerDeadline))
	_, raw, err := ws.ReadMessage()
	if err != nil {
		ws.Close()
		return "", nil, false
	}
	var env protocol.Envelope
	if err := json.Unmarshal(raw, &env); err != nil || env.Type != protocol.TypeRegister {
		h.closeWithCode(ws, closeInvalidAuth, "first message must be register")
		return "", nil, false
	}
	payload, err := protocol.DecodePayload(env)
	if err != nil {
		h.closeWithCode(ws, closeInvalidAuth, "malformed register")
		return "", nil, false
	}
	reg := payload.(*protocol.RegisterPayload)
	node := protocol.NodeID(reg.NodeID)
	want, known := h.tokens[node]
	if !known || subtle.ConstantTimeCompare([]byte(want), []byte(reg.Token)) != 1 {
		h.log.Warn("register rejected", "node_id", reg.NodeID)
		h.closeWithCode(ws, closeInvalidAuth, "invalid token")
		return "", nil, false
	}
	ws.SetReadDeadline(time.Time{})
	return node, reg, true
}

func (h *Hub) closeWithCode(ws *websocket.Conn, code int, text string) {
	msg := websocket.FormatCloseMessage(code, text)
	ws.WriteControl(websocket.CloseMessage, msg, time.Now().Add(time.Second))
	ws.Close()
}

func (h *Hub) writer(node protocol.NodeID, c *conn) {
	for {
		select {
		case <-c.stop:
			return
		case env := <-c.send:
			c.ws.SetWriteDeadline(time.Now().Add(writeDeadline))
			if err := c.ws.WriteJSON(env); err != nil {
				h.log.Warn("write failed", "node", node, "err", err)
				c.close()
				return
			}
			h.log.Debug("sent", "node", node, "type", env.Type, "id", env.ID)
		}
	}
}

func (h *Hub) reader(node protocol.NodeID, c *conn) {
	defer func() {
		c.close()
		h.mu.Lock()
		if h.conns[node] == c {
			delete(h.conns, node)
		}
		h.mu.Unlock()
	}()
	for {
		_, raw, err := c.ws.ReadMessage()
		if err != nil {
			select {
			case <-c.stop: // replaced by a newer connection: not a liveness signal
			default:
				h.log.Info("connection lost", "node", node, "err", err)
			}
			return
		}
		t2 := time.Now().UnixMilli()

		var env protocol.Envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			h.log.Warn("bad frame", "node", node, "err", err)
			continue
		}
		if !protocol.KnownType(env.Type) {
			h.log.Warn("unknown message type ignored", "node", node, "type", env.Type) // spec 8.6
			continue
		}
		payload, err := protocol.DecodePayload(env)
		if err != nil {
			h.log.Warn("bad payload", "node", node, "type", env.Type, "err", err)
			continue
		}

		h.mu.Lock()
		h.lastSeen[node] = time.Now()
		cameBack := !h.online[node]
		h.online[node] = true
		if cameBack {
			h.emitLocked(EvOnline{Node: node})
		}
		h.mu.Unlock()

		// Protocol clock-sync ping is answered inline: t2/t3 accuracy matters
		// (spec 8.5), the session loop must not delay it.
		if ping, isPing := payload.(*protocol.PingPayload); isPing {
			pong := protocol.PongPayload{T1: ping.T1, T2: t2, T3: time.Now().UnixMilli()}
			env, err := protocol.NewEnvelope(ulid.NewMessageID(time.Now()), pong.T3, protocol.TypePong, &pong)
			if err == nil {
				select {
				case c.send <- env:
				case <-c.stop:
					return
				}
			}
			continue
		}

		h.log.Debug("received", "node", node, "type", env.Type, "id", env.ID)
		select {
		case h.Events <- EvMessage{Node: node, Env: env, Payload: payload}:
		case <-c.stop:
			return
		}
	}
}
