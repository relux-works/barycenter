package main

import (
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

type TargetsInboxSurfaceState string

const (
	TargetsInboxLoading          TargetsInboxSurfaceState = "loading"
	TargetsInboxReady            TargetsInboxSurfaceState = "ready"
	TargetsInboxStale            TargetsInboxSurfaceState = "stale"
	TargetsInboxOffline          TargetsInboxSurfaceState = "offline"
	TargetsInboxCoordinatorError TargetsInboxSurfaceState = "coordinator_error"
	TargetsInboxExplicitAudience PhaseOneRoute            = "explicit"
)

type TargetsInboxLocalizedLabel struct {
	Key string `json:"key"`
	EN  string `json:"en"`
	RU  string `json:"ru"`
}

func (label TargetsInboxLocalizedLabel) Text(locale ShellLocale) string {
	if locale == ShellRussian {
		return label.RU
	}
	return label.EN
}

type TargetsInboxActionCapability struct {
	Action string                     `json:"action"`
	Label  TargetsInboxLocalizedLabel `json:"label"`
}

type TargetsInboxAudienceChoice struct {
	Kind  PhaseOneRoute
	Label TargetsInboxLocalizedLabel
}

type TargetsInboxTargetChoice struct {
	Reference       string
	Kind            string
	ExpiresAt       time.Time
	CapabilityState string
	Capabilities    []string
	Label           TargetsInboxLocalizedLabel
}

func (choice TargetsInboxTargetChoice) String() string   { return "TargetsInboxTargetChoice{<opaque>}" }
func (choice TargetsInboxTargetChoice) GoString() string { return choice.String() }

type TargetsInboxInboxItem struct {
	ID                string
	HistoryItemID     string
	Title             string
	ExpiresAt         time.Time
	Availability      string
	Sender            TargetsInboxLocalizedLabel
	Source            TargetsInboxLocalizedLabel
	RequestedDelivery TargetsInboxLocalizedLabel
	EffectiveDelivery TargetsInboxLocalizedLabel
	Receipt           TargetsInboxLocalizedLabel
	Actions           []TargetsInboxActionCapability
}

type TargetsInboxHistoryItem struct {
	ID          string
	Title       string
	Status      TargetsInboxLocalizedLabel
	Actions     []TargetsInboxActionCapability
	Played      int
	Other       int
	ReceiptPage TargetsInboxReceiptPage
}

type TargetsInboxReceipt struct {
	TargetLabel string
	Status      TargetsInboxLocalizedLabel
}

type TargetsInboxReceiptPage struct {
	Items      []TargetsInboxReceipt
	NextCursor string
}

type TargetsInboxSnapshot struct {
	State               TargetsInboxSurfaceState
	StateLabel          TargetsInboxLocalizedLabel
	ActiveAirTitle      string
	AvailableAudiences  []TargetsInboxAudienceChoice
	SelectedAudience    PhaseOneRoute
	Targets             []TargetsInboxTargetChoice
	SelectedReferences  []string
	IncludeOrigin       bool
	TargetedTrackPolicy string
	ContentPolicyState  string
	Inbox               []TargetsInboxInboxItem
	InboxNextCursor     string
	History             []TargetsInboxHistoryItem
	HistoryNextCursor   string
}

type TargetsInboxCommandKind string

const (
	TargetsInboxRefresh          TargetsInboxCommandKind = "refresh"
	TargetsInboxSetAudience      TargetsInboxCommandKind = "set_audience"
	TargetsInboxSelectTargets    TargetsInboxCommandKind = "select_targets"
	TargetsInboxSetIncludeOrigin TargetsInboxCommandKind = "set_include_origin"
	TargetsInboxLoadMoreInbox    TargetsInboxCommandKind = "load_more_inbox"
	TargetsInboxLoadMoreHistory  TargetsInboxCommandKind = "load_more_history"
	TargetsInboxLoadMoreReceipts TargetsInboxCommandKind = "load_more_receipts"
	TargetsInboxReplayInbox      TargetsInboxCommandKind = "replay_inbox"
	TargetsInboxDismissInbox     TargetsInboxCommandKind = "dismiss_inbox"
	TargetsInboxDeleteHistory    TargetsInboxCommandKind = "delete_history"
	TargetsInboxReportInbox      TargetsInboxCommandKind = "report_inbox"
	TargetsInboxReportHistory    TargetsInboxCommandKind = "report_history"
	TargetsInboxMuteSender       TargetsInboxCommandKind = "mute_sender"
)

type TargetsInboxCommand struct {
	Kind       TargetsInboxCommandKind
	ObjectID   string
	Audience   PhaseOneRoute
	Cursor     string
	References []string
	Enabled    bool
	Delivery   PhaseOneDelivery
	Reason     PhaseOneModerationReason
	Details    string
}

func (command TargetsInboxCommand) String() string {
	return "TargetsInboxCommand{" + string(command.Kind) + ",<opaque>}"
}
func (command TargetsInboxCommand) GoString() string { return command.String() }

type TargetsInboxModel struct {
	mu       sync.RWMutex
	snapshot TargetsInboxSnapshot
}

var (
	targetReferencePattern   = regexp.MustCompile(`^trf_[A-Za-z0-9_-]{43}$`)
	inboxCapabilityPattern   = regexp.MustCompile(`^ib_[0-9A-HJKMNP-TV-Z]{26}$`)
	historyCapabilityPattern = regexp.MustCompile(`^hi_[0-9A-HJKMNP-TV-Z]{26}$`)
	inboxCursorPattern       = regexp.MustCompile(`^ic_[0-9a-f]{64}$`)
	historyCursorPattern     = regexp.MustCompile(`^hc_[0-9a-f]{64}$`)
	receiptCursorPattern     = regexp.MustCompile(`^rc_[0-9a-f]{64}$`)
	targetsInboxEnumPattern  = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
)

func NewTargetsInboxModel() *TargetsInboxModel {
	return &TargetsInboxModel{snapshot: TargetsInboxSnapshot{State: TargetsInboxLoading}}
}

func (model *TargetsInboxModel) Replace(snapshot TargetsInboxSnapshot, now time.Time) {
	if model == nil {
		return
	}
	snapshot = normalizeTargetsInboxSnapshot(snapshot, now)
	model.mu.Lock()
	model.snapshot = snapshot
	model.mu.Unlock()
}

func (model *TargetsInboxModel) Snapshot() TargetsInboxSnapshot {
	if model == nil {
		return TargetsInboxSnapshot{State: TargetsInboxCoordinatorError}
	}
	model.mu.RLock()
	defer model.mu.RUnlock()
	return cloneTargetsInboxSnapshot(model.snapshot)
}

func normalizeTargetsInboxSnapshot(snapshot TargetsInboxSnapshot, now time.Time) TargetsInboxSnapshot {
	snapshot = cloneTargetsInboxSnapshot(snapshot)
	switch snapshot.State {
	case TargetsInboxLoading, TargetsInboxReady, TargetsInboxStale, TargetsInboxOffline, TargetsInboxCoordinatorError:
	default:
		snapshot.State = TargetsInboxCoordinatorError
	}
	current := make(map[string]bool, len(snapshot.Targets))
	targets := snapshot.Targets[:0]
	for _, target := range snapshot.Targets {
		if !targetReferencePattern.MatchString(target.Reference) || !target.ExpiresAt.After(now) || current[target.Reference] {
			continue
		}
		current[target.Reference] = true
		target.Capabilities = canonicalTargetsInboxEnums(target.Capabilities)
		targets = append(targets, target)
	}
	snapshot.Targets = targets
	audiences := snapshot.AvailableAudiences[:0]
	availableAudiences := make(map[PhaseOneRoute]bool, len(snapshot.AvailableAudiences))
	for _, audience := range snapshot.AvailableAudiences {
		if !validTargetsInboxAudience(audience.Kind) || availableAudiences[audience.Kind] ||
			audience.Kind == PhaseOneCurrentAir && strings.TrimSpace(snapshot.ActiveAirTitle) == "" ||
			audience.Kind == TargetsInboxExplicitAudience && len(snapshot.Targets) == 0 {
			continue
		}
		availableAudiences[audience.Kind] = true
		audiences = append(audiences, audience)
	}
	snapshot.AvailableAudiences = audiences
	if !availableAudiences[snapshot.SelectedAudience] {
		snapshot.SelectedAudience = ""
	}
	selected := snapshot.SelectedReferences[:0]
	seen := map[string]bool{}
	for _, reference := range snapshot.SelectedReferences {
		if current[reference] && !seen[reference] && len(selected) < 64 {
			seen[reference] = true
			selected = append(selected, reference)
		}
	}
	snapshot.SelectedReferences = selected
	for index := range snapshot.Inbox {
		snapshot.Inbox[index].Actions = canonicalTargetsInboxActions(snapshot.Inbox[index].Actions)
		if !inboxCapabilityPattern.MatchString(snapshot.Inbox[index].ID) ||
			!historyCapabilityPattern.MatchString(snapshot.Inbox[index].HistoryItemID) {
			snapshot.Inbox[index].Actions = nil
		}
		if !snapshot.Inbox[index].ExpiresAt.After(now) {
			snapshot.Inbox[index].Availability = "expired"
			snapshot.Inbox[index].Actions = nil
		}
	}
	for index := range snapshot.History {
		snapshot.History[index].Actions = canonicalTargetsInboxActions(snapshot.History[index].Actions)
		if !historyCapabilityPattern.MatchString(snapshot.History[index].ID) {
			snapshot.History[index].Actions = nil
		}
	}
	if snapshot.State != TargetsInboxReady {
		// Retained rows remain visible, but capability-bearing selections are
		// cleared until a fresh model arrives.
		snapshot.SelectedReferences = nil
	}
	return snapshot
}

func canonicalTargetsInboxEnums(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if !targetsInboxEnumPattern.MatchString(value) || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func canonicalTargetsInboxActions(values []TargetsInboxActionCapability) []TargetsInboxActionCapability {
	seen := map[string]bool{}
	result := make([]TargetsInboxActionCapability, 0, len(values))
	for _, value := range values {
		if !targetsInboxEnumPattern.MatchString(value.Action) || seen[value.Action] {
			continue
		}
		seen[value.Action] = true
		result = append(result, value)
	}
	return result
}

func validTargetsInboxAudience(value PhaseOneRoute) bool {
	return validPhaseOneRoute(value) || value == TargetsInboxExplicitAudience
}

func cloneTargetsInboxSnapshot(value TargetsInboxSnapshot) TargetsInboxSnapshot {
	value.Targets = append([]TargetsInboxTargetChoice(nil), value.Targets...)
	value.AvailableAudiences = append([]TargetsInboxAudienceChoice(nil), value.AvailableAudiences...)
	for index := range value.Targets {
		value.Targets[index].Capabilities = append([]string(nil), value.Targets[index].Capabilities...)
	}
	value.SelectedReferences = append([]string(nil), value.SelectedReferences...)
	value.Inbox = append([]TargetsInboxInboxItem(nil), value.Inbox...)
	for index := range value.Inbox {
		value.Inbox[index].Actions = append([]TargetsInboxActionCapability(nil), value.Inbox[index].Actions...)
	}
	value.History = append([]TargetsInboxHistoryItem(nil), value.History...)
	for index := range value.History {
		value.History[index].Actions = append([]TargetsInboxActionCapability(nil), value.History[index].Actions...)
		value.History[index].ReceiptPage.Items = append([]TargetsInboxReceipt(nil), value.History[index].ReceiptPage.Items...)
	}
	return value
}

func targetsInboxHasAction(actions []TargetsInboxActionCapability, action string) bool {
	for _, capability := range actions {
		if capability.Action == action {
			return true
		}
	}
	return false
}

func (model *TargetsInboxModel) BuildCommand(request TargetsInboxCommand) (TargetsInboxCommand, bool) {
	snapshot := model.Snapshot()
	if request.Kind == TargetsInboxRefresh {
		return TargetsInboxCommand{Kind: TargetsInboxRefresh}, true
	}
	if snapshot.State != TargetsInboxReady {
		return TargetsInboxCommand{}, false
	}
	switch request.Kind {
	case TargetsInboxSetAudience:
		for _, audience := range snapshot.AvailableAudiences {
			if audience.Kind == request.Audience &&
				(request.Audience != TargetsInboxExplicitAudience || len(snapshot.SelectedReferences) > 0) {
				return TargetsInboxCommand{Kind: request.Kind, Audience: request.Audience}, true
			}
		}
	case TargetsInboxSelectTargets:
		allowed := make(map[string]bool, len(snapshot.Targets))
		for _, target := range snapshot.Targets {
			allowed[target.Reference] = true
		}
		if len(request.References) == 0 || len(request.References) > 64 {
			return TargetsInboxCommand{}, false
		}
		seen := map[string]bool{}
		for _, reference := range request.References {
			if !allowed[reference] || seen[reference] {
				return TargetsInboxCommand{}, false
			}
			seen[reference] = true
		}
		request.References = append([]string(nil), request.References...)
		return request, true
	case TargetsInboxSetIncludeOrigin:
		return TargetsInboxCommand{Kind: request.Kind, Enabled: request.Enabled}, true
	case TargetsInboxLoadMoreInbox:
		return request, request.Cursor == snapshot.InboxNextCursor && inboxCursorPattern.MatchString(request.Cursor)
	case TargetsInboxLoadMoreHistory:
		return request, request.Cursor == snapshot.HistoryNextCursor && historyCursorPattern.MatchString(request.Cursor)
	case TargetsInboxLoadMoreReceipts:
		for _, item := range snapshot.History {
			if item.ID == request.ObjectID && historyCapabilityPattern.MatchString(item.ID) &&
				request.Cursor == item.ReceiptPage.NextCursor && receiptCursorPattern.MatchString(request.Cursor) {
				return request, true
			}
		}
	case TargetsInboxReplayInbox, TargetsInboxDismissInbox:
		needed := "replay"
		if request.Kind == TargetsInboxDismissInbox {
			needed = "dismiss"
		}
		for _, item := range snapshot.Inbox {
			if item.ID == request.ObjectID && inboxCapabilityPattern.MatchString(item.ID) && targetsInboxHasAction(item.Actions, needed) {
				if needed == "replay" && (snapshot.ContentPolicyState != "current" || !validPhaseOneDelivery(request.Delivery)) {
					return TargetsInboxCommand{}, false
				}
				return request, true
			}
		}
	case TargetsInboxReportInbox:
		if !validPhaseOneModerationReason(request.Reason) || !validPhaseOneDisplayText(request.Details, 2000, true) {
			return TargetsInboxCommand{}, false
		}
		for _, item := range snapshot.Inbox {
			if item.ID == request.ObjectID && inboxCapabilityPattern.MatchString(item.ID) &&
				historyCapabilityPattern.MatchString(item.HistoryItemID) && targetsInboxHasAction(item.Actions, "report") {
				request.ObjectID = item.HistoryItemID
				return request, true
			}
		}
	case TargetsInboxDeleteHistory, TargetsInboxReportHistory:
		needed := map[TargetsInboxCommandKind]string{
			TargetsInboxDeleteHistory: "delete", TargetsInboxReportHistory: "report",
		}[request.Kind]
		for _, item := range snapshot.History {
			if item.ID != request.ObjectID || !historyCapabilityPattern.MatchString(item.ID) || !targetsInboxHasAction(item.Actions, needed) {
				continue
			}
			if request.Kind == TargetsInboxReportHistory && (!validPhaseOneModerationReason(request.Reason) || !validPhaseOneDisplayText(request.Details, 2000, true)) {
				return TargetsInboxCommand{}, false
			}
			return request, true
		}
	case TargetsInboxMuteSender:
		for _, item := range snapshot.History {
			if item.ID == request.ObjectID && historyCapabilityPattern.MatchString(item.ID) && targetsInboxHasAction(item.Actions, "block_actor") {
				return request, true
			}
		}
		for _, item := range snapshot.Inbox {
			if item.ID == request.ObjectID && inboxCapabilityPattern.MatchString(item.ID) &&
				historyCapabilityPattern.MatchString(item.HistoryItemID) && targetsInboxHasAction(item.Actions, "block_actor") {
				request.ObjectID = item.HistoryItemID
				return request, true
			}
		}
	}
	return TargetsInboxCommand{}, false
}
