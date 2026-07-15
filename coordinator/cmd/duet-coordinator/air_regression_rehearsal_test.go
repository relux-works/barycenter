package main

import (
	"encoding/json"
	"fmt"
	"testing"

	"relux.works/duet/coordinator/internal/hub"
	"relux.works/duet/coordinator/internal/protocol"
	"relux.works/duet/coordinator/internal/store"
)

type airRegressionMetrics struct {
	Barycenters       int `json:"barycenters"`
	Pulsars           int `json:"pulsars"`
	LoadCommands      int `json:"load_commands"`
	UniqueTargets     int `json:"unique_targets"`
	DuplicateCommands int `json:"duplicate_commands"`
	RuntimeInstances  int `json:"runtime_instances"`
	LegacyGroups      int `json:"legacy_groups"`
}

func TestAirRegressionEightBarycentersTwentyPulsarsExactFanout(t *testing.T) {
	l, fake := newTestLoop(t)
	fake.online = map[int64]map[protocol.NodeID]bool{
		1: {protocol.NodeA: true, protocol.NodeB: true},
	}

	const firstUserID int64 = 8800
	orbits := []int64{1}
	slotCounts := []int{3, 3, 3, 3, 2, 2, 2}
	for index, slotCount := range slotCounts {
		userID := firstUserID + int64(index)
		orbit, err := l.st.CreateOrbit(fmt.Sprintf("Capacity Barycenter %d", index+2), userID)
		if err != nil {
			t.Fatal(err)
		}
		orbits = append(orbits, orbit.ID)
		fake.online[orbit.ID] = map[protocol.NodeID]bool{}
		for slotIndex := 0; slotIndex < slotCount; slotIndex++ {
			slot, _, err := l.st.PairSlot(orbit.ID, userID)
			if err != nil {
				t.Fatal(err)
			}
			fake.online[orbit.ID][protocol.NodeID(slot)] = true
		}
	}

	if _, err := l.st.CutoverLinksToAirs(1, 100); err != nil {
		t.Fatal(err)
	}
	air, err := l.st.CreateAir(store.CreateAirParams{
		Title: "Eight by twenty rehearsal", OwnerOrbitID: 1, CreatedAt: 110,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := l.st.ActivateAir(1, air.ID, "none", 120); err != nil {
		t.Fatal(err)
	}
	for index, orbitID := range orbits[1:] {
		member, err := l.st.AddPendingAirMember(air.ID, orbitID, "member", int64(130+index))
		if err != nil {
			t.Fatal(err)
		}
		if err := l.st.ConfirmAirMember(member.ID, 1, true, "none", int64(140+index)); err != nil {
			t.Fatal(err)
		}
	}

	runtime := l.stateFor(1)
	if runtime.airID != air.ID || len(runtime.orbits) != 8 || len(runtime.sess.Peers) != 20 {
		t.Fatalf("runtime air=%q orbits=%v peers=%v", runtime.airID, runtime.orbits, runtime.sess.Peers)
	}
	for _, orbitID := range orbits {
		if l.stateFor(orbitID) != runtime {
			t.Fatalf("orbit %d escaped the exact current Air runtime", orbitID)
		}
	}

	fake.drain()
	l.handleBot(cmdEvent(t, "a", link, &replies{}))
	loads := fake.ofType(protocol.TypeLoad)
	unique := make(map[hub.NodeKey]struct{}, len(loads))
	for _, load := range loads {
		unique[load.key] = struct{}{}
	}
	metrics := airRegressionMetrics{
		Barycenters: 8, Pulsars: len(runtime.sess.Peers), LoadCommands: len(loads),
		UniqueTargets: len(unique), DuplicateCommands: len(loads) - len(unique),
		RuntimeInstances: len(l.airs), LegacyGroups: len(l.groups),
	}
	if metrics.Pulsars != 20 || metrics.LoadCommands != 20 || metrics.UniqueTargets != 20 ||
		metrics.DuplicateCommands != 0 || metrics.RuntimeInstances != 1 || metrics.LegacyGroups != 0 ||
		len(l.linkOf) != 0 {
		t.Fatalf("capacity metrics=%+v legacy_links=%v sent=%+v", metrics, l.linkOf, loads)
	}
	evidence, err := json.Marshal(metrics)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("AIR_REGRESSION_METRICS %s", evidence)
}
