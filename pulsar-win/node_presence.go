// Durable node-local DND intent and the last privacy-bounded presence
// projection. The persisted wire types contain no microphone, device, token,
// media URL, local path or audio-content fields.
package main

import (
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sync"

	protocol "relux.works/duet/pulsar-win/wire"
)

var (
	ErrInvalidLocalDND     = errors.New("local DND value is invalid")
	ErrPresencePersistence = errors.New("node presence persistence failed")
)

const maximumLocalDNDMS int64 = 30 * 24 * 60 * 60 * 1000
const maximumNodePresenceStateBytes int64 = 1 << 20

type persistedNodePresence struct {
	LocalDND *protocol.SetDNDPayload         `json:"local_dnd,omitempty"`
	Presence *protocol.PresenceUpdatePayload `json:"presence,omitempty"`
}

type NodePresenceStore struct {
	path string
	log  *slog.Logger

	mu    sync.Mutex
	state persistedNodePresence
}

func NewNodePresenceStore(path string, log *slog.Logger) *NodePresenceStore {
	store := &NodePresenceStore{path: filepath.Clean(path), log: log}
	info, statErr := os.Stat(store.path)
	if statErr == nil && info.Size() > maximumNodePresenceStateBytes {
		log.Warn("node presence state ignored: file is oversized")
		return store
	}
	raw, err := os.ReadFile(store.path)
	if err == nil {
		if json.Unmarshal(raw, &store.state) != nil {
			store.state = persistedNodePresence{}
			log.Warn("node presence state ignored: invalid persisted JSON")
		} else {
			if !validPersistedLocalDND(store.state.LocalDND) {
				store.state.LocalDND = nil
			}
			if store.state.Presence != nil && store.state.Presence.Revision <= 0 {
				store.state.Presence = nil
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		log.Warn("node presence state unavailable")
	}
	return store
}

func validPersistedLocalDND(value *protocol.SetDNDPayload) bool {
	if value == nil {
		return true
	}
	if value.Revision <= 0 {
		return false
	}
	switch value.Mode {
	case "allow_all", "messages_only":
		return value.MutedUntilCoordMS == nil
	case "muted_until":
		return value.MutedUntilCoordMS != nil
	default:
		return false
	}
}

// NextLocalDND persists the new monotonic revision before returning it, so a
// caller never announces an intent that would disappear on process restart.
func (s *NodePresenceStore) NextLocalDND(mode string, mutedUntilCoordMS *int64, coordinatorNowMS int64) (*protocol.SetDNDPayload, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch mode {
	case "allow_all", "messages_only":
		if mutedUntilCoordMS != nil {
			return nil, ErrInvalidLocalDND
		}
	case "muted_until":
		if mutedUntilCoordMS == nil || *mutedUntilCoordMS <= coordinatorNowMS ||
			*mutedUntilCoordMS-coordinatorNowMS > maximumLocalDNDMS {
			return nil, ErrInvalidLocalDND
		}
	default:
		return nil, ErrInvalidLocalDND
	}

	revision := int64(1)
	if s.state.LocalDND != nil && s.state.LocalDND.Revision >= 0 {
		if s.state.LocalDND.Revision == math.MaxInt64 {
			return nil, ErrPresencePersistence
		}
		revision = s.state.LocalDND.Revision + 1
	}
	payload := &protocol.SetDNDPayload{
		Revision: revision, Mode: mode, MutedUntilCoordMS: cloneInt64(mutedUntilCoordMS),
	}
	previous := s.state.LocalDND
	s.state.LocalDND = payload
	if err := s.writeLocked(); err != nil {
		s.state.LocalDND = previous
		return nil, ErrPresencePersistence
	}
	return cloneSetDND(payload), nil
}

// AcceptPresence applies only monotonic projections. An equal revision is an
// idempotent resend only when the entire typed body is identical.
func (s *NodePresenceStore) AcceptPresence(update *protocol.PresenceUpdatePayload) bool {
	if update == nil || update.Revision <= 0 {
		return false
	}
	copyUpdate := clonePresenceUpdate(update)
	s.mu.Lock()
	defer s.mu.Unlock()
	if current := s.state.Presence; current != nil {
		if copyUpdate.Revision < current.Revision {
			return false
		}
		if copyUpdate.Revision == current.Revision {
			return reflect.DeepEqual(copyUpdate, current)
		}
	}
	previous := s.state.Presence
	s.state.Presence = copyUpdate
	if err := s.writeLocked(); err != nil {
		s.state.Presence = previous
		s.log.Warn("presence state persistence failed")
		return false
	}
	return true
}

func (s *NodePresenceStore) CurrentLocalDND() *protocol.SetDNDPayload {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneSetDND(s.state.LocalDND)
}

func (s *NodePresenceStore) LatestPresence() *protocol.PresenceUpdatePayload {
	s.mu.Lock()
	defer s.mu.Unlock()
	return clonePresenceUpdate(s.state.Presence)
}

func (s *NodePresenceStore) writeLocked() error {
	parent := filepath.Dir(s.path)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	temporary, err := os.CreateTemp(parent, ".node-presence-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	remove := true
	defer func() {
		_ = temporary.Close()
		if remove {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return err
	}
	if _, err := temporary.Write(raw); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := replaceStateFile(temporaryPath, s.path); err != nil {
		return err
	}
	remove = false
	_ = os.Chmod(s.path, 0o600)
	return nil
}

func cloneSetDND(value *protocol.SetDNDPayload) *protocol.SetDNDPayload {
	if value == nil {
		return nil
	}
	copyValue := *value
	copyValue.MutedUntilCoordMS = cloneInt64(value.MutedUntilCoordMS)
	return &copyValue
}

func clonePresenceUpdate(value *protocol.PresenceUpdatePayload) *protocol.PresenceUpdatePayload {
	if value == nil {
		return nil
	}
	copyValue := *value
	copyValue.Nodes = make([]protocol.PresenceNode, len(value.Nodes))
	for index, node := range value.Nodes {
		copyValue.Nodes[index] = node
		copyValue.Nodes[index].DNDUntilCoordMS = cloneInt64(node.DNDUntilCoordMS)
		copyValue.Nodes[index].Capabilities = append([]string(nil), node.Capabilities...)
	}
	return &copyValue
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}
