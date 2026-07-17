package main

import (
	"context"
	"net/http"
	"sort"
	"strings"
)

type SoundboardCue struct {
	ID               string
	Title            string
	SourceKind       string
	MediaID          string
	BuiltinAssetID   string
	SourceSHA256     string
	SourceBytes      int64
	SourceDurationMS int64
	State            string
	Revision         int64
	SourceGeneration int64
	Position         int
}

type SoundboardCueList struct {
	OrderRevision int64
	Cues          []SoundboardCue
}

type SoundboardTriggerIntent struct {
	Route         PhaseOneRoute
	Delivery      PhaseOneDelivery
	IncludeOrigin bool
	Fallback      *PhaseOneFallbackConfirmation
}

type SoundboardTriggerReceipt struct {
	ExecutionID string
	PhaseOneTransmissionReceipt
}

type SoundboardAppService interface {
	SoundboardCues(context.Context) (SoundboardCueList, error)
	CreateSoundboardMediaCue(context.Context, string, string, string) (SoundboardCueList, error)
	RenameSoundboardCue(context.Context, string, string, int64, string) (SoundboardCueList, error)
	DeleteSoundboardCue(context.Context, string, int64, string) (SoundboardCueList, error)
	ReorderSoundboardCues(context.Context, []string, int64, string) (SoundboardCueList, error)
	TriggerSoundboardCue(context.Context, string, SoundboardTriggerIntent, string) (SoundboardTriggerReceipt, error)
}

type soundboardCueResponse struct {
	CueID            string `json:"cue_id"`
	Title            string `json:"title"`
	SourceKind       string `json:"source_kind"`
	MediaID          string `json:"media_id"`
	BuiltinAssetID   string `json:"builtin_asset_id"`
	SourceSHA256     string `json:"source_sha256"`
	SourceBytes      int64  `json:"source_bytes"`
	SourceDurationMS int64  `json:"source_duration_ms"`
	State            string `json:"state"`
	Revision         int64  `json:"revision"`
	SourceGeneration int64  `json:"source_generation"`
	Position         *int   `json:"position"`
}

type soundboardListResponse struct {
	OrderRevision int64                   `json:"order_revision"`
	Cues          []soundboardCueResponse `json:"cues"`
}

type soundboardMutationResponse struct {
	Cue           soundboardCueResponse `json:"cue"`
	OrderRevision int64                 `json:"order_revision"`
	Replayed      bool                  `json:"replayed"`
}

func decodeSoundboardCue(value soundboardCueResponse) (SoundboardCue, error) {
	validSource := value.SourceKind == "media" && validPhaseOnePublicID(value.MediaID, "m_") && value.BuiltinAssetID == "" ||
		value.SourceKind == "builtin" && value.MediaID == "" && value.BuiltinAssetID == "pulsar.recording-cue.v1"
	if !validPhaseOnePublicID(value.CueID, "cq_") || !validPhaseOneDisplayText(value.Title, 128, false) ||
		!validSource || len(value.SourceSHA256) != 64 || strings.Trim(value.SourceSHA256, "0123456789abcdef") != "" ||
		value.SourceBytes <= 0 || value.SourceDurationMS <= 0 || value.Revision <= 0 ||
		value.SourceGeneration <= 0 || value.State != "active" && value.State != "deleted" && value.State != "source_revoked" {
		return SoundboardCue{}, phaseOneError(PhaseOneInvalidResponse)
	}
	position := -1
	if value.Position != nil {
		position = *value.Position
		if position < 0 {
			return SoundboardCue{}, phaseOneError(PhaseOneInvalidResponse)
		}
	}
	return SoundboardCue{ID: value.CueID, Title: value.Title, SourceKind: value.SourceKind,
		MediaID: value.MediaID, BuiltinAssetID: value.BuiltinAssetID, SourceSHA256: value.SourceSHA256,
		SourceBytes: value.SourceBytes, SourceDurationMS: value.SourceDurationMS, State: value.State,
		Revision: value.Revision, SourceGeneration: value.SourceGeneration, Position: position}, nil
}

func decodeSoundboardList(raw []byte) (SoundboardCueList, error) {
	var response soundboardListResponse
	if decodePhaseOneJSON(raw, &response) != nil || response.OrderRevision < 0 || len(response.Cues) > 128 {
		return SoundboardCueList{}, phaseOneError(PhaseOneInvalidResponse)
	}
	result := SoundboardCueList{OrderRevision: response.OrderRevision}
	seen := map[string]bool{}
	for _, value := range response.Cues {
		cue, err := decodeSoundboardCue(value)
		if err != nil || seen[cue.ID] {
			return SoundboardCueList{}, phaseOneError(PhaseOneInvalidResponse)
		}
		seen[cue.ID] = true
		result.Cues = append(result.Cues, cue)
	}
	sort.SliceStable(result.Cues, func(i, j int) bool { return result.Cues[i].Position < result.Cues[j].Position })
	return result, nil
}

func (c *PhaseOneAppClient) SoundboardCues(ctx context.Context) (SoundboardCueList, error) {
	raw, _, err := c.request(ctx, http.MethodGet, "/v1/soundboard/cues", c.token, nil, nil, true, http.StatusOK)
	if err != nil {
		return SoundboardCueList{}, err
	}
	return decodeSoundboardList(raw)
}

func decodeSoundboardMutation(raw []byte) (SoundboardCueList, error) {
	var response soundboardMutationResponse
	if decodePhaseOneJSON(raw, &response) != nil || response.OrderRevision < 0 {
		return SoundboardCueList{}, phaseOneError(PhaseOneInvalidResponse)
	}
	cue, err := decodeSoundboardCue(response.Cue)
	if err != nil {
		return SoundboardCueList{}, err
	}
	return SoundboardCueList{OrderRevision: response.OrderRevision, Cues: []SoundboardCue{cue}}, nil
}

func (c *PhaseOneAppClient) CreateSoundboardMediaCue(ctx context.Context, title, mediaID, key string) (SoundboardCueList, error) {
	if !validPhaseOneDisplayText(title, 128, false) || !validPhaseOnePublicID(mediaID, "m_") || !validPhaseOneIdempotencyKey(key) {
		return SoundboardCueList{}, phaseOneError(PhaseOneInvalidRequest)
	}
	raw, _, err := c.requestJSON(ctx, http.MethodPost, "/v1/soundboard/cues", c.token,
		map[string]string{"Idempotency-Key": key}, map[string]any{"title": title,
			"source": map[string]any{"kind": "media", "media_id": mediaID}}, http.StatusCreated)
	if err != nil {
		return SoundboardCueList{}, err
	}
	return decodeSoundboardMutation(raw)
}

func (c *PhaseOneAppClient) RenameSoundboardCue(ctx context.Context, cueID, title string, revision int64, key string) (SoundboardCueList, error) {
	if !validPhaseOnePublicID(cueID, "cq_") || !validPhaseOneDisplayText(title, 128, false) || revision <= 0 || !validPhaseOneIdempotencyKey(key) {
		return SoundboardCueList{}, phaseOneError(PhaseOneInvalidRequest)
	}
	raw, _, err := c.requestJSON(ctx, http.MethodPatch, "/v1/soundboard/cues/"+cueID, c.token,
		map[string]string{"Idempotency-Key": key}, map[string]any{"title": title, "expected_revision": revision}, http.StatusOK)
	if err != nil {
		return SoundboardCueList{}, err
	}
	return decodeSoundboardMutation(raw)
}

func (c *PhaseOneAppClient) DeleteSoundboardCue(ctx context.Context, cueID string, revision int64, key string) (SoundboardCueList, error) {
	if !validPhaseOnePublicID(cueID, "cq_") || revision <= 0 || !validPhaseOneIdempotencyKey(key) {
		return SoundboardCueList{}, phaseOneError(PhaseOneInvalidRequest)
	}
	raw, _, err := c.requestJSON(ctx, http.MethodDelete, "/v1/soundboard/cues/"+cueID, c.token,
		map[string]string{"Idempotency-Key": key}, map[string]any{"expected_revision": revision}, http.StatusOK)
	if err != nil {
		return SoundboardCueList{}, err
	}
	return decodeSoundboardMutation(raw)
}

func (c *PhaseOneAppClient) ReorderSoundboardCues(ctx context.Context, cueIDs []string, revision int64, key string) (SoundboardCueList, error) {
	if revision <= 0 || len(cueIDs) > 128 || !validPhaseOneIdempotencyKey(key) {
		return SoundboardCueList{}, phaseOneError(PhaseOneInvalidRequest)
	}
	seen := map[string]bool{}
	for _, cueID := range cueIDs {
		if !validPhaseOnePublicID(cueID, "cq_") || seen[cueID] {
			return SoundboardCueList{}, phaseOneError(PhaseOneInvalidRequest)
		}
		seen[cueID] = true
	}
	_, _, err := c.requestJSON(ctx, http.MethodPut, "/v1/soundboard/cues/order", c.token,
		map[string]string{"Idempotency-Key": key}, map[string]any{"expected_order_revision": revision, "cue_ids": cueIDs}, http.StatusOK)
	if err != nil {
		return SoundboardCueList{}, err
	}
	return c.SoundboardCues(ctx)
}

func (c *PhaseOneAppClient) TriggerSoundboardCue(ctx context.Context, cueID string, intent SoundboardTriggerIntent, key string) (SoundboardTriggerReceipt, error) {
	if !validPhaseOnePublicID(cueID, "cq_") || !validPhaseOneRoute(intent.Route) ||
		!validPhaseOneDelivery(intent.Delivery) || !validPhaseOneIdempotencyKey(key) {
		return SoundboardTriggerReceipt{}, phaseOneError(PhaseOneInvalidRequest)
	}
	body := map[string]any{"audience": map[string]any{"kind": string(intent.Route)},
		"delivery": string(intent.Delivery), "include_origin": intent.IncludeOrigin}
	if intent.Fallback != nil {
		body["fallback_confirmation"] = phaseOneFallbackBody(intent.Fallback)
	}
	raw, _, err := c.requestJSON(ctx, http.MethodPost, "/v1/soundboard/cues/"+cueID+"/trigger", c.token,
		map[string]string{"Idempotency-Key": key}, body, http.StatusOK, http.StatusCreated)
	if err != nil {
		return SoundboardTriggerReceipt{}, err
	}
	var response struct {
		phaseOneTransmissionResponse
		ExecutionID string `json:"execution_id"`
	}
	if decodePhaseOneJSON(raw, &response) != nil || !validPhaseOnePublicID(response.ExecutionID, "mx_") {
		return SoundboardTriggerReceipt{}, phaseOneError(PhaseOneInvalidResponse)
	}
	receipt, err := decodePhaseOneTransmission(raw, "")
	if err != nil {
		return SoundboardTriggerReceipt{}, err
	}
	return SoundboardTriggerReceipt{ExecutionID: response.ExecutionID, PhaseOneTransmissionReceipt: receipt}, nil
}
