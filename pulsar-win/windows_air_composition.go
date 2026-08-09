package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

type WindowsAirSnapshot struct {
	Available     bool
	Saved         []AirDetail
	Selected      int
	Pending       *AirJoinPreview
	Invite        *AirInvite
	InviteAirID   string
	InviteRole    AirRole
	Outcome       string
	Failure       string
	Busy          bool
	ConfirmAction string
	ConfirmAirID  string
}

// WindowsAirComposition owns Air polling and lifecycle mutations. Opaque IDs
// remain action handles; only the shell projection is rendered by Win32.
type WindowsAirComposition struct {
	mu          sync.RWMutex
	service     AirAppService
	ctx         context.Context
	cancel      context.CancelFunc
	refreshGate chan struct{}
	epoch       uint64
	state       WindowsAirSnapshot
	retryKeys   map[string]string
}

func NewWindowsAirComposition(service AirAppService) (*WindowsAirComposition, error) {
	if service == nil {
		return nil, airError(AirInvalidConfiguration)
	}
	ctx, cancel := context.WithCancel(context.Background())
	c := &WindowsAirComposition{
		service: service, ctx: ctx, cancel: cancel, refreshGate: make(chan struct{}, 1),
		state:     WindowsAirSnapshot{InviteRole: AirRoleMember, Failure: "refresh_pending"},
		retryKeys: map[string]string{},
	}
	go c.runRefreshLoop()
	c.Refresh()
	return c, nil
}

func newProductionWindowsAirComposition(dir string) (*WindowsAirComposition, error) {
	repository, err := newDefaultCredentialRepository(dir)
	if err != nil {
		return nil, err
	}
	bundle, err := repository.LoadBundle()
	if err != nil || bundle == nil {
		return nil, airError(AirInvalidConfiguration)
	}
	client, err := NewAirAppClient(*bundle, nil)
	if err != nil {
		return nil, err
	}
	return NewWindowsAirComposition(client)
}

func (c *WindowsAirComposition) Snapshot() WindowsAirSnapshot {
	if c == nil {
		return WindowsAirSnapshot{InviteRole: AirRoleMember, Failure: "air_unavailable"}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state.Invite != nil && !c.state.Invite.Expires.After(time.Now()) {
		c.state.Invite.Code = ""
		c.state.Invite, c.state.InviteAirID = nil, ""
		c.state.Outcome, c.state.Failure = "", "invite_unavailable"
		clearAirInviteClipboard()
	}
	result := c.state
	result.Saved = append([]AirDetail(nil), c.state.Saved...)
	if c.state.Pending != nil {
		copy := *c.state.Pending
		result.Pending = &copy
	}
	if c.state.Invite != nil {
		copy := *c.state.Invite
		result.Invite = &copy
	}
	return result
}

func (c *WindowsAirComposition) ApplyShellSnapshot(shell *ShellSnapshot) {
	if c == nil || shell == nil {
		return
	}
	state := c.Snapshot()
	shell.SelectedAir = state.Selected
	shell.AirAvailable = state.Available
	shell.AirBusy = state.Busy
	shell.AirOutcome = state.Outcome
	shell.AirFailure = state.Failure
	shell.AirConfirmAction = state.ConfirmAction
	shell.AirInviteRole = state.InviteRole
	for _, detail := range state.Saved {
		shell.Airs = append(shell.Airs, ShellAirItem{
			AirID: detail.AirID, Title: detail.Title, Status: detail.Status, Revision: detail.Revision,
			MembershipStatus: detail.MembershipStatus, MembershipRevision: detail.MembershipRevision,
			Role: detail.Role, MemberCount: detail.MemberCount, ActiveMemberCount: detail.ActiveMemberCount,
			OnlinePulsarCount: detail.OnlinePulsarCount, Capacity: detail.Capacity,
			Policy: detail.Policy, Current: detail.Current,
		})
	}
	if state.Pending != nil {
		shell.PendingAirJoin = &ShellPendingAirJoin{
			AirID: state.Pending.AirID, Title: state.Pending.Title,
			OwnerDisplayName: state.Pending.OwnerDisplayName, Role: state.Pending.Role,
			MembershipRevision: state.Pending.MembershipRevision,
			MemberCount:        state.Pending.MemberCount, Capacity: state.Pending.Capacity,
			ActivationWouldSwitch: state.Pending.ActivationWouldSwitch,
		}
	}
	if state.Invite != nil {
		shell.AirInviteAvailable = true
		shell.AirInviteExpires = state.Invite.Expires
	}
}

func (c *WindowsAirComposition) Refresh() {
	if c == nil {
		return
	}
	c.mu.RLock()
	busy := c.state.Busy
	c.mu.RUnlock()
	if busy {
		return
	}
	select {
	case c.refreshGate <- struct{}{}:
		go func() {
			defer func() { <-c.refreshGate }()
			_ = c.refreshProjection(nil)
		}()
	default:
	}
}

func (c *WindowsAirComposition) SelectNextAir() {
	c.mu.Lock()
	if len(c.state.Saved) > 0 {
		c.state.Selected = (c.state.Selected + 1) % len(c.state.Saved)
	}
	c.clearConfirmationLocked()
	c.mu.Unlock()
}

func (c *WindowsAirComposition) SelectNextInviteRole() {
	c.mu.Lock()
	if c.state.InviteRole == AirRoleMember {
		c.state.InviteRole = AirRoleAdmin
	} else {
		c.state.InviteRole = AirRoleMember
	}
	c.mu.Unlock()
}

func (c *WindowsAirComposition) Create(title string) {
	if !c.beginMutation() {
		return
	}
	createOperation := "create:" + title
	createKey := c.retryKey(createOperation)
	go func() {
		detail, err := c.service.Create(c.ctx, title, createKey)
		if err != nil {
			c.finishMutation("", airFailureCode(err))
			return
		}
		inviteOperation := "initial-invite:" + detail.AirID
		invite, err := c.service.IssueInvite(
			c.ctx, detail.AirID, AirRoleMember, c.retryKey(inviteOperation))
		if err != nil {
			c.finishMutation("", airFailureCode(err))
			return
		}
		if refreshErr := c.refreshProjection(nil); refreshErr != nil {
			c.finishMutation("", airFailureCode(refreshErr))
			return
		}
		c.mu.Lock()
		c.state.Invite, c.state.InviteAirID = &invite, detail.AirID
		delete(c.retryKeys, createOperation)
		delete(c.retryKeys, inviteOperation)
		c.mu.Unlock()
		c.finishMutation("created_with_invite", "")
	}()
}

func (c *WindowsAirComposition) ConsumeInvite(code string) {
	if !c.beginMutation() {
		return
	}
	go func() {
		preview, err := c.service.ConsumeInvite(c.ctx, code, airKey("consume"))
		if err != nil {
			c.finishMutation("", airFailureCode(err))
			return
		}
		if refreshErr := c.refreshProjection(&preview); refreshErr != nil {
			c.finishMutation("", airFailureCode(refreshErr))
			return
		}
		c.finishMutation("invite_reviewed", "")
	}()
}

func (c *WindowsAirComposition) ConfirmJoin(activate bool) {
	state := c.Snapshot()
	if state.Pending == nil {
		c.setFailure("membership_confirmation_required")
		return
	}
	if activate && state.Pending.ActivationWouldSwitch {
		c.armConfirmation("join_switch", state.Pending.AirID)
		return
	}
	c.confirmJoinNow(*state.Pending, activate)
}

func (c *WindowsAirComposition) confirmJoinNow(pending AirJoinPreview, activate bool) {
	expected := c.currentAirID()
	c.mutate("join_confirmed", func(ctx context.Context) error {
		return c.service.ConfirmJoin(ctx, pending.AirID, pending.MembershipRevision, activate, expected, airKey("confirm"))
	})
}

func (c *WindowsAirComposition) DeclineJoin() {
	state := c.Snapshot()
	if state.Pending == nil {
		return
	}
	pending := *state.Pending
	c.mutate("join_declined", func(ctx context.Context) error {
		return c.service.DeclineJoin(ctx, pending.AirID, pending.MembershipRevision, airKey("decline"))
	})
}

func (c *WindowsAirComposition) IssueInvite() {
	air, ok := c.selectedAir()
	if !ok || air.MembershipStatus != AirJoined {
		return
	}
	state := c.Snapshot()
	if !c.beginMutation() {
		return
	}
	go func() {
		invite, err := c.service.IssueInvite(c.ctx, air.AirID, state.InviteRole, airKey("invite"))
		if err != nil {
			c.finishMutation("", airFailureCode(err))
			return
		}
		c.mu.Lock()
		c.state.Invite, c.state.InviteAirID = &invite, air.AirID
		c.mu.Unlock()
		c.finishMutation("invite_issued", "")
	}()
}

func (c *WindowsAirComposition) WithdrawInvite() {
	state := c.Snapshot()
	if state.Invite == nil || state.InviteAirID == "" {
		return
	}
	invite, airID := *state.Invite, state.InviteAirID
	c.mutate("invite_withdrawn", func(ctx context.Context) error {
		return c.service.WithdrawInvite(ctx, airID, invite.InviteID, invite.Revision, airKey("withdraw"))
	})
	c.HideInvite()
}

func (c *WindowsAirComposition) HideInvite() {
	c.mu.Lock()
	if c.state.Invite != nil {
		c.state.Invite.Code = ""
	}
	c.state.Invite, c.state.InviteAirID = nil, ""
	c.mu.Unlock()
	clearAirInviteClipboard()
}

// InviteCode is for an explicit clipboard action only. It must never be
// formatted, logged, placed in control text, or exposed as an accessible name.
func (c *WindowsAirComposition) InviteCode() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.state.Invite == nil {
		return ""
	}
	return c.state.Invite.Code
}

func (c *WindowsAirComposition) RequestActivate() {
	air, ok := c.selectedAir()
	if !ok || air.MembershipStatus != AirJoined {
		return
	}
	if air.Current {
		c.armConfirmation("deactivate", air.AirID)
		return
	}
	if current := c.currentAirID(); current != "" && current != air.AirID {
		c.armConfirmation("switch", air.AirID)
		return
	}
	c.activateNow(air)
}

func (c *WindowsAirComposition) activateNow(air AirDetail) {
	expected := c.currentAirID()
	c.mutate("activated", func(ctx context.Context) error {
		return c.service.Activate(ctx, air.AirID, air.MembershipRevision, expected, airKey("activate"))
	})
}

func (c *WindowsAirComposition) RequestLeave() {
	air, ok := c.selectedAir()
	if !ok || air.Role == AirRoleOwner || air.MembershipStatus != AirJoined {
		return
	}
	c.armConfirmation("leave", air.AirID)
}

func (c *WindowsAirComposition) RequestDissolve() {
	air, ok := c.selectedAir()
	if !ok || air.Role != AirRoleOwner {
		return
	}
	c.armConfirmation("dissolve", air.AirID)
}

func (c *WindowsAirComposition) ConfirmDisruptive() {
	state := c.Snapshot()
	if state.ConfirmAction == "" {
		return
	}
	if state.ConfirmAction == "join_switch" && state.Pending != nil && state.Pending.AirID == state.ConfirmAirID {
		c.confirmJoinNow(*state.Pending, true)
		return
	}
	air, ok := airByID(state.Saved, state.ConfirmAirID)
	if !ok {
		c.CancelDisruptive()
		return
	}
	switch state.ConfirmAction {
	case "switch":
		c.activateNow(air)
	case "deactivate":
		c.mutate("deactivated", func(ctx context.Context) error {
			return c.service.Deactivate(ctx, air.AirID, air.MembershipRevision, air.AirID, airKey("deactivate"))
		})
	case "leave":
		expected := c.currentAirID()
		c.mutate("left", func(ctx context.Context) error {
			return c.service.Leave(ctx, air.AirID, air.MembershipRevision, expected, airKey("leave"))
		})
	case "dissolve":
		c.mutate("dissolved", func(ctx context.Context) error {
			return c.service.Dissolve(ctx, air.AirID, air.Revision, airKey("dissolve"))
		})
	}
}

func (c *WindowsAirComposition) CancelDisruptive() {
	c.mu.Lock()
	c.clearConfirmationLocked()
	c.mu.Unlock()
}

func (c *WindowsAirComposition) CyclePolicy() {
	air, ok := c.selectedAir()
	if !ok || air.Role != AirRoleOwner {
		return
	}
	policy := air.Policy
	switch policy.Invite {
	case AirInviteOwnerPrimary:
		policy.Invite = AirInviteAdminPrimary
		policy.Overlay, policy.Queue, policy.Replace = AirPlaybackAllMemberPrimarys, AirPlaybackAllMemberPrimarys, AirPlaybackAllMemberPrimarys
	case AirInviteAdminPrimary:
		policy.Invite = AirInviteAllMemberPrimarys
		policy.Overlay, policy.Queue = AirPlaybackPrimaryCompanion, AirPlaybackPrimaryCompanion
		policy.Replace = AirPlaybackAdminPrimary
	default:
		policy.Invite = AirInviteOwnerPrimary
		policy.Overlay, policy.Queue, policy.Replace = AirPlaybackDisabled, AirPlaybackDisabled, AirPlaybackDisabled
	}
	c.mutate("policy_updated", func(ctx context.Context) error {
		return c.service.ReplacePolicy(ctx, air.AirID, policy, airKey("policy"))
	})
}

func (c *WindowsAirComposition) Close() {
	if c == nil {
		return
	}
	c.cancel()
	c.mu.Lock()
	c.epoch++
	if c.state.Invite != nil {
		c.state.Invite.Code = ""
	}
	c.state = WindowsAirSnapshot{InviteRole: AirRoleMember}
	c.mu.Unlock()
	clearAirInviteClipboard()
}

func (c *WindowsAirComposition) runRefreshLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.Refresh()
		}
	}
}

func (c *WindowsAirComposition) refreshProjection(pendingOverride *AirJoinPreview) error {
	c.mu.RLock()
	epoch := c.epoch
	c.mu.RUnlock()
	availability, err := c.service.Availability(c.ctx)
	if err != nil {
		if c.currentEpoch(epoch) {
			c.setFailure(airFailureCode(err))
		}
		return err
	}
	if !availability.Enabled {
		c.mu.Lock()
		if epoch == c.epoch {
			c.state.Available = false
			c.state.Saved, c.state.Pending = nil, nil
			c.state.Failure = ""
		}
		c.mu.Unlock()
		return nil
	}
	list, err := c.service.List(c.ctx)
	if err != nil {
		if c.currentEpoch(epoch) {
			c.setFailure(airFailureCode(err))
		}
		return err
	}
	details := make([]AirDetail, 0, len(list.Saved))
	for _, item := range list.Saved {
		detail, detailErr := c.service.Detail(c.ctx, item.AirID)
		if detailErr != nil {
			if c.currentEpoch(epoch) {
				c.setFailure(airFailureCode(detailErr))
			}
			return detailErr
		}
		details = append(details, detail)
	}
	c.mu.Lock()
	if epoch != c.epoch {
		c.mu.Unlock()
		return nil
	}
	selectedID := ""
	if c.state.Selected >= 0 && c.state.Selected < len(c.state.Saved) {
		selectedID = c.state.Saved[c.state.Selected].AirID
	}
	priorPending := c.state.Pending
	c.state.Available, c.state.Saved, c.state.Failure = true, details, ""
	c.state.Selected = 0
	for index := range details {
		if details[index].AirID == selectedID {
			c.state.Selected = index
		}
	}
	if pendingOverride != nil {
		copy := *pendingOverride
		c.state.Pending = &copy
	} else {
		c.state.Pending = nil
		for _, detail := range details {
			if detail.MembershipStatus == AirPendingConfirmation {
				preview := AirJoinPreview{AirID: detail.AirID, Title: detail.Title, Role: detail.Role,
					MembershipRevision: detail.MembershipRevision, Policy: detail.Policy,
					MemberCount: detail.MemberCount, Capacity: detail.Capacity,
					ActivationWouldSwitch: list.CurrentAirID != nil && *list.CurrentAirID != detail.AirID}
				if priorPending != nil && priorPending.AirID == detail.AirID {
					preview.OwnerDisplayName = priorPending.OwnerDisplayName
				}
				c.state.Pending = &preview
				break
			}
		}
	}
	c.mu.Unlock()
	return nil
}

func (c *WindowsAirComposition) mutate(outcome string, operation func(context.Context) error) {
	if !c.beginMutation() {
		return
	}
	go func() {
		err := operation(c.ctx)
		if err == nil {
			if refreshErr := c.refreshProjection(nil); refreshErr != nil {
				c.finishMutation("", airFailureCode(refreshErr))
				return
			}
			c.finishMutation(outcome, "")
		} else {
			code := airFailureCode(err)
			c.finishMutation("", code)
			if code == "revision_conflict" || code == "active_air_changed" || code == "air_dissolved" {
				c.Refresh()
			}
		}
	}()
}

func (c *WindowsAirComposition) beginMutation() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state.Busy {
		return false
	}
	c.state.Busy, c.state.Outcome, c.state.Failure = true, "", ""
	c.epoch++
	c.clearConfirmationLocked()
	return true
}

func (c *WindowsAirComposition) finishMutation(outcome, failure string) {
	select {
	case <-c.ctx.Done():
		return
	default:
	}
	c.mu.Lock()
	c.state.Busy, c.state.Outcome, c.state.Failure = false, outcome, failure
	c.mu.Unlock()
}

func (c *WindowsAirComposition) setFailure(code string) {
	select {
	case <-c.ctx.Done():
		return
	default:
	}
	c.mu.Lock()
	c.state.Failure = code
	c.mu.Unlock()
}

func (c *WindowsAirComposition) setOutcome(code string) {
	select {
	case <-c.ctx.Done():
		return
	default:
	}
	c.mu.Lock()
	c.state.Outcome, c.state.Failure = code, ""
	c.mu.Unlock()
}

func (c *WindowsAirComposition) currentEpoch(epoch uint64) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.epoch == epoch
}

func (c *WindowsAirComposition) armConfirmation(action, airID string) {
	c.mu.Lock()
	if !c.state.Busy {
		c.state.ConfirmAction, c.state.ConfirmAirID = action, airID
		c.state.Outcome, c.state.Failure = "", ""
	}
	c.mu.Unlock()
}

func (c *WindowsAirComposition) clearConfirmationLocked() {
	c.state.ConfirmAction, c.state.ConfirmAirID = "", ""
}

func (c *WindowsAirComposition) retryKey(operation string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if existing := c.retryKeys[operation]; existing != "" {
		return existing
	}
	created := airKey("retry")
	c.retryKeys[operation] = created
	return created
}

func (c *WindowsAirComposition) selectedAir() (AirDetail, bool) {
	state := c.Snapshot()
	if state.Selected < 0 || state.Selected >= len(state.Saved) {
		return AirDetail{}, false
	}
	return state.Saved[state.Selected], true
}

func (c *WindowsAirComposition) currentAirID() string {
	state := c.Snapshot()
	for _, air := range state.Saved {
		if air.Current {
			return air.AirID
		}
	}
	return ""
}

func airByID(airs []AirDetail, id string) (AirDetail, bool) {
	for _, air := range airs {
		if air.AirID == id {
			return air, true
		}
	}
	return AirDetail{}, false
}

func airKey(operation string) string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "windows-air-" + operation + "-fallback-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "-" + strconv.FormatUint(airFallbackCounter.Add(1), 10)
	}
	return "windows-air-" + operation + "-" + hex.EncodeToString(raw)
}

var airFallbackCounter atomic.Uint64

func airFailureCode(err error) string {
	if api, ok := err.(*AirClientError); ok {
		switch api.Kind {
		case AirTransport:
			return "coordinator_unavailable"
		case AirRedirectRejected:
			return "redirect_rejected"
		case AirResponseTooLarge:
			return "response_too_large"
		case AirInvalidConfiguration:
			return "credential_unavailable"
		case AirInvalidRequest:
			return "invalid_request"
		case AirInvalidResponse:
			return "invalid_response"
		case AirRejected:
			return api.Code
		}
	}
	return "coordinator_unavailable"
}
