package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"relux.works/duet/coordinator/internal/protocol"
	"relux.works/duet/coordinator/internal/store"
)

const presencePolicyContract = "p1-history-presence-telegram-v1"
const presencePolicyRequestMaxBytes = int64(8 << 10)

type dndLayerJSON struct {
	Mode       store.DNDMode `json:"mode"`
	MutedUntil string        `json:"muted_until,omitempty"`
	Revision   int64         `json:"revision"`
}

type effectiveDNDJSON struct {
	Mode       store.DNDMode `json:"mode"`
	MutedUntil string        `json:"muted_until,omitempty"`
	Source     string        `json:"source"`
}

type presenceNodeJSON struct {
	OrbitID              int64            `json:"orbit_id"`
	Slot                 string           `json:"slot"`
	Online               bool             `json:"online"`
	LastSeenAt           string           `json:"last_seen_at,omitempty"`
	OutputState          string           `json:"output_state"`
	PlaybackState        string           `json:"playback_state"`
	LocalDND             dndLayerJSON     `json:"local_dnd"`
	EffectiveDND         effectiveDNDJSON `json:"effective_dnd"`
	Capabilities         []string         `json:"capabilities"`
	InterruptResumeReady bool             `json:"interrupt_resume_ready"`
}

func coordTime(value int64) string {
	if value <= 0 {
		return ""
	}
	return time.UnixMilli(value).UTC().Format("2006-01-02T15:04:05.000Z")
}

func dndLayer(setting store.DNDSetting, present bool) dndLayerJSON {
	if !present {
		return dndLayerJSON{Mode: store.DNDAllowAll}
	}
	result := dndLayerJSON{Mode: setting.Mode, Revision: setting.Revision}
	if setting.Mode == store.DNDMutedUntil {
		result.MutedUntil = coordTime(setting.MutedUntil)
	}
	return result
}

func effectiveDND(layer store.DNDLayers) effectiveDNDJSON {
	result := effectiveDNDJSON{Mode: layer.Effective.Mode, Source: "none"}
	localActive := layer.HasLocal && layer.Local.Mode != store.DNDAllowAll
	orbitActive := layer.HasOrbit && layer.Orbit.Mode != store.DNDAllowAll
	switch {
	case localActive && orbitActive:
		result.Source = "local_and_orbit"
	case layer.Effective.Reason == store.TransmissionReasonLocalDND:
		result.Source = "local"
	case layer.Effective.Reason == store.TransmissionReasonOrbitDND:
		result.Source = "orbit"
	}
	if layer.Effective.Mode == store.DNDMutedUntil {
		result.MutedUntil = coordTime(layer.Effective.MutedUntil)
	}
	return result
}

func presenceCapabilities(state transmissionPresenceState) []string {
	result := make([]string, 0, 3)
	if state.InterruptCapable {
		result = append(result, protocol.CapabilityInterruptResume)
	}
	if state.MediaClipCapable {
		result = append(result, protocol.CapabilityMediaClip)
	}
	if state.OverlayCapable {
		result = append(result, protocol.CapabilityOverlayMix)
	}
	sort.Strings(result)
	return result
}

func (api *onboardingAPI) presence(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		apiError(w, http.StatusMethodNotAllowed, errorInvalidRequest, 0)
		return
	}
	if r.URL.RawQuery != "" {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	actor := r.Context().Value(actorRequestKey{}).(actorRequest)
	domain, err := api.store.AuthorizedPresenceDomain(actor.Context.ActorID, actor.Bearer)
	if err != nil {
		api.presencePolicyError(w, "read presence domain", err)
		return
	}
	now := api.transmissionNow().UnixMilli()
	runtime := api.transmissionPresence()
	response := struct {
		Contract    string             `json:"contract"`
		Revision    int64              `json:"revision"`
		GeneratedAt string             `json:"generated_at"`
		OrbitDND    dndLayerJSON       `json:"orbit_dnd"`
		Nodes       []presenceNodeJSON `json:"nodes"`
	}{Contract: presencePolicyContract, Revision: now, GeneratedAt: coordTime(now),
		OrbitDND: dndLayerJSON{Mode: store.DNDAllowAll}, Nodes: []presenceNodeJSON{}}
	for _, target := range domain.Targets {
		layers, err := api.store.PresenceDNDLayers(store.MediaTargetIdentity{
			OrbitID: target.OrbitID, ActorID: target.ActorID, Slot: target.Slot,
		}, now)
		if err != nil {
			api.internalError(w, "read presence DND", err)
			return
		}
		if target.OrbitID == domain.CallerOrbitID && layers.HasOrbit {
			response.OrbitDND = dndLayer(layers.Orbit, true)
		}
		state := runtime[transmissionPresenceKey{OrbitID: target.OrbitID, Slot: target.Slot}]
		if state.CredentialTokenHash != target.NodeTokenHash {
			state = transmissionPresenceState{}
		}
		online := state.Connected && state.LastSeenAt > now-int64((12*time.Second)/time.Millisecond)
		output, playback := "unavailable", "unknown"
		if online {
			if state.MediaClipCapable && !state.OutputDegraded {
				output = "ready"
			} else {
				output = "degraded"
			}
			playback = state.PlaybackState
			if playback == "" {
				playback = "unknown"
			}
		}
		response.Nodes = append(response.Nodes, presenceNodeJSON{
			OrbitID: target.OrbitID, Slot: target.Slot, Online: online,
			LastSeenAt: coordTime(state.LastSeenAt), OutputState: output,
			PlaybackState: playback, LocalDND: dndLayer(layers.Local, layers.HasLocal),
			EffectiveDND: effectiveDND(layers), Capabilities: presenceCapabilities(state),
			InterruptResumeReady: online && state.InterruptResumeReady,
		})
	}
	writeJSON(w, http.StatusOK, response)
}

type dndMutationRequest struct {
	ExpectedRevision int64  `json:"expected_revision"`
	Mode             string `json:"mode"`
	MutedUntil       string `json:"muted_until,omitempty"`
}

type dndMutationResponse struct {
	Scope      string        `json:"scope"`
	Mode       store.DNDMode `json:"mode"`
	MutedUntil string        `json:"muted_until,omitempty"`
	Revision   int64         `json:"revision"`
	Changed    bool          `json:"changed"`
}

func canonicalPolicyRequest(value any) string {
	raw, _ := json.Marshal(value)
	return transmissionDigest(string(raw))
}

func (api *onboardingAPI) mutateDND(w http.ResponseWriter, r *http.Request, scope string) {
	if r.Method != http.MethodPut {
		w.Header().Set("Allow", http.MethodPut)
		apiError(w, http.StatusMethodNotAllowed, errorInvalidRequest, 0)
		return
	}
	if r.URL.RawQuery != "" {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	key, ok := singleRequestHeader(r, "Idempotency-Key")
	if !ok || !transmissionIdempotencyKey.MatchString(key) {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	var request dndMutationRequest
	if !decodeStrictJSON(w, r, presencePolicyRequestMaxBytes, &request) || request.ExpectedRevision < 0 {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	mode := store.DNDMode(request.Mode)
	mutedUntil := int64(0)
	if mode == store.DNDMutedUntil {
		parsed, err := time.Parse(time.RFC3339Nano, request.MutedUntil)
		if err != nil || request.MutedUntil != coordTime(parsed.UnixMilli()) {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		mutedUntil = parsed.UnixMilli()
	} else if request.MutedUntil != "" {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	actor := r.Context().Value(actorRequestKey{}).(actorRequest)
	now := api.transmissionNow().UnixMilli()
	mutation, err := api.store.AuthorizedSetDND(store.AuthorizedDNDMutationParams{
		ExpectedActorID: actor.Context.ActorID, Bearer: actor.Bearer, Layer: scope,
		Mode: mode, MutedUntil: mutedUntil, ExpectedRevision: request.ExpectedRevision,
		IdempotencyKeyHash: transmissionDigest(key), RequestHash: canonicalPolicyRequest(request), UpdatedAt: now,
	})
	if errors.Is(err, store.ErrDNDRevisionConflict) {
		layers, readErr := api.store.PresenceDNDLayers(store.MediaTargetIdentity{
			OrbitID: actor.Context.OrbitID, ActorID: actor.Context.ActorID, Slot: actor.Context.Slot,
		}, now)
		if readErr != nil {
			api.internalError(w, "read conflicting DND layer", readErr)
			return
		}
		current := dndLayer(layers.Local, layers.HasLocal)
		if scope == "orbit" {
			current = dndLayer(layers.Orbit, layers.HasOrbit)
		}
		writeJSON(w, http.StatusConflict, map[string]any{"error": map[string]any{
			"code":    errorDNDRevisionConflict,
			"message": "The DND layer changed; retry with its current revision.",
			"current": current,
		}})
		return
	}
	if err != nil {
		api.presencePolicyError(w, "mutate DND", err)
		return
	}
	response := dndMutationResponse{Scope: scope, Mode: mutation.Setting.Mode,
		Revision: mutation.Setting.Revision, Changed: mutation.Changed}
	if mutation.Setting.Mode == store.DNDMutedUntil {
		response.MutedUntil = coordTime(mutation.Setting.MutedUntil)
	}
	if mutation.Changed && mutation.Setting.Mode != store.DNDAllowAll {
		api.enforceDND(actor, scope, now)
	}
	writeJSON(w, http.StatusOK, response)
}

func (api *onboardingAPI) localDND(w http.ResponseWriter, r *http.Request) {
	api.mutateDND(w, r, "local")
}
func (api *onboardingAPI) orbitDND(w http.ResponseWriter, r *http.Request) {
	api.mutateDND(w, r, "orbit")
}

func (api *onboardingAPI) enforceDND(actor actorRequest, scope string, now int64) {
	if scope == "local" {
		results, err := api.store.CancelTransmissionNode(actor.Context.OrbitID, actor.Context.ActorID, actor.Context.Slot, store.TransmissionReasonDNDEnabled, now)
		if err != nil {
			api.log.Error("enforce local DND", "err", err)
			return
		}
		for _, result := range results {
			api.transmissionCancelled(result)
		}
		return
	}
	// Orbit DND is applied to every current installation in the owned orbit.
	domain, err := api.store.AuthorizedPresenceDomain(actor.Context.ActorID, actor.Bearer)
	if err != nil {
		api.log.Error("resolve orbit DND targets", "err", err)
		return
	}
	for _, target := range domain.Targets {
		if target.OrbitID != actor.Context.OrbitID {
			continue
		}
		results, err := api.store.CancelTransmissionNode(target.OrbitID, target.ActorID, target.Slot, store.TransmissionReasonDNDEnabled, now)
		if err != nil {
			api.log.Error("enforce orbit DND", "err", err)
			continue
		}
		for _, result := range results {
			api.transmissionCancelled(result)
		}
	}
}

type blockMutationRequest struct {
	Scope      string `json:"scope"`
	SubjectRef string `json:"subject_ref"`
}

type blockJSON struct {
	BlockID     string                `json:"block_id"`
	Scope       store.BlockOwnerScope `json:"scope,omitempty"`
	SubjectRef  string                `json:"subject_ref,omitempty"`
	DisplayName string                `json:"display_name,omitempty"`
	CreatedAt   string                `json:"created_at,omitempty"`
	Revision    int64                 `json:"revision,omitempty"`
	Reused      *bool                 `json:"reused,omitempty"`
	Changed     *bool                 `json:"changed,omitempty"`
}

func blockResponse(block store.PublicTransmissionBlock) blockJSON {
	return blockJSON{BlockID: block.ID, Scope: block.OwnerScope,
		SubjectRef: block.SubjectRef, DisplayName: block.DisplayName,
		CreatedAt: coordTime(block.CreatedAt), Revision: block.Revision}
}

func (api *onboardingAPI) blocks(w http.ResponseWriter, r *http.Request) {
	actor := r.Context().Value(actorRequestKey{}).(actorRequest)
	if r.URL.RawQuery != "" {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	switch r.Method {
	case http.MethodGet:
		blocks, err := api.store.AuthorizedListTransmissionBlocks(actor.Context.ActorID, actor.Bearer)
		if err != nil {
			api.presencePolicyError(w, "list blocks", err)
			return
		}
		rows := make([]blockJSON, 0, len(blocks))
		for _, block := range blocks {
			rows = append(rows, blockResponse(block))
		}
		writeJSON(w, http.StatusOK, map[string]any{"blocks": rows})
	case http.MethodPost:
		key, ok := singleRequestHeader(r, "Idempotency-Key")
		if !ok || !transmissionIdempotencyKey.MatchString(key) {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		var request blockMutationRequest
		if !decodeStrictJSON(w, r, presencePolicyRequestMaxBytes, &request) {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		scope := store.BlockOwnerScope(request.Scope)
		if (scope == store.BlockOwnerActor && !strings.HasPrefix(request.SubjectRef, "ar_")) ||
			(scope == store.BlockOwnerOrbit && !strings.HasPrefix(request.SubjectRef, "or_")) ||
			(scope != store.BlockOwnerActor && scope != store.BlockOwnerOrbit) {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		now := api.transmissionNow().UnixMilli()
		block, err := api.store.AuthorizedCreateTransmissionBlock(store.AuthorizedCreateBlockParams{
			ExpectedActorID: actor.Context.ActorID, Bearer: actor.Bearer, OwnerScope: scope,
			SubjectRef: request.SubjectRef, IdempotencyKeyHash: transmissionDigest(key),
			RequestHash: canonicalPolicyRequest(request), CreatedAt: now,
		})
		if err != nil {
			api.presencePolicyError(w, "create block", err)
			return
		}
		reused := block.Reused
		response := blockResponse(block)
		response.Reused = &reused
		if !reused {
			api.enforceBlock(actor, block, now)
		}
		status := http.StatusCreated
		if reused {
			status = http.StatusOK
		}
		writeJSON(w, status, response)
	default:
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodPost)
		apiError(w, http.StatusMethodNotAllowed, errorInvalidRequest, 0)
	}
}

func (api *onboardingAPI) blockItem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		w.Header().Set("Allow", http.MethodDelete)
		apiError(w, http.StatusMethodNotAllowed, errorInvalidRequest, 0)
		return
	}
	if r.URL.RawQuery != "" {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/v1/blocks/")
	if !strings.HasPrefix(id, "bl_") || strings.Contains(id, "/") {
		apiError(w, http.StatusNotFound, errorBlockNotFound, 0)
		return
	}
	actor := r.Context().Value(actorRequestKey{}).(actorRequest)
	now := api.transmissionNow().UnixMilli()
	block, changed, err := api.store.AuthorizedDeleteTransmissionBlock(actor.Context.ActorID, actor.Bearer, id, now)
	if err != nil {
		api.presencePolicyError(w, "delete block", err)
		return
	}
	writeJSON(w, http.StatusOK, blockJSON{BlockID: block.ID, Changed: &changed})
}

func (api *onboardingAPI) enforceBlock(actor actorRequest, block store.PublicTransmissionBlock, now int64) {
	domain, err := api.store.AuthorizedPresenceDomain(actor.Context.ActorID, actor.Bearer)
	if err != nil {
		api.log.Error("resolve block enforcement targets", "err", err)
		return
	}
	for _, target := range domain.Targets {
		if block.OwnerScope == store.BlockOwnerActor && target.ActorID != actor.Context.ActorID {
			continue
		}
		if block.OwnerScope == store.BlockOwnerOrbit && target.OrbitID != actor.Context.OrbitID {
			continue
		}
		var results []store.CancelTransmissionResult
		if block.SubjectKind == store.BlockedSubjectActor {
			results, err = api.store.CancelTransmissionsFromSourceActorToNode(block.Internal.BlockedActorID,
				target.OrbitID, target.ActorID, target.Slot, store.TransmissionReasonActorBlocked, now)
		} else {
			results, err = api.store.CancelTransmissionsFromSourceOrbitToNode(block.Internal.BlockedOrbitID,
				target.OrbitID, target.ActorID, target.Slot, store.TransmissionReasonOrbitBlocked, now)
		}
		if err != nil {
			api.log.Error("enforce transmission block", "err", err)
			continue
		}
		for _, result := range results {
			api.transmissionCancelled(result)
		}
	}
}

func (api *onboardingAPI) presencePolicyError(w http.ResponseWriter, operation string, err error) {
	switch {
	case errors.Is(err, store.ErrDNDRevisionConflict):
		apiError(w, http.StatusConflict, errorDNDRevisionConflict, 0)
	case errors.Is(err, store.ErrTransmissionPolicyIdempotency):
		apiError(w, http.StatusConflict, errorPolicyIdempotency, 0)
	case errors.Is(err, store.ErrTransmissionNotFound):
		code := errorBlockNotFound
		if strings.Contains(operation, "create block") {
			code = errorBlockSubjectNotFound
		}
		apiError(w, http.StatusNotFound, code, 0)
	case errors.Is(err, store.ErrTransmissionPolicyForbidden), errors.Is(err, store.ErrInsufficientCapability):
		apiError(w, http.StatusForbidden, errorInsufficientCapability, 0)
	case errors.Is(err, store.ErrTransmissionInvalid):
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
	default:
		api.internalError(w, operation, err)
	}
}
