package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

type WindowsTargetsInboxSnapshot struct {
	Projection      TargetsInboxSnapshot
	SelectedTarget  int
	SelectedInbox   int
	SelectedHistory int
	Delivery        PhaseOneDelivery
	Reason          PhaseOneModerationReason
	ActionOutcome   string
	FailureCode     string
	Busy            bool
}

// WindowsTargetsInboxComposition serializes projection refresh and mutations.
// Retained rows may remain readable during reconnect, but the shared model
// strips all capability-bearing selection until a fresh authoritative result.
type WindowsTargetsInboxComposition struct {
	mu         sync.RWMutex
	service    TargetsInboxAppService
	model      *TargetsInboxModel
	phaseOne   *WindowsPhaseOneComposition
	ctx        context.Context
	cancel     context.CancelFunc
	wake       chan bool
	state      WindowsTargetsInboxSnapshot
	refreshing bool
}

func NewWindowsTargetsInboxComposition(service TargetsInboxAppService, phaseOne *WindowsPhaseOneComposition) (*WindowsTargetsInboxComposition, error) {
	if service == nil || phaseOne == nil {
		return nil, ErrPhaseOnePersistence
	}
	ctx, cancel := context.WithCancel(context.Background())
	model := NewTargetsInboxModel()
	c := &WindowsTargetsInboxComposition{
		service: service, model: model, phaseOne: phaseOne, ctx: ctx, cancel: cancel, wake: make(chan bool, 1),
		state: WindowsTargetsInboxSnapshot{Projection: model.Snapshot(), Delivery: PhaseOneOverlay, Reason: PhaseOneReportSpam, FailureCode: "refresh_pending"},
	}
	go c.runRefreshLoop()
	return c, nil
}

func newProductionWindowsTargetsInboxComposition(dir string, phaseOne *WindowsPhaseOneComposition) (*WindowsTargetsInboxComposition, error) {
	repository, err := newDefaultCredentialRepository(dir)
	if err != nil {
		return nil, err
	}
	bundle, err := repository.LoadBundle()
	if err != nil || bundle == nil {
		return nil, ErrPhaseOnePersistence
	}
	client, err := NewTargetsInboxAppClient(*bundle, nil)
	if err != nil {
		return nil, err
	}
	return NewWindowsTargetsInboxComposition(client, phaseOne)
}

func (c *WindowsTargetsInboxComposition) Snapshot() WindowsTargetsInboxSnapshot {
	if c == nil {
		return WindowsTargetsInboxSnapshot{Projection: TargetsInboxSnapshot{State: TargetsInboxCoordinatorError}, Delivery: PhaseOneOverlay, Reason: PhaseOneReportSpam, FailureCode: "targets_inbox_unavailable"}
	}
	c.mu.RLock()
	result := c.state
	result.Busy = result.Busy || c.refreshing
	c.mu.RUnlock()
	result.Projection = c.model.Snapshot()
	return result
}

func (c *WindowsTargetsInboxComposition) ApplyShellSnapshot(shell *ShellSnapshot) {
	if c == nil || shell == nil {
		return
	}
	activeTitle := ""
	for _, air := range shell.Airs {
		if air.Current {
			activeTitle = air.Title
			break
		}
	}
	projection := c.model.Snapshot()
	if projection.ActiveAirTitle != activeTitle {
		projection.ActiveAirTitle = activeTitle
		projection.AvailableAudiences = targetsInboxAudienceChoices(activeTitle, len(projection.Targets) > 0)
		c.model.Replace(projection, time.Now())
	}
	state := c.Snapshot()
	shell.TargetsInbox = state.Projection
	shell.SelectedTarget = state.SelectedTarget
	shell.SelectedInbox = state.SelectedInbox
	shell.SelectedTargetsHistory = state.SelectedHistory
	shell.TargetsInboxDelivery = state.Delivery
	shell.TargetsInboxReason = state.Reason
	shell.TargetsInboxActionOutcome = state.ActionOutcome
	shell.TargetsInboxFailure = state.FailureCode
	shell.TargetsInboxBusy = state.Busy
}

func (c *WindowsTargetsInboxComposition) Refresh() { c.requestRefresh(true) }

func (c *WindowsTargetsInboxComposition) SelectNextAudience() {
	snapshot := c.model.Snapshot()
	if snapshot.State != TargetsInboxReady || len(snapshot.AvailableAudiences) == 0 {
		return
	}
	start := -1
	for index, audience := range snapshot.AvailableAudiences {
		if audience.Kind == snapshot.SelectedAudience {
			start = index
			break
		}
	}
	for offset := 1; offset <= len(snapshot.AvailableAudiences); offset++ {
		candidate := snapshot.AvailableAudiences[(start+offset)%len(snapshot.AvailableAudiences)].Kind
		command, ok := c.model.BuildCommand(TargetsInboxCommand{Kind: TargetsInboxSetAudience, Audience: candidate})
		if ok {
			snapshot.SelectedAudience = command.Audience
			c.replaceLocal(snapshot)
			return
		}
	}
}

func (c *WindowsTargetsInboxComposition) SelectNextTarget() {
	c.mu.Lock()
	if count := len(c.model.Snapshot().Targets); count > 0 {
		c.state.SelectedTarget = (c.state.SelectedTarget + 1) % count
	}
	c.mu.Unlock()
}

func (c *WindowsTargetsInboxComposition) ToggleSelectedTarget() {
	snapshot := c.model.Snapshot()
	c.mu.RLock()
	index := c.state.SelectedTarget
	c.mu.RUnlock()
	if snapshot.State != TargetsInboxReady || index < 0 || index >= len(snapshot.Targets) {
		return
	}
	reference := snapshot.Targets[index].Reference
	selected := append([]string(nil), snapshot.SelectedReferences...)
	found := -1
	for i, value := range selected {
		if value == reference {
			found = i
			break
		}
	}
	if found >= 0 {
		selected = append(selected[:found], selected[found+1:]...)
		if len(selected) == 0 {
			return
		}
	} else {
		selected = append(selected, reference)
	}
	command, ok := c.model.BuildCommand(TargetsInboxCommand{Kind: TargetsInboxSelectTargets, References: selected})
	if !ok {
		return
	}
	snapshot.SelectedReferences = command.References
	if snapshot.SelectedAudience == "" {
		snapshot.SelectedAudience = TargetsInboxExplicitAudience
	}
	c.replaceLocal(snapshot)
}

func (c *WindowsTargetsInboxComposition) ToggleIncludeOrigin() {
	snapshot := c.model.Snapshot()
	command, ok := c.model.BuildCommand(TargetsInboxCommand{Kind: TargetsInboxSetIncludeOrigin, Enabled: !snapshot.IncludeOrigin})
	if !ok {
		return
	}
	snapshot.IncludeOrigin = command.Enabled
	c.replaceLocal(snapshot)
}

func (c *WindowsTargetsInboxComposition) SelectNextDelivery() {
	deliveries := []PhaseOneDelivery{PhaseOneOverlay, PhaseOneInterrupt, PhaseOneAfterCurrent}
	c.mu.Lock()
	defer c.mu.Unlock()
	for index, delivery := range deliveries {
		if c.state.Delivery == delivery {
			c.state.Delivery = deliveries[(index+1)%len(deliveries)]
			return
		}
	}
	c.state.Delivery = PhaseOneOverlay
}

func (c *WindowsTargetsInboxComposition) SendSelectedDraft() {
	projection := c.model.Snapshot()
	c.mu.RLock()
	delivery := c.state.Delivery
	c.mu.RUnlock()
	if projection.State != TargetsInboxReady || projection.SelectedAudience == "" {
		return
	}
	if projection.SelectedAudience == TargetsInboxExplicitAudience {
		if _, ok := c.model.BuildCommand(TargetsInboxCommand{Kind: TargetsInboxSetAudience, Audience: TargetsInboxExplicitAudience}); !ok {
			return
		}
		c.phaseOne.SendSelectedDraftIntent(TargetsInboxExplicitAudience, projection.SelectedReferences, projection.IncludeOrigin, delivery)
		return
	}
	if _, ok := c.model.BuildCommand(TargetsInboxCommand{Kind: TargetsInboxSetAudience, Audience: projection.SelectedAudience}); !ok {
		return
	}
	c.phaseOne.SendSelectedDraftIntent(projection.SelectedAudience, nil, projection.SelectedAudience == PhaseOneThisPulsar, delivery)
}

func (c *WindowsTargetsInboxComposition) SelectNextInbox() {
	c.mu.Lock()
	if count := len(c.model.Snapshot().Inbox); count > 0 {
		c.state.SelectedInbox = (c.state.SelectedInbox + 1) % count
	}
	c.clearOutcomeLocked()
	c.mu.Unlock()
}

func (c *WindowsTargetsInboxComposition) SelectNextHistory() {
	c.mu.Lock()
	if count := len(c.model.Snapshot().History); count > 0 {
		c.state.SelectedHistory = (c.state.SelectedHistory + 1) % count
	}
	c.clearOutcomeLocked()
	c.mu.Unlock()
}

func (c *WindowsTargetsInboxComposition) SelectNextReason() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clearOutcomeLocked()
	for index, reason := range phaseOneModerationReasons {
		if c.state.Reason == reason {
			c.state.Reason = phaseOneModerationReasons[(index+1)%len(phaseOneModerationReasons)]
			return
		}
	}
	c.state.Reason = PhaseOneReportSpam
}

func (c *WindowsTargetsInboxComposition) LoadMoreInbox() {
	command, ok := c.model.BuildCommand(TargetsInboxCommand{Kind: TargetsInboxLoadMoreInbox, Cursor: c.model.Snapshot().InboxNextCursor})
	if ok {
		c.runRemote(command)
	}
}

func (c *WindowsTargetsInboxComposition) LoadMoreHistory() {
	command, ok := c.model.BuildCommand(TargetsInboxCommand{Kind: TargetsInboxLoadMoreHistory, Cursor: c.model.Snapshot().HistoryNextCursor})
	if ok {
		c.runRemote(command)
	}
}

func (c *WindowsTargetsInboxComposition) LoadMoreReceipts() {
	item, ok := c.selectedHistoryItem()
	if !ok {
		return
	}
	command, valid := c.model.BuildCommand(TargetsInboxCommand{Kind: TargetsInboxLoadMoreReceipts, ObjectID: item.ID, Cursor: item.ReceiptPage.NextCursor})
	if valid {
		c.runRemote(command)
	}
}

func (c *WindowsTargetsInboxComposition) ReplaySelectedInbox() {
	item, ok := c.selectedInboxItem()
	if !ok {
		return
	}
	c.mu.RLock()
	delivery := c.state.Delivery
	c.mu.RUnlock()
	command, valid := c.model.BuildCommand(TargetsInboxCommand{Kind: TargetsInboxReplayInbox, ObjectID: item.ID, Delivery: delivery})
	if valid {
		c.runRemote(command)
	}
}

func (c *WindowsTargetsInboxComposition) DismissSelectedInbox() {
	item, ok := c.selectedInboxItem()
	if !ok {
		return
	}
	command, valid := c.model.BuildCommand(TargetsInboxCommand{Kind: TargetsInboxDismissInbox, ObjectID: item.ID})
	if valid {
		c.runRemote(command)
	}
}

func (c *WindowsTargetsInboxComposition) DeleteSelectedHistory() {
	item, ok := c.selectedHistoryItem()
	if !ok {
		return
	}
	command, valid := c.model.BuildCommand(TargetsInboxCommand{Kind: TargetsInboxDeleteHistory, ObjectID: item.ID})
	if valid {
		c.runRemote(command)
	}
}

func (c *WindowsTargetsInboxComposition) ReportSelectedInbox(details string) {
	item, ok := c.selectedInboxItem()
	if !ok {
		return
	}
	c.mu.RLock()
	reason := c.state.Reason
	c.mu.RUnlock()
	command, valid := c.model.BuildCommand(TargetsInboxCommand{Kind: TargetsInboxReportInbox, ObjectID: item.ID, Reason: reason, Details: strings.TrimSpace(details)})
	if valid {
		c.runRemote(command)
	}
}

func (c *WindowsTargetsInboxComposition) ReportSelectedHistory(details string) {
	item, ok := c.selectedHistoryItem()
	if !ok {
		return
	}
	c.mu.RLock()
	reason := c.state.Reason
	c.mu.RUnlock()
	command, valid := c.model.BuildCommand(TargetsInboxCommand{Kind: TargetsInboxReportHistory, ObjectID: item.ID, Reason: reason, Details: strings.TrimSpace(details)})
	if valid {
		c.runRemote(command)
	}
}

func (c *WindowsTargetsInboxComposition) MuteSelectedInbox() {
	item, ok := c.selectedInboxItem()
	if !ok {
		return
	}
	command, valid := c.model.BuildCommand(TargetsInboxCommand{Kind: TargetsInboxMuteSender, ObjectID: item.ID})
	if valid {
		c.runRemote(command)
	}
}

func (c *WindowsTargetsInboxComposition) MuteSelectedHistory() {
	item, ok := c.selectedHistoryItem()
	if !ok {
		return
	}
	command, valid := c.model.BuildCommand(TargetsInboxCommand{Kind: TargetsInboxMuteSender, ObjectID: item.ID})
	if valid {
		c.runRemote(command)
	}
}

func (c *WindowsTargetsInboxComposition) selectedInboxItem() (TargetsInboxInboxItem, bool) {
	snapshot := c.model.Snapshot()
	c.mu.RLock()
	index := c.state.SelectedInbox
	c.mu.RUnlock()
	if index < 0 || index >= len(snapshot.Inbox) {
		return TargetsInboxInboxItem{}, false
	}
	return snapshot.Inbox[index], true
}

func (c *WindowsTargetsInboxComposition) selectedHistoryItem() (TargetsInboxHistoryItem, bool) {
	snapshot := c.model.Snapshot()
	c.mu.RLock()
	index := c.state.SelectedHistory
	c.mu.RUnlock()
	if index < 0 || index >= len(snapshot.History) {
		return TargetsInboxHistoryItem{}, false
	}
	return snapshot.History[index], true
}

func (c *WindowsTargetsInboxComposition) runRemote(command TargetsInboxCommand) {
	if _, ok := c.model.BuildCommand(command); !ok || !c.beginMutation() {
		return
	}
	go func() {
		outcome, err := c.execute(command)
		if err == nil && command.Kind != TargetsInboxLoadMoreInbox && command.Kind != TargetsInboxLoadMoreHistory && command.Kind != TargetsInboxLoadMoreReceipts {
			err = c.refreshProjection(true, outcome)
		}
		if cursorExpired(err) {
			c.endMutation("", "")
			c.requestRefresh(true)
			return
		}
		c.endMutation(phaseOneFailureCodeOrEmpty(err), outcome)
	}()
}

func (c *WindowsTargetsInboxComposition) execute(command TargetsInboxCommand) (string, error) {
	switch command.Kind {
	case TargetsInboxLoadMoreInbox:
		page, err := c.service.Inbox(c.ctx, command.Cursor)
		if err == nil {
			c.appendInbox(page)
		}
		return "", err
	case TargetsInboxLoadMoreHistory:
		page, err := c.service.History(c.ctx, command.Cursor)
		if err == nil {
			c.appendHistory(page)
		}
		return "", err
	case TargetsInboxLoadMoreReceipts:
		page, err := c.service.Receipts(c.ctx, command.ObjectID, command.Cursor)
		if err == nil {
			c.appendReceipts(command.ObjectID, page)
		}
		return "", err
	case TargetsInboxReplayInbox:
		return c.service.ReplayInbox(c.ctx, command.ObjectID, command.Delivery, targetsInboxKey("inbox-replay"))
	case TargetsInboxDismissInbox:
		return c.service.DismissInbox(c.ctx, command.ObjectID)
	case TargetsInboxDeleteHistory:
		return c.service.DeleteTargetsHistory(c.ctx, command.ObjectID)
	case TargetsInboxReportInbox, TargetsInboxReportHistory:
		return c.service.ReportTargetsHistory(c.ctx, command.ObjectID, command.Reason, command.Details)
	case TargetsInboxMuteSender:
		return c.service.MuteTargetsHistorySender(c.ctx, command.ObjectID, targetsInboxKey("mute-sender"))
	default:
		return "", ErrPhaseOneInvalidDraft
	}
}

func (c *WindowsTargetsInboxComposition) runRefreshLoop() {
	c.requestRefresh(true)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-c.ctx.Done():
			return
		case force := <-c.wake:
			_ = c.refreshProjection(force, "")
		case <-ticker.C:
			_ = c.refreshProjection(false, "")
		}
	}
}

func (c *WindowsTargetsInboxComposition) requestRefresh(force bool) {
	select {
	case c.wake <- force:
	default:
	}
}

func (c *WindowsTargetsInboxComposition) refreshProjection(force bool, preservingOutcome string) error {
	if c == nil || c.ctx.Err() != nil {
		return context.Canceled
	}
	previous := c.model.Snapshot()
	if !force && len(previous.SelectedReferences) > 0 {
		expiresByReference := map[string]time.Time{}
		for _, target := range previous.Targets {
			expiresByReference[target.Reference] = target.ExpiresAt
		}
		expired := false
		for _, reference := range previous.SelectedReferences {
			if expiry, ok := expiresByReference[reference]; !ok || !expiry.After(time.Now()) {
				expired = true
				break
			}
		}
		if !expired {
			return nil
		}
		force = true
	}
	c.mu.Lock()
	if c.refreshing || c.state.Busy && preservingOutcome == "" {
		c.mu.Unlock()
		return nil
	}
	c.refreshing = true
	defer func() { c.mu.Lock(); c.refreshing = false; c.mu.Unlock() }()
	retained := len(previous.Targets) > 0 || len(previous.Inbox) > 0 || len(previous.History) > 0
	if retained {
		previous.State = TargetsInboxStale
	} else {
		previous.State = TargetsInboxLoading
	}
	previous.StateLabel = targetsStateLabel(previous.State)
	c.model.Replace(previous, time.Now())
	c.mu.Unlock()
	ctx, cancel := context.WithTimeout(c.ctx, 4*time.Second)
	defer cancel()
	projection, err := c.service.Projection(ctx)
	if err != nil {
		failed := previous
		failed.State = TargetsInboxCoordinatorError
		var client *PhaseOneClientError
		if errorAsPhaseOne(err, &client) && client.Kind == PhaseOneTransport {
			failed.State = TargetsInboxOffline
		}
		failed.StateLabel = targetsStateLabel(failed.State)
		c.model.Replace(failed, time.Now())
		c.mu.Lock()
		c.state.FailureCode = phaseOneFailureCode(err)
		c.mu.Unlock()
		return err
	}
	next := mapTargetsProjection(projection, previous, time.Now())
	c.model.Replace(next, time.Now())
	c.mu.Lock()
	c.normalizeSelectionsLocked(next)
	if preservingOutcome != "" {
		c.state.ActionOutcome = preservingOutcome
	}
	c.state.FailureCode = ""
	c.mu.Unlock()
	return nil
}

func mapTargetsProjection(value TargetsInboxProjection, previous TargetsInboxSnapshot, now time.Time) TargetsInboxSnapshot {
	allowed := map[string]bool{}
	for _, target := range value.Targets {
		allowed[target.Reference] = true
	}
	selected := make([]string, 0, len(previous.SelectedReferences))
	for _, reference := range previous.SelectedReferences {
		if allowed[reference] {
			selected = append(selected, reference)
		}
	}
	activeTitle := previous.ActiveAirTitle
	result := TargetsInboxSnapshot{State: TargetsInboxReady, StateLabel: targetsStateLabel(TargetsInboxReady), ActiveAirTitle: activeTitle,
		AvailableAudiences: targetsInboxAudienceChoices(activeTitle, len(value.Targets) > 0), SelectedAudience: previous.SelectedAudience,
		Targets: value.Targets, SelectedReferences: selected, IncludeOrigin: previous.IncludeOrigin,
		TargetedTrackPolicy: "unsupported", ContentPolicyState: value.ContentPolicyState,
		Inbox: value.Inbox.Items, InboxNextCursor: value.Inbox.NextCursor, History: value.History.Items, HistoryNextCursor: value.History.NextCursor}
	if result.SelectedAudience == TargetsInboxExplicitAudience && len(selected) == 0 {
		result.SelectedAudience = ""
	}
	return normalizeTargetsInboxSnapshot(result, now)
}

func targetsInboxAudienceChoices(activeTitle string, hasTargets bool) []TargetsInboxAudienceChoice {
	values := []TargetsInboxAudienceChoice{
		{Kind: PhaseOneThisPulsar, Label: targetsLabel("audience.this_pulsar", "This Pulsar", "Этот Пульсар")},
		{Kind: PhaseOneOwnBarycenter, Label: targetsLabel("audience.own_barycenter", "My Barycenter", "Мой Барицентр")},
	}
	if strings.TrimSpace(activeTitle) != "" {
		values = append(values, TargetsInboxAudienceChoice{Kind: PhaseOneCurrentAir, Label: targetsLabel("audience.current_air_named", "Current Air with «"+activeTitle+"»", "Текущий эфир с «"+activeTitle+"»")})
	}
	if hasTargets {
		values = append(values, TargetsInboxAudienceChoice{Kind: TargetsInboxExplicitAudience, Label: targetsLabel("audience.explicit", "Selected recipients", "Выбранные получатели")})
	}
	return values
}

func targetsStateLabel(state TargetsInboxSurfaceState) TargetsInboxLocalizedLabel {
	switch state {
	case TargetsInboxLoading:
		return targetsLabel("surface.loading", "Loading", "Загрузка")
	case TargetsInboxReady:
		return targetsLabel("surface.ready", "Up to date", "Актуально")
	case TargetsInboxStale:
		return targetsLabel("surface.stale", "May be out of date", "Данные могут быть устаревшими")
	case TargetsInboxOffline:
		return targetsLabel("surface.offline", "Offline", "Нет сети")
	default:
		return targetsLabel("surface.coordinator_error", "Coordinator unavailable", "Координатор недоступен")
	}
}

func targetsLabel(key, en, ru string) TargetsInboxLocalizedLabel {
	return TargetsInboxLocalizedLabel{Key: key, EN: en, RU: ru}
}

func (c *WindowsTargetsInboxComposition) replaceLocal(snapshot TargetsInboxSnapshot) {
	c.model.Replace(snapshot, time.Now())
	c.mu.Lock()
	c.clearOutcomeLocked()
	c.mu.Unlock()
}

func (c *WindowsTargetsInboxComposition) beginMutation() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state.Busy || c.refreshing || c.ctx.Err() != nil {
		return false
	}
	c.state.Busy = true
	c.clearOutcomeLocked()
	return true
}

func (c *WindowsTargetsInboxComposition) endMutation(failure, outcome string) {
	c.mu.Lock()
	c.state.Busy = false
	c.state.FailureCode = failure
	c.state.ActionOutcome = outcome
	c.mu.Unlock()
}

func (c *WindowsTargetsInboxComposition) clearOutcomeLocked() {
	c.state.ActionOutcome = ""
	c.state.FailureCode = ""
}

func (c *WindowsTargetsInboxComposition) normalizeSelectionsLocked(snapshot TargetsInboxSnapshot) {
	if c.state.SelectedTarget >= len(snapshot.Targets) {
		c.state.SelectedTarget = 0
	}
	if c.state.SelectedInbox >= len(snapshot.Inbox) {
		c.state.SelectedInbox = 0
	}
	if c.state.SelectedHistory >= len(snapshot.History) {
		c.state.SelectedHistory = 0
	}
}

func (c *WindowsTargetsInboxComposition) appendInbox(page TargetsInboxPage) {
	snapshot := c.model.Snapshot()
	seen := map[string]bool{}
	for _, item := range snapshot.Inbox {
		seen[item.ID] = true
	}
	for _, item := range page.Items {
		if !seen[item.ID] {
			snapshot.Inbox = append(snapshot.Inbox, item)
			seen[item.ID] = true
		}
	}
	snapshot.InboxNextCursor = page.NextCursor
	c.model.Replace(snapshot, time.Now())
}

func (c *WindowsTargetsInboxComposition) appendHistory(page TargetsInboxHistoryPage) {
	snapshot := c.model.Snapshot()
	seen := map[string]bool{}
	for _, item := range snapshot.History {
		seen[item.ID] = true
	}
	for _, item := range page.Items {
		if !seen[item.ID] {
			snapshot.History = append(snapshot.History, item)
			seen[item.ID] = true
		}
	}
	snapshot.HistoryNextCursor = page.NextCursor
	c.model.Replace(snapshot, time.Now())
}

func (c *WindowsTargetsInboxComposition) appendReceipts(historyID string, page TargetsInboxReceiptPage) {
	snapshot := c.model.Snapshot()
	for index := range snapshot.History {
		if snapshot.History[index].ID == historyID {
			seen := map[string]bool{}
			for _, item := range snapshot.History[index].ReceiptPage.Items {
				seen[item.TargetLabel] = true
			}
			for _, item := range page.Items {
				if !seen[item.TargetLabel] {
					snapshot.History[index].ReceiptPage.Items = append(snapshot.History[index].ReceiptPage.Items, item)
					seen[item.TargetLabel] = true
				}
			}
			snapshot.History[index].ReceiptPage.NextCursor = page.NextCursor
			break
		}
	}
	c.model.Replace(snapshot, time.Now())
}

func targetsInboxKey(prefix string) string {
	return fmt.Sprintf("windows-%s-%d", prefix, time.Now().UnixNano())
}

func cursorExpired(err error) bool {
	var client *PhaseOneClientError
	return errorAsPhaseOne(err, &client) && client.Kind == PhaseOneRejected && client.Status == 410 && client.Code == "cursor_expired"
}

func errorAsPhaseOne(err error, target **PhaseOneClientError) bool { return errors.As(err, target) }

func (c *WindowsTargetsInboxComposition) Close() {
	if c != nil && c.cancel != nil {
		c.cancel()
	}
}

func selectedTargetCount(snapshot TargetsInboxSnapshot) int { return len(snapshot.SelectedReferences) }

func targetIsSelected(snapshot TargetsInboxSnapshot, reference string) bool {
	for _, value := range snapshot.SelectedReferences {
		if value == reference {
			return true
		}
	}
	return false
}
