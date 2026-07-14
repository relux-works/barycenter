package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	protocol "relux.works/duet/pulsar-win/wire"
)

func testPresenceUpdate(revision int64) *protocol.PresenceUpdatePayload {
	until := int64(2_000)
	return &protocol.PresenceUpdatePayload{
		Revision: revision, GeneratedAtCoordMS: 1_100,
		Nodes: []protocol.PresenceNode{{
			OrbitID: 42, Slot: "a", Online: true, LastSeenAtCoordMS: 1_099,
			OutputState: "ready", PlaybackState: "main", DNDMode: "muted_until",
			DNDRevision: 2, DNDUntilCoordMS: &until,
			Capabilities: []string{protocol.CapabilityMediaClip}, InterruptResumeReady: false,
		}},
	}
}

func TestNodePresenceStorePersistsDNDAndPrivacyBoundedProjection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "node-presence.v1.json")
	store := NewNodePresenceStore(path, testLogger())
	first, err := store.NextLocalDND("messages_only", nil, 1_000)
	if err != nil {
		t.Fatal(err)
	}
	until := int64(2_000)
	second, err := store.NextLocalDND("muted_until", &until, 1_000)
	if err != nil {
		t.Fatal(err)
	}
	if first.Revision != 1 || second.Revision != 2 {
		t.Fatalf("DND revisions %d, %d", first.Revision, second.Revision)
	}
	presence := testPresenceUpdate(9)
	if !store.AcceptPresence(presence) {
		t.Fatal("presence update rejected")
	}

	reloaded := NewNodePresenceStore(path, testLogger())
	if !reflect.DeepEqual(reloaded.CurrentLocalDND(), second) || !reflect.DeepEqual(reloaded.LatestPresence(), presence) {
		t.Fatalf("reloaded DND=%+v presence=%+v", reloaded.CurrentLocalDND(), reloaded.LatestPresence())
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"microphone", "audio_level", "token", "local_path", "media_url"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("persisted state contains forbidden field %q: %s", forbidden, raw)
		}
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("state permissions too broad: info=%v err=%v", info, err)
	}
}

func TestNodePresenceStoreRejectsInvalidDNDWithoutAdvancingRevision(t *testing.T) {
	store := NewNodePresenceStore(filepath.Join(t.TempDir(), "state.json"), testLogger())
	if _, err := store.NextLocalDND("muted_until", nil, 1_000); !errors.Is(err, ErrInvalidLocalDND) {
		t.Fatalf("missing until error %v", err)
	}
	tooLate := int64(1_000 + maximumLocalDNDMS + 1)
	if _, err := store.NextLocalDND("muted_until", &tooLate, 1_000); !errors.Is(err, ErrInvalidLocalDND) {
		t.Fatalf("overlong mute error %v", err)
	}
	valid, err := store.NextLocalDND("allow_all", nil, 1_000)
	if err != nil {
		t.Fatal(err)
	}
	if valid.Revision != 1 {
		t.Fatalf("revision %d, want 1", valid.Revision)
	}
}

func TestNodePresenceStoreRequiresMonotonicIdempotentProjection(t *testing.T) {
	store := NewNodePresenceStore(filepath.Join(t.TempDir(), "state.json"), testLogger())
	current := testPresenceUpdate(4)
	if !store.AcceptPresence(current) || !store.AcceptPresence(current) {
		t.Fatal("new and identical same-revision presence must be accepted")
	}
	older := testPresenceUpdate(3)
	if store.AcceptPresence(older) {
		t.Fatal("older presence accepted")
	}
	conflict := testPresenceUpdate(4)
	conflict.GeneratedAtCoordMS++
	if store.AcceptPresence(conflict) {
		t.Fatal("conflicting same-revision presence accepted")
	}
	newer := testPresenceUpdate(5)
	if !store.AcceptPresence(newer) || !reflect.DeepEqual(store.LatestPresence(), newer) {
		t.Fatal("newer presence not committed")
	}
}

func TestNodePresenceStoreReturnsDefensiveCopies(t *testing.T) {
	store := NewNodePresenceStore(filepath.Join(t.TempDir(), "state.json"), testLogger())
	until := int64(2_000)
	dnd, err := store.NextLocalDND("muted_until", &until, 1_000)
	if err != nil {
		t.Fatal(err)
	}
	dnd.Mode = "mutated"
	*dnd.MutedUntilCoordMS = 9_999
	if current := store.CurrentLocalDND(); current.Mode != "muted_until" || *current.MutedUntilCoordMS != 2_000 {
		t.Fatalf("caller mutated stored DND: %+v", current)
	}
	presence := testPresenceUpdate(1)
	if !store.AcceptPresence(presence) {
		t.Fatal("presence rejected")
	}
	presence.Nodes[0].Capabilities[0] = "mutated"
	if got := store.LatestPresence().Nodes[0].Capabilities[0]; got != protocol.CapabilityMediaClip {
		t.Fatalf("caller mutated stored presence capability %q", got)
	}
}
