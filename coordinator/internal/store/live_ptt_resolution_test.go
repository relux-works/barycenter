package store

import (
	"errors"
	"testing"
	"time"
)

func TestResolveLivePTTTargetsProvesSocketAndSealsOnlyCapablePeer(t *testing.T) {
	st, source := newMediaIngestTestStore(t)
	companion := addTransmissionInstallation(t, st, source, "companion")
	now := time.Now().UnixMilli()
	availability := []LivePTTAvailability{
		{OrbitID: source.OrbitID, Slot: source.Slot, Connected: true, LastSeenAt: now, CredentialTokenHash: hashToken(source.NodeToken), SupportsLivePTT: true},
		{OrbitID: companion.OrbitID, Slot: companion.Slot, Connected: true, LastSeenAt: now, CredentialTokenHash: hashToken(companion.NodeToken), SupportsLivePTT: true},
	}
	resolution, err := st.ResolveLivePTTTargets(source.OrbitID, source.Slot, hashToken(source.NodeToken), availability, now)
	if err != nil {
		t.Fatal(err)
	}
	if resolution.SourceActorID != source.ActorID || resolution.DomainKind != "barycenter" || resolution.DomainID != source.OrbitID || len(resolution.Targets) != 1 || resolution.Targets[0].Slot != companion.Slot || len(resolution.Excluded) != 0 {
		t.Fatalf("resolution=%+v", resolution)
	}
	availability[1].SupportsLivePTT = false
	unsupported, err := st.ResolveLivePTTTargets(source.OrbitID, source.Slot, hashToken(source.NodeToken), availability, now)
	if err != nil || len(unsupported.Targets) != 0 || len(unsupported.Excluded) != 1 || unsupported.Excluded[0].Reason != "unsupported" {
		t.Fatalf("unsupported-only=%+v err=%v", unsupported, err)
	}
	if _, err := st.ResolveLivePTTTargets(source.OrbitID, source.Slot, "0000000000000000000000000000000000000000000000000000000000000000", availability, now); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("stale socket err=%v", err)
	}
}

func TestResolveLivePTTTargetsEnforcesRecipientDND(t *testing.T) {
	st, source := newMediaIngestTestStore(t)
	companion := addTransmissionInstallation(t, st, source, "companion")
	now := time.Now().UnixMilli()
	if _, err := st.SetNodeDND(SetNodeDNDParams{OrbitID: companion.OrbitID,
		ActorID: companion.ActorID, Slot: companion.Slot, Mode: DNDMutedUntil,
		MutedUntil: now + 60000, Revision: 1, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	availability := []LivePTTAvailability{{OrbitID: source.OrbitID, Slot: source.Slot, Connected: true, LastSeenAt: now, CredentialTokenHash: hashToken(source.NodeToken), SupportsLivePTT: true}, {OrbitID: companion.OrbitID, Slot: companion.Slot, Connected: true, LastSeenAt: now, CredentialTokenHash: hashToken(companion.NodeToken), SupportsLivePTT: true}}
	resolution, err := st.ResolveLivePTTTargets(source.OrbitID, source.Slot, hashToken(source.NodeToken), availability, now+1)
	if err != nil || len(resolution.Targets) != 0 || len(resolution.Excluded) != 1 || resolution.Excluded[0].Reason != "dnd" {
		t.Fatalf("DND live target=%+v err=%v", resolution, err)
	}
}
