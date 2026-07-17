package main

// This file is a dormant audit model. It is deliberately not connected to
// main, wsclient, capability advertisement, storage, playback, or capture.

import (
	"bytes"
	"encoding/json"
	"strings"
)

const (
	e2eeAuditContract   = "e2ee-media-audit.v1"
	e2eeAuditCapability = "e2ee_media_v1"
)

type e2eeAuditMetadata struct {
	Contract                string `json:"contract"`
	Capability              string `json:"capability"`
	Suite                   string `json:"suite"`
	EventID                 string `json:"event_id"`
	GroupID                 string `json:"group_id"`
	ActorID                 string `json:"actor_id"`
	DeviceID                string `json:"device_id"`
	AirID                   string `json:"air_id"`
	TargetSnapshotDigest    string `json:"target_snapshot_digest"`
	ObjectKind              string `json:"object_kind"`
	ObjectID                string `json:"object_id"`
	Epoch                   uint64 `json:"epoch"`
	Generation              uint64 `json:"generation"`
	Sequence                uint64 `json:"sequence"`
	Nonce                   string `json:"nonce"`
	ExpiresAtMS             int64  `json:"expires_at_ms"`
	ManifestDigest          string `json:"manifest_digest"`
	AuthenticatedDataDigest string `json:"authenticated_data_digest"`
	Signature               string `json:"signature"`
	CiphertextURL           string `json:"ciphertext_url"`
}

type e2eeAuditVerifier func(string, string) bool

type e2eeAuditState struct {
	groupID, airID, targetDigest string
	commitDigest                 string
	epoch                        uint64
	seenEvents, seenNonces       map[string]bool
}

type e2eeAuditCommit struct {
	Contract                string `json:"contract"`
	Capability              string `json:"capability"`
	Suite                   string `json:"suite"`
	EventID                 string `json:"event_id"`
	GroupID                 string `json:"group_id"`
	ActorID                 string `json:"actor_id"`
	DeviceID                string `json:"device_id"`
	AirID                   string `json:"air_id"`
	PreviousEpoch           uint64 `json:"previous_epoch"`
	Epoch                   uint64 `json:"epoch"`
	PreviousCommitDigest    string `json:"previous_commit_digest"`
	CommitDigest            string `json:"commit_digest"`
	TargetSnapshotDigest    string `json:"target_snapshot_digest"`
	AuthenticatedDataDigest string `json:"authenticated_data_digest"`
	Signature               string `json:"signature"`
}

func decodeE2EEAuditMetadata(raw []byte) (e2eeAuditMetadata, string) {
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil {
		return e2eeAuditMetadata{}, "malformed"
	}
	for _, key := range []string{"plaintext", "content_key", "epoch_secret", "sender_key", "recovery_secret", "history_grant_secret", "private_key", "key_package_private_key"} {
		if _, ok := object[key]; ok {
			return e2eeAuditMetadata{}, "malformed"
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var value e2eeAuditMetadata
	if decoder.Decode(&value) != nil {
		return e2eeAuditMetadata{}, "malformed"
	}
	return value, ""
}

func (s *e2eeAuditState) accept(value e2eeAuditMetadata, trustedManifestDigest string, nowMS int64, suites map[string]bool, verify e2eeAuditVerifier) string {
	if value.Contract != e2eeAuditContract || value.Capability != e2eeAuditCapability {
		return "downgrade"
	}
	if !suites[value.Suite] {
		return "unknown_suite"
	}
	if value.EventID == "" || value.GroupID == "" || value.ActorID == "" || value.DeviceID == "" ||
		value.AirID == "" || value.ObjectID == "" || value.Nonce == "" || value.Generation == 0 ||
		value.Sequence == 0 || len(value.TargetSnapshotDigest) != 64 || len(value.ManifestDigest) != 64 ||
		len(value.AuthenticatedDataDigest) != 64 || !strings.HasPrefix(value.CiphertextURL, "/") {
		return "malformed"
	}
	switch value.ObjectKind {
	case "clip", "track", "saved_cue", "live_ptt":
	default:
		return "malformed"
	}
	if value.ManifestDigest != trustedManifestDigest {
		return "tampered_manifest"
	}
	if verify == nil || !verify(value.AuthenticatedDataDigest, value.Signature) {
		return "invalid_signature"
	}
	if value.GroupID != s.groupID || value.AirID != s.airID || value.TargetSnapshotDigest != s.targetDigest {
		return "foreign_target"
	}
	if value.Epoch < s.epoch {
		return "stale_epoch"
	}
	if value.Epoch > s.epoch {
		return "forked_epoch"
	}
	if s.seenEvents[value.EventID] {
		return "replay"
	}
	if s.seenNonces[value.Nonce] {
		return "nonce_reuse"
	}
	if value.ExpiresAtMS <= nowMS {
		return "expired_grant"
	}
	s.seenEvents[value.EventID] = true
	s.seenNonces[value.Nonce] = true
	return ""
}

func (s *e2eeAuditState) applyCommit(value e2eeAuditCommit, suites map[string]bool, verify e2eeAuditVerifier) string {
	if value.Contract != e2eeAuditContract || value.Capability != e2eeAuditCapability {
		return "downgrade"
	}
	if !suites[value.Suite] {
		return "unknown_suite"
	}
	if verify == nil || !verify(value.AuthenticatedDataDigest, value.Signature) {
		return "invalid_signature"
	}
	if value.GroupID != s.groupID || value.AirID != s.airID || value.TargetSnapshotDigest == "" {
		return "foreign_target"
	}
	if s.seenEvents[value.EventID] {
		return "replay"
	}
	if value.PreviousEpoch < s.epoch || value.Epoch <= s.epoch {
		return "stale_epoch"
	}
	if value.PreviousEpoch != s.epoch || value.Epoch != s.epoch+1 || value.PreviousCommitDigest != s.commitDigest {
		return "forked_epoch"
	}
	if value.ActorID == "" || value.DeviceID == "" || len(value.CommitDigest) != 64 {
		return "malformed"
	}
	s.epoch = value.Epoch
	s.commitDigest = value.CommitDigest
	s.targetDigest = value.TargetSnapshotDigest
	s.seenEvents[value.EventID] = true
	return ""
}

func newE2EEAuditState(groupID, airID, targetDigest string, epoch uint64, commitDigest string) *e2eeAuditState {
	return &e2eeAuditState{groupID: groupID, airID: airID, targetDigest: targetDigest, epoch: epoch, commitDigest: commitDigest,
		seenEvents: map[string]bool{}, seenNonces: map[string]bool{}}
}
