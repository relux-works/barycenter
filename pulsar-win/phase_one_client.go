package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type PhaseOneRoute string

const (
	PhaseOneThisPulsar    PhaseOneRoute = "this_pulsar"
	PhaseOneOwnBarycenter PhaseOneRoute = "own_barycenter"
	PhaseOneCurrentAir    PhaseOneRoute = "current_air"
)

type PhaseOneDelivery string

const (
	PhaseOneOverlay      PhaseOneDelivery = "overlay"
	PhaseOneInterrupt    PhaseOneDelivery = "interrupt"
	PhaseOneAfterCurrent PhaseOneDelivery = "after_current"
)

type PhaseOneOriginKind string

const (
	PhaseOneMicrophone PhaseOneOriginKind = "microphone"
	PhaseOneFile       PhaseOneOriginKind = "file"
)

type ContentPolicyLocale string

const (
	ContentPolicyEN ContentPolicyLocale = "en"
	ContentPolicyRU ContentPolicyLocale = "ru"
)

type ContentPolicyManifest struct {
	Version              string
	PolicyHash           string
	Locale               ContentPolicyLocale
	LocaleHash           string
	EffectiveAt          time.Time
	TermsURL             string
	ContentGuidelinesURL string
	Title                string
	RightsText           string
	ConsentText          string
	ControllingLanguage  string
}

type ContentPolicyGrant struct {
	Version       string
	PolicyHash    string
	Locale        ContentPolicyLocale
	AcceptedAt    time.Time
	RevokedAt     *time.Time
	Revision      int64
	Current       bool
	TermsAccepted bool
}

type PhaseOneUploadConfirmation struct {
	MediaID string
	Reused  bool
}

type PhaseOneTransmissionReceipt struct {
	TransmissionID    string
	RequestedDelivery PhaseOneDelivery
	EffectiveDelivery PhaseOneDelivery
	DowngradeReason   string
	Status            string
	Reused            bool
}

type PhaseOneFallbackConfirmation struct {
	Token    string
	Delivery PhaseOneDelivery
}

type PhaseOneFallbackAlternative struct {
	Delivery  PhaseOneDelivery
	Available bool
	Reason    string
}

type PhaseOnePresenceNode struct {
	Slot          string
	Online        bool
	OutputState   string
	PlaybackState string
	EffectiveDND  string
}

type PhaseOneHistoryItem struct {
	ID                string
	Direction         string
	OccurredAt        time.Time
	Title             string
	SenderName        string
	RequestedDelivery string
	EffectiveDelivery string
	DowngradeReason   string
	Status            string
	ReasonCode        string
	PlayedCount       int
	OtherCount        int
	Actions           []string
}

type PhaseOneHistoryPage struct {
	Items      []PhaseOneHistoryItem
	NextCursor string
}

type PhaseOneModerationReason string

const (
	PhaseOneReportSpam          PhaseOneModerationReason = "spam"
	PhaseOneReportHarassment    PhaseOneModerationReason = "harassment"
	PhaseOneReportIllegal       PhaseOneModerationReason = "illegal"
	PhaseOneReportSexualContent PhaseOneModerationReason = "sexual_content"
	PhaseOneReportViolence      PhaseOneModerationReason = "violence"
	PhaseOneReportOther         PhaseOneModerationReason = "other"
)

var phaseOneModerationReasons = []PhaseOneModerationReason{
	PhaseOneReportSpam, PhaseOneReportHarassment, PhaseOneReportIllegal,
	PhaseOneReportSexualContent, PhaseOneReportViolence, PhaseOneReportOther,
}

type PhaseOneHistoryActionReceipt struct {
	Outcome string
	Reused  bool
}

type PhaseOneClientErrorKind string

const (
	PhaseOneInvalidConfiguration PhaseOneClientErrorKind = "invalid_configuration"
	PhaseOneInvalidRequest       PhaseOneClientErrorKind = "invalid_request"
	PhaseOneTransport            PhaseOneClientErrorKind = "transport"
	PhaseOneRedirectRejected     PhaseOneClientErrorKind = "redirect_rejected"
	PhaseOneResponseTooLarge     PhaseOneClientErrorKind = "response_too_large"
	PhaseOneInvalidResponse      PhaseOneClientErrorKind = "invalid_response"
	PhaseOneRejected             PhaseOneClientErrorKind = "rejected"
)

type PhaseOneClientError struct {
	Kind              PhaseOneClientErrorKind
	Status            int
	Code              string
	RetryAfterSeconds int
	ConfirmationToken string
	Alternatives      []PhaseOneFallbackAlternative
}

func (e *PhaseOneClientError) Error() string {
	if e == nil {
		return "phase_one_error"
	}
	if e.Kind == PhaseOneRejected && e.Code != "" {
		return "phase_one_rejected:" + e.Code
	}
	return "phase_one_" + string(e.Kind)
}

func (e *PhaseOneClientError) String() string { return e.Error() }
func (e *PhaseOneClientError) GoString() string {
	if e == nil {
		return "PhaseOneClientError{}"
	}
	return fmt.Sprintf("PhaseOneClientError{kind:%s status:%d code:<redacted> retry:%d}", e.Kind, e.Status, e.RetryAfterSeconds)
}

type PhaseOneAppService interface {
	Upload(context.Context, string, string, string, bool) (PhaseOneUploadConfirmation, error)
	ContentPolicy(context.Context, ContentPolicyLocale) (ContentPolicyManifest, error)
	CurrentContentPolicyGrant(context.Context) (ContentPolicyGrant, error)
	AcceptContentPolicy(context.Context, ContentPolicyManifest) (ContentPolicyGrant, error)
	RevokeContentPolicy(context.Context, ContentPolicyLocale) (ContentPolicyGrant, error)
	Transmit(context.Context, string, PhaseOneRoute, PhaseOneDelivery, PhaseOneOriginKind, string, *PhaseOneFallbackConfirmation) (PhaseOneTransmissionReceipt, error)
	DeleteMedia(context.Context, string) error
	Presence(context.Context) ([]PhaseOnePresenceNode, error)
	History(context.Context, int, string) (PhaseOneHistoryPage, error)
	DeleteHistoryItem(context.Context, string) (PhaseOneHistoryActionReceipt, error)
	ReportHistoryItem(context.Context, string, PhaseOneModerationReason, string) (PhaseOneHistoryActionReceipt, error)
	BlockHistoryActor(context.Context, string, string) (PhaseOneHistoryActionReceipt, error)
	ReplayHistoryItem(context.Context, string, PhaseOneRoute, PhaseOneDelivery, string, *PhaseOneFallbackConfirmation) (PhaseOneTransmissionReceipt, error)
}

type PhaseOneAppClient struct {
	origin CoordinatorOrigin
	token  string
	doer   HTTPDoer
}

func NewPhaseOneAppClient(bundle CredentialBundle, doer HTTPDoer) (*PhaseOneAppClient, error) {
	if bundle.validate() != nil || bundle.Control == nil || bundle.Control.Context != ControlContextActive ||
		!lowerHexTokenPattern.MatchString(bundle.Control.ControlToken) {
		return nil, &PhaseOneClientError{Kind: PhaseOneInvalidConfiguration}
	}
	origin, err := CanonicalCoordinatorOrigin(bundle.CoordinatorOrigin)
	if err != nil || !origin.permitsSecrets() {
		return nil, &PhaseOneClientError{Kind: PhaseOneInvalidConfiguration}
	}
	if doer == nil {
		doer = &http.Client{
			Timeout:       30 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
	} else if client, ok := doer.(*http.Client); ok {
		clone := *client
		clone.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		doer = &clone
	}
	return &PhaseOneAppClient{origin: origin, token: bundle.Control.ControlToken, doer: doer}, nil
}

func (c *PhaseOneAppClient) String() string   { return "PhaseOneAppClient{<redacted>}" }
func (c *PhaseOneAppClient) GoString() string { return c.String() }

func (c *PhaseOneAppClient) Upload(ctx context.Context, filePath, title, idempotencyKey string, rightsAcknowledged bool) (PhaseOneUploadConfirmation, error) {
	title = strings.TrimSpace(title)
	info, err := os.Stat(filePath)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || !validPhaseOneDisplayText(title, 512, false) || !validPhaseOneIdempotencyKey(idempotencyKey) || !rightsAcknowledged {
		return PhaseOneUploadConfirmation{}, phaseOneError(PhaseOneInvalidRequest)
	}
	body := struct {
		Kind               string `json:"kind"`
		Title              string `json:"title"`
		SizeBytes          int64  `json:"size_bytes"`
		RightsAcknowledged bool   `json:"rights_acknowledged"`
	}{"voice_clip", title, info.Size(), rightsAcknowledged}
	raw, _, err := c.requestJSON(ctx, http.MethodPost, "/v1/media/uploads", c.token,
		map[string]string{"Idempotency-Key": idempotencyKey}, body, http.StatusOK, http.StatusCreated)
	if err != nil {
		return PhaseOneUploadConfirmation{}, err
	}
	var session phaseOneUploadSession
	if decodePhaseOneJSON(raw, &session) != nil || !validPhaseOnePublicID(session.UploadID, "up_") ||
		!validPhaseOnePublicID(session.MediaID, "m_") || session.UploadLength != info.Size() ||
		session.UploadOffset < 0 || session.UploadOffset > session.UploadLength {
		return PhaseOneUploadConfirmation{}, phaseOneError(PhaseOneInvalidResponse)
	}
	if session.Status == "completed" && session.UploadOffset == session.UploadLength {
		return PhaseOneUploadConfirmation{MediaID: session.MediaID, Reused: true}, nil
	}
	if !lowerHexTokenPattern.MatchString(session.UploadToken) {
		return PhaseOneUploadConfirmation{}, phaseOneError(PhaseOneInvalidResponse)
	}
	file, err := os.Open(filePath)
	if err != nil {
		return PhaseOneUploadConfirmation{}, phaseOneError(PhaseOneInvalidRequest)
	}
	defer file.Close()
	if _, err := file.Seek(session.UploadOffset, io.SeekStart); err != nil {
		return PhaseOneUploadConfirmation{}, phaseOneError(PhaseOneInvalidRequest)
	}
	remainingLength := session.UploadLength - session.UploadOffset
	remaining, err := io.ReadAll(io.LimitReader(file, remainingLength+1))
	if err != nil || int64(len(remaining)) != remainingLength {
		return PhaseOneUploadConfirmation{}, phaseOneError(PhaseOneInvalidRequest)
	}
	uploadedRaw, _, err := c.request(ctx, http.MethodPut, "/v1/media/uploads/"+session.UploadID,
		session.UploadToken, map[string]string{
			"Upload-Offset": strconv.FormatInt(session.UploadOffset, 10),
			"Content-Type":  "application/octet-stream",
		}, bytes.NewReader(remaining), true, http.StatusOK)
	if err != nil {
		return PhaseOneUploadConfirmation{}, err
	}
	var completed phaseOneUploadSession
	if decodePhaseOneJSON(uploadedRaw, &completed) != nil || completed.UploadID != session.UploadID ||
		completed.MediaID != session.MediaID || completed.UploadLength != info.Size() ||
		completed.UploadOffset != completed.UploadLength || completed.Status != "completed" {
		return PhaseOneUploadConfirmation{}, phaseOneError(PhaseOneInvalidResponse)
	}
	return PhaseOneUploadConfirmation{MediaID: completed.MediaID, Reused: session.Reused}, nil
}

func (c *PhaseOneAppClient) ContentPolicy(ctx context.Context, locale ContentPolicyLocale) (ContentPolicyManifest, error) {
	if locale != ContentPolicyEN && locale != ContentPolicyRU {
		return ContentPolicyManifest{}, phaseOneError(PhaseOneInvalidRequest)
	}
	raw, _, err := c.request(ctx, http.MethodGet, "/v1/content-policy?locale="+string(locale), c.token, nil, nil, true, http.StatusOK)
	if err != nil {
		return ContentPolicyManifest{}, err
	}
	return decodeContentPolicy(raw, locale)
}

func (c *PhaseOneAppClient) AcceptContentPolicy(ctx context.Context, manifest ContentPolicyManifest) (ContentPolicyGrant, error) {
	if !validContentPolicyVersion(manifest.Version) || !validContentPolicyHash(manifest.PolicyHash) ||
		(manifest.Locale != ContentPolicyEN && manifest.Locale != ContentPolicyRU) {
		return ContentPolicyGrant{}, phaseOneError(PhaseOneInvalidRequest)
	}
	body := struct {
		Version       string `json:"version"`
		PolicyHash    string `json:"policy_hash"`
		Locale        string `json:"locale"`
		TermsAccepted bool   `json:"terms_accepted"`
	}{manifest.Version, manifest.PolicyHash, string(manifest.Locale), true}
	raw, _, err := c.requestJSON(ctx, http.MethodPut, "/v1/content-policy/acceptance", c.token, nil, body, http.StatusOK)
	if err != nil {
		return ContentPolicyGrant{}, err
	}
	return decodeContentPolicyGrant(raw, manifest.Locale, manifest.Version, manifest.PolicyHash)
}

func (c *PhaseOneAppClient) CurrentContentPolicyGrant(ctx context.Context) (ContentPolicyGrant, error) {
	raw, _, err := c.request(ctx, http.MethodGet, "/v1/content-policy/acceptance", c.token, nil, nil, true, http.StatusOK)
	if err != nil {
		return ContentPolicyGrant{}, err
	}
	var response contentPolicyGrantResponse
	if decodePhaseOneJSON(raw, &response) != nil {
		return ContentPolicyGrant{}, phaseOneError(PhaseOneInvalidResponse)
	}
	locale := ContentPolicyLocale(response.Locale)
	if locale != ContentPolicyEN && locale != ContentPolicyRU {
		return ContentPolicyGrant{}, phaseOneError(PhaseOneInvalidResponse)
	}
	return decodeContentPolicyGrant(raw, locale, "", "")
}

func (c *PhaseOneAppClient) RevokeContentPolicy(ctx context.Context, locale ContentPolicyLocale) (ContentPolicyGrant, error) {
	if locale != ContentPolicyEN && locale != ContentPolicyRU {
		return ContentPolicyGrant{}, phaseOneError(PhaseOneInvalidRequest)
	}
	raw, _, err := c.request(ctx, http.MethodDelete, "/v1/content-policy/acceptance?locale="+string(locale), c.token, nil, nil, true, http.StatusOK)
	if err != nil {
		return ContentPolicyGrant{}, err
	}
	return decodeContentPolicyGrant(raw, locale, "", "")
}

func (c *PhaseOneAppClient) Transmit(ctx context.Context, mediaID string, route PhaseOneRoute, delivery PhaseOneDelivery, originKind PhaseOneOriginKind, idempotencyKey string, fallback *PhaseOneFallbackConfirmation) (PhaseOneTransmissionReceipt, error) {
	if !validPhaseOnePublicID(mediaID, "m_") || !validPhaseOneRoute(route) || !validPhaseOneDelivery(delivery) ||
		(originKind != PhaseOneMicrophone && originKind != PhaseOneFile) || !validPhaseOneIdempotencyKey(idempotencyKey) {
		return PhaseOneTransmissionReceipt{}, phaseOneError(PhaseOneInvalidRequest)
	}
	if fallback != nil && (!validPhaseOneConfirmationToken(fallback.Token) || fallback.Delivery != PhaseOneAfterCurrent || delivery != PhaseOneInterrupt) {
		return PhaseOneTransmissionReceipt{}, phaseOneError(PhaseOneInvalidRequest)
	}
	body := struct {
		MediaID              string                        `json:"media_id"`
		Audience             any                           `json:"audience"`
		Delivery             string                        `json:"delivery"`
		OriginKind           string                        `json:"origin_kind"`
		IncludeOrigin        bool                          `json:"include_origin"`
		FallbackConfirmation *phaseOneFallbackConfirmation `json:"fallback_confirmation,omitempty"`
	}{mediaID, struct {
		Kind string `json:"kind"`
	}{string(route)}, string(delivery), string(originKind), route == PhaseOneThisPulsar, phaseOneFallbackBody(fallback)}
	raw, _, err := c.requestJSON(ctx, http.MethodPost, "/v1/transmissions", c.token,
		map[string]string{"Idempotency-Key": idempotencyKey}, body, http.StatusOK, http.StatusCreated)
	if err != nil {
		return PhaseOneTransmissionReceipt{}, err
	}
	return decodePhaseOneTransmission(raw, mediaID)
}

func (c *PhaseOneAppClient) DeleteMedia(ctx context.Context, mediaID string) error {
	if !validPhaseOnePublicID(mediaID, "m_") {
		return phaseOneError(PhaseOneInvalidRequest)
	}
	_, _, err := c.request(ctx, http.MethodDelete, "/v1/media/"+mediaID, c.token, nil, nil, false, http.StatusNoContent)
	var rejected *PhaseOneClientError
	if errors.As(err, &rejected) && rejected.Kind == PhaseOneRejected && rejected.Status == http.StatusNotFound && rejected.Code == "media_not_found" {
		return nil
	}
	return err
}

func (c *PhaseOneAppClient) Presence(ctx context.Context) ([]PhaseOnePresenceNode, error) {
	raw, _, err := c.request(ctx, http.MethodGet, "/v1/presence", c.token, nil, nil, true, http.StatusOK)
	if err != nil {
		return nil, err
	}
	var response phaseOnePresenceResponse
	if decodePhaseOneJSON(raw, &response) != nil || response.Contract != "p1-history-presence-telegram-v1" {
		return nil, phaseOneError(PhaseOneInvalidResponse)
	}
	result := make([]PhaseOnePresenceNode, 0, len(response.Nodes))
	seen := map[string]bool{}
	for _, node := range response.Nodes {
		if len(node.Slot) != 1 || node.Slot[0] < 'a' || node.Slot[0] > 'z' || seen[node.Slot] ||
			!validPhaseOneBoundedLabel(node.OutputState) || !validPhaseOneBoundedLabel(node.PlaybackState) ||
			(node.EffectiveDND.Mode != "allow_all" && node.EffectiveDND.Mode != "messages_only" && node.EffectiveDND.Mode != "muted_until") {
			return nil, phaseOneError(PhaseOneInvalidResponse)
		}
		seen[node.Slot] = true
		result = append(result, PhaseOnePresenceNode{node.Slot, node.Online, node.OutputState, node.PlaybackState, node.EffectiveDND.Mode})
	}
	return result, nil
}

func (c *PhaseOneAppClient) History(ctx context.Context, limit int, cursor string) (PhaseOneHistoryPage, error) {
	if limit < 1 || limit > 100 || strings.TrimSpace(cursor) != cursor {
		return PhaseOneHistoryPage{}, phaseOneError(PhaseOneInvalidRequest)
	}
	query := url.Values{"limit": {strconv.Itoa(limit)}}
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	raw, _, err := c.request(ctx, http.MethodGet, "/v1/history?"+query.Encode(), c.token, nil, nil, true, http.StatusOK)
	if err != nil {
		return PhaseOneHistoryPage{}, err
	}
	var response phaseOneHistoryResponse
	if decodePhaseOneJSON(raw, &response) != nil || response.Contract != "p1-history-presence-telegram-v1" {
		return PhaseOneHistoryPage{}, phaseOneError(PhaseOneInvalidResponse)
	}
	page := PhaseOneHistoryPage{NextCursor: response.NextCursor}
	for _, item := range response.Items {
		occurred, parseErr := time.Parse(time.RFC3339Nano, item.OccurredAt)
		if parseErr != nil || !validPhaseOnePublicID(item.HistoryItemID, "hi_") || item.Direction != "sent" && item.Direction != "received" ||
			!validPhaseOneDisplayText(item.Media.Title, 512, false) || !validPhaseOneDisplayText(item.Sender.DisplayName, 256, true) ||
			!validOptionalPhaseOneDelivery(item.RequestedDelivery) || !validOptionalPhaseOneDelivery(item.EffectiveDelivery) ||
			!validPhaseOneBoundedLabel(item.Status) || item.TargetCounts.Played < 0 || item.TargetCounts.Other < 0 ||
			!validPhaseOneHistoryActions(item.Actions) {
			return PhaseOneHistoryPage{}, phaseOneError(PhaseOneInvalidResponse)
		}
		page.Items = append(page.Items, PhaseOneHistoryItem{
			ID: item.HistoryItemID, Direction: item.Direction, OccurredAt: occurred, Title: item.Media.Title,
			SenderName: item.Sender.DisplayName, RequestedDelivery: item.RequestedDelivery,
			EffectiveDelivery: item.EffectiveDelivery, DowngradeReason: item.DowngradeReason,
			Status: item.Status, ReasonCode: item.ReasonCode, PlayedCount: item.TargetCounts.Played,
			OtherCount: item.TargetCounts.Other, Actions: append([]string(nil), item.Actions...),
		})
	}
	return page, nil
}

func (c *PhaseOneAppClient) DeleteHistoryItem(ctx context.Context, historyID string) (PhaseOneHistoryActionReceipt, error) {
	if !validPhaseOnePublicID(historyID, "hi_") {
		return PhaseOneHistoryActionReceipt{}, phaseOneError(PhaseOneInvalidRequest)
	}
	raw, _, err := c.requestJSON(ctx, http.MethodPost, "/v1/history/"+historyID+"/actions/delete", c.token, nil, struct{}{}, http.StatusOK)
	if err != nil {
		return PhaseOneHistoryActionReceipt{}, err
	}
	var response phaseOneHistoryDeleteResponse
	if decodePhaseOneJSON(raw, &response) != nil || response.HistoryItemID != historyID || !response.Deleted {
		return PhaseOneHistoryActionReceipt{}, phaseOneError(PhaseOneInvalidResponse)
	}
	return PhaseOneHistoryActionReceipt{Outcome: "media_deleted"}, nil
}

func (c *PhaseOneAppClient) ReportHistoryItem(ctx context.Context, historyID string, reason PhaseOneModerationReason, details string) (PhaseOneHistoryActionReceipt, error) {
	if !validPhaseOnePublicID(historyID, "hi_") || !validPhaseOneModerationReason(reason) ||
		!validPhaseOneDisplayText(details, 2000, true) {
		return PhaseOneHistoryActionReceipt{}, phaseOneError(PhaseOneInvalidRequest)
	}
	body := struct {
		Reason  PhaseOneModerationReason `json:"reason"`
		Details string                   `json:"details"`
	}{reason, details}
	raw, _, err := c.requestJSON(ctx, http.MethodPost, "/v1/history/"+historyID+"/actions/report", c.token, nil,
		body, http.StatusOK, http.StatusCreated)
	if err != nil {
		return PhaseOneHistoryActionReceipt{}, err
	}
	var response phaseOneHistoryReportResponse
	if decodePhaseOneJSON(raw, &response) != nil || response.HistoryItemID != historyID ||
		!validPhaseOnePublicID(response.ID, "rp_") || response.Reason != reason ||
		response.Status != "received" && response.Status != "reviewed" {
		return PhaseOneHistoryActionReceipt{}, phaseOneError(PhaseOneInvalidResponse)
	}
	outcome := "report_received"
	if response.Reused {
		outcome = "report_already_received"
	}
	return PhaseOneHistoryActionReceipt{Outcome: outcome, Reused: response.Reused}, nil
}

func (c *PhaseOneAppClient) BlockHistoryActor(ctx context.Context, historyID, idempotencyKey string) (PhaseOneHistoryActionReceipt, error) {
	if !validPhaseOnePublicID(historyID, "hi_") || !validPhaseOneIdempotencyKey(idempotencyKey) {
		return PhaseOneHistoryActionReceipt{}, phaseOneError(PhaseOneInvalidRequest)
	}
	raw, _, err := c.requestJSON(ctx, http.MethodPost, "/v1/history/"+historyID+"/actions/block_actor", c.token,
		map[string]string{"Idempotency-Key": idempotencyKey}, struct{}{}, http.StatusOK, http.StatusCreated)
	if err != nil {
		return PhaseOneHistoryActionReceipt{}, err
	}
	var response phaseOneHistoryBlockResponse
	if decodePhaseOneJSON(raw, &response) != nil || !validPhaseOnePublicID(response.BlockID, "bl_") || response.Reused == nil {
		return PhaseOneHistoryActionReceipt{}, phaseOneError(PhaseOneInvalidResponse)
	}
	outcome := "sender_blocked"
	if *response.Reused {
		outcome = "sender_already_blocked"
	}
	return PhaseOneHistoryActionReceipt{Outcome: outcome, Reused: *response.Reused}, nil
}

func (c *PhaseOneAppClient) ReplayHistoryItem(ctx context.Context, historyID string, route PhaseOneRoute, delivery PhaseOneDelivery, idempotencyKey string, fallback *PhaseOneFallbackConfirmation) (PhaseOneTransmissionReceipt, error) {
	if !validPhaseOnePublicID(historyID, "hi_") || !validPhaseOneRoute(route) || !validPhaseOneDelivery(delivery) || !validPhaseOneIdempotencyKey(idempotencyKey) {
		return PhaseOneTransmissionReceipt{}, phaseOneError(PhaseOneInvalidRequest)
	}
	if fallback != nil && (!validPhaseOneConfirmationToken(fallback.Token) || fallback.Delivery != PhaseOneAfterCurrent || delivery != PhaseOneInterrupt) {
		return PhaseOneTransmissionReceipt{}, phaseOneError(PhaseOneInvalidRequest)
	}
	body := struct {
		Audience             any                           `json:"audience"`
		Delivery             string                        `json:"delivery"`
		IncludeOrigin        bool                          `json:"include_origin"`
		FallbackConfirmation *phaseOneFallbackConfirmation `json:"fallback_confirmation,omitempty"`
	}{struct {
		Kind string `json:"kind"`
	}{string(route)}, string(delivery), route == PhaseOneThisPulsar, phaseOneFallbackBody(fallback)}
	raw, _, err := c.requestJSON(ctx, http.MethodPost, "/v1/history/"+historyID+"/actions/replay", c.token,
		map[string]string{"Idempotency-Key": idempotencyKey}, body, http.StatusOK, http.StatusCreated)
	if err != nil {
		return PhaseOneTransmissionReceipt{}, err
	}
	return decodePhaseOneTransmission(raw, "")
}

func (c *PhaseOneAppClient) requestJSON(ctx context.Context, method, path, bearer string, headers map[string]string, body any, success ...int) ([]byte, *http.Response, error) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, nil, phaseOneError(PhaseOneInvalidRequest)
	}
	if headers == nil {
		headers = map[string]string{}
	}
	headers["Content-Type"] = "application/json"
	return c.request(ctx, method, path, bearer, headers, bytes.NewReader(encoded), true, success...)
}

func (c *PhaseOneAppClient) request(ctx context.Context, method, path, bearer string, headers map[string]string, body io.Reader, requireJSON bool, success ...int) ([]byte, *http.Response, error) {
	if c == nil || !lowerHexTokenPattern.MatchString(bearer) || !strings.HasPrefix(path, "/") || strings.Contains(path, "\\") {
		return nil, nil, phaseOneError(PhaseOneInvalidRequest)
	}
	endpoint, err := url.Parse(c.origin.String() + path)
	if err != nil || endpoint.User != nil || endpoint.Fragment != "" {
		return nil, nil, phaseOneError(PhaseOneInvalidRequest)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return nil, nil, phaseOneError(PhaseOneInvalidRequest)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+bearer)
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := c.doer.Do(request)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, nil, err
		}
		return nil, nil, phaseOneError(PhaseOneTransport)
	}
	if response == nil || response.Body == nil {
		return nil, nil, phaseOneError(PhaseOneInvalidResponse)
	}
	defer response.Body.Close()
	finalURL := endpoint
	if response.Request != nil && response.Request.URL != nil {
		finalURL = response.Request.URL
	}
	finalOrigin, originErr := CanonicalCoordinatorOrigin(finalURL.Scheme + "://" + finalURL.Host)
	if originErr != nil || finalOrigin != c.origin || response.StatusCode >= 300 && response.StatusCode < 400 {
		return nil, response, phaseOneError(PhaseOneRedirectRejected)
	}
	raw, readErr := readBoundedResponse(response.Body)
	if readErr != nil {
		if errors.Is(readErr, context.Canceled) || errors.Is(readErr, context.DeadlineExceeded) {
			return nil, response, readErr
		}
		return nil, response, phaseOneError(PhaseOneResponseTooLarge)
	}
	ok := false
	for _, status := range success {
		ok = ok || response.StatusCode == status
	}
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(response.Header.Get("Content-Type"), ";")[0]))
	if requireJSON || !ok {
		if contentType != "application/json" {
			return nil, response, phaseOneError(PhaseOneInvalidResponse)
		}
	}
	if !ok {
		return nil, response, decodePhaseOneAPIError(raw, response.StatusCode)
	}
	return raw, response, nil
}

func decodePhaseOneJSON(raw []byte, target any) error {
	if _, err := parseStrictJSONObject(raw); err != nil {
		return err
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return errInvalidResponse
	}
	return nil
}

func decodePhaseOneAPIError(raw []byte, status int) error {
	var envelope struct {
		Error struct {
			Code              string `json:"code"`
			RetryAfterSeconds int    `json:"retry_after_seconds"`
			Details           struct {
				ConfirmationToken string `json:"confirmation_token"`
				Alternatives      []struct {
					Delivery  string `json:"delivery"`
					Available bool   `json:"available"`
					Reason    string `json:"reason"`
				} `json:"alternatives"`
			} `json:"details"`
		} `json:"error"`
	}
	if decodePhaseOneJSON(raw, &envelope) != nil || !validPhaseOneErrorCode(envelope.Error.Code) ||
		envelope.Error.RetryAfterSeconds < 0 || envelope.Error.RetryAfterSeconds > 86_400 {
		return phaseOneError(PhaseOneInvalidResponse)
	}
	result := &PhaseOneClientError{Kind: PhaseOneRejected, Status: status, Code: envelope.Error.Code, RetryAfterSeconds: envelope.Error.RetryAfterSeconds}
	if envelope.Error.Code == "requires_confirmation" {
		if !validPhaseOneConfirmationToken(envelope.Error.Details.ConfirmationToken) || len(envelope.Error.Details.Alternatives) == 0 {
			return phaseOneError(PhaseOneInvalidResponse)
		}
		result.ConfirmationToken = envelope.Error.Details.ConfirmationToken
		for _, alternative := range envelope.Error.Details.Alternatives {
			delivery := PhaseOneDelivery(alternative.Delivery)
			if !validPhaseOneDelivery(delivery) || !validPhaseOneDisplayText(alternative.Reason, 256, true) {
				return phaseOneError(PhaseOneInvalidResponse)
			}
			result.Alternatives = append(result.Alternatives, PhaseOneFallbackAlternative{Delivery: delivery, Available: alternative.Available, Reason: alternative.Reason})
		}
	}
	return result
}

func decodePhaseOneTransmission(raw []byte, expectedMediaID string) (PhaseOneTransmissionReceipt, error) {
	var response phaseOneTransmissionResponse
	if decodePhaseOneJSON(raw, &response) != nil || !validPhaseOnePublicID(response.TransmissionID, "tr_") ||
		(expectedMediaID != "" && response.MediaID != expectedMediaID) || !validPhaseOneDelivery(PhaseOneDelivery(response.RequestedDelivery)) ||
		!validPhaseOneDelivery(PhaseOneDelivery(response.EffectiveDelivery)) || response.Status == "" {
		return PhaseOneTransmissionReceipt{}, phaseOneError(PhaseOneInvalidResponse)
	}
	return PhaseOneTransmissionReceipt{
		TransmissionID: response.TransmissionID, RequestedDelivery: PhaseOneDelivery(response.RequestedDelivery),
		EffectiveDelivery: PhaseOneDelivery(response.EffectiveDelivery), DowngradeReason: response.DowngradeReason,
		Status: response.Status, Reused: response.Reused,
	}, nil
}

func validPhaseOneRoute(value PhaseOneRoute) bool {
	return value == PhaseOneThisPulsar || value == PhaseOneOwnBarycenter || value == PhaseOneCurrentAir
}

func validPhaseOneDelivery(value PhaseOneDelivery) bool {
	return value == PhaseOneOverlay || value == PhaseOneInterrupt || value == PhaseOneAfterCurrent
}

func validPhaseOneIdempotencyKey(value string) bool {
	if len(value) < 1 || len(value) > 128 {
		return false
	}
	for index, character := range value {
		allowed := character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' || strings.ContainsRune("._:-", character)
		if !allowed || index == 0 && !((character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9')) {
			return false
		}
	}
	return true
}

func validContentPolicyVersion(value string) bool {
	return len(value) >= 1 && len(value) <= 32 && strings.TrimSpace(value) == value
}

func validContentPolicyHash(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if !(character >= '0' && character <= '9' || character >= 'a' && character <= 'f') {
			return false
		}
	}
	return true
}

func decodeContentPolicy(raw []byte, expected ContentPolicyLocale) (ContentPolicyManifest, error) {
	var response contentPolicyResponse
	if decodePhaseOneJSON(raw, &response) != nil || response.Contract != "p2-content-policy-consent.v1" ||
		!validContentPolicyVersion(response.Version) || !validContentPolicyHash(response.PolicyHash) ||
		!validContentPolicyHash(response.LocaleHash) || ContentPolicyLocale(response.Locale) != expected ||
		response.ControllingLanguage != "en" ||
		!validPhaseOneDisplayText(response.Title, 512, false) ||
		!validPhaseOneDisplayText(response.RightsText, 4_096, false) ||
		!validPhaseOneDisplayText(response.ConsentText, 4_096, false) ||
		response.TermsURL != "https://barycenter.live/legal/terms" ||
		response.ContentGuidelinesURL != "https://barycenter.live/legal/content-guidelines" {
		return ContentPolicyManifest{}, phaseOneError(PhaseOneInvalidResponse)
	}
	effectiveAt, err := time.Parse(time.RFC3339, response.EffectiveAt)
	if err != nil {
		return ContentPolicyManifest{}, phaseOneError(PhaseOneInvalidResponse)
	}
	return ContentPolicyManifest{
		Version: response.Version, PolicyHash: response.PolicyHash,
		Locale: expected, LocaleHash: response.LocaleHash, EffectiveAt: effectiveAt,
		TermsURL: response.TermsURL, ContentGuidelinesURL: response.ContentGuidelinesURL,
		Title: response.Title, RightsText: response.RightsText, ConsentText: response.ConsentText,
		ControllingLanguage: response.ControllingLanguage,
	}, nil
}

func decodeContentPolicyGrant(raw []byte, locale ContentPolicyLocale, version, hash string) (ContentPolicyGrant, error) {
	var response contentPolicyGrantResponse
	if decodePhaseOneJSON(raw, &response) != nil || response.Contract != "p2-content-policy-consent.v1" ||
		!validContentPolicyVersion(response.Version) || !validContentPolicyHash(response.PolicyHash) ||
		(version != "" && response.Version != version) || (hash != "" && response.PolicyHash != hash) ||
		ContentPolicyLocale(response.Locale) != locale || response.Revision <= 0 {
		return ContentPolicyGrant{}, phaseOneError(PhaseOneInvalidResponse)
	}
	acceptedAt, err := time.Parse(time.RFC3339, response.AcceptedAt)
	if err != nil {
		return ContentPolicyGrant{}, phaseOneError(PhaseOneInvalidResponse)
	}
	var revokedAt *time.Time
	if response.RevokedAt != "" {
		parsed, parseErr := time.Parse(time.RFC3339, response.RevokedAt)
		if parseErr != nil {
			return ContentPolicyGrant{}, phaseOneError(PhaseOneInvalidResponse)
		}
		revokedAt = &parsed
	}
	if response.Current != (response.TermsAccepted && revokedAt == nil) {
		return ContentPolicyGrant{}, phaseOneError(PhaseOneInvalidResponse)
	}
	return ContentPolicyGrant{
		Version: response.Version, PolicyHash: response.PolicyHash,
		Locale: locale, AcceptedAt: acceptedAt, RevokedAt: revokedAt,
		Revision: response.Revision, Current: response.Current,
		TermsAccepted: response.TermsAccepted,
	}, nil
}

func validPhaseOnePublicID(value, prefix string) bool {
	if !strings.HasPrefix(value, prefix) || len(value) != len(prefix)+26 {
		return false
	}
	for _, character := range value[len(prefix):] {
		if !strings.ContainsRune("0123456789ABCDEFGHJKMNPQRSTVWXYZ", character) {
			return false
		}
	}
	return true
}

func validPhaseOneConfirmationToken(value string) bool {
	return strings.HasPrefix(value, "fc_") && lowerHexTokenPattern.MatchString(strings.TrimPrefix(value, "fc_"))
}

func validOptionalPhaseOneDelivery(value string) bool {
	return value == "" || validPhaseOneDelivery(PhaseOneDelivery(value))
}

func validPhaseOneBoundedLabel(value string) bool {
	if len(value) < 1 || len(value) > 64 {
		return false
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '_') {
			return false
		}
	}
	return true
}

func validPhaseOneErrorCode(value string) bool { return validPhaseOneBoundedLabel(value) }

func validPhaseOneDisplayText(value string, maximum int, allowEmpty bool) bool {
	if strings.TrimSpace(value) != value || len([]byte(value)) > maximum || !allowEmpty && value == "" {
		return false
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}

func validPhaseOneHistoryActions(actions []string) bool {
	allowed := map[string]bool{"cancel": true, "delete": true, "replay": true, "report": true, "block_actor": true, "block_orbit": true, "unblock": true}
	seen := map[string]bool{}
	for _, action := range actions {
		if !allowed[action] || seen[action] {
			return false
		}
		seen[action] = true
	}
	return true
}

func validPhaseOneModerationReason(reason PhaseOneModerationReason) bool {
	for _, candidate := range phaseOneModerationReasons {
		if reason == candidate {
			return true
		}
	}
	return false
}

type phaseOneFallbackConfirmation struct {
	Token    string `json:"token"`
	Delivery string `json:"delivery"`
}

func phaseOneFallbackBody(value *PhaseOneFallbackConfirmation) *phaseOneFallbackConfirmation {
	if value == nil {
		return nil
	}
	return &phaseOneFallbackConfirmation{Token: value.Token, Delivery: string(value.Delivery)}
}

func phaseOneError(kind PhaseOneClientErrorKind) error { return &PhaseOneClientError{Kind: kind} }

type phaseOneUploadSession struct {
	UploadID     string `json:"upload_id"`
	MediaID      string `json:"media_id"`
	UploadToken  string `json:"upload_token"`
	UploadOffset int64  `json:"upload_offset"`
	UploadLength int64  `json:"upload_length"`
	Status       string `json:"status"`
	Reused       bool   `json:"reused"`
}

type contentPolicyResponse struct {
	Contract             string `json:"contract"`
	Version              string `json:"version"`
	PolicyHash           string `json:"policy_hash"`
	Locale               string `json:"locale"`
	LocaleHash           string `json:"locale_hash"`
	EffectiveAt          string `json:"effective_at"`
	TermsURL             string `json:"terms_url"`
	ContentGuidelinesURL string `json:"content_guidelines_url"`
	Title                string `json:"title"`
	RightsText           string `json:"rights_text"`
	ConsentText          string `json:"consent_text"`
	ControllingLanguage  string `json:"controlling_language"`
}

type contentPolicyGrantResponse struct {
	Contract      string `json:"contract"`
	Version       string `json:"version"`
	PolicyHash    string `json:"policy_hash"`
	Locale        string `json:"locale"`
	AcceptedAt    string `json:"accepted_at"`
	RevokedAt     string `json:"revoked_at"`
	Revision      int64  `json:"revision"`
	Current       bool   `json:"current"`
	TermsAccepted bool   `json:"terms_accepted"`
}

type phaseOneTransmissionResponse struct {
	TransmissionID    string `json:"transmission_id"`
	MediaID           string `json:"media_id"`
	RequestedDelivery string `json:"requested_delivery"`
	EffectiveDelivery string `json:"effective_delivery"`
	DowngradeReason   string `json:"downgrade_reason"`
	Status            string `json:"status"`
	Reused            bool   `json:"reused"`
}

type phaseOnePresenceResponse struct {
	Contract string `json:"contract"`
	Nodes    []struct {
		Slot          string `json:"slot"`
		Online        bool   `json:"online"`
		OutputState   string `json:"output_state"`
		PlaybackState string `json:"playback_state"`
		EffectiveDND  struct {
			Mode string `json:"mode"`
		} `json:"effective_dnd"`
	} `json:"nodes"`
}

type phaseOneHistoryResponse struct {
	Contract   string `json:"contract"`
	NextCursor string `json:"next_cursor"`
	Items      []struct {
		HistoryItemID     string   `json:"history_item_id"`
		Direction         string   `json:"direction"`
		OccurredAt        string   `json:"occurred_at"`
		RequestedDelivery string   `json:"requested_delivery"`
		EffectiveDelivery string   `json:"effective_delivery"`
		DowngradeReason   string   `json:"downgrade_reason"`
		Status            string   `json:"status"`
		ReasonCode        string   `json:"reason_code"`
		Actions           []string `json:"actions"`
		Media             struct {
			Title string `json:"title"`
		} `json:"media"`
		Sender struct {
			DisplayName string `json:"display_name"`
		} `json:"sender"`
		TargetCounts struct{ Played, Other int } `json:"target_counts"`
	} `json:"items"`
}

type phaseOneHistoryDeleteResponse struct {
	HistoryItemID string `json:"history_item_id"`
	MediaID       string `json:"media_id"`
	Deleted       bool   `json:"deleted"`
}

type phaseOneHistoryReportResponse struct {
	ID            string                   `json:"id"`
	MediaID       string                   `json:"media_id"`
	HistoryItemID string                   `json:"history_item_id"`
	Reason        PhaseOneModerationReason `json:"reason"`
	Status        string                   `json:"status"`
	CreatedAt     string                   `json:"created_at"`
	UpdatedAt     string                   `json:"updated_at"`
	Reused        bool                     `json:"reused"`
}

type phaseOneHistoryBlockResponse struct {
	BlockID     string `json:"block_id"`
	Scope       string `json:"scope"`
	SubjectRef  string `json:"subject_ref"`
	DisplayName string `json:"display_name"`
	CreatedAt   string `json:"created_at"`
	Revision    int64  `json:"revision"`
	Reused      *bool  `json:"reused"`
}
