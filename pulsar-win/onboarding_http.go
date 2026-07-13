package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

var installationAttemptPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{16,128}$`)

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type ClientErrorKind string

const (
	ClientErrorInvalidRequest  ClientErrorKind = "invalid_request"
	ClientErrorUnauthorized    ClientErrorKind = "unauthorized"
	ClientErrorCapability      ClientErrorKind = "insufficient_capability"
	ClientErrorCredential      ClientErrorKind = "credential_invalid"
	ClientErrorRateLimited     ClientErrorKind = "too_many_attempts"
	ClientErrorConflict        ClientErrorKind = "conflict"
	ClientErrorServer          ClientErrorKind = "server_error"
	ClientErrorTransport       ClientErrorKind = "transport_error"
	ClientErrorCancelled       ClientErrorKind = "cancelled"
	ClientErrorInvalidResponse ClientErrorKind = "invalid_response"
)

// OnboardingClientError deliberately retains no request/response body, URL,
// bearer, redirect target, or wrapped transport error.
type OnboardingClientError struct {
	Kind              ClientErrorKind
	Status            int
	Code              string
	RetryAfterSeconds int
}

func (e *OnboardingClientError) Error() string {
	switch e.Kind {
	case ClientErrorInvalidRequest:
		return "The request is malformed or contains invalid parameters."
	case ClientErrorUnauthorized:
		return "Authentication is required."
	case ClientErrorCapability:
		return "This token does not have the required capability."
	case ClientErrorCredential:
		return "The provided credential is not valid."
	case ClientErrorRateLimited:
		return "Too many attempts. Please wait before retrying."
	case ClientErrorConflict:
		return "The requested identity is already linked."
	case ClientErrorCancelled:
		return "The request was cancelled."
	case ClientErrorTransport:
		return "The coordinator is unavailable."
	case ClientErrorServer:
		return "The coordinator could not process the request."
	default:
		return "The coordinator returned an invalid response."
	}
}

func (e *OnboardingClientError) String() string { return e.Error() }
func (e *OnboardingClientError) GoString() string {
	return fmt.Sprintf("OnboardingClientError{status:%d retry:%d}", e.Status, e.RetryAfterSeconds)
}

type OnboardingClient struct {
	origin CoordinatorOrigin
	doer   HTTPDoer
}

func NewOnboardingClient(rawOrigin string, doer HTTPDoer) (*OnboardingClient, error) {
	origin, err := CanonicalCoordinatorOrigin(rawOrigin)
	if err != nil || !origin.permitsSecrets() {
		return nil, errInvalidCoordinatorOrigin
	}
	if doer == nil {
		doer = &http.Client{
			Timeout: 15 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	} else if client, ok := doer.(*http.Client); ok {
		clone := *client
		clone.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}
		doer = &clone
	}
	return &OnboardingClient{origin: origin, doer: doer}, nil
}

func (c *OnboardingClient) Origin() CoordinatorOrigin { return c.origin }

type CreateOrbitResult struct {
	OrbitID  int64
	Title    string
	ActorID  int64
	Role     string
	Bundle   CredentialBundle
	Recovery *RecoveryMaterial
}

func (r CreateOrbitResult) String() string {
	return fmt.Sprintf("CreateOrbitResult{orbit:%d actor:%d credentials:<redacted> recovery:<redacted>}", r.OrbitID, r.ActorID)
}
func (r CreateOrbitResult) GoString() string { return r.String() }

type JoinOrbitResult struct {
	OrbitID int64
	Title   string
	ActorID int64
	Role    string
	Bundle  CredentialBundle
}

func (r JoinOrbitResult) String() string {
	return fmt.Sprintf("JoinOrbitResult{orbit:%d actor:%d credentials:<redacted>}", r.OrbitID, r.ActorID)
}
func (r JoinOrbitResult) GoString() string { return r.String() }

type ActorContext struct {
	OrbitID int64
	ActorID int64
	Role    string
}

type OneTimeCode struct {
	mu        sync.Mutex
	value     []byte
	discarded bool
}

func newOneTimeCode(value string) *OneTimeCode { return &OneTimeCode{value: []byte(value)} }
func (c *OneTimeCode) String() string          { return "OneTimeCode{<redacted>}" }
func (c *OneTimeCode) GoString() string        { return c.String() }
func (c *OneTimeCode) RevealForDisplay() (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.discarded || len(c.value) == 0 {
		return "", false
	}
	return string(c.value), true
}
func (c *OneTimeCode) Discard() {
	c.mu.Lock()
	defer c.mu.Unlock()
	zeroBytes(c.value)
	c.value = nil
	c.discarded = true
}

type DeviceInvite struct {
	Code         *OneTimeCode
	IntendedRole string
	ExpiresAt    time.Time
}

func (i DeviceInvite) String() string {
	return "DeviceInvite{code:<redacted>}"
}
func (i DeviceInvite) GoString() string { return i.String() }

type TelegramLink struct {
	Code        *OneTimeCode
	DesiredRole string
	ExpiresAt   time.Time
	BotUsername string
}

func (l TelegramLink) String() string {
	return "TelegramLink{code:<redacted>}"
}
func (l TelegramLink) GoString() string { return l.String() }

func (c *OnboardingClient) CreateOrbit(ctx context.Context, title, installationAttemptID string) (CreateOrbitResult, error) {
	title = strings.TrimSpace(title)
	if title == "" || len(title) > 120 || !utf8.ValidString(title) || !installationAttemptPattern.MatchString(installationAttemptID) {
		return CreateOrbitResult{}, clientInputError()
	}
	body, _ := json.Marshal(struct {
		Title                 string `json:"title"`
		InstallationAttemptID string `json:"installation_attempt_id"`
	}{title, installationAttemptID})
	defer zeroBytes(body)
	raw, err := c.request(ctx, http.MethodPost, "/v1/onboarding/orbits", body, "", http.StatusCreated, true, true)
	if err != nil {
		return CreateOrbitResult{}, err
	}
	defer zeroBytes(raw)
	return c.decodeCreate(raw, title)
}

func (c *OnboardingClient) IssueDeviceInvite(ctx context.Context, capability ControlCapability, intendedRole string) (DeviceInvite, error) {
	if intendedRole == "" {
		intendedRole = "companion"
	}
	if intendedRole != "companion" && intendedRole != "satellite" {
		return DeviceInvite{}, clientInputError()
	}
	token, err := c.controlBearer(capability)
	if err != nil {
		return DeviceInvite{}, err
	}
	body, _ := json.Marshal(struct {
		IntendedRole string `json:"intended_role"`
	}{intendedRole})
	defer zeroBytes(body)
	raw, err := c.request(ctx, http.MethodPost, "/v1/device-invites", body, token, http.StatusCreated, true, true)
	if err != nil {
		return DeviceInvite{}, err
	}
	defer zeroBytes(raw)
	object, err := parseStrictJSONObject(raw)
	if err != nil || !exactObjectKeys(object, "invite_code", "intended_role", "expires_at") {
		return DeviceInvite{}, invalidResponseError()
	}
	code, okCode := jsonString(object, "invite_code")
	role, okRole := jsonString(object, "intended_role")
	expires, okExpires := jsonString(object, "expires_at")
	when, timeErr := time.Parse(time.RFC3339, expires)
	if !okCode || !okRole || !okExpires || !humanSecretPattern.MatchString(code) || role != intendedRole || timeErr != nil {
		return DeviceInvite{}, invalidResponseError()
	}
	return DeviceInvite{Code: newOneTimeCode(code), IntendedRole: role, ExpiresAt: when}, nil
}

func (c *OnboardingClient) JoinOrbit(ctx context.Context, inviteCode string) (JoinOrbitResult, error) {
	canonical, ok := normalizeHumanSecret(inviteCode)
	if !ok || !humanSecretPattern.MatchString(canonical) {
		return JoinOrbitResult{}, clientInputError()
	}
	body, _ := json.Marshal(struct {
		InviteCode string `json:"invite_code"`
	}{canonical})
	defer zeroBytes(body)
	raw, err := c.request(ctx, http.MethodPost, "/v1/device-invites/consume", body, "", http.StatusOK, true, true)
	if err != nil {
		return JoinOrbitResult{}, err
	}
	defer zeroBytes(raw)
	return c.decodeJoin(raw)
}

func (c *OnboardingClient) pairLegacy(ctx context.Context, code string) (Credentials, error) {
	if code == "" || len(code) > 128 || strings.ContainsAny(code, "\r\n") {
		return Credentials{}, clientInputError()
	}
	body, _ := json.Marshal(struct {
		Code string `json:"code"`
	}{code})
	defer zeroBytes(body)
	raw, err := c.request(ctx, http.MethodPost, "/pair", body, "", http.StatusOK, false, false)
	if err != nil {
		return Credentials{}, err
	}
	defer zeroBytes(raw)
	object, err := parseStrictJSONObject(raw)
	if err != nil || !exactObjectKeys(object, "orbit_id", "slot", "token", "ws_url") {
		return Credentials{}, invalidResponseError()
	}
	orbitID, okOrbit := jsonInt64(object, "orbit_id")
	slot, okSlot := jsonString(object, "slot")
	token, okToken := jsonString(object, "token")
	wsURL, okWS := jsonString(object, "ws_url")
	creds := Credentials{OrbitID: orbitID, Slot: slot, Token: token, WSURL: wsURL}
	if !okOrbit || !okSlot || !okToken || !okWS || ValidateCredentials(creds) != nil {
		return Credentials{}, invalidResponseError()
	}
	return creds, nil
}

func (c *OnboardingClient) ActorContext(ctx context.Context, capability ActorCapability) (ActorContext, error) {
	origin, token, expectedActorID, expectedOrbitID, ok := actorCapabilityIdentity(capability)
	if !ok || origin != c.origin || !lowerHexTokenPattern.MatchString(token) {
		return ActorContext{}, clientInputError()
	}
	raw, err := c.request(ctx, http.MethodGet, "/v1/actor/context", nil, token, http.StatusOK, true, true)
	if err != nil {
		return ActorContext{}, err
	}
	defer zeroBytes(raw)
	result, err := decodeActorContext(raw)
	if err != nil || (expectedActorID != 0 && result.ActorID != expectedActorID) || (expectedOrbitID != 0 && result.OrbitID != expectedOrbitID) {
		return ActorContext{}, invalidResponseError()
	}
	return result, nil
}

func actorCapabilityIdentity(capability ActorCapability) (origin CoordinatorOrigin, token string, actorID, orbitID int64, ok bool) {
	switch value := capability.(type) {
	case NodeCapability:
		if value.value.OrbitID <= 0 {
			return CoordinatorOrigin{}, "", 0, 0, false
		}
		origin, token = value.actorBearer()
		return origin, token, 0, value.value.OrbitID, true
	case *NodeCapability:
		if value == nil || value.value.OrbitID <= 0 {
			return CoordinatorOrigin{}, "", 0, 0, false
		}
		origin, token = value.actorBearer()
		return origin, token, 0, value.value.OrbitID, true
	case ControlCapability:
		if value.value.ActorID <= 0 {
			return CoordinatorOrigin{}, "", 0, 0, false
		}
		origin, token = value.actorBearer()
		return origin, token, value.value.ActorID, 0, true
	case *ControlCapability:
		if value == nil || value.value.ActorID <= 0 {
			return CoordinatorOrigin{}, "", 0, 0, false
		}
		origin, token = value.actorBearer()
		return origin, token, value.value.ActorID, 0, true
	default:
		return CoordinatorOrigin{}, "", 0, 0, false
	}
}

func (c *OnboardingClient) RotateRecovery(ctx context.Context, capability ControlCapability) (*RecoveryMaterial, error) {
	token, err := c.controlBearer(capability)
	if err != nil {
		return nil, err
	}
	body := []byte("{}")
	defer zeroBytes(body)
	raw, err := c.request(ctx, http.MethodPost, "/v1/recovery/rotate", body, token, http.StatusOK, true, true)
	if err != nil {
		return nil, err
	}
	defer zeroBytes(raw)
	material, err := decodeRecoveryMaterial(raw)
	if err != nil {
		return nil, err
	}
	actorID, _, ok := material.metadata()
	if !ok || actorID != capability.value.ActorID {
		material.discard()
		return nil, invalidResponseError()
	}
	return material, nil
}

func (c *OnboardingClient) IssueTelegramLink(ctx context.Context, capability ControlCapability, desiredRole string) (TelegramLink, error) {
	if desiredRole == "" {
		desiredRole = "companion"
	}
	if desiredRole != "companion" && desiredRole != "satellite" {
		return TelegramLink{}, clientInputError()
	}
	token, err := c.controlBearer(capability)
	if err != nil {
		return TelegramLink{}, err
	}
	body, _ := json.Marshal(struct {
		DesiredRole string `json:"desired_role"`
	}{desiredRole})
	defer zeroBytes(body)
	raw, err := c.request(ctx, http.MethodPost, "/v1/telegram-links", body, token, http.StatusCreated, true, true)
	if err != nil {
		return TelegramLink{}, err
	}
	defer zeroBytes(raw)
	object, err := parseStrictJSONObject(raw)
	if err != nil || !exactObjectKeys(object, "link_code", "desired_role", "expires_at", "bot_username") {
		return TelegramLink{}, invalidResponseError()
	}
	code, okCode := jsonString(object, "link_code")
	role, okRole := jsonString(object, "desired_role")
	expires, okExpires := jsonString(object, "expires_at")
	bot, okBot := jsonString(object, "bot_username")
	when, timeErr := time.Parse(time.RFC3339, expires)
	if !okCode || !okRole || !okExpires || !okBot || !humanSecretPattern.MatchString(code) || role != desiredRole || timeErr != nil || !validTelegramBotUsername(bot) {
		return TelegramLink{}, invalidResponseError()
	}
	return TelegramLink{Code: newOneTimeCode(code), DesiredRole: role, ExpiresAt: when, BotUsername: bot}, nil
}

func (c *OnboardingClient) consumeRecovery(ctx context.Context, recoveryID string, recoverySecret []byte, replacementToken string) (ActorContext, error) {
	if !recoveryIDPattern.MatchString(recoveryID) || !humanSecretPattern.Match(recoverySecret) || !lowerHexTokenPattern.MatchString(replacementToken) {
		return ActorContext{}, clientInputError()
	}
	body, _ := json.Marshal(struct {
		RecoveryID              string `json:"recovery_id"`
		RecoverySecret          string `json:"recovery_secret"`
		ReplacementControlToken string `json:"replacement_control_token"`
	}{recoveryID, string(recoverySecret), replacementToken})
	defer zeroBytes(body)
	raw, err := c.request(ctx, http.MethodPost, "/v1/recovery/consume", body, "", http.StatusOK, true, true)
	if err != nil {
		return ActorContext{}, err
	}
	defer zeroBytes(raw)
	return decodeActorContext(raw)
}

func (c *OnboardingClient) controlBearer(capability ControlCapability) (string, error) {
	origin, token := capability.actorBearer()
	if origin != c.origin || !lowerHexTokenPattern.MatchString(token) {
		return "", clientInputError()
	}
	return token, nil
}

func (c *OnboardingClient) request(ctx context.Context, method, path string, body []byte, bearer string, expectedStatus int, requireNoStore, strictErrors bool) ([]byte, error) {
	u, err := c.origin.URL(path)
	if err != nil {
		return nil, clientInputError()
	}
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), reader)
	if err != nil {
		return nil, clientInputError()
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, doErr := c.doer.Do(req)
	if doErr != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		if ctx.Err() != nil || errors.Is(doErr, context.Canceled) || errors.Is(doErr, context.DeadlineExceeded) {
			return nil, &OnboardingClientError{Kind: ClientErrorCancelled}
		}
		return nil, &OnboardingClientError{Kind: ClientErrorTransport}
	}
	if resp == nil || resp.Body == nil {
		return nil, invalidResponseError()
	}
	if resp.Request == nil || resp.Request.URL == nil || resp.Request.Method != method || resp.Request.URL.String() != u.String() {
		_ = resp.Body.Close()
		return nil, invalidResponseError()
	}
	raw, readErr := readBoundedResponse(resp.Body)
	closeErr := resp.Body.Close()
	if readErr != nil || closeErr != nil {
		zeroBytes(raw)
		return nil, invalidResponseError()
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		zeroBytes(raw)
		return nil, invalidResponseError()
	}
	if resp.StatusCode != expectedStatus && !strictErrors {
		zeroBytes(raw)
		return nil, &OnboardingClientError{Kind: kindForLegacyStatus(resp.StatusCode), Status: resp.StatusCode}
	}
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0]))
	if contentType != "application/json" {
		zeroBytes(raw)
		return nil, invalidResponseError()
	}
	if requireNoStore && !headerHasNoStore(resp.Header.Get("Cache-Control")) {
		zeroBytes(raw)
		return nil, invalidResponseError()
	}
	if resp.StatusCode != expectedStatus {
		decoded := decodeClientError(path, resp.StatusCode, resp.Header, raw)
		zeroBytes(raw)
		return nil, decoded
	}
	return raw, nil
}

func headerHasNoStore(value string) bool {
	for _, part := range strings.Split(value, ",") {
		if strings.EqualFold(strings.TrimSpace(part), "no-store") {
			return true
		}
	}
	return false
}

func decodeClientError(path string, status int, header http.Header, raw []byte) error {
	object, err := parseStrictJSONObject(raw)
	if err != nil || !exactObjectKeys(object, "error") {
		return invalidResponseError()
	}
	detail, ok := object["error"].(map[string]any)
	if !ok || !exactObjectKeys(detail, "code", "message", "retry_after_seconds") {
		return invalidResponseError()
	}
	code, okCode := jsonString(detail, "code")
	message, okMessage := jsonString(detail, "message")
	if !okCode || !okMessage {
		return invalidResponseError()
	}
	kind, expectedMessage, valid := expectedError(status, code)
	if !valid || message != expectedMessage || !endpointAllowsError(path, status, code) {
		return invalidResponseError()
	}
	retry := 0
	if status == http.StatusTooManyRequests {
		seconds, ok := jsonInt64(detail, "retry_after_seconds")
		headerSeconds, headerErr := strconv.Atoi(header.Get("Retry-After"))
		if !ok || seconds <= 0 || seconds > int64(^uint(0)>>1) || headerErr != nil || headerSeconds != int(seconds) {
			return invalidResponseError()
		}
		retry = int(seconds)
	} else if detail["retry_after_seconds"] != nil || header.Get("Retry-After") != "" {
		return invalidResponseError()
	}
	return &OnboardingClientError{Kind: kind, Status: status, Code: code, RetryAfterSeconds: retry}
}

func endpointAllowsError(path string, status int, code string) bool {
	if status == http.StatusInternalServerError && code == "internal_error" {
		return true
	}
	allowed := map[string]map[string]bool{
		"/v1/onboarding/orbits": {
			"400:invalid_request": true, "429:too_many_attempts": true,
		},
		"/v1/device-invites": {
			"400:invalid_request": true, "401:unauthorized": true, "403:insufficient_capability": true,
		},
		"/v1/device-invites/consume": {
			"400:invalid_request": true, "403:credential_invalid": true, "429:too_many_attempts": true,
		},
		"/v1/actor/context": {
			"400:invalid_request": true, "401:unauthorized": true, "403:insufficient_capability": true, "429:too_many_attempts": true,
		},
		"/v1/recovery/consume": {
			"400:invalid_request": true, "403:credential_invalid": true, "429:too_many_attempts": true,
		},
		"/v1/recovery/rotate": {
			"400:invalid_request": true, "401:unauthorized": true, "403:insufficient_capability": true, "429:too_many_attempts": true,
		},
		"/v1/telegram-links": {
			"400:invalid_request": true, "401:unauthorized": true, "403:insufficient_capability": true, "429:too_many_attempts": true,
		},
	}
	return allowed[path][strconv.Itoa(status)+":"+code]
}

func expectedError(status int, code string) (ClientErrorKind, string, bool) {
	entries := map[string]struct {
		status  int
		kind    ClientErrorKind
		message string
	}{
		"invalid_request":                {400, ClientErrorInvalidRequest, "The request is malformed or contains invalid parameters."},
		"unauthorized":                   {401, ClientErrorUnauthorized, "Authentication is required."},
		"insufficient_capability":        {403, ClientErrorCapability, "This token does not have the required capability."},
		"credential_invalid":             {403, ClientErrorCredential, "The provided credential is not valid."},
		"already_linked_same_orbit":      {409, ClientErrorConflict, "This Telegram account is already linked to this orbit."},
		"telegram_member_of_other_orbit": {409, ClientErrorConflict, "This Telegram account belongs to a different orbit."},
		"too_many_attempts":              {429, ClientErrorRateLimited, "Too many attempts. Please wait before retrying."},
		"internal_error":                 {500, ClientErrorServer, "An internal error occurred."},
	}
	entry, ok := entries[code]
	return entry.kind, entry.message, ok && entry.status == status
}

func (c *OnboardingClient) decodeCreate(raw []byte, expectedTitle string) (CreateOrbitResult, error) {
	object, err := parseStrictJSONObject(raw)
	if err != nil || !exactObjectKeys(object, "orbit_id", "title", "actor_id", "role", "slot", "node_token", "control_token", "recovery_id", "recovery_secret", "shown_once") {
		return CreateOrbitResult{}, invalidResponseError()
	}
	orbitID, okOrbit := jsonInt64(object, "orbit_id")
	title, okTitle := jsonString(object, "title")
	actorID, okActor := jsonInt64(object, "actor_id")
	role, okRole := jsonString(object, "role")
	slot, okSlot := jsonString(object, "slot")
	nodeToken, okNode := jsonString(object, "node_token")
	controlToken, okControl := jsonString(object, "control_token")
	recoveryID, okRecoveryID := jsonString(object, "recovery_id")
	recoverySecret, okRecoverySecret := jsonString(object, "recovery_secret")
	shownOnce, okShown := jsonBool(object, "shown_once")
	wsURL, wsErr := c.origin.WebSocketURL()
	if !okOrbit || !okTitle || !okActor || !okRole || !okSlot || !okNode || !okControl || !okRecoveryID || !okRecoverySecret || !okShown || !shownOnce || orbitID <= 0 || actorID <= 0 || role != "primary" || !validResponseTitle(title) || title != expectedTitle || wsErr != nil {
		return CreateOrbitResult{}, invalidResponseError()
	}
	bundle := CredentialBundle{Version: credentialBundleVersion, Node: &NodeCredential{OrbitID: orbitID, Slot: slot, NodeToken: nodeToken, WSURL: wsURL}, Control: &ControlCredential{ActorID: actorID, OrbitID: orbitID, Role: role, ControlToken: controlToken, Context: ControlContextActive}, RecoveryID: recoveryID, CoordinatorOrigin: c.origin.String()}
	if bundle.validate() != nil {
		return CreateOrbitResult{}, invalidResponseError()
	}
	recovery, err := newRecoveryMaterial(actorID, recoveryID, recoverySecret)
	if err != nil {
		return CreateOrbitResult{}, invalidResponseError()
	}
	return CreateOrbitResult{OrbitID: orbitID, Title: title, ActorID: actorID, Role: role, Bundle: bundle, Recovery: recovery}, nil
}

func (c *OnboardingClient) decodeJoin(raw []byte) (JoinOrbitResult, error) {
	object, err := parseStrictJSONObject(raw)
	if err != nil || !exactObjectKeys(object, "orbit_id", "title", "actor_id", "role", "slot", "node_token", "control_token") {
		return JoinOrbitResult{}, invalidResponseError()
	}
	orbitID, okOrbit := jsonInt64(object, "orbit_id")
	title, okTitle := jsonString(object, "title")
	actorID, okActor := jsonInt64(object, "actor_id")
	role, okRole := jsonString(object, "role")
	slot, okSlot := jsonString(object, "slot")
	nodeToken, okNode := jsonString(object, "node_token")
	controlToken, okControl := jsonString(object, "control_token")
	wsURL, wsErr := c.origin.WebSocketURL()
	if !okOrbit || !okTitle || !okActor || !okRole || !okSlot || !okNode || !okControl || orbitID <= 0 || actorID <= 0 || (role != "companion" && role != "satellite") || !validResponseTitle(title) || wsErr != nil {
		return JoinOrbitResult{}, invalidResponseError()
	}
	bundle := CredentialBundle{Version: credentialBundleVersion, Node: &NodeCredential{OrbitID: orbitID, Slot: slot, NodeToken: nodeToken, WSURL: wsURL}, Control: &ControlCredential{ActorID: actorID, OrbitID: orbitID, Role: role, ControlToken: controlToken, Context: ControlContextActive}, CoordinatorOrigin: c.origin.String()}
	if bundle.validate() != nil {
		return JoinOrbitResult{}, invalidResponseError()
	}
	return JoinOrbitResult{OrbitID: orbitID, Title: title, ActorID: actorID, Role: role, Bundle: bundle}, nil
}

func validResponseTitle(value string) bool {
	return value != "" && len(value) <= 120 && utf8.ValidString(value) && strings.TrimSpace(value) != ""
}

func validTelegramBotUsername(value string) bool {
	if len(value) < 5 || len(value) > 32 {
		return false
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		if (character < 'A' || character > 'Z') && (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '_' {
			return false
		}
	}
	return true
}

func decodeActorContext(raw []byte) (ActorContext, error) {
	object, err := parseStrictJSONObject(raw)
	if err != nil || !exactObjectKeys(object, "orbit_id", "actor_id", "role") {
		return ActorContext{}, invalidResponseError()
	}
	orbitID, okOrbit := jsonInt64(object, "orbit_id")
	actorID, okActor := jsonInt64(object, "actor_id")
	role, okRole := jsonString(object, "role")
	if !okOrbit || !okActor || !okRole || orbitID <= 0 || actorID <= 0 || !validRole(role) {
		return ActorContext{}, invalidResponseError()
	}
	return ActorContext{OrbitID: orbitID, ActorID: actorID, Role: role}, nil
}

func decodeRecoveryMaterial(raw []byte) (*RecoveryMaterial, error) {
	object, err := parseStrictJSONObject(raw)
	if err != nil || !exactObjectKeys(object, "actor_id", "recovery_id", "recovery_secret", "shown_once") {
		return nil, invalidResponseError()
	}
	actorID, okActor := jsonInt64(object, "actor_id")
	recoveryID, okID := jsonString(object, "recovery_id")
	secret, okSecret := jsonString(object, "recovery_secret")
	shown, okShown := jsonBool(object, "shown_once")
	if !okActor || !okID || !okSecret || !okShown || !shown {
		return nil, invalidResponseError()
	}
	material, err := newRecoveryMaterial(actorID, recoveryID, secret)
	if err != nil {
		return nil, invalidResponseError()
	}
	return material, nil
}

func clientInputError() error {
	return &OnboardingClientError{Kind: ClientErrorInvalidRequest, Status: 400, Code: "invalid_request"}
}
func invalidResponseError() error { return &OnboardingClientError{Kind: ClientErrorInvalidResponse} }
func kindForLegacyStatus(status int) ClientErrorKind {
	switch status {
	case http.StatusForbidden:
		return ClientErrorCredential
	case http.StatusConflict:
		return ClientErrorConflict
	case http.StatusTooManyRequests:
		return ClientErrorRateLimited
	default:
		return ClientErrorServer
	}
}
