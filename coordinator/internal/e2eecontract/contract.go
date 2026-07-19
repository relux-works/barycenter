// Package e2eecontract is a dormant, audit-only model of the E2EE media
// routing and key-lifecycle contract. It deliberately contains no crypto and
// is not wired into the production protocol or capability advertisement.
package e2eecontract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	Contract   = "e2ee-media-audit.v1"
	Capability = "e2ee_media_v1"
)

type ErrorCode string

const (
	ErrDowngrade        ErrorCode = "downgrade"
	ErrExpiredGrant     ErrorCode = "expired_grant"
	ErrForeignTarget    ErrorCode = "foreign_target"
	ErrForkedEpoch      ErrorCode = "forked_epoch"
	ErrInvalidSignature ErrorCode = "invalid_signature"
	ErrNonceReuse       ErrorCode = "nonce_reuse"
	ErrReplay           ErrorCode = "replay"
	ErrStaleEpoch       ErrorCode = "stale_epoch"
	ErrTamperedManifest ErrorCode = "tampered_manifest"
	ErrUnknownSuite     ErrorCode = "unknown_suite"
	ErrMalformed        ErrorCode = "malformed"
)

type ValidationError struct{ Code ErrorCode }

func (e *ValidationError) Error() string { return string(e.Code) }

func fail(code ErrorCode) error { return &ValidationError{Code: code} }

func Code(err error) ErrorCode {
	if typed, ok := err.(*ValidationError); ok {
		return typed.Code
	}
	return ""
}

// Verifier is intentionally injected. Tests may model success and failure,
// but this package must never grow a home-made signature implementation.
type Verifier interface {
	Verify(authenticatedDataDigest, signature string) bool
}

type VerifierFunc func(authenticatedDataDigest, signature string) bool

func (f VerifierFunc) Verify(digest, signature string) bool { return f(digest, signature) }

type Config struct {
	AllowedSuites map[string]struct{}
	Verifier      Verifier
}

// ProductionConfig remains fail-closed until a reviewed library, suite and
// canonical serialization have been selected by the independent gate.
func ProductionConfig() Config {
	return Config{AllowedSuites: map[string]struct{}{}, Verifier: nil}
}

type RoutingMetadata struct {
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

var allowedKinds = map[string]struct{}{
	"clip": {}, "live_ptt": {}, "saved_cue": {}, "track": {},
}

func validateCommon(config Config, value RoutingMetadata) error {
	if value.Contract != Contract || value.Capability != Capability {
		return fail(ErrDowngrade)
	}
	if _, ok := config.AllowedSuites[value.Suite]; !ok {
		return fail(ErrUnknownSuite)
	}
	if value.EventID == "" || value.GroupID == "" || value.ActorID == "" ||
		value.DeviceID == "" || value.AirID == "" || value.ObjectID == "" ||
		value.Nonce == "" || value.Generation == 0 || value.Sequence == 0 ||
		len(value.TargetSnapshotDigest) != 64 || len(value.ManifestDigest) != 64 ||
		len(value.AuthenticatedDataDigest) != 64 || !strings.HasPrefix(value.CiphertextURL, "/") {
		return fail(ErrMalformed)
	}
	if _, ok := allowedKinds[value.ObjectKind]; !ok {
		return fail(ErrMalformed)
	}
	if config.Verifier == nil || !config.Verifier.Verify(value.AuthenticatedDataDigest, value.Signature) {
		return fail(ErrInvalidSignature)
	}
	return nil
}

// DecodeCoordinatorMetadata strictly limits the coordinator-visible envelope
// and rejects secret-bearing or additive fields before routing.
func DecodeCoordinatorMetadata(raw []byte) (RoutingMetadata, error) {
	if err := rejectCoordinatorForbiddenFields(raw); err != nil {
		return RoutingMetadata{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var value RoutingMetadata
	if err := decoder.Decode(&value); err != nil {
		return RoutingMetadata{}, fail(ErrMalformed)
	}
	return value, nil
}

var coordinatorForbiddenFields = []string{
	"plaintext", "content_key", "epoch_secret", "sender_key", "recovery_secret",
	"history_grant_secret", "private_key", "key_package_private_key",
}

func rejectCoordinatorForbiddenFields(raw []byte) error {
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		return fail(ErrMalformed)
	}
	for _, key := range coordinatorForbiddenFields {
		if _, ok := keys[key]; ok {
			return fail(ErrMalformed)
		}
	}
	return nil
}

func decodeCoordinatorEnvelope(raw []byte, destination any) error {
	if err := rejectCoordinatorForbiddenFields(raw); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fail(ErrMalformed)
	}
	return nil
}

type State struct {
	GroupID              string
	AirID                string
	TargetSnapshotDigest string
	Epoch                uint64
	CommitDigest         string
	seenEvents           map[string]struct{}
	seenNonces           map[string]struct{}
	lastSequences        map[string]uint64
}

func NewState(groupID, airID, targetDigest string, epoch uint64, commitDigest string) *State {
	return &State{
		GroupID: groupID, AirID: airID, TargetSnapshotDigest: targetDigest,
		Epoch: epoch, CommitDigest: commitDigest, seenEvents: map[string]struct{}{},
		seenNonces: map[string]struct{}{}, lastSequences: map[string]uint64{},
	}
}

func (s *State) RememberEvent(eventID string) { s.seenEvents[eventID] = struct{}{} }
func (s *State) RememberNonce(nonce string)   { s.seenNonces[nonce] = struct{}{} }
func (s *State) RememberSequence(deviceID, objectID string, generation, sequence uint64) {
	s.lastSequences[fmt.Sprintf("%s/%s/%d", deviceID, objectID, generation)] = sequence
}

// AcceptContent validates state before any plaintext release. Callers must
// persist the updated replay state atomically with accepting the ciphertext.
func (s *State) AcceptContent(config Config, value RoutingMetadata, trustedManifestDigest string, nowMS int64) error {
	if err := validateCommon(config, value); err != nil {
		return err
	}
	if value.ManifestDigest != trustedManifestDigest {
		return fail(ErrTamperedManifest)
	}
	if value.GroupID != s.GroupID || value.AirID != s.AirID ||
		value.TargetSnapshotDigest != s.TargetSnapshotDigest {
		return fail(ErrForeignTarget)
	}
	if value.Epoch < s.Epoch {
		return fail(ErrStaleEpoch)
	}
	if value.Epoch > s.Epoch {
		return fail(ErrForkedEpoch)
	}
	if _, ok := s.seenEvents[value.EventID]; ok {
		return fail(ErrReplay)
	}
	if _, ok := s.seenNonces[value.Nonce]; ok {
		return fail(ErrNonceReuse)
	}
	if value.ExpiresAtMS <= nowMS {
		return fail(ErrExpiredGrant)
	}
	sequenceKey := fmt.Sprintf("%s/%s/%d", value.DeviceID, value.ObjectID, value.Generation)
	if previous := s.lastSequences[sequenceKey]; value.Sequence <= previous {
		return fail(ErrReplay)
	}
	if value.Generation > 1 && value.Sequence != 1 {
		previousGeneration := fmt.Sprintf("%s/%s/%d", value.DeviceID, value.ObjectID, value.Generation-1)
		if _, ok := s.lastSequences[previousGeneration]; ok {
			return fail(ErrReplay)
		}
	}
	s.seenEvents[value.EventID] = struct{}{}
	s.seenNonces[value.Nonce] = struct{}{}
	s.lastSequences[sequenceKey] = value.Sequence
	return nil
}

type Commit struct {
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

func DecodeCoordinatorCommit(raw []byte) (Commit, error) {
	var value Commit
	if err := decodeCoordinatorEnvelope(raw, &value); err != nil {
		return Commit{}, err
	}
	return value, nil
}

type Proposal struct {
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
	TargetSnapshotDigest    string `json:"target_snapshot_digest"`
	ProposalDigest          string `json:"proposal_digest"`
	AuthenticatedDataDigest string `json:"authenticated_data_digest"`
	Signature               string `json:"signature"`
}

func DecodeCoordinatorProposal(raw []byte) (Proposal, error) {
	var value Proposal
	if err := decodeCoordinatorEnvelope(raw, &value); err != nil {
		return Proposal{}, err
	}
	return value, nil
}

// ValidateProposal applies the same fail-closed envelope, suite, signature,
// target, replay and epoch ordering as ApplyCommit without mutating group
// state. Membership authorization and durable serialization remain the
// coordinator store's responsibility because they depend on current actor,
// device and Air rows.
func (s *State) ValidateProposal(config Config, value Proposal) error {
	if value.Contract != Contract || value.Capability != Capability {
		return fail(ErrDowngrade)
	}
	if _, ok := config.AllowedSuites[value.Suite]; !ok {
		return fail(ErrUnknownSuite)
	}
	if value.EventID == "" || value.ActorID == "" || value.DeviceID == "" ||
		len(value.ProposalDigest) != 64 || len(value.TargetSnapshotDigest) != 64 ||
		len(value.AuthenticatedDataDigest) != 64 {
		return fail(ErrMalformed)
	}
	if config.Verifier == nil || !config.Verifier.Verify(value.AuthenticatedDataDigest, value.Signature) {
		return fail(ErrInvalidSignature)
	}
	if value.GroupID != s.GroupID || value.AirID != s.AirID {
		return fail(ErrForeignTarget)
	}
	if value.PreviousEpoch < s.Epoch || value.Epoch <= s.Epoch {
		return fail(ErrStaleEpoch)
	}
	if value.PreviousEpoch != s.Epoch || value.Epoch != s.Epoch+1 {
		return fail(ErrForkedEpoch)
	}
	if _, ok := s.seenEvents[value.EventID]; ok {
		return fail(ErrReplay)
	}
	return nil
}

type Welcome struct {
	Contract                string `json:"contract"`
	Capability              string `json:"capability"`
	Suite                   string `json:"suite"`
	EventID                 string `json:"event_id"`
	GroupID                 string `json:"group_id"`
	ActorID                 string `json:"actor_id"`
	DeviceID                string `json:"device_id"`
	RecipientDeviceID       string `json:"recipient_device_id"`
	AirID                   string `json:"air_id"`
	Epoch                   uint64 `json:"epoch"`
	TargetSnapshotDigest    string `json:"target_snapshot_digest"`
	WelcomeDigest           string `json:"welcome_digest"`
	ExpiresAtMS             int64  `json:"expires_at_ms"`
	CiphertextURL           string `json:"ciphertext_url"`
	AuthenticatedDataDigest string `json:"authenticated_data_digest"`
	Signature               string `json:"signature"`
}

func DecodeCoordinatorWelcome(raw []byte) (Welcome, error) {
	var value Welcome
	if err := decodeCoordinatorEnvelope(raw, &value); err != nil {
		return Welcome{}, err
	}
	return value, nil
}

type KeyPackage struct {
	Contract                string `json:"contract"`
	Capability              string `json:"capability"`
	Suite                   string `json:"suite"`
	EventID                 string `json:"event_id"`
	ActorID                 string `json:"actor_id"`
	DeviceID                string `json:"device_id"`
	KeyPackageDigest        string `json:"key_package_digest"`
	PublicPackageURL        string `json:"public_package_url"`
	ExpiresAtMS             int64  `json:"expires_at_ms"`
	AuthenticatedDataDigest string `json:"authenticated_data_digest"`
	Signature               string `json:"signature"`
}

func DecodeCoordinatorKeyPackage(raw []byte) (KeyPackage, error) {
	var value KeyPackage
	if err := decodeCoordinatorEnvelope(raw, &value); err != nil {
		return KeyPackage{}, err
	}
	return value, nil
}

type HistoryGrant struct {
	Contract                string `json:"contract"`
	Capability              string `json:"capability"`
	Suite                   string `json:"suite"`
	EventID                 string `json:"event_id"`
	GroupID                 string `json:"group_id"`
	ActorID                 string `json:"actor_id"`
	DeviceID                string `json:"device_id"`
	RecipientDeviceID       string `json:"recipient_device_id"`
	AirID                   string `json:"air_id"`
	ObjectID                string `json:"object_id"`
	FirstEpoch              uint64 `json:"first_epoch"`
	LastEpoch               uint64 `json:"last_epoch"`
	TargetSnapshotDigest    string `json:"target_snapshot_digest"`
	GrantDigest             string `json:"grant_digest"`
	ExpiresAtMS             int64  `json:"expires_at_ms"`
	CiphertextURL           string `json:"ciphertext_url"`
	AuthenticatedDataDigest string `json:"authenticated_data_digest"`
	Signature               string `json:"signature"`
}

func DecodeCoordinatorHistoryGrant(raw []byte) (HistoryGrant, error) {
	var value HistoryGrant
	if err := decodeCoordinatorEnvelope(raw, &value); err != nil {
		return HistoryGrant{}, err
	}
	return value, nil
}

func (s *State) ApplyCommit(config Config, value Commit) error {
	if value.Contract != Contract || value.Capability != Capability {
		return fail(ErrDowngrade)
	}
	if _, ok := config.AllowedSuites[value.Suite]; !ok {
		return fail(ErrUnknownSuite)
	}
	if config.Verifier == nil || !config.Verifier.Verify(value.AuthenticatedDataDigest, value.Signature) {
		return fail(ErrInvalidSignature)
	}
	if value.GroupID != s.GroupID || value.AirID != s.AirID || value.TargetSnapshotDigest == "" {
		return fail(ErrForeignTarget)
	}
	if _, ok := s.seenEvents[value.EventID]; ok {
		return fail(ErrReplay)
	}
	if value.PreviousEpoch < s.Epoch || value.Epoch <= s.Epoch {
		return fail(ErrStaleEpoch)
	}
	if value.PreviousEpoch != s.Epoch || value.Epoch != s.Epoch+1 ||
		value.PreviousCommitDigest != s.CommitDigest {
		return fail(ErrForkedEpoch)
	}
	if value.ActorID == "" || value.DeviceID == "" || len(value.CommitDigest) != 64 {
		return fail(ErrMalformed)
	}
	s.Epoch = value.Epoch
	s.CommitDigest = value.CommitDigest
	s.TargetSnapshotDigest = value.TargetSnapshotDigest
	s.seenEvents[value.EventID] = struct{}{}
	return nil
}
