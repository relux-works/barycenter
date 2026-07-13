package store

import (
	"database/sql"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func blockingCheckpoint(stage string) (func(string) error, <-chan struct{}, func()) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	hook := func(name string) error {
		if name == stage {
			once.Do(func() { close(entered) })
			<-release
		}
		return nil
	}
	return hook, entered, func() { close(release) }
}

func assertOperationBlocked(t *testing.T, result <-chan error) {
	t.Helper()
	select {
	case err := <-result:
		t.Fatalf("serialized operation returned before barrier release: %v", err)
	case <-time.After(75 * time.Millisecond):
	}
}

func assertCoherentOrbitProjection(t *testing.T, s *Store, orbitID int64, wantExists bool) {
	t.Helper()
	var orbitCount int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM orbits WHERE id = ?`, orbitID).Scan(&orbitCount); err != nil {
		t.Fatal(err)
	}
	if !wantExists {
		if orbitCount != 0 {
			t.Fatalf("orbit %d still exists", orbitID)
		}
		for _, tableAndColumn := range [][2]string{
			{"members", "orbit_id"},
			{"slots", "orbit_id"},
			{"memberships", "orbit_id"},
			{"installation_credentials", "slot_orbit_id"},
		} {
			var count int
			if err := s.db.QueryRow(`SELECT COUNT(*) FROM `+tableAndColumn[0]+` WHERE `+tableAndColumn[1]+` = ?`, orbitID).Scan(&count); err != nil || count != 0 {
				t.Fatalf("dissolved %s count=%d err=%v", tableAndColumn[0], count, err)
			}
		}
		return
	}
	if orbitCount != 1 {
		t.Fatalf("orbit %d count=%d", orbitID, orbitCount)
	}
	var members, primaries, additive int
	if err := s.db.QueryRow(`SELECT COUNT(*), SUM(CASE WHEN role = 'primary' THEN 1 ELSE 0 END)
FROM members WHERE orbit_id = ?`, orbitID).Scan(&members, &primaries); err != nil {
		t.Fatal(err)
	}
	if members == 0 || primaries != 1 {
		t.Fatalf("active orbit members=%d primaries=%d", members, primaries)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*)
FROM memberships m JOIN actors a ON a.id = m.actor_id
WHERE m.orbit_id = ? AND m.left_at IS NULL AND a.kind = 'telegram_user'`, orbitID).Scan(&additive); err != nil {
		t.Fatal(err)
	}
	if additive != members {
		t.Fatalf("legacy/additive active member counts = %d/%d", members, additive)
	}
	var divergent int
	if err := s.db.QueryRow(`SELECT COUNT(*)
FROM members legacy
JOIN actors a ON a.kind = 'telegram_user' AND a.external_ref = CAST(legacy.tg_user_id AS TEXT)
JOIN memberships m ON m.actor_id = a.id AND m.orbit_id = legacy.orbit_id AND m.left_at IS NULL
WHERE legacy.orbit_id = ? AND legacy.role != m.role`, orbitID).Scan(&divergent); err != nil || divergent != 0 {
		t.Fatalf("role projection divergence=%d err=%v", divergent, err)
	}
}

// R5: two stores that both attempt to leave from a two-member orbit serialize
// under BEGIN IMMEDIATE; the second observes the first commit and dissolves.
func TestR5ConcurrentLeaveLeaveMaintainsOrbitInvariant(t *testing.T) {
	path := filepath.Join(t.TempDir(), "leave-leave.db")
	s1, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s1.Close()
	orbit, _ := s1.CreateOrbit("Leave leave", 101)
	if err := s1.AddMember(orbit.ID, 202, "companion"); err != nil {
		t.Fatal(err)
	}
	s2, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()

	hook, entered, release := blockingCheckpoint("leave_orbit_after_snapshot")
	s1.testCheckpoint = hook
	type leaveResult struct {
		dissolved bool
		err       error
	}
	first := make(chan leaveResult, 1)
	go func() {
		dissolved, _, err := s1.LeaveOrbit(orbit.ID, 101)
		first <- leaveResult{dissolved: dissolved, err: err}
	}()
	<-entered
	second := make(chan leaveResult, 1)
	secondErr := make(chan error, 1)
	go func() {
		dissolved, _, err := s2.LeaveOrbit(orbit.ID, 202)
		second <- leaveResult{dissolved: dissolved, err: err}
		secondErr <- err
	}()
	assertOperationBlocked(t, secondErr)
	release()
	r1, r2 := <-first, <-second
	if r1.err != nil || r2.err != nil || r1.dissolved || !r2.dissolved {
		t.Fatalf("leave results first=%+v second=%+v", r1, r2)
	}
	assertCoherentOrbitProjection(t, s1, orbit.ID, false)
}

// R5: both linearizations of leave versus TransferPrimary preserve one
// primary and never allow a stale pre-transaction role read.
func TestR5ConcurrentLeaveTransferPrimaryMaintainsOrbitInvariant(t *testing.T) {
	t.Run("leave companion commits first", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "leave-first.db")
		s1, _ := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
		defer s1.Close()
		orbit, _ := s1.CreateOrbit("Leave first", 101)
		_ = s1.AddMember(orbit.ID, 202, "companion")
		s2, _ := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
		defer s2.Close()
		hook, entered, release := blockingCheckpoint("leave_orbit_after_snapshot")
		s1.testCheckpoint = hook
		leaveResult := make(chan error, 1)
		go func() {
			_, _, err := s1.LeaveOrbit(orbit.ID, 202)
			leaveResult <- err
		}()
		<-entered
		transferResult := make(chan error, 1)
		go func() { transferResult <- s2.TransferPrimary(orbit.ID, 202) }()
		assertOperationBlocked(t, transferResult)
		release()
		if err := <-leaveResult; err != nil {
			t.Fatal(err)
		}
		if err := <-transferResult; err == nil {
			t.Fatal("transfer to the committed leaver unexpectedly succeeded")
		}
		assertCoherentOrbitProjection(t, s1, orbit.ID, true)
	})

	t.Run("transfer commits first", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "transfer-first.db")
		s1, _ := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
		defer s1.Close()
		orbit, _ := s1.CreateOrbit("Transfer first", 101)
		_ = s1.AddMember(orbit.ID, 202, "companion")
		s2, _ := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
		defer s2.Close()
		hook, entered, release := blockingCheckpoint("transfer_primary_after_begin")
		s1.testCheckpoint = hook
		transferResult := make(chan error, 1)
		go func() { transferResult <- s1.TransferPrimary(orbit.ID, 202) }()
		<-entered
		leaveResult := make(chan error, 1)
		go func() {
			_, _, err := s2.LeaveOrbit(orbit.ID, 202)
			leaveResult <- err
		}()
		assertOperationBlocked(t, leaveResult)
		release()
		if err := <-transferResult; err != nil {
			t.Fatal(err)
		}
		if err := <-leaveResult; err != nil {
			t.Fatal(err)
		}
		assertCoherentOrbitProjection(t, s1, orbit.ID, true)
	})
}

// R5: last-leave and AddMember are one serialized decision. Test both orders
// so neither an active zero-member orbit nor an accidental dissolve survives.
func TestR5ConcurrentLastLeaveAddMemberMaintainsOrbitInvariant(t *testing.T) {
	t.Run("last leave commits first", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "last-leave-first.db")
		s1, _ := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
		defer s1.Close()
		orbit, _ := s1.CreateOrbit("Last leave", 101)
		s2, _ := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
		defer s2.Close()
		hook, entered, release := blockingCheckpoint("leave_orbit_after_snapshot")
		s1.testCheckpoint = hook
		leaveResult := make(chan error, 1)
		go func() {
			_, _, err := s1.LeaveOrbit(orbit.ID, 101)
			leaveResult <- err
		}()
		<-entered
		addResult := make(chan error, 1)
		go func() { addResult <- s2.AddMember(orbit.ID, 202, "companion") }()
		assertOperationBlocked(t, addResult)
		release()
		if err := <-leaveResult; err != nil {
			t.Fatal(err)
		}
		if err := <-addResult; !errorsIsSQLNoRows(err) {
			t.Fatalf("add after dissolve error = %v", err)
		}
		assertCoherentOrbitProjection(t, s1, orbit.ID, false)
	})

	t.Run("add member commits first", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "add-first.db")
		s1, _ := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
		defer s1.Close()
		orbit, _ := s1.CreateOrbit("Add first", 101)
		s2, _ := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
		defer s2.Close()
		hook, entered, release := blockingCheckpoint("add_member_after_snapshot")
		s1.testCheckpoint = hook
		addResult := make(chan error, 1)
		go func() { addResult <- s1.AddMember(orbit.ID, 202, "companion") }()
		<-entered
		leaveResult := make(chan error, 1)
		go func() {
			_, _, err := s2.LeaveOrbit(orbit.ID, 101)
			leaveResult <- err
		}()
		assertOperationBlocked(t, leaveResult)
		release()
		if err := <-addResult; err != nil {
			t.Fatal(err)
		}
		if err := <-leaveResult; err != nil {
			t.Fatal(err)
		}
		assertCoherentOrbitProjection(t, s1, orbit.ID, true)
	})
}

func errorsIsSQLNoRows(err error) bool {
	return err != nil && err == sql.ErrNoRows
}

func TestR5ConcurrentLegacyBootstrapClaimsSeedOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bootstrap-race.db")
	s1, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s1.Close()
	s2, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	hook, entered, release := blockingCheckpoint("bootstrap_legacy_after_eligibility")
	s1.testCheckpoint = hook
	nodeToken := randomHex(32)
	tokens := map[string]string{"a": nodeToken}
	users := map[int64]string{101: "a"}
	type result struct {
		orbit *Orbit
		err   error
	}
	first := make(chan result, 1)
	go func() {
		orbit, err := s1.BootstrapLegacyOrbit(tokens, users)
		first <- result{orbit: orbit, err: err}
	}()
	<-entered
	second := make(chan result, 1)
	secondErr := make(chan error, 1)
	go func() {
		orbit, err := s2.BootstrapLegacyOrbit(tokens, users)
		second <- result{orbit: orbit, err: err}
		secondErr <- err
	}()
	assertOperationBlocked(t, secondErr)
	release()
	r1, r2 := <-first, <-second
	if r1.err != nil || r2.err != nil || r1.orbit == nil || r2.orbit != nil {
		t.Fatalf("bootstrap results first=%+v second=%+v", r1, r2)
	}
	var orbits int
	if err := s1.db.QueryRow(`SELECT COUNT(*) FROM orbits`).Scan(&orbits); err != nil || orbits != 1 {
		t.Fatalf("bootstrap orbit count=%d err=%v", orbits, err)
	}
	ctx, err := s1.ResolveTokenActorContext(nodeToken)
	if err != nil || ctx.OrbitID != r1.orbit.ID {
		t.Fatalf("bootstrapped token context=%+v err=%v", ctx, err)
	}
}
