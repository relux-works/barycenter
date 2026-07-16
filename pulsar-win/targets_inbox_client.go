package main

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const targetsInboxContract = "p2-targets-inbox-parity.v1"

type TargetsInboxPage struct {
	Items      []TargetsInboxInboxItem
	NextCursor string
}

type TargetsInboxHistoryPage struct {
	Items      []TargetsInboxHistoryItem
	NextCursor string
}

type TargetsInboxProjection struct {
	Targets            []TargetsInboxTargetChoice
	Inbox              TargetsInboxPage
	History            TargetsInboxHistoryPage
	ContentPolicyState string
}

type TargetsInboxAppService interface {
	Projection(context.Context) (TargetsInboxProjection, error)
	Inbox(context.Context, string) (TargetsInboxPage, error)
	History(context.Context, string) (TargetsInboxHistoryPage, error)
	Receipts(context.Context, string, string) (TargetsInboxReceiptPage, error)
	DismissInbox(context.Context, string) (string, error)
	ReplayInbox(context.Context, string, PhaseOneDelivery, string) (string, error)
	DeleteTargetsHistory(context.Context, string) (string, error)
	ReportTargetsHistory(context.Context, string, PhaseOneModerationReason, string) (string, error)
	MuteTargetsHistorySender(context.Context, string, string) (string, error)
}

// TargetsInboxAppClient shares the hardened Phase 1 authenticated transport.
// It never exposes a playback/read URL and only accepts server-minted opaque
// references and action capabilities from the frozen presentation contract.
type TargetsInboxAppClient struct{ base *PhaseOneAppClient }

func NewTargetsInboxAppClient(bundle CredentialBundle, doer HTTPDoer) (*TargetsInboxAppClient, error) {
	base, err := NewPhaseOneAppClient(bundle, doer)
	if err != nil {
		return nil, err
	}
	return &TargetsInboxAppClient{base: base}, nil
}

func (c *TargetsInboxAppClient) String() string   { return "TargetsInboxAppClient{<redacted>}" }
func (c *TargetsInboxAppClient) GoString() string { return c.String() }

func (c *TargetsInboxAppClient) Projection(ctx context.Context) (TargetsInboxProjection, error) {
	targets, err := c.targets(ctx)
	if err != nil {
		return TargetsInboxProjection{}, err
	}
	inbox, err := c.Inbox(ctx, "")
	if err != nil {
		return TargetsInboxProjection{}, err
	}
	history, err := c.History(ctx, "")
	if err != nil {
		return TargetsInboxProjection{}, err
	}
	policy, err := c.contentPolicyState(ctx)
	if err != nil {
		return TargetsInboxProjection{}, err
	}
	return TargetsInboxProjection{Targets: targets, Inbox: inbox, History: history, ContentPolicyState: policy}, nil
}

func (c *TargetsInboxAppClient) Inbox(ctx context.Context, cursor string) (TargetsInboxPage, error) {
	if !validTargetsCursor(cursor, "ic_") {
		return TargetsInboxPage{}, phaseOneError(PhaseOneInvalidRequest)
	}
	raw, _, err := c.base.request(ctx, http.MethodGet, targetsPagePath("/v1/inbox", 20, cursor), c.base.token, nil, nil, true, http.StatusOK)
	if err != nil {
		return TargetsInboxPage{}, err
	}
	var response targetsInboxResponse
	if decodePhaseOneJSON(raw, &response) != nil || response.Contract != targetsInboxContract ||
		!validTargetsCursor(response.NextCursor, "ic_") || !uniqueTargetsStrings(inboxIDs(response.Items)) {
		return TargetsInboxPage{}, phaseOneError(PhaseOneInvalidResponse)
	}
	items := make([]TargetsInboxInboxItem, 0, len(response.Items))
	for _, item := range response.Items {
		mapped, mapErr := decodeTargetsInboxItem(item)
		if mapErr != nil {
			return TargetsInboxPage{}, mapErr
		}
		items = append(items, mapped)
	}
	return TargetsInboxPage{Items: items, NextCursor: response.NextCursor}, nil
}

func (c *TargetsInboxAppClient) History(ctx context.Context, cursor string) (TargetsInboxHistoryPage, error) {
	if !validTargetsCursor(cursor, "hc_") {
		return TargetsInboxHistoryPage{}, phaseOneError(PhaseOneInvalidRequest)
	}
	raw, _, err := c.base.request(ctx, http.MethodGet, targetsPagePath("/v1/history", 30, cursor), c.base.token, nil, nil, true, http.StatusOK)
	if err != nil {
		return TargetsInboxHistoryPage{}, err
	}
	var response targetsHistoryResponse
	if decodePhaseOneJSON(raw, &response) != nil || response.Contract != "p1-history-presence-telegram-v1" ||
		!validTargetsCursor(response.NextCursor, "hc_") || !uniqueTargetsStrings(historyIDs(response.Items)) {
		return TargetsInboxHistoryPage{}, phaseOneError(PhaseOneInvalidResponse)
	}
	items := make([]TargetsInboxHistoryItem, 0, len(response.Items))
	for _, item := range response.Items {
		mapped, mapErr := decodeTargetsHistoryItem(item)
		if mapErr != nil {
			return TargetsInboxHistoryPage{}, mapErr
		}
		items = append(items, mapped)
	}
	return TargetsInboxHistoryPage{Items: items, NextCursor: response.NextCursor}, nil
}

func (c *TargetsInboxAppClient) Receipts(ctx context.Context, historyItemID, cursor string) (TargetsInboxReceiptPage, error) {
	if !historyCapabilityPattern.MatchString(historyItemID) || !validTargetsCursor(cursor, "rc_") {
		return TargetsInboxReceiptPage{}, phaseOneError(PhaseOneInvalidRequest)
	}
	path := targetsPagePath("/v1/history/"+historyItemID+"/receipts", 20, cursor)
	raw, _, err := c.base.request(ctx, http.MethodGet, path, c.base.token, nil, nil, true, http.StatusOK)
	if err != nil {
		return TargetsInboxReceiptPage{}, err
	}
	var response targetsReceiptResponse
	labels := make([]string, 0, len(response.Items))
	if decodePhaseOneJSON(raw, &response) != nil || response.Contract != targetsInboxContract ||
		response.HistoryItemID != historyItemID || !validTargetsCursor(response.NextCursor, "rc_") {
		return TargetsInboxReceiptPage{}, phaseOneError(PhaseOneInvalidResponse)
	}
	items := make([]TargetsInboxReceipt, 0, len(response.Items))
	for _, item := range response.Items {
		if !validTargetsHumanText(item.TargetLabel) {
			return TargetsInboxReceiptPage{}, phaseOneError(PhaseOneInvalidResponse)
		}
		labels = append(labels, item.TargetLabel)
		status, labelErr := decodeTargetsLabel(item.Presentation.Status)
		if labelErr != nil {
			return TargetsInboxReceiptPage{}, labelErr
		}
		items = append(items, TargetsInboxReceipt{TargetLabel: item.TargetLabel, Status: status})
	}
	if !uniqueTargetsStrings(labels) {
		return TargetsInboxReceiptPage{}, phaseOneError(PhaseOneInvalidResponse)
	}
	return TargetsInboxReceiptPage{Items: items, NextCursor: response.NextCursor}, nil
}

func (c *TargetsInboxAppClient) DismissInbox(ctx context.Context, inboxID string) (string, error) {
	if !inboxCapabilityPattern.MatchString(inboxID) {
		return "", phaseOneError(PhaseOneInvalidRequest)
	}
	raw, _, err := c.base.request(ctx, http.MethodDelete, "/v1/inbox/"+inboxID, c.base.token, nil, nil, true, http.StatusOK)
	if err != nil {
		return "", err
	}
	var response targetsInboxMutationResponse
	if decodePhaseOneJSON(raw, &response) != nil || response.Contract != targetsInboxContract ||
		response.Item.ID != inboxID || response.Item.Availability != "dismissed" {
		return "", phaseOneError(PhaseOneInvalidResponse)
	}
	return "inbox_dismissed", nil
}

func (c *TargetsInboxAppClient) ReplayInbox(ctx context.Context, inboxID string, delivery PhaseOneDelivery, key string) (string, error) {
	if !inboxCapabilityPattern.MatchString(inboxID) || !validPhaseOneDelivery(delivery) || !validTargetsIdempotencyKey(key) {
		return "", phaseOneError(PhaseOneInvalidRequest)
	}
	raw, _, err := c.base.requestJSON(ctx, http.MethodPost, "/v1/inbox/"+inboxID+"/replays", c.base.token,
		map[string]string{"Idempotency-Key": key}, struct {
			Delivery string `json:"delivery"`
		}{string(delivery)}, http.StatusOK, http.StatusCreated)
	if err != nil {
		return "", err
	}
	var response targetsReplayResponse
	if decodePhaseOneJSON(raw, &response) != nil || response.Contract != targetsInboxContract ||
		!historyCapabilityPattern.MatchString(response.HistoryItemID) ||
		!validPhaseOneDelivery(PhaseOneDelivery(response.RequestedDelivery)) ||
		!validPhaseOneDelivery(PhaseOneDelivery(response.EffectiveDelivery)) {
		return "", phaseOneError(PhaseOneInvalidResponse)
	}
	if response.Reused {
		return "replay_already_accepted", nil
	}
	return "replay_accepted", nil
}

func (c *TargetsInboxAppClient) DeleteTargetsHistory(ctx context.Context, historyItemID string) (string, error) {
	if !historyCapabilityPattern.MatchString(historyItemID) {
		return "", phaseOneError(PhaseOneInvalidRequest)
	}
	raw, _, err := c.base.requestJSON(ctx, http.MethodPost, "/v1/history/"+historyItemID+"/actions/delete", c.base.token, nil, struct{}{}, http.StatusOK)
	if err != nil {
		return "", err
	}
	var response targetsDeleteResponse
	if decodePhaseOneJSON(raw, &response) != nil || response.HistoryItemID != historyItemID || !response.Deleted {
		return "", phaseOneError(PhaseOneInvalidResponse)
	}
	return "media_deleted", nil
}

func (c *TargetsInboxAppClient) ReportTargetsHistory(ctx context.Context, historyItemID string, reason PhaseOneModerationReason, details string) (string, error) {
	if !historyCapabilityPattern.MatchString(historyItemID) || !validPhaseOneModerationReason(reason) ||
		strings.TrimSpace(details) != details || !validPhaseOneDisplayText(details, 2000, true) {
		return "", phaseOneError(PhaseOneInvalidRequest)
	}
	raw, _, err := c.base.requestJSON(ctx, http.MethodPost, "/v1/history/"+historyItemID+"/actions/report", c.base.token, nil, struct {
		Reason  string `json:"reason"`
		Details string `json:"details"`
	}{string(reason), details}, http.StatusOK, http.StatusCreated)
	if err != nil {
		return "", err
	}
	var response targetsReportResponse
	if decodePhaseOneJSON(raw, &response) != nil || response.HistoryItemID != historyItemID {
		return "", phaseOneError(PhaseOneInvalidResponse)
	}
	if response.Reused {
		return "report_already_received", nil
	}
	return "report_received", nil
}

func (c *TargetsInboxAppClient) MuteTargetsHistorySender(ctx context.Context, historyItemID, key string) (string, error) {
	if !historyCapabilityPattern.MatchString(historyItemID) || !validTargetsIdempotencyKey(key) {
		return "", phaseOneError(PhaseOneInvalidRequest)
	}
	raw, _, err := c.base.requestJSON(ctx, http.MethodPost, "/v1/history/"+historyItemID+"/actions/block_actor", c.base.token,
		map[string]string{"Idempotency-Key": key}, struct{}{}, http.StatusOK, http.StatusCreated)
	if err != nil {
		return "", err
	}
	var response targetsBlockResponse
	if decodePhaseOneJSON(raw, &response) != nil || !validPhaseOnePublicID(response.BlockID, "bl_") {
		return "", phaseOneError(PhaseOneInvalidResponse)
	}
	if response.Reused {
		return "sender_already_blocked", nil
	}
	return "sender_blocked", nil
}

func (c *TargetsInboxAppClient) targets(ctx context.Context) ([]TargetsInboxTargetChoice, error) {
	raw, _, err := c.base.request(ctx, http.MethodGet, "/v1/transmission-targets", c.base.token, nil, nil, true, http.StatusOK)
	if err != nil {
		return nil, err
	}
	var response targetsResponse
	refs := make([]string, 0, len(response.Targets))
	if decodePhaseOneJSON(raw, &response) != nil || response.Contract != targetsInboxContract {
		return nil, phaseOneError(PhaseOneInvalidResponse)
	}
	result := make([]TargetsInboxTargetChoice, 0, len(response.Targets))
	for _, target := range response.Targets {
		refs = append(refs, target.Reference)
		expiresAt, parseErr := time.Parse(time.RFC3339Nano, target.ExpiresAt)
		label, labelErr := decodeTargetsLabel(target.Presentation.Label)
		if parseErr != nil || labelErr != nil || !targetReferencePattern.MatchString(target.Reference) ||
			(target.Kind != "barycenter" && target.Kind != "pulsar") ||
			(target.CapabilityState != "known" && target.CapabilityState != "mixed" && target.CapabilityState != "unknown") ||
			strings.Join(target.Capabilities, "\x00") != strings.Join(target.Presentation.Capabilities, "\x00") ||
			!validTargetsEnumsExact(target.Capabilities) {
			return nil, phaseOneError(PhaseOneInvalidResponse)
		}
		result = append(result, TargetsInboxTargetChoice{Reference: target.Reference, Kind: target.Kind,
			ExpiresAt: expiresAt, CapabilityState: target.CapabilityState,
			Capabilities: append([]string(nil), target.Capabilities...), Label: label})
	}
	if !uniqueTargetsStrings(refs) {
		return nil, phaseOneError(PhaseOneInvalidResponse)
	}
	return result, nil
}

func (c *TargetsInboxAppClient) contentPolicyState(ctx context.Context) (string, error) {
	raw, _, err := c.base.request(ctx, http.MethodGet, "/v1/content-policy/acceptance", c.base.token, nil, nil, true, http.StatusOK)
	if err != nil {
		var rejected *PhaseOneClientError
		if asPhaseOneRejected(err, &rejected) && rejected.Status == http.StatusPreconditionRequired && rejected.Code == "content_policy_acceptance_required" {
			return "required", nil
		}
		return "", err
	}
	var response targetsPolicyResponse
	if decodePhaseOneJSON(raw, &response) != nil || response.Contract != "p2-content-policy-consent.v1" {
		return "", phaseOneError(PhaseOneInvalidResponse)
	}
	if response.Current && response.TermsAccepted {
		return "current", nil
	}
	return "stale", nil
}

func asPhaseOneRejected(err error, target **PhaseOneClientError) bool {
	value, ok := err.(*PhaseOneClientError)
	if ok {
		*target = value
	}
	return ok && value.Kind == PhaseOneRejected
}

func targetsPagePath(base string, limit int, cursor string) string {
	query := url.Values{"limit": {strconv.Itoa(limit)}}
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	return base + "?" + query.Encode()
}

func validTargetsCursor(value, prefix string) bool {
	if value == "" {
		return true
	}
	if !strings.HasPrefix(value, prefix) || len(value) != len(prefix)+64 {
		return false
	}
	for _, character := range value[len(prefix):] {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return false
		}
	}
	return true
}

func validTargetsIdempotencyKey(value string) bool {
	return len(value) >= 16 && validPhaseOneIdempotencyKey(value)
}

func validTargetsHumanText(value string) bool {
	return strings.TrimSpace(value) != "" && len([]byte(value)) <= 512
}

func validTargetsEnumKey(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '_' || character == '.') {
			return false
		}
	}
	return true
}

func validTargetsEnumsExact(values []string) bool {
	seen := map[string]bool{}
	for _, value := range values {
		if !targetsInboxEnumPattern.MatchString(value) || seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}

func uniqueTargetsStrings(values []string) bool {
	seen := map[string]bool{}
	for _, value := range values {
		if seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}

type targetsWireLabel struct {
	Key string `json:"key"`
	EN  string `json:"en"`
	RU  string `json:"ru"`
}

type targetsWireAction struct {
	Action string           `json:"action"`
	Label  targetsWireLabel `json:"label"`
}

func decodeTargetsLabel(value targetsWireLabel) (TargetsInboxLocalizedLabel, error) {
	if !validTargetsEnumKey(value.Key) || !validTargetsHumanText(value.EN) || !validTargetsHumanText(value.RU) {
		return TargetsInboxLocalizedLabel{}, phaseOneError(PhaseOneInvalidResponse)
	}
	return TargetsInboxLocalizedLabel{Key: value.Key, EN: value.EN, RU: value.RU}, nil
}

func decodeTargetsActions(values []targetsWireAction, exact []string) ([]TargetsInboxActionCapability, error) {
	if len(values) != len(exact) || !validTargetsEnumsExact(exact) {
		return nil, phaseOneError(PhaseOneInvalidResponse)
	}
	result := make([]TargetsInboxActionCapability, 0, len(values))
	for index, value := range values {
		if value.Action != exact[index] {
			return nil, phaseOneError(PhaseOneInvalidResponse)
		}
		label, err := decodeTargetsLabel(value.Label)
		if err != nil {
			return nil, err
		}
		result = append(result, TargetsInboxActionCapability{Action: value.Action, Label: label})
	}
	return result, nil
}

type targetsResponse struct {
	Contract string `json:"contract"`
	Targets  []struct {
		Reference       string   `json:"reference"`
		Kind            string   `json:"kind"`
		CapabilityState string   `json:"capability_state"`
		Capabilities    []string `json:"capabilities"`
		ExpiresAt       string   `json:"expires_at"`
		Presentation    struct {
			Label        targetsWireLabel `json:"label"`
			Capabilities []string         `json:"capabilities"`
		} `json:"presentation"`
	} `json:"targets"`
}

type targetsInboxWireItem struct {
	ID            string `json:"id"`
	HistoryItemID string `json:"history_item_id"`
	Media         struct {
		Title string `json:"title"`
	} `json:"media"`
	Availability string   `json:"availability"`
	ExpiresAt    string   `json:"expires_at"`
	Actions      []string `json:"actions"`
	Presentation struct {
		Sender            targetsWireLabel    `json:"sender"`
		Source            targetsWireLabel    `json:"source"`
		RequestedDelivery targetsWireLabel    `json:"requested_delivery"`
		EffectiveDelivery targetsWireLabel    `json:"effective_delivery"`
		Receipt           targetsWireLabel    `json:"receipt"`
		Actions           []targetsWireAction `json:"actions"`
	} `json:"presentation"`
}

type targetsInboxResponse struct {
	Contract   string                 `json:"contract"`
	NextCursor string                 `json:"next_cursor"`
	Items      []targetsInboxWireItem `json:"items"`
}

func inboxIDs(items []targetsInboxWireItem) []string {
	result := make([]string, len(items))
	for index := range items {
		result[index] = items[index].ID
	}
	return result
}

func decodeTargetsInboxItem(item targetsInboxWireItem) (TargetsInboxInboxItem, error) {
	expiresAt, err := time.Parse(time.RFC3339Nano, item.ExpiresAt)
	if err != nil || !inboxCapabilityPattern.MatchString(item.ID) || !historyCapabilityPattern.MatchString(item.HistoryItemID) ||
		!validTargetsHumanText(item.Media.Title) || !containsTargetsValue([]string{"available", "dismissed", "replayed", "unavailable", "expired"}, item.Availability) {
		return TargetsInboxInboxItem{}, phaseOneError(PhaseOneInvalidResponse)
	}
	actions, err := decodeTargetsActions(item.Presentation.Actions, item.Actions)
	if err != nil {
		return TargetsInboxInboxItem{}, err
	}
	sender, err := decodeTargetsLabel(item.Presentation.Sender)
	if err != nil {
		return TargetsInboxInboxItem{}, err
	}
	source, err := decodeTargetsLabel(item.Presentation.Source)
	if err != nil {
		return TargetsInboxInboxItem{}, err
	}
	requested, err := decodeTargetsLabel(item.Presentation.RequestedDelivery)
	if err != nil {
		return TargetsInboxInboxItem{}, err
	}
	effective, err := decodeTargetsLabel(item.Presentation.EffectiveDelivery)
	if err != nil {
		return TargetsInboxInboxItem{}, err
	}
	receipt, err := decodeTargetsLabel(item.Presentation.Receipt)
	if err != nil {
		return TargetsInboxInboxItem{}, err
	}
	return TargetsInboxInboxItem{ID: item.ID, HistoryItemID: item.HistoryItemID, Title: item.Media.Title,
		ExpiresAt: expiresAt, Availability: item.Availability, Sender: sender, Source: source,
		RequestedDelivery: requested, EffectiveDelivery: effective, Receipt: receipt, Actions: actions}, nil
}

type targetsHistoryWireItem struct {
	HistoryItemID string `json:"history_item_id"`
	Media         struct {
		Title string `json:"title"`
	} `json:"media"`
	TargetCounts *struct{ Played, Other int } `json:"target_counts"`
	Actions      []string                     `json:"actions"`
	Presentation struct {
		Status  targetsWireLabel    `json:"status"`
		Actions []targetsWireAction `json:"actions"`
	} `json:"presentation"`
}

type targetsHistoryResponse struct {
	Contract   string                   `json:"contract"`
	NextCursor string                   `json:"next_cursor"`
	Items      []targetsHistoryWireItem `json:"items"`
}

func historyIDs(items []targetsHistoryWireItem) []string {
	result := make([]string, len(items))
	for index := range items {
		result[index] = items[index].HistoryItemID
	}
	return result
}

func decodeTargetsHistoryItem(item targetsHistoryWireItem) (TargetsInboxHistoryItem, error) {
	if !historyCapabilityPattern.MatchString(item.HistoryItemID) || !validTargetsHumanText(item.Media.Title) {
		return TargetsInboxHistoryItem{}, phaseOneError(PhaseOneInvalidResponse)
	}
	status, err := decodeTargetsLabel(item.Presentation.Status)
	if err != nil {
		return TargetsInboxHistoryItem{}, err
	}
	actions, err := decodeTargetsActions(item.Presentation.Actions, item.Actions)
	if err != nil {
		return TargetsInboxHistoryItem{}, err
	}
	played, other := 0, 0
	if item.TargetCounts != nil {
		played, other = item.TargetCounts.Played, item.TargetCounts.Other
	}
	if played < 0 {
		played = 0
	}
	if other < 0 {
		other = 0
	}
	return TargetsInboxHistoryItem{ID: item.HistoryItemID, Title: item.Media.Title, Status: status, Actions: actions, Played: played, Other: other}, nil
}

type targetsReceiptResponse struct {
	Contract      string `json:"contract"`
	HistoryItemID string `json:"history_item_id"`
	NextCursor    string `json:"next_cursor"`
	Items         []struct {
		TargetLabel  string `json:"target_label"`
		Presentation struct {
			Status targetsWireLabel `json:"status"`
		} `json:"presentation"`
	} `json:"items"`
}

type targetsInboxMutationResponse struct {
	Contract string                            `json:"contract"`
	Item     struct{ ID, Availability string } `json:"item"`
}
type targetsPolicyResponse struct {
	Contract      string `json:"contract"`
	Current       bool   `json:"current"`
	TermsAccepted bool   `json:"terms_accepted"`
}
type targetsReplayResponse struct {
	Contract          string `json:"contract"`
	HistoryItemID     string `json:"history_item_id"`
	RequestedDelivery string `json:"requested_delivery"`
	EffectiveDelivery string `json:"effective_delivery"`
	Reused            bool   `json:"reused"`
}
type targetsDeleteResponse struct {
	HistoryItemID string `json:"history_item_id"`
	Deleted       bool   `json:"deleted"`
}
type targetsReportResponse struct {
	HistoryItemID string `json:"history_item_id"`
	Reused        bool   `json:"reused"`
}
type targetsBlockResponse struct {
	BlockID string `json:"block_id"`
	Reused  bool   `json:"reused"`
}

func containsTargetsValue(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}
