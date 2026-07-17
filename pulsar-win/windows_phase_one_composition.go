package main

import (
	"context"
	"errors"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type WindowsPhaseOneSnapshot struct {
	Drafts           []PhaseOneDraftSnapshot
	SelectedDraft    int
	SelectedRoute    PhaseOneRoute
	SelectedDelivery PhaseOneDelivery
	Presence         []PhaseOnePresenceNode
	History          []PhaseOneHistoryItem
	SelectedHistory  int
	SelectedReason   PhaseOneModerationReason
	ActionOutcome    string
	FailureCode      string
}

// WindowsPhaseOneComposition owns polling and mutations independently so a
// five-second refresh can never cancel an upload or make a pending request look
// successful. Opaque IDs remain inside the composition; the shell receives
// canonical labels and allowed-action flags only.
type WindowsPhaseOneComposition struct {
	mu            sync.RWMutex
	service       PhaseOneAppService
	outbox        *PhaseOneDraftOutbox
	ctx           context.Context
	cancel        context.CancelFunc
	state         WindowsPhaseOneSnapshot
	busy          bool
	actionFailure bool
	wake          chan struct{}
}

func NewWindowsPhaseOneComposition(service PhaseOneAppService, outbox *PhaseOneDraftOutbox, workflow *WindowsCaptureWorkflowController) (*WindowsPhaseOneComposition, error) {
	if service == nil || outbox == nil || workflow == nil || workflow.CaptureMediaStore() == nil {
		return nil, ErrPhaseOnePersistence
	}
	ctx, cancel := context.WithCancel(context.Background())
	c := &WindowsPhaseOneComposition{
		service: service, outbox: outbox, ctx: ctx, cancel: cancel, wake: make(chan struct{}, 1),
		state: WindowsPhaseOneSnapshot{SelectedRoute: PhaseOneThisPulsar, SelectedDelivery: PhaseOneOverlay,
			SelectedReason: PhaseOneReportSpam, FailureCode: "refresh_pending"},
	}
	c.refreshDrafts()
	workflow.SetNormalDraftHandler(func(handle CaptureMediaHandle, originKind PhaseOneOriginKind) {
		if err := outbox.Attach(handle, "Pulsar recording", originKind); err != nil {
			c.setFailure(phaseOneFailureCode(err))
		}
		c.refreshDrafts()
	})
	go c.runRefreshLoop()
	return c, nil
}

func newProductionWindowsPhaseOneComposition(dir string, workflow *WindowsCaptureWorkflowController) (*WindowsPhaseOneComposition, error) {
	repository, err := newDefaultCredentialRepository(dir)
	if err != nil {
		return nil, err
	}
	bundle, err := repository.LoadBundle()
	if err != nil || bundle == nil {
		return nil, ErrPhaseOnePersistence
	}
	client, err := NewPhaseOneAppClient(*bundle, nil)
	if err != nil {
		return nil, err
	}
	store := workflow.CaptureMediaStore()
	if store == nil {
		return nil, ErrPhaseOnePersistence
	}
	outbox, err := NewPhaseOneDraftOutbox(client, store, filepath.Join(dir, "phase-one", "draft-outbox-v1.json"), workflow.RecoveredUserDrafts())
	if err != nil {
		return nil, err
	}
	return NewWindowsPhaseOneComposition(client, outbox, workflow)
}

func (c *WindowsPhaseOneComposition) Snapshot() WindowsPhaseOneSnapshot {
	if c == nil {
		return WindowsPhaseOneSnapshot{SelectedRoute: PhaseOneThisPulsar, SelectedDelivery: PhaseOneOverlay,
			SelectedReason: PhaseOneReportSpam, FailureCode: "phase_one_unavailable"}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := c.state
	result.Drafts = append([]PhaseOneDraftSnapshot(nil), c.state.Drafts...)
	result.Presence = append([]PhaseOnePresenceNode(nil), c.state.Presence...)
	result.History = append([]PhaseOneHistoryItem(nil), c.state.History...)
	for index := range result.History {
		result.History[index].Actions = append([]string(nil), result.History[index].Actions...)
	}
	return result
}

func (c *WindowsPhaseOneComposition) ApplyShellSnapshot(shell *ShellSnapshot) {
	if shell == nil {
		return
	}
	state := c.Snapshot()
	shell.SelectedPhaseOneDraft = state.SelectedDraft
	shell.SelectedPhaseOneRoute = state.SelectedRoute
	shell.SelectedPhaseOneDelivery = state.SelectedDelivery
	shell.SelectedHistoryItem = state.SelectedHistory
	shell.SelectedReportReason = state.SelectedReason
	shell.PhaseOneActionOutcome = state.ActionOutcome
	shell.PhaseOneFailure = state.FailureCode
	shell.RecordingDraftAvailable = len(state.Drafts) > 0
	shell.HistoryCount = len(state.History)
	shell.PresenceAvailable = len(state.Presence) > 0
	shell.PresenceOnline, shell.PresenceTotal, shell.PresenceReady = 0, len(state.Presence), 0
	for _, node := range state.Presence {
		if node.Online {
			shell.PresenceOnline++
			if node.OutputState == "ready" {
				shell.PresenceReady++
			}
		}
	}
	for _, draft := range state.Drafts {
		shell.PhaseOneDrafts = append(shell.PhaseOneDrafts, ShellPhaseOneDraft{
			Title: draft.Title, State: draft.State, Route: draft.Route,
			RequestedDelivery: draft.RequestedDelivery, EffectiveDelivery: draft.EffectiveDelivery,
			DowngradeReason: draft.DowngradeReason, Status: draft.Status,
			FailureCode: draft.FailureCode, LocalBytesRetained: draft.LocalBytesRetained,
			FallbackConfirmationAvailable: draft.FallbackConfirmationAvailable,
			ExplicitTargetCount:           draft.ExplicitTargetCount,
		})
	}
	for _, item := range state.History {
		projected := ShellPhaseOneHistoryItem{
			Title: item.Title, SenderName: item.SenderName, Direction: item.Direction, Status: item.Status,
			RequestedDelivery: item.RequestedDelivery, EffectiveDelivery: item.EffectiveDelivery,
			DowngradeReason: item.DowngradeReason, PlayedCount: item.PlayedCount, OtherCount: item.OtherCount,
			CanDelete: phaseOneActionAllowed(item.Actions, "delete"), CanReplay: phaseOneActionAllowed(item.Actions, "replay"),
			CanReport: phaseOneActionAllowed(item.Actions, "report"), CanBlock: phaseOneActionAllowed(item.Actions, "block_actor"),
			CanDisableSchedule: phaseOneActionAllowed(item.Actions, "disable_schedule"), CanRevokePrincipal: phaseOneActionAllowed(item.Actions, "revoke_principal"),
			CanEmergencyDisable: phaseOneActionAllowed(item.Actions, "emergency_disable_automation"),
		}
		if item.Automation != nil {
			projected.AutomationTrigger = item.Automation.TriggerKind
			projected.AutomationActor = item.Automation.PrincipalLabel
			if projected.AutomationActor == "" { projected.AutomationActor = item.Automation.PrincipalRef }
			projected.AutomationSchedule, projected.AutomationCue = item.Automation.ScheduleLabel, item.Automation.CueLabel
			projected.AutomationReason = item.Automation.ReasonCode
		}
		shell.PhaseOneHistory = append(shell.PhaseOneHistory, projected)
	}
}

func (c *WindowsPhaseOneComposition) SelectNextDraft() {
	c.mu.Lock()
	if len(c.state.Drafts) > 0 {
		c.state.SelectedDraft = (c.state.SelectedDraft + 1) % len(c.state.Drafts)
	}
	c.mu.Unlock()
}

func (c *WindowsPhaseOneComposition) SelectNextRoute() {
	routes := []PhaseOneRoute{PhaseOneThisPulsar, PhaseOneOwnBarycenter, PhaseOneCurrentAir}
	c.mu.Lock()
	for index, route := range routes {
		if route == c.state.SelectedRoute {
			c.state.SelectedRoute = routes[(index+1)%len(routes)]
			c.mu.Unlock()
			return
		}
	}
	c.state.SelectedRoute = PhaseOneThisPulsar
	c.mu.Unlock()
}

func (c *WindowsPhaseOneComposition) SelectNextDelivery() {
	deliveries := []PhaseOneDelivery{PhaseOneOverlay, PhaseOneInterrupt, PhaseOneAfterCurrent}
	c.mu.Lock()
	for index, delivery := range deliveries {
		if delivery == c.state.SelectedDelivery {
			c.state.SelectedDelivery = deliveries[(index+1)%len(deliveries)]
			c.mu.Unlock()
			return
		}
	}
	c.state.SelectedDelivery = PhaseOneOverlay
	c.mu.Unlock()
}

func (c *WindowsPhaseOneComposition) SelectNextHistory() {
	c.mu.Lock()
	if len(c.state.History) > 0 {
		c.state.SelectedHistory = (c.state.SelectedHistory + 1) % len(c.state.History)
	}
	c.state.ActionOutcome = ""
	c.state.FailureCode = ""
	c.actionFailure = false
	c.mu.Unlock()
}

func (c *WindowsPhaseOneComposition) SelectNextReportReason() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state.ActionOutcome = ""
	c.state.FailureCode = ""
	c.actionFailure = false
	for index, reason := range phaseOneModerationReasons {
		if reason == c.state.SelectedReason {
			c.state.SelectedReason = phaseOneModerationReasons[(index+1)%len(phaseOneModerationReasons)]
			return
		}
	}
	c.state.SelectedReason = PhaseOneReportSpam
}

func (c *WindowsPhaseOneComposition) SendSelectedDraft() {
	c.mu.RLock()
	state := c.state
	c.mu.RUnlock()
	c.SendSelectedDraftIntent(state.SelectedRoute, nil, state.SelectedRoute == PhaseOneThisPulsar, state.SelectedDelivery)
}

func (c *WindowsPhaseOneComposition) SendSelectedDraftIntent(audience PhaseOneRoute, references []string, includeOrigin bool, delivery PhaseOneDelivery) {
	if audience != TargetsInboxExplicitAudience && !validPhaseOneRoute(audience) || !validPhaseOneDelivery(delivery) {
		return
	}
	if !c.beginMutation() {
		return
	}
	c.mu.RLock()
	state := c.state
	c.mu.RUnlock()
	if state.SelectedDraft < 0 || state.SelectedDraft >= len(state.Drafts) {
		c.endMutation("invalid_draft")
		return
	}
	draftID := state.Drafts[state.SelectedDraft].DraftID
	originKind := state.Drafts[state.SelectedDraft].OriginKind
	go func() {
		var err error
		if err = c.ensureContentPolicy(); err != nil {
			c.endMutation(phaseOneFailureCodeOrEmpty(err))
			return
		}
		if state.Drafts[state.SelectedDraft].ExplicitTargetCount > 0 {
			_, err = c.outbox.RetryExplicit(c.ctx, draftID, true)
		} else if audience == TargetsInboxExplicitAudience {
			_, err = c.outbox.SendExplicit(c.ctx, draftID, references, includeOrigin, delivery, originKind, true)
		} else if state.Drafts[state.SelectedDraft].FallbackConfirmationAvailable {
			_, err = c.outbox.ConfirmFallback(c.ctx, draftID, PhaseOneAfterCurrent, originKind)
		} else {
			_, err = c.outbox.Send(c.ctx, draftID, audience, delivery, originKind, true)
		}
		c.refreshDrafts()
		c.endMutation(phaseOneFailureCodeOrEmpty(err))
		c.wakeRefresh()
	}()
}

func (c *WindowsPhaseOneComposition) ensureContentPolicy() error {
	if grant, err := c.service.CurrentContentPolicyGrant(c.ctx); err == nil {
		if grant.Current && grant.TermsAccepted {
			return nil
		}
		return &PhaseOneClientError{Kind: PhaseOneInvalidResponse}
	} else {
		var rejected *PhaseOneClientError
		if !errors.As(err, &rejected) || rejected.Kind != PhaseOneRejected ||
			rejected.Code != "content_policy_acceptance_required" {
			return err
		}
	}
	manifest, err := c.service.ContentPolicy(c.ctx, ContentPolicyEN)
	if err != nil {
		return err
	}
	_, err = c.service.AcceptContentPolicy(c.ctx, manifest)
	return err
}

func (c *WindowsPhaseOneComposition) DeleteSelectedDraft() {
	if !c.beginMutation() {
		return
	}
	c.mu.RLock()
	state := c.state
	c.mu.RUnlock()
	if state.SelectedDraft < 0 || state.SelectedDraft >= len(state.Drafts) {
		c.endMutation("invalid_draft")
		return
	}
	draftID := state.Drafts[state.SelectedDraft].DraftID
	go func() {
		err := c.outbox.Delete(c.ctx, draftID)
		c.refreshDrafts()
		c.endMutation(phaseOneFailureCodeOrEmpty(err))
	}()
}

func (c *WindowsPhaseOneComposition) DeleteSelectedHistoryItem() {
	c.mutateHistory("delete", func(ctx context.Context, item PhaseOneHistoryItem, _ WindowsPhaseOneSnapshot) (string, error) {
		receipt, err := c.service.DeleteHistoryItem(ctx, item.ID)
		return receipt.Outcome, err
	})
}

func (c *WindowsPhaseOneComposition) ReplaySelectedHistoryItem() {
	c.mutateHistory("replay", func(ctx context.Context, item PhaseOneHistoryItem, state WindowsPhaseOneSnapshot) (string, error) {
		receipt, err := c.service.ReplayHistoryItem(ctx, item.ID, state.SelectedRoute, state.SelectedDelivery, "windows-history-replay-"+item.ID, nil)
		if err != nil {
			return "", err
		}
		if receipt.Reused {
			return "replay_already_accepted", nil
		}
		return "replay_accepted", nil
	})
}

func (c *WindowsPhaseOneComposition) BlockSelectedHistoryActor() {
	c.mutateHistory("block_actor", func(ctx context.Context, item PhaseOneHistoryItem, _ WindowsPhaseOneSnapshot) (string, error) {
		receipt, err := c.service.BlockHistoryActor(ctx, item.ID, "windows-history-block-"+item.ID)
		return receipt.Outcome, err
	})
}

func (c *WindowsPhaseOneComposition) ReportSelectedHistoryItem(details string) {
	details = strings.TrimSpace(details)
	c.mutateHistory("report", func(ctx context.Context, item PhaseOneHistoryItem, state WindowsPhaseOneSnapshot) (string, error) {
		receipt, err := c.service.ReportHistoryItem(ctx, item.ID, state.SelectedReason, details)
		return receipt.Outcome, err
	})
}

func (c *WindowsPhaseOneComposition) mutateHistory(requiredAction string, operation func(context.Context, PhaseOneHistoryItem, WindowsPhaseOneSnapshot) (string, error)) {
	if !c.beginMutation() {
		return
	}
	c.mu.RLock()
	state := c.state
	c.mu.RUnlock()
	if state.SelectedHistory < 0 || state.SelectedHistory >= len(state.History) {
		c.endMutation("history_item_unavailable")
		return
	}
	item := state.History[state.SelectedHistory]
	if !phaseOneActionAllowed(item.Actions, requiredAction) {
		c.endMutationResult("action_not_allowed", "")
		return
	}
	go func() {
		outcome, err := operation(c.ctx, item, state)
		if err != nil {
			outcome = ""
		}
		c.endMutationResult(phaseOneFailureCodeOrEmpty(err), outcome)
		c.wakeRefresh()
	}()
}

func (c *WindowsPhaseOneComposition) runRefreshLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		c.refreshRemote()
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
		case <-c.wake:
		}
	}
}

func (c *WindowsPhaseOneComposition) refreshRemote() {
	ctx, cancel := context.WithTimeout(c.ctx, 4*time.Second)
	defer cancel()
	presence, presenceErr := c.service.Presence(ctx)
	history, historyErr := c.service.History(ctx, 30, "")
	c.mu.Lock()
	if presenceErr == nil {
		c.state.Presence = presence
	}
	if historyErr == nil {
		c.state.History = history.Items
		if c.state.SelectedHistory >= len(c.state.History) {
			c.state.SelectedHistory = 0
		}
	}
	if presenceErr != nil || historyErr != nil {
		c.state.FailureCode = "coordinator_unavailable"
	} else if !c.busy && !c.actionFailure {
		c.state.FailureCode = ""
	}
	c.mu.Unlock()
}

func (c *WindowsPhaseOneComposition) refreshDrafts() {
	drafts := c.outbox.Snapshots()
	sort.Slice(drafts, func(i, j int) bool { return drafts[i].DraftID < drafts[j].DraftID })
	c.mu.Lock()
	c.state.Drafts = drafts
	if c.state.SelectedDraft >= len(drafts) {
		c.state.SelectedDraft = 0
	}
	c.mu.Unlock()
}

func (c *WindowsPhaseOneComposition) beginMutation() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.busy || c.ctx.Err() != nil {
		return false
	}
	c.busy = true
	c.actionFailure = false
	c.state.FailureCode = ""
	c.state.ActionOutcome = ""
	return true
}

func (c *WindowsPhaseOneComposition) endMutation(failure string) {
	c.endMutationResult(failure, "")
}

func (c *WindowsPhaseOneComposition) endMutationResult(failure, outcome string) {
	c.mu.Lock()
	c.busy = false
	c.actionFailure = failure != ""
	c.state.FailureCode = failure
	c.state.ActionOutcome = outcome
	c.mu.Unlock()
}

func (c *WindowsPhaseOneComposition) setFailure(failure string) {
	c.mu.Lock()
	c.state.FailureCode = failure
	c.mu.Unlock()
}

func (c *WindowsPhaseOneComposition) wakeRefresh() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

func (c *WindowsPhaseOneComposition) Close() {
	if c != nil && c.cancel != nil {
		c.cancel()
	}
}

func phaseOneFailureCodeOrEmpty(err error) string {
	if err == nil {
		return ""
	}
	return phaseOneFailureCode(err)
}

func phaseOneActionAllowed(actions []string, required string) bool {
	for _, action := range actions {
		if action == required {
			return true
		}
	}
	return false
}
