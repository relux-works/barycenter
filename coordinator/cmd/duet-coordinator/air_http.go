package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"relux.works/duet/coordinator/internal/store"
)

var (
	airIDPattern       = regexp.MustCompile(`^air_[0-9A-HJKMNP-TV-Z]{26}$`)
	airInviteIDPattern = regexp.MustCompile(`^ai_[0-9A-HJKMNP-TV-Z]{26}$`)
	airMemberIDPattern = regexp.MustCompile(`^aim_[0-9A-HJKMNP-TV-Z]{26}$`)
)

type airCreateRequest struct {
	Title string `json:"title"`
}

type airInviteRequest struct {
	AirRole string `json:"air_role"`
}

type airInviteConsumeRequest struct {
	Code string `json:"code"`
}

type airRevisionRequest struct {
	MembershipRevision int64 `json:"membership_revision"`
}

type airConfirmRequest struct {
	MembershipRevision  int64  `json:"membership_revision"`
	Activate            bool   `json:"activate"`
	ExpectedActiveAirID string `json:"expected_active_air_id,omitempty"`
}

type airActivationRequest struct {
	MembershipRevision  int64  `json:"membership_revision"`
	ExpectedActiveAirID string `json:"expected_active_air_id"`
}

type airInviteWithdrawRequest struct {
	InviteRevision int64 `json:"invite_revision"`
}

type airRoleRequest struct {
	AirRevision        int64  `json:"air_revision"`
	MembershipRevision int64  `json:"membership_revision"`
	AirRole            string `json:"air_role"`
}

type airOwnershipRequest struct {
	AirRevision        int64  `json:"air_revision"`
	MembershipID       string `json:"membership_id"`
	MembershipRevision int64  `json:"membership_revision"`
}

type airPolicyRequest struct {
	PolicyRevision int64  `json:"policy_revision"`
	Invite         string `json:"invite"`
	Overlay        string `json:"overlay"`
	Queue          string `json:"queue"`
	Replace        string `json:"replace"`
}

type airDissolveRequest struct {
	AirRevision int64 `json:"air_revision"`
}

type airInviteHTTPResponse struct {
	InviteID  string `json:"invite_id"`
	Revision  int64  `json:"revision"`
	ExpiresAt string `json:"expires_at"`
	Code      string `json:"code,omitempty"`
}

type airSavedHTTPResponse struct {
	AirID             string                `json:"air_id"`
	Title             string                `json:"title"`
	Status            string                `json:"status"`
	MembershipStatus  string                `json:"membership_status"`
	AirRole           string                `json:"air_role"`
	MemberCount       int                   `json:"member_count"`
	ActiveMemberCount int                   `json:"active_member_count"`
	OnlinePulsarCount int                   `json:"online_pulsar_count"`
	Capacity          store.AirCapacityView `json:"capacity"`
	PolicyRevision    int64                 `json:"policy_revision"`
	IsCurrent         bool                  `json:"is_current"`
}

type airListHTTPResponse struct {
	CurrentAirID          string                 `json:"current_air_id,omitempty"`
	ActivePointerRevision int64                  `json:"active_pointer_revision"`
	Saved                 []airSavedHTTPResponse `json:"saved"`
}

func airListHTTP(view store.AirListView) airListHTTPResponse {
	response := airListHTTPResponse{
		CurrentAirID: view.CurrentAirID, ActivePointerRevision: view.ActivePointerRevision,
		Saved: make([]airSavedHTTPResponse, 0, len(view.Saved)),
	}
	for _, air := range view.Saved {
		response.Saved = append(response.Saved, airSavedHTTPResponse{
			AirID: air.AirID, Title: air.Title, Status: air.Status,
			MembershipStatus: air.MembershipStatus, AirRole: air.AirRole,
			MemberCount: air.MemberCount, ActiveMemberCount: air.ActiveMemberCount,
			OnlinePulsarCount: air.OnlinePulsarCount, Capacity: air.Capacity,
			PolicyRevision: air.Policy.Revision, IsCurrent: air.IsCurrent,
		})
	}
	return response
}

func airInviteHTTP(result store.AirInviteIssueResult) airInviteHTTPResponse {
	return airInviteHTTPResponse{
		InviteID: result.InviteID, Revision: result.Revision,
		ExpiresAt: time.UnixMilli(result.ExpiresAt).UTC().Format("2006-01-02T15:04:05.000Z"),
		Code:      result.Code,
	}
}

func hashAirHTTP(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func airMutationAuth(r *http.Request, actor actorRequest, request any, now time.Time) (store.AirMutationAuth, bool) {
	key, ok := singleRequestHeader(r, "Idempotency-Key")
	if !ok || !transmissionIdempotencyKey.MatchString(key) {
		return store.AirMutationAuth{}, false
	}
	canonical, err := json.Marshal(request)
	if err != nil {
		return store.AirMutationAuth{}, false
	}
	return store.AirMutationAuth{
		ExpectedActorID:    actor.Context.ActorID,
		Bearer:             actor.Bearer,
		IdempotencyKeyHash: hashAirHTTP(key),
		RequestHash:        hashAirHTTP(string(canonical)),
		Now:                now.UnixMilli(),
	}, true
}

func validExpectedAirID(value string) bool {
	return value == "none" || airIDPattern.MatchString(value)
}

func (api *onboardingAPI) withAirControl(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			apiError(w, http.StatusUnauthorized, errorUnauthenticated, 0)
			return
		}
		ctx, err := api.store.ResolveTokenActorContext(token)
		if errors.Is(err, store.ErrUnauthorized) {
			apiError(w, http.StatusUnauthorized, errorUnauthenticated, 0)
			return
		}
		if err != nil && !errors.Is(err, store.ErrInsufficientCapability) {
			api.internalError(w, "resolve Air control context", err)
			return
		}
		if !ctx.Capabilities.Has(store.CapabilityControl) {
			apiError(w, http.StatusForbidden, errorForbidden, 0)
			return
		}
		if api.testAfterAuth != nil {
			api.testAfterAuth(ctx)
		}
		request := actorRequest{Context: ctx, Bearer: token}
		next(w, r.WithContext(context.WithValue(r.Context(), actorRequestKey{}, request)))
	}
}

func (api *onboardingAPI) airsCollection(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/v1/airs" || r.URL.RawQuery != "" {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	actor := r.Context().Value(actorRequestKey{}).(actorRequest)
	switch r.Method {
	case http.MethodGet:
		view, err := api.store.AuthorizedAirList(actor.Context.ActorID, actor.Bearer)
		if api.airStoreError(w, "list Airs", err) {
			return
		}
		writeJSON(w, http.StatusOK, airListHTTP(view))
	case http.MethodPost:
		var request airCreateRequest
		if !decodeBoundedJSON(w, r, 1024, &request) || !utf8.ValidString(request.Title) {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		auth, ok := airMutationAuth(r, actor, request, api.airNow())
		if !ok {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		result, err := api.store.CreateAuthorizedAir(auth, request.Title)
		if api.airStoreError(w, "create Air", err) {
			return
		}
		writeJSON(w, http.StatusCreated, result)
	default:
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
	}
}

func (api *onboardingAPI) consumeAirInvite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/v1/air-invites/consume" || r.URL.RawQuery != "" {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	actor := r.Context().Value(actorRequestKey{}).(actorRequest)
	var request airInviteConsumeRequest
	if !decodeBoundedJSON(w, r, 1024, &request) {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	auth, ok := airMutationAuth(r, actor, request, api.airNow())
	if !ok {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	actorAllowed, actorRetry, actorReservation := api.airInviteConsumeActor.reserveReleasable(
		strconv.FormatInt(actor.Context.ActorID, 10))
	ipAllowed, ipRetry, ipReservation := api.airInviteConsumeIP.reserveReleasable(
		onboardingClientIP(r, api.config.TrustedProxy))
	if !actorAllowed || !ipAllowed {
		retry := actorRetry
		if ipRetry > retry {
			retry = ipRetry
		}
		apiError(w, http.StatusTooManyRequests, errorTooManyAttempts, retry)
		return
	}
	result, err := api.store.ConsumeAuthorizedAirInvite(auth, request.Code)
	if !errors.Is(err, store.ErrAirInviteUnavailable) {
		api.airInviteConsumeActor.release(actorReservation)
		api.airInviteConsumeIP.release(ipReservation)
	}
	if api.airStoreError(w, "consume Air invite", err) {
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}

func (api *onboardingAPI) airItem(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/airs/"), "/")
	if len(parts) == 0 || !airIDPattern.MatchString(parts[0]) {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	airID := parts[0]
	actor := r.Context().Value(actorRequestKey{}).(actorRequest)
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		result, err := api.store.AuthorizedAir(actor.Context.ActorID, actor.Bearer, airID)
		if api.airStoreError(w, "read Air", err) {
			return
		}
		writeJSON(w, http.StatusOK, result)
		return
	}
	if len(parts) == 2 {
		switch parts[1] {
		case "invites":
			api.issueAirInvite(w, r, actor, airID)
		case "activate":
			api.activateAir(w, r, actor, airID, false)
		case "deactivate":
			api.activateAir(w, r, actor, airID, true)
		case "leave":
			api.leaveAir(w, r, actor, airID)
		case "policy":
			api.replaceAirPolicy(w, r, actor, airID)
		case "dissolve":
			api.dissolveAir(w, r, actor, airID)
		default:
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		}
		return
	}
	if len(parts) == 3 && parts[1] == "join" {
		switch parts[2] {
		case "confirm":
			api.confirmAirJoin(w, r, actor, airID)
		case "decline":
			api.declineAirJoin(w, r, actor, airID)
		default:
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		}
		return
	}
	if len(parts) == 3 && parts[1] == "ownership" && parts[2] == "transfer" {
		api.transferAirOwnership(w, r, actor, airID)
		return
	}
	if len(parts) == 4 && parts[1] == "invites" && airInviteIDPattern.MatchString(parts[2]) && parts[3] == "withdraw" {
		api.withdrawAirInvite(w, r, actor, airID, parts[2])
		return
	}
	if len(parts) == 4 && parts[1] == "members" && airMemberIDPattern.MatchString(parts[2]) && parts[3] == "role" {
		api.replaceAirMemberRole(w, r, actor, airID, parts[2])
		return
	}
	apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
}

func (api *onboardingAPI) issueAirInvite(w http.ResponseWriter, r *http.Request, actor actorRequest, airID string) {
	if r.Method != http.MethodPost {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	var request airInviteRequest
	if !decodeBoundedJSON(w, r, 1024, &request) {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	auth, ok := airMutationAuth(r, actor, request, api.airNow())
	if !ok {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	result, err := api.store.IssueAuthorizedAirInvite(auth, airID, request.AirRole)
	if api.airStoreError(w, "issue Air invite", err) {
		return
	}
	writeJSON(w, http.StatusCreated, airInviteHTTP(result))
}

func (api *onboardingAPI) withdrawAirInvite(w http.ResponseWriter, r *http.Request, actor actorRequest, airID, inviteID string) {
	if r.Method != http.MethodPost {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	var request airInviteWithdrawRequest
	if !decodeBoundedJSON(w, r, 1024, &request) || request.InviteRevision <= 0 {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	auth, ok := airMutationAuth(r, actor, request, api.airNow())
	if !ok {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	result, err := api.store.WithdrawAuthorizedAirInvite(auth, airID, inviteID, request.InviteRevision)
	if api.airStoreError(w, "withdraw Air invite", err) {
		return
	}
	writeJSON(w, http.StatusOK, airInviteHTTP(result))
}

func (api *onboardingAPI) confirmAirJoin(w http.ResponseWriter, r *http.Request, actor actorRequest, airID string) {
	if r.Method != http.MethodPost {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	var request airConfirmRequest
	if !decodeBoundedJSON(w, r, 1024, &request) || request.MembershipRevision <= 0 ||
		(request.Activate && !validExpectedAirID(request.ExpectedActiveAirID)) ||
		(!request.Activate && request.ExpectedActiveAirID != "") {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	auth, ok := airMutationAuth(r, actor, request, api.airNow())
	if !ok {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	result, err := api.store.ConfirmAuthorizedAirJoin(auth, airID, request.MembershipRevision,
		request.Activate, request.ExpectedActiveAirID)
	if api.airStoreError(w, "confirm Air join", err) {
		return
	}
	if !api.acceptAirRuntimeChange(w) {
		return
	}
	writeJSON(w, http.StatusOK, result.Projection)
}

func (api *onboardingAPI) declineAirJoin(w http.ResponseWriter, r *http.Request, actor actorRequest, airID string) {
	if r.Method != http.MethodPost {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	var request airRevisionRequest
	if !decodeBoundedJSON(w, r, 1024, &request) || request.MembershipRevision <= 0 {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	auth, ok := airMutationAuth(r, actor, request, api.airNow())
	if !ok {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	result, err := api.store.DeclineAuthorizedAirJoin(auth, airID, request.MembershipRevision)
	if api.airStoreError(w, "decline Air join", err) {
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (api *onboardingAPI) activateAir(w http.ResponseWriter, r *http.Request, actor actorRequest, airID string, deactivate bool) {
	if r.Method != http.MethodPost {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	var request airActivationRequest
	if !decodeBoundedJSON(w, r, 1024, &request) || request.MembershipRevision <= 0 ||
		!validExpectedAirID(request.ExpectedActiveAirID) {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	auth, ok := airMutationAuth(r, actor, request, api.airNow())
	if !ok {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	var result store.AirLifecycleResult
	var err error
	if deactivate {
		result, err = api.store.DeactivateAuthorizedAir(auth, airID, request.MembershipRevision, request.ExpectedActiveAirID)
	} else {
		result, err = api.store.ActivateAuthorizedAir(auth, airID, request.MembershipRevision, request.ExpectedActiveAirID)
	}
	if api.airStoreError(w, "change active Air", err) {
		return
	}
	if !api.acceptAirRuntimeChange(w) {
		return
	}
	writeJSON(w, http.StatusOK, result.Projection)
}

func (api *onboardingAPI) leaveAir(w http.ResponseWriter, r *http.Request, actor actorRequest, airID string) {
	if r.Method != http.MethodPost {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	var request airActivationRequest
	if !decodeBoundedJSON(w, r, 1024, &request) || request.MembershipRevision <= 0 ||
		!validExpectedAirID(request.ExpectedActiveAirID) {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	auth, ok := airMutationAuth(r, actor, request, api.airNow())
	if !ok {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	result, err := api.store.LeaveAuthorizedAir(auth, airID, request.MembershipRevision, request.ExpectedActiveAirID)
	if api.airStoreError(w, "leave Air", err) {
		return
	}
	if !api.acceptAirRuntimeChange(w) {
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (api *onboardingAPI) replaceAirMemberRole(w http.ResponseWriter, r *http.Request, actor actorRequest, airID, membershipID string) {
	if r.Method != http.MethodPut {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	var request airRoleRequest
	if !decodeBoundedJSON(w, r, 1024, &request) || request.AirRevision <= 0 || request.MembershipRevision <= 0 {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	auth, ok := airMutationAuth(r, actor, request, api.airNow())
	if !ok {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	result, err := api.store.ReplaceAuthorizedAirMemberRole(auth, airID, membershipID,
		request.AirRevision, request.MembershipRevision, request.AirRole)
	if api.airStoreError(w, "replace Air member role", err) {
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (api *onboardingAPI) transferAirOwnership(w http.ResponseWriter, r *http.Request, actor actorRequest, airID string) {
	if r.Method != http.MethodPost {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	var request airOwnershipRequest
	if !decodeBoundedJSON(w, r, 1024, &request) || request.AirRevision <= 0 ||
		request.MembershipRevision <= 0 || !airMemberIDPattern.MatchString(request.MembershipID) {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	auth, ok := airMutationAuth(r, actor, request, api.airNow())
	if !ok {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	result, err := api.store.TransferAuthorizedAirOwnership(auth, airID, request.MembershipID,
		request.AirRevision, request.MembershipRevision)
	if api.airStoreError(w, "transfer Air ownership", err) {
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (api *onboardingAPI) replaceAirPolicy(w http.ResponseWriter, r *http.Request, actor actorRequest, airID string) {
	if r.Method != http.MethodPut {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	var request airPolicyRequest
	if !decodeBoundedJSON(w, r, 2048, &request) || request.PolicyRevision <= 0 {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	auth, ok := airMutationAuth(r, actor, request, api.airNow())
	if !ok {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	result, err := api.store.ReplaceAuthorizedAirPolicy(auth, airID, store.AirPolicyView{
		Revision: request.PolicyRevision, Invite: request.Invite, Overlay: request.Overlay,
		Queue: request.Queue, Replace: request.Replace,
	})
	if api.airStoreError(w, "replace Air policy", err) {
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (api *onboardingAPI) dissolveAir(w http.ResponseWriter, r *http.Request, actor actorRequest, airID string) {
	if r.Method != http.MethodPost {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	var request airDissolveRequest
	if !decodeBoundedJSON(w, r, 1024, &request) || request.AirRevision <= 0 {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	auth, ok := airMutationAuth(r, actor, request, api.airNow())
	if !ok {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	result, err := api.store.DissolveAuthorizedAir(auth, airID, request.AirRevision)
	if api.airStoreError(w, "dissolve Air", err) {
		return
	}
	if !api.acceptAirRuntimeChange(w) {
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (api *onboardingAPI) airStoreError(w http.ResponseWriter, operation string, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, store.ErrUnauthorized):
		apiError(w, http.StatusUnauthorized, errorUnauthenticated, 0)
	case errors.Is(err, store.ErrInsufficientCapability), errors.Is(err, store.ErrAirForbidden):
		apiError(w, http.StatusForbidden, errorForbidden, 0)
	case errors.Is(err, store.ErrAirPolicyDenied):
		apiError(w, http.StatusForbidden, errorAirPolicyDenied, 0)
	case errors.Is(err, store.ErrAirRateLimited):
		apiError(w, http.StatusTooManyRequests, errorTooManyAttempts, time.Hour)
	case errors.Is(err, store.ErrAirNotFound):
		apiError(w, http.StatusNotFound, errorAirNotFound, 0)
	case errors.Is(err, store.ErrAirMembershipNotFound), errors.Is(err, store.ErrAirNotJoined):
		apiError(w, http.StatusNotFound, errorAirMembershipNotFound, 0)
	case errors.Is(err, store.ErrAirInviteUnavailable):
		apiError(w, http.StatusNotFound, errorAirInviteUnavailable, 0)
	case errors.Is(err, store.ErrAirIdempotencyConflict):
		apiError(w, http.StatusConflict, errorAirIdempotency, 0)
	case errors.Is(err, store.ErrAirRevision):
		apiError(w, http.StatusConflict, errorAirRevision, 0)
	case errors.Is(err, store.ErrAirActiveChanged):
		apiError(w, http.StatusConflict, errorAirActiveChanged, 0)
	case errors.Is(err, store.ErrAirDissolved):
		apiError(w, http.StatusConflict, errorAirDissolved, 0)
	case errors.Is(err, store.ErrAirAlreadyMember):
		apiError(w, http.StatusConflict, errorAirAlreadyMember, 0)
	case errors.Is(err, store.ErrAirConfirmationRequired):
		apiError(w, http.StatusConflict, errorAirConfirmationRequired, 0)
	case errors.Is(err, store.ErrAirCapacity):
		apiError(w, http.StatusConflict, errorAirCapacity, 0)
	case errors.Is(err, store.ErrAirOwnerLeave):
		apiError(w, http.StatusConflict, errorAirOwnerTransfer, 0)
	case errors.Is(err, store.ErrAirInvalid):
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
	default:
		api.internalError(w, operation, err)
	}
	return true
}

func (api *onboardingAPI) acceptAirRuntimeChange(w http.ResponseWriter) bool {
	if err := api.airRuntimeChanged(); err != nil {
		api.log.Error("Air runtime did not accept committed lifecycle mutation", "err", err)
		apiError(w, http.StatusServiceUnavailable, errorServiceUnavailable, 0)
		return false
	}
	return true
}
