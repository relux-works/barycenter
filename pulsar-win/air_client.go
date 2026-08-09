package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

type AirRole string

const (
	AirRoleOwner  AirRole = "owner"
	AirRoleAdmin  AirRole = "admin"
	AirRoleMember AirRole = "member"
)

type AirMembershipStatus string

const (
	AirPendingConfirmation AirMembershipStatus = "pending_confirmation"
	AirJoined              AirMembershipStatus = "joined"
)

type AirInvitePolicy string

const (
	AirInviteOwnerPrimary      AirInvitePolicy = "owner_primary"
	AirInviteAdminPrimary      AirInvitePolicy = "air_admin_primary"
	AirInviteAllMemberPrimarys AirInvitePolicy = "all_member_primaries"
)

type AirPlaybackPolicy string

const (
	AirPlaybackOwnerPrimary      AirPlaybackPolicy = "owner_primary"
	AirPlaybackAdminPrimary      AirPlaybackPolicy = "air_admin_primary"
	AirPlaybackAllMemberPrimarys AirPlaybackPolicy = "all_member_primaries"
	AirPlaybackPrimaryCompanion  AirPlaybackPolicy = "primary_companion"
	AirPlaybackDisabled          AirPlaybackPolicy = "disabled"
)

type AirCapacity struct {
	Barycenters   int `json:"barycenters"`
	OnlinePulsars int `json:"online_pulsars"`
}

type AirPolicy struct {
	Revision int64             `json:"revision"`
	Invite   AirInvitePolicy   `json:"invite"`
	Overlay  AirPlaybackPolicy `json:"overlay"`
	Queue    AirPlaybackPolicy `json:"queue"`
	Replace  AirPlaybackPolicy `json:"replace"`
}

type AirSummary struct {
	AirID             string              `json:"air_id"`
	Title             string              `json:"title"`
	Status            string              `json:"status"`
	MembershipStatus  AirMembershipStatus `json:"membership_status"`
	Role              AirRole             `json:"air_role"`
	MemberCount       int                 `json:"member_count"`
	ActiveMemberCount int                 `json:"active_member_count"`
	OnlinePulsarCount int                 `json:"online_pulsar_count"`
	Capacity          AirCapacity         `json:"capacity"`
	PolicyRevision    int64               `json:"policy_revision"`
	Current           bool                `json:"is_current"`
}

type AirList struct {
	CurrentAirID          *string      `json:"current_air_id"`
	ActivePointerRevision int64        `json:"active_pointer_revision"`
	Saved                 []AirSummary `json:"saved"`
}

type AirDetail struct {
	AirID              string              `json:"air_id"`
	Title              string              `json:"title"`
	Status             string              `json:"status"`
	Revision           int64               `json:"revision"`
	MembershipID       string              `json:"membership_id"`
	MembershipStatus   AirMembershipStatus `json:"membership_status"`
	MembershipRevision int64               `json:"membership_revision"`
	Role               AirRole             `json:"air_role"`
	MemberCount        int                 `json:"member_count"`
	ActiveMemberCount  int                 `json:"active_member_count"`
	OnlinePulsarCount  int                 `json:"online_pulsar_count"`
	Capacity           AirCapacity         `json:"capacity"`
	Policy             AirPolicy           `json:"policy"`
	Current            bool                `json:"is_current"`
}

type AirInvite struct {
	InviteID string    `json:"invite_id"`
	Revision int64     `json:"revision"`
	Expires  time.Time `json:"-"`
	Code     string    `json:"-"`
}

func (i AirInvite) String() string {
	return fmt.Sprintf("AirInvite{id:%s revision:%d expires:%s code:<redacted>}", i.InviteID, i.Revision, i.Expires.Format(time.RFC3339))
}

func (i AirInvite) GoString() string { return i.String() }

type AirJoinPreview struct {
	AirID                 string
	Title                 string
	OwnerDisplayName      string
	Role                  AirRole
	MembershipRevision    int64
	Policy                AirPolicy
	MemberCount           int
	Capacity              AirCapacity
	ActivationWouldSwitch bool
}

type AirFeatureAvailability struct {
	Enabled        bool
	AuthorityState string
}

type AirClientErrorKind string

const (
	AirInvalidConfiguration AirClientErrorKind = "invalid_configuration"
	AirInvalidRequest       AirClientErrorKind = "invalid_request"
	AirTransport            AirClientErrorKind = "transport"
	AirRedirectRejected     AirClientErrorKind = "redirect_rejected"
	AirResponseTooLarge     AirClientErrorKind = "response_too_large"
	AirInvalidResponse      AirClientErrorKind = "invalid_response"
	AirRejected             AirClientErrorKind = "rejected"
)

type AirClientError struct {
	Kind              AirClientErrorKind
	Status            int
	Code              string
	RetryAfterSeconds int
}

func (e *AirClientError) Error() string {
	if e == nil {
		return "air_error"
	}
	if e.Kind == AirRejected && e.Code != "" {
		return "air_rejected:" + e.Code
	}
	return "air_" + string(e.Kind)
}

func (e *AirClientError) String() string { return e.Error() }
func (e *AirClientError) GoString() string {
	if e == nil {
		return "AirClientError{}"
	}
	return fmt.Sprintf("AirClientError{kind:%s status:%d code:<redacted> retry:%d}", e.Kind, e.Status, e.RetryAfterSeconds)
}

type AirAppService interface {
	Availability(context.Context) (AirFeatureAvailability, error)
	List(context.Context) (AirList, error)
	Detail(context.Context, string) (AirDetail, error)
	Create(context.Context, string, string) (AirDetail, error)
	IssueInvite(context.Context, string, AirRole, string) (AirInvite, error)
	WithdrawInvite(context.Context, string, string, int64, string) error
	ConsumeInvite(context.Context, string, string) (AirJoinPreview, error)
	ConfirmJoin(context.Context, string, int64, bool, string, string) error
	DeclineJoin(context.Context, string, int64, string) error
	Activate(context.Context, string, int64, string, string) error
	Deactivate(context.Context, string, int64, string, string) error
	Leave(context.Context, string, int64, string, string) error
	ReplacePolicy(context.Context, string, AirPolicy, string) error
	Dissolve(context.Context, string, int64, string) error
}

type AirAppClient struct {
	origin CoordinatorOrigin
	token  string
	doer   HTTPDoer
}

func NewAirAppClient(bundle CredentialBundle, doer HTTPDoer) (*AirAppClient, error) {
	if bundle.validate() != nil || bundle.Control == nil || bundle.Control.Context != ControlContextActive ||
		!lowerHexTokenPattern.MatchString(bundle.Control.ControlToken) {
		return nil, airError(AirInvalidConfiguration)
	}
	origin, err := CanonicalCoordinatorOrigin(bundle.CoordinatorOrigin)
	if err != nil || !origin.permitsSecrets() {
		return nil, airError(AirInvalidConfiguration)
	}
	if doer == nil {
		doer = &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	} else if client, ok := doer.(*http.Client); ok {
		clone := *client
		clone.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		doer = &clone
	}
	return &AirAppClient{origin: origin, token: bundle.Control.ControlToken, doer: doer}, nil
}

func (c *AirAppClient) String() string   { return "AirAppClient{<redacted>}" }
func (c *AirAppClient) GoString() string { return c.String() }

func (c *AirAppClient) Availability(ctx context.Context) (AirFeatureAvailability, error) {
	raw, err := c.request(ctx, http.MethodGet, "/healthz", "", nil, http.StatusOK)
	if err != nil {
		return AirFeatureAvailability{}, err
	}
	var wire struct {
		Phase2 struct {
			Enabled        bool   `json:"air_rooms_enabled"`
			AuthorityState string `json:"air_authority_state"`
		} `json:"phase2"`
	}
	if decodeAirJSON(raw, &wire) != nil || wire.Phase2.AuthorityState == "" {
		return AirFeatureAvailability{}, airError(AirInvalidResponse)
	}
	return AirFeatureAvailability{
		Enabled: wire.Phase2.Enabled, AuthorityState: wire.Phase2.AuthorityState,
	}, nil
}

func (c *AirAppClient) List(ctx context.Context) (AirList, error) {
	raw, err := c.request(ctx, http.MethodGet, "/v1/airs", "", nil, http.StatusOK)
	if err != nil {
		return AirList{}, err
	}
	var result AirList
	if decodeAirJSON(raw, &result) != nil || result.ActivePointerRevision < 0 {
		return AirList{}, airError(AirInvalidResponse)
	}
	seen, current := map[string]bool{}, 0
	for _, item := range result.Saved {
		if validateAirSummary(item) != nil || seen[item.AirID] {
			return AirList{}, airError(AirInvalidResponse)
		}
		seen[item.AirID] = true
		if item.Current {
			current++
			if result.CurrentAirID == nil || *result.CurrentAirID != item.AirID {
				return AirList{}, airError(AirInvalidResponse)
			}
		}
	}
	if result.CurrentAirID != nil {
		if !validAirID(*result.CurrentAirID) || current != 1 {
			return AirList{}, airError(AirInvalidResponse)
		}
	} else if current != 0 {
		return AirList{}, airError(AirInvalidResponse)
	}
	return result, nil
}

func (c *AirAppClient) Detail(ctx context.Context, airID string) (AirDetail, error) {
	if !validAirID(airID) {
		return AirDetail{}, airError(AirInvalidRequest)
	}
	raw, err := c.request(ctx, http.MethodGet, "/v1/airs/"+airID, "", nil, http.StatusOK)
	if err != nil {
		return AirDetail{}, err
	}
	return decodeAirDetail(raw)
}

func (c *AirAppClient) Create(ctx context.Context, title, key string) (AirDetail, error) {
	title = strings.TrimSpace(title)
	if !validAirTitle(title) {
		return AirDetail{}, airError(AirInvalidRequest)
	}
	raw, err := c.request(ctx, http.MethodPost, "/v1/airs", key, struct {
		Title string `json:"title"`
	}{title}, http.StatusCreated)
	if err != nil {
		return AirDetail{}, err
	}
	return decodeAirDetail(raw)
}

func (c *AirAppClient) IssueInvite(ctx context.Context, airID string, role AirRole, key string) (AirInvite, error) {
	if !validAirID(airID) || role != AirRoleAdmin && role != AirRoleMember {
		return AirInvite{}, airError(AirInvalidRequest)
	}
	raw, err := c.request(ctx, http.MethodPost, "/v1/airs/"+airID+"/invites", key, struct {
		Role AirRole `json:"air_role"`
	}{role}, http.StatusCreated)
	if err != nil {
		return AirInvite{}, err
	}
	var wire struct {
		InviteID string `json:"invite_id"`
		Revision int64  `json:"revision"`
		Expires  string `json:"expires_at"`
		Code     string `json:"code"`
	}
	if decodeAirJSON(raw, &wire) != nil || !validAirInviteID(wire.InviteID) || wire.Revision <= 0 ||
		!validPhaseOneDisplayText(wire.Code, 512, false) {
		return AirInvite{}, airError(AirInvalidResponse)
	}
	expires, err := time.Parse(time.RFC3339Nano, wire.Expires)
	if err != nil {
		return AirInvite{}, airError(AirInvalidResponse)
	}
	return AirInvite{InviteID: wire.InviteID, Revision: wire.Revision, Expires: expires, Code: wire.Code}, nil
}

func (c *AirAppClient) WithdrawInvite(ctx context.Context, airID, inviteID string, revision int64, key string) error {
	if !validAirID(airID) || !validAirInviteID(inviteID) || revision <= 0 {
		return airError(AirInvalidRequest)
	}
	_, err := c.request(ctx, http.MethodPost, "/v1/airs/"+airID+"/invites/"+inviteID+"/withdraw", key, struct {
		Revision int64 `json:"invite_revision"`
	}{revision}, http.StatusOK)
	return err
}

func (c *AirAppClient) ConsumeInvite(ctx context.Context, code, key string) (AirJoinPreview, error) {
	code = strings.TrimSpace(code)
	if !validPhaseOneDisplayText(code, 512, false) {
		return AirJoinPreview{}, airError(AirInvalidRequest)
	}
	raw, err := c.request(ctx, http.MethodPost, "/v1/air-invites/consume", key, struct {
		Code string `json:"code"`
	}{code}, http.StatusAccepted)
	if err != nil {
		return AirJoinPreview{}, err
	}
	var wire struct {
		AirID                 string      `json:"air_id"`
		Title                 string      `json:"title"`
		OwnerDisplayName      string      `json:"owner_display_name"`
		Role                  AirRole     `json:"air_role"`
		MembershipID          string      `json:"membership_id"`
		MembershipRevision    int64       `json:"membership_revision"`
		Policy                AirPolicy   `json:"policy"`
		MemberCount           int         `json:"member_count"`
		Capacity              AirCapacity `json:"capacity"`
		ActivationWouldSwitch bool        `json:"activation_would_switch"`
	}
	if decodeAirJSON(raw, &wire) != nil || !validAirID(wire.AirID) || !validAirMemberID(wire.MembershipID) ||
		!validAirTitle(wire.Title) || !validPhaseOneDisplayText(wire.OwnerDisplayName, 256, true) ||
		!validAirRole(wire.Role) || wire.Role == AirRoleOwner || wire.MembershipRevision <= 0 || wire.MemberCount < 0 ||
		validateAirCapacity(wire.Capacity) != nil || validateAirPolicy(wire.Policy) != nil {
		return AirJoinPreview{}, airError(AirInvalidResponse)
	}
	if wire.MemberCount > wire.Capacity.Barycenters {
		return AirJoinPreview{}, airError(AirInvalidResponse)
	}
	return AirJoinPreview{AirID: wire.AirID, Title: wire.Title, OwnerDisplayName: wire.OwnerDisplayName,
		Role: wire.Role, MembershipRevision: wire.MembershipRevision, Policy: wire.Policy,
		MemberCount: wire.MemberCount, Capacity: wire.Capacity, ActivationWouldSwitch: wire.ActivationWouldSwitch}, nil
}

func (c *AirAppClient) ConfirmJoin(ctx context.Context, airID string, revision int64, activate bool, expected, key string) error {
	if !validAirID(airID) || revision <= 0 || activate && !validExpectedAirID(expected) {
		return airError(AirInvalidRequest)
	}
	body := struct {
		Revision int64   `json:"membership_revision"`
		Activate bool    `json:"activate"`
		Expected *string `json:"expected_active_air_id,omitempty"`
	}{Revision: revision, Activate: activate}
	if activate {
		if expected == "" {
			expected = "none"
		}
		body.Expected = &expected
	}
	_, err := c.request(ctx, http.MethodPost, "/v1/airs/"+airID+"/join/confirm", key, body, http.StatusOK)
	return err
}

func (c *AirAppClient) DeclineJoin(ctx context.Context, airID string, revision int64, key string) error {
	return c.membershipMutation(ctx, "/v1/airs/"+airID+"/join/decline", airID, revision, key)
}

func (c *AirAppClient) Activate(ctx context.Context, airID string, revision int64, expected, key string) error {
	return c.activationMutation(ctx, "/v1/airs/"+airID+"/activate", airID, revision, expected, key)
}

func (c *AirAppClient) Deactivate(ctx context.Context, airID string, revision int64, expected, key string) error {
	if expected == "" {
		return airError(AirInvalidRequest)
	}
	return c.activationMutation(ctx, "/v1/airs/"+airID+"/deactivate", airID, revision, expected, key)
}

func (c *AirAppClient) Leave(ctx context.Context, airID string, revision int64, expected, key string) error {
	return c.activationMutation(ctx, "/v1/airs/"+airID+"/leave", airID, revision, expected, key)
}

func (c *AirAppClient) ReplacePolicy(ctx context.Context, airID string, policy AirPolicy, key string) error {
	if !validAirID(airID) || validateAirPolicy(policy) != nil {
		return airError(AirInvalidRequest)
	}
	body := struct {
		Revision int64             `json:"policy_revision"`
		Invite   AirInvitePolicy   `json:"invite"`
		Overlay  AirPlaybackPolicy `json:"overlay"`
		Queue    AirPlaybackPolicy `json:"queue"`
		Replace  AirPlaybackPolicy `json:"replace"`
	}{policy.Revision, policy.Invite, policy.Overlay, policy.Queue, policy.Replace}
	_, err := c.request(ctx, http.MethodPut, "/v1/airs/"+airID+"/policy", key, body, http.StatusOK)
	return err
}

func (c *AirAppClient) Dissolve(ctx context.Context, airID string, revision int64, key string) error {
	if !validAirID(airID) || revision <= 0 {
		return airError(AirInvalidRequest)
	}
	_, err := c.request(ctx, http.MethodPost, "/v1/airs/"+airID+"/dissolve", key, struct {
		Revision int64 `json:"air_revision"`
	}{revision}, http.StatusOK)
	return err
}

func (c *AirAppClient) membershipMutation(ctx context.Context, path, airID string, revision int64, key string) error {
	if !validAirID(airID) || revision <= 0 {
		return airError(AirInvalidRequest)
	}
	_, err := c.request(ctx, http.MethodPost, path, key, struct {
		Revision int64 `json:"membership_revision"`
	}{revision}, http.StatusOK)
	return err
}

func (c *AirAppClient) activationMutation(ctx context.Context, path, airID string, revision int64, expected, key string) error {
	if !validAirID(airID) || revision <= 0 || !validExpectedAirID(expected) {
		return airError(AirInvalidRequest)
	}
	if expected == "" {
		expected = "none"
	}
	_, err := c.request(ctx, http.MethodPost, path, key, struct {
		Revision int64  `json:"membership_revision"`
		Expected string `json:"expected_active_air_id"`
	}{revision, expected}, http.StatusOK)
	return err
}

func (c *AirAppClient) request(ctx context.Context, method, path, key string, body any, success ...int) ([]byte, error) {
	if c == nil || !lowerHexTokenPattern.MatchString(c.token) || !strings.HasPrefix(path, "/") || strings.Contains(path, "\\") ||
		(body != nil || key != "") && !validAirIdempotencyKey(key) {
		return nil, airError(AirInvalidRequest)
	}
	endpoint, err := c.origin.URL(path)
	if err != nil {
		return nil, airError(AirInvalidRequest)
	}
	var payload *bytes.Reader
	if body != nil {
		raw, marshalErr := json.Marshal(body)
		if marshalErr != nil {
			return nil, airError(AirInvalidRequest)
		}
		payload = bytes.NewReader(raw)
	} else {
		payload = bytes.NewReader(nil)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), payload)
	if err != nil {
		return nil, airError(AirInvalidRequest)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", key)
	}
	response, err := c.doer.Do(request)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, airError(AirTransport)
	}
	if response == nil || response.Body == nil {
		return nil, airError(AirInvalidResponse)
	}
	defer response.Body.Close()
	finalURL := endpoint
	if response.Request != nil && response.Request.URL != nil {
		finalURL = response.Request.URL
	}
	finalOrigin, originErr := CanonicalCoordinatorOrigin(finalURL.Scheme + "://" + finalURL.Host)
	if originErr != nil || finalOrigin != c.origin || response.StatusCode >= 300 && response.StatusCode < 400 {
		return nil, airError(AirRedirectRejected)
	}
	raw, readErr := readBoundedResponse(response.Body)
	if readErr != nil {
		if errors.Is(readErr, context.Canceled) || errors.Is(readErr, context.DeadlineExceeded) {
			return nil, readErr
		}
		return nil, airError(AirResponseTooLarge)
	}
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(response.Header.Get("Content-Type"), ";")[0]))
	if contentType != "application/json" {
		return nil, airError(AirInvalidResponse)
	}
	ok := false
	for _, status := range success {
		ok = ok || response.StatusCode == status
	}
	if !ok {
		return nil, decodeAirAPIError(raw, response.StatusCode)
	}
	return raw, nil
}

func decodeAirJSON(raw []byte, target any) error {
	if _, err := parseStrictJSONObject(raw); err != nil {
		return err
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return errInvalidResponse
	}
	return nil
}

func decodeAirAPIError(raw []byte, status int) error {
	var envelope struct {
		Error struct {
			Code              string `json:"code"`
			RetryAfterSeconds int    `json:"retry_after_seconds"`
		} `json:"error"`
	}
	if decodeAirJSON(raw, &envelope) != nil || !validPhaseOneErrorCode(envelope.Error.Code) ||
		envelope.Error.RetryAfterSeconds < 0 || envelope.Error.RetryAfterSeconds > 86_400 {
		return airError(AirInvalidResponse)
	}
	return &AirClientError{Kind: AirRejected, Status: status, Code: envelope.Error.Code, RetryAfterSeconds: envelope.Error.RetryAfterSeconds}
}

func decodeAirDetail(raw []byte) (AirDetail, error) {
	var result AirDetail
	if decodeAirJSON(raw, &result) != nil || validateAirDetail(result) != nil {
		return AirDetail{}, airError(AirInvalidResponse)
	}
	return result, nil
}

func validateAirSummary(value AirSummary) error {
	if !validAirID(value.AirID) || !validAirTitle(value.Title) || !validAirStatus(value.Status) ||
		!validAirMembership(value.MembershipStatus) || !validAirRole(value.Role) || value.MemberCount < 0 ||
		value.ActiveMemberCount < 0 || value.ActiveMemberCount > value.MemberCount || value.OnlinePulsarCount < 0 ||
		value.PolicyRevision <= 0 || validateAirCapacity(value.Capacity) != nil || value.MemberCount > value.Capacity.Barycenters ||
		value.OnlinePulsarCount > value.Capacity.OnlinePulsars || value.Current && value.MembershipStatus != AirJoined {
		return errInvalidResponse
	}
	return nil
}

func validateAirDetail(value AirDetail) error {
	if validateAirSummary(AirSummary{AirID: value.AirID, Title: value.Title, Status: value.Status,
		MembershipStatus: value.MembershipStatus, Role: value.Role, MemberCount: value.MemberCount,
		ActiveMemberCount: value.ActiveMemberCount, OnlinePulsarCount: value.OnlinePulsarCount,
		Capacity: value.Capacity, PolicyRevision: value.Policy.Revision, Current: value.Current}) != nil ||
		!validAirMemberID(value.MembershipID) || value.Revision <= 0 || value.MembershipRevision <= 0 ||
		validateAirPolicy(value.Policy) != nil {
		return errInvalidResponse
	}
	return nil
}

func validateAirCapacity(value AirCapacity) error {
	if value.Barycenters <= 0 || value.OnlinePulsars <= 0 {
		return errInvalidResponse
	}
	return nil
}

func validateAirPolicy(value AirPolicy) error {
	playback := value.Overlay == AirPlaybackAdminPrimary || value.Overlay == AirPlaybackAllMemberPrimarys ||
		value.Overlay == AirPlaybackPrimaryCompanion || value.Overlay == AirPlaybackDisabled
	queue := value.Queue == AirPlaybackAdminPrimary || value.Queue == AirPlaybackAllMemberPrimarys ||
		value.Queue == AirPlaybackPrimaryCompanion || value.Queue == AirPlaybackDisabled
	replace := value.Replace == AirPlaybackOwnerPrimary || value.Replace == AirPlaybackAdminPrimary ||
		value.Replace == AirPlaybackAllMemberPrimarys || value.Replace == AirPlaybackDisabled
	invite := value.Invite == AirInviteOwnerPrimary || value.Invite == AirInviteAdminPrimary || value.Invite == AirInviteAllMemberPrimarys
	if value.Revision <= 0 || !invite || !playback || !queue || !replace {
		return errInvalidResponse
	}
	return nil
}

func validAirTitle(value string) bool {
	if value == "" || strings.TrimSpace(value) != value || !utf8.ValidString(value) || utf8.RuneCountInString(value) > 80 {
		return false
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}
func validAirStatus(value string) bool { return value == "parked" || value == "active" }
func validAirRole(value AirRole) bool {
	return value == AirRoleOwner || value == AirRoleAdmin || value == AirRoleMember
}
func validAirMembership(value AirMembershipStatus) bool {
	return value == AirPendingConfirmation || value == AirJoined
}
func validAirID(value string) bool       { return validPhaseOnePublicID(value, "air_") }
func validAirInviteID(value string) bool { return validPhaseOnePublicID(value, "ai_") }
func validAirMemberID(value string) bool { return validPhaseOnePublicID(value, "aim_") }
func validExpectedAirID(value string) bool {
	return value == "" || value == "none" || validAirID(value)
}
func validAirIdempotencyKey(value string) bool {
	return len(value) >= 16 && validPhaseOneIdempotencyKey(value)
}
func airError(kind AirClientErrorKind) error { return &AirClientError{Kind: kind} }
