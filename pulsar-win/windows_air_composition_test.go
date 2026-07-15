package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeAirService struct {
	mu         sync.Mutex
	details    []AirDetail
	pending    *AirJoinPreview
	invite     AirInvite
	calls      []string
	lastPolicy AirPolicy
}

type staleAirService struct {
	*fakeAirService
	once            sync.Once
	started         chan struct{}
	release         chan struct{}
	old             AirDetail
	mu              sync.Mutex
	staleNextDetail bool
}

func (s *staleAirService) List(ctx context.Context) (AirList, error) {
	first := false
	s.once.Do(func() { first = true })
	if !first {
		return s.fakeAirService.List(ctx)
	}
	close(s.started)
	select {
	case <-s.release:
	case <-ctx.Done():
		return AirList{}, ctx.Err()
	}
	s.mu.Lock()
	s.staleNextDetail = true
	s.mu.Unlock()
	id := s.old.AirID
	return AirList{CurrentAirID: &id, ActivePointerRevision: 1, Saved: []AirSummary{{
		AirID: s.old.AirID, Title: s.old.Title, Status: s.old.Status,
		MembershipStatus: s.old.MembershipStatus, Role: s.old.Role, MemberCount: s.old.MemberCount,
		ActiveMemberCount: s.old.ActiveMemberCount, OnlinePulsarCount: s.old.OnlinePulsarCount,
		Capacity: s.old.Capacity, PolicyRevision: s.old.Policy.Revision, Current: true,
	}}}, nil
}

func (s *staleAirService) Detail(ctx context.Context, id string) (AirDetail, error) {
	s.mu.Lock()
	if s.staleNextDetail {
		s.staleNextDetail = false
		old := s.old
		s.mu.Unlock()
		return old, nil
	}
	s.mu.Unlock()
	return s.fakeAirService.Detail(ctx, id)
}

func (f *fakeAirService) record(call string) {
	f.mu.Lock()
	f.calls = append(f.calls, call)
	f.mu.Unlock()
}
func (f *fakeAirService) List(context.Context) (AirList, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	result := AirList{ActivePointerRevision: 1}
	for _, detail := range f.details {
		result.Saved = append(result.Saved, AirSummary{AirID: detail.AirID, Title: detail.Title, Status: detail.Status,
			MembershipStatus: detail.MembershipStatus, Role: detail.Role, MemberCount: detail.MemberCount,
			ActiveMemberCount: detail.ActiveMemberCount, OnlinePulsarCount: detail.OnlinePulsarCount,
			Capacity: detail.Capacity, PolicyRevision: detail.Policy.Revision, Current: detail.Current})
		if detail.Current {
			id := detail.AirID
			result.CurrentAirID = &id
		}
	}
	return result, nil
}
func (f *fakeAirService) Detail(_ context.Context, id string) (AirDetail, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, detail := range f.details {
		if detail.AirID == id {
			return detail, nil
		}
	}
	return AirDetail{}, &AirClientError{Kind: AirRejected, Code: "air_dissolved"}
}
func (f *fakeAirService) Create(context.Context, string, string) (AirDetail, error) {
	f.record("create")
	return AirDetail{}, nil
}
func (f *fakeAirService) IssueInvite(context.Context, string, AirRole, string) (AirInvite, error) {
	f.record("invite")
	return f.invite, nil
}
func (f *fakeAirService) WithdrawInvite(context.Context, string, string, int64, string) error {
	f.record("withdraw")
	return nil
}
func (f *fakeAirService) ConsumeInvite(context.Context, string, string) (AirJoinPreview, error) {
	f.record("consume")
	f.mu.Lock()
	defer f.mu.Unlock()
	preview := *f.pending
	f.details = append(f.details, airDetailFixture(preview.AirID, "P", preview.Title, false, AirPendingConfirmation, preview.Role))
	return preview, nil
}
func (f *fakeAirService) ConfirmJoin(_ context.Context, id string, _ int64, activate bool, _ string, _ string) error {
	f.record("confirm")
	f.mu.Lock()
	defer f.mu.Unlock()
	for index := range f.details {
		if activate {
			f.details[index].Current = f.details[index].AirID == id
		}
		if f.details[index].AirID == id {
			f.details[index].MembershipStatus = AirJoined
		}
	}
	return nil
}
func (f *fakeAirService) DeclineJoin(context.Context, string, int64, string) error {
	f.record("decline")
	return nil
}
func (f *fakeAirService) Activate(_ context.Context, id string, _ int64, _ string, _ string) error {
	f.record("activate")
	f.mu.Lock()
	defer f.mu.Unlock()
	for index := range f.details {
		f.details[index].Current = f.details[index].AirID == id
	}
	return nil
}
func (f *fakeAirService) Deactivate(_ context.Context, id string, _ int64, _ string, _ string) error {
	f.record("deactivate")
	f.mu.Lock()
	defer f.mu.Unlock()
	for index := range f.details {
		if f.details[index].AirID == id {
			f.details[index].Current = false
		}
	}
	return nil
}
func (f *fakeAirService) Leave(_ context.Context, id string, _ int64, _ string, _ string) error {
	f.record("leave")
	f.mu.Lock()
	defer f.mu.Unlock()
	kept := f.details[:0]
	for _, detail := range f.details {
		if detail.AirID != id {
			kept = append(kept, detail)
		}
	}
	f.details = kept
	return nil
}
func (f *fakeAirService) ReplacePolicy(_ context.Context, _ string, policy AirPolicy, _ string) error {
	f.record("policy")
	f.mu.Lock()
	f.lastPolicy = policy
	f.mu.Unlock()
	return nil
}
func (f *fakeAirService) Dissolve(_ context.Context, id string, _ int64, _ string) error {
	f.record("dissolve")
	f.mu.Lock()
	defer f.mu.Unlock()
	kept := f.details[:0]
	for _, detail := range f.details {
		if detail.AirID != id {
			kept = append(kept, detail)
		}
	}
	f.details = kept
	return nil
}

func airDetailFixture(id, memberSuffix, title string, current bool, membership AirMembershipStatus, role AirRole) AirDetail {
	member := "aim_" + strings.Repeat(memberSuffix, 26)
	return AirDetail{AirID: id, Title: title, Status: "active", Revision: 2, MembershipID: member,
		MembershipStatus: membership, MembershipRevision: 3, Role: role, MemberCount: 2,
		ActiveMemberCount: 2, OnlinePulsarCount: 3, Capacity: AirCapacity{Barycenters: 8, OnlinePulsars: 16},
		Policy: AirPolicy{Revision: 4, Invite: AirInviteOwnerPrimary, Overlay: AirPlaybackAdminPrimary,
			Queue: AirPlaybackPrimaryCompanion, Replace: AirPlaybackAllMemberPrimarys}, Current: current}
}

func waitForAir(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timed out waiting for Air composition")
}

func fakeHasCall(fake *fakeAirService, call string) bool {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	for _, candidate := range fake.calls {
		if candidate == call {
			return true
		}
	}
	return false
}

func TestWindowsAirCompositionRequiresExplicitDisruptiveConfirmation(t *testing.T) {
	first := "air_" + strings.Repeat("A", 26)
	second := "air_" + strings.Repeat("B", 26)
	fake := &fakeAirService{details: []AirDetail{
		airDetailFixture(first, "C", "Family room", true, AirJoined, AirRoleOwner),
		airDetailFixture(second, "D", "Studio", false, AirJoined, AirRoleMember),
	}}
	composition, err := NewWindowsAirComposition(fake)
	if err != nil {
		t.Fatal(err)
	}
	defer composition.Close()
	waitForAir(t, func() bool { return len(composition.Snapshot().Saved) == 2 })
	composition.CyclePolicy()
	waitForAir(t, func() bool { return fakeHasCall(fake, "policy") && !composition.Snapshot().Busy })
	fake.mu.Lock()
	policy := fake.lastPolicy
	fake.mu.Unlock()
	if policy.Invite != AirInviteAdminPrimary || validateAirPolicy(policy) != nil {
		t.Fatalf("owner policy preset was incomplete or invalid: %+v", policy)
	}
	composition.SelectNextAir()
	composition.RequestActivate()
	if got := composition.Snapshot().ConfirmAction; got != "switch" {
		t.Fatalf("confirmation=%q", got)
	}
	if fakeHasCall(fake, "activate") {
		t.Fatal("switch happened before confirmation")
	}
	composition.ConfirmDisruptive()
	waitForAir(t, func() bool { return fakeHasCall(fake, "activate") && !composition.Snapshot().Busy })
	state := composition.Snapshot()
	if len(state.Saved) != 2 || !state.Saved[state.Selected].Current {
		t.Fatalf("switch projection=%+v", state)
	}
	composition.RequestLeave()
	if composition.Snapshot().ConfirmAction != "leave" {
		t.Fatal("leave was not armed")
	}
	composition.CancelDisruptive()
	if fakeHasCall(fake, "leave") || composition.Snapshot().ConfirmAction != "" {
		t.Fatal("cancel executed leave")
	}
	composition.RequestLeave()
	composition.ConfirmDisruptive()
	waitForAir(t, func() bool { return fakeHasCall(fake, "leave") && len(composition.Snapshot().Saved) == 1 })
}

func TestWindowsAirCompositionPreservesPendingPreviewAndRedactsInvite(t *testing.T) {
	first := "air_" + strings.Repeat("A", 26)
	second := "air_" + strings.Repeat("B", 26)
	secret := "air-invite-secret-value"
	fake := &fakeAirService{
		details: []AirDetail{airDetailFixture(first, "C", "Family room", true, AirJoined, AirRoleOwner)},
		pending: &AirJoinPreview{AirID: second, Title: "Alias room", OwnerDisplayName: "Ivan", Role: AirRoleMember,
			MembershipRevision: 3, Policy: airDetailFixture(second, "D", "Alias room", false, AirPendingConfirmation, AirRoleMember).Policy,
			MemberCount: 2, Capacity: AirCapacity{Barycenters: 8, OnlinePulsars: 16}, ActivationWouldSwitch: true},
		invite: AirInvite{InviteID: "ai_" + strings.Repeat("E", 26), Revision: 2, Expires: time.Now().Add(time.Hour), Code: secret},
	}
	composition, err := NewWindowsAirComposition(fake)
	if err != nil {
		t.Fatal(err)
	}
	defer composition.Close()
	waitForAir(t, func() bool { return len(composition.Snapshot().Saved) == 1 })
	composition.IssueInvite()
	waitForAir(t, func() bool { return composition.InviteCode() == secret })
	if strings.Contains(fmt.Sprintf("%+v", composition.Snapshot()), secret) {
		t.Fatal("composition formatting leaked the invite secret")
	}
	var shell ShellSnapshot
	composition.ApplyShellSnapshot(&shell)
	if !shell.AirAvailable {
		t.Fatal("configured Air composition projected unavailable controls")
	}
	projection := NewShellCopy(ShellEnglish).AirProjection(shell)
	if strings.Contains(projection, secret) || strings.Contains(projection, first) {
		t.Fatalf("projection leaked secret/id: %q", projection)
	}
	composition.HideInvite()
	if composition.InviteCode() != "" {
		t.Fatal("hidden invite remained in memory projection")
	}
	composition.ConsumeInvite("one-time")
	waitForAir(t, func() bool { return composition.Snapshot().Pending != nil && !composition.Snapshot().Busy })
	pending := composition.Snapshot().Pending
	if pending.OwnerDisplayName != "Ivan" || pending.Title != "Alias room" {
		t.Fatalf("pending=%+v", pending)
	}
	composition.ConfirmJoin(true)
	if composition.Snapshot().ConfirmAction != "join_switch" || fakeHasCall(fake, "confirm") {
		t.Fatal("join-and-switch bypassed confirmation")
	}
	composition.ConfirmDisruptive()
	waitForAir(t, func() bool { return fakeHasCall(fake, "confirm") && !composition.Snapshot().Busy })
}

func TestWindowsAirCompositionHasNoPhaseOneTargetOrInboxDependency(t *testing.T) {
	raw, err := os.ReadFile("windows_air_composition.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, forbidden := range []string{"PhaseOneDraftOutbox", "PhaseOneAppClient", "explicit_target", "target_snapshot", "inbox"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("Air composition depends on %q", forbidden)
		}
	}
}

func TestWindowsAirCompositionExpiresInviteSecretInMemory(t *testing.T) {
	fake := &fakeAirService{}
	composition, err := NewWindowsAirComposition(fake)
	if err != nil {
		t.Fatal(err)
	}
	defer composition.Close()
	composition.mu.Lock()
	composition.state.Invite = &AirInvite{InviteID: "ai_" + strings.Repeat("A", 26), Revision: 1, Expires: time.Now().Add(-time.Second), Code: "expired-secret"}
	composition.state.InviteAirID = "air_" + strings.Repeat("B", 26)
	composition.mu.Unlock()
	state := composition.Snapshot()
	if state.Invite != nil || composition.InviteCode() != "" || state.Failure != "invite_unavailable" {
		t.Fatalf("expired invite survived: %+v", state)
	}
}

func TestWindowsAirCompositionFencesPreMutationRefresh(t *testing.T) {
	id := "air_" + strings.Repeat("A", 26)
	old := airDetailFixture(id, "B", "Old title", true, AirJoined, AirRoleOwner)
	newer := old
	newer.Title, newer.Revision = "New title", 3
	service := &staleAirService{fakeAirService: &fakeAirService{details: []AirDetail{newer}},
		started: make(chan struct{}), release: make(chan struct{}), old: old}
	composition, err := NewWindowsAirComposition(service)
	if err != nil {
		t.Fatal(err)
	}
	defer composition.Close()
	select {
	case <-service.started:
	case <-time.After(time.Second):
		t.Fatal("initial refresh did not start")
	}
	composition.mu.Lock()
	composition.state.Saved = []AirDetail{old}
	composition.mu.Unlock()
	composition.Create("New title")
	waitForAir(t, func() bool {
		state := composition.Snapshot()
		return !state.Busy && len(state.Saved) == 1 && state.Saved[0].Title == "New title"
	})
	close(service.release)
	time.Sleep(30 * time.Millisecond)
	state := composition.Snapshot()
	if len(state.Saved) != 1 || state.Saved[0].Title != "New title" {
		t.Fatalf("stale pre-mutation refresh overwrote accepted state: %+v", state)
	}
}
