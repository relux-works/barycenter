package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

type PhaseOneDraftState string

const (
	PhaseOneDraftRetained         PhaseOneDraftState = "retained"
	PhaseOneDraftUploading        PhaseOneDraftState = "uploading"
	PhaseOneDraftUploaded         PhaseOneDraftState = "uploaded"
	PhaseOneDraftTransmitting     PhaseOneDraftState = "transmitting"
	PhaseOneDraftAccepted         PhaseOneDraftState = "accepted"
	PhaseOneDraftRetryableFailure PhaseOneDraftState = "retryable_failure"
)

type PhaseOneDraftSnapshot struct {
	DraftID                       string
	Title                         string
	OriginKind                    PhaseOneOriginKind
	State                         PhaseOneDraftState
	Route                         PhaseOneRoute
	RequestedDelivery             PhaseOneDelivery
	EffectiveDelivery             PhaseOneDelivery
	DowngradeReason               string
	Status                        string
	FailureCode                   string
	LocalBytesRetained            bool
	FallbackConfirmationAvailable bool
}

var (
	ErrPhaseOneInvalidDraft = errors.New("phase_one_invalid_draft")
	ErrPhaseOneDraftBusy    = errors.New("phase_one_draft_busy")
	ErrPhaseOnePersistence  = errors.New("phase_one_persistence")
	ErrPhaseOneLocalCleanup = errors.New("phase_one_local_cleanup")
	ErrPhaseOneRemoteDelete = errors.New("phase_one_remote_delete")
)

type phaseOneDraftRecord struct {
	DraftID            string             `json:"draft_id"`
	Title              string             `json:"title"`
	OriginKind         PhaseOneOriginKind `json:"origin_kind"`
	UploadKey          string             `json:"upload_key"`
	TransmissionKey    string             `json:"transmission_key"`
	State              PhaseOneDraftState `json:"state"`
	Route              PhaseOneRoute      `json:"route,omitempty"`
	RequestedDelivery  PhaseOneDelivery   `json:"requested_delivery,omitempty"`
	MediaID            string             `json:"media_id,omitempty"`
	TransmissionID     string             `json:"transmission_id,omitempty"`
	EffectiveDelivery  PhaseOneDelivery   `json:"effective_delivery,omitempty"`
	DowngradeReason    string             `json:"downgrade_reason,omitempty"`
	Status             string             `json:"status,omitempty"`
	FailureCode        string             `json:"failure_code,omitempty"`
	LocalBytesRetained bool               `json:"local_bytes_retained"`
}

type phaseOneDraftEnvelope struct {
	Version int                   `json:"version"`
	Records []phaseOneDraftRecord `json:"records"`
}

// PhaseOneDraftOutbox is the durable boundary between finalized user media and
// the coordinator. Its metadata contains no file path or credential. Network
// work is serialized per draft; every retry reuses the original upload and
// transmission keys and the first selected intent.
type PhaseOneDraftOutbox struct {
	mu         sync.Mutex
	service    PhaseOneAppService
	store      *CaptureMediaStore
	statePath  string
	records    map[string]phaseOneDraftRecord
	handles    map[string]CaptureMediaHandle
	active     map[string]bool
	challenges map[string]PhaseOneFallbackConfirmation
}

func NewPhaseOneDraftOutbox(service PhaseOneAppService, store *CaptureMediaStore, statePath string, recoveredDrafts []CaptureMediaHandle) (*PhaseOneDraftOutbox, error) {
	if service == nil || store == nil || statePath == "" {
		return nil, ErrPhaseOnePersistence
	}
	records, err := loadPhaseOneDraftRecords(statePath)
	if err != nil {
		return nil, err
	}
	outbox := &PhaseOneDraftOutbox{
		service: service, store: store, statePath: filepath.Clean(statePath),
		records: map[string]phaseOneDraftRecord{}, handles: map[string]CaptureMediaHandle{}, active: map[string]bool{}, challenges: map[string]PhaseOneFallbackConfirmation{},
	}
	for _, record := range records {
		outbox.records[record.DraftID] = record
	}
	for _, handle := range recoveredDrafts {
		if handle.Class != CaptureUserRecording || handle.State != CaptureDurableUnsent || !isCaptureMediaID(handle.ID) {
			continue
		}
		outbox.handles[handle.ID] = handle
		record, ok := outbox.records[handle.ID]
		if !ok {
			record = newPhaseOneDraftRecord(handle.ID, "Pulsar recording", PhaseOneMicrophone)
		}
		record.LocalBytesRetained = true
		outbox.records[handle.ID] = record
	}
	for id, record := range outbox.records {
		if _, ok := outbox.handles[id]; !ok {
			if record.MediaID == "" {
				delete(outbox.records, id)
			} else {
				record.LocalBytesRetained = false
				outbox.records[id] = record
			}
		}
	}
	if err := outbox.persistLocked(); err != nil {
		return nil, err
	}
	return outbox, nil
}

func (o *PhaseOneDraftOutbox) Attach(handle CaptureMediaHandle, title string, originKind PhaseOneOriginKind) error {
	if o == nil || handle.Class != CaptureUserRecording || handle.State != CaptureDurableUnsent || !isCaptureMediaID(handle.ID) ||
		(originKind != PhaseOneMicrophone && originKind != PhaseOneFile) {
		return ErrPhaseOneInvalidDraft
	}
	if title == "" {
		title = "Pulsar recording"
	}
	if len([]byte(title)) > 512 {
		return ErrPhaseOneInvalidDraft
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.handles[handle.ID] = handle
	record, ok := o.records[handle.ID]
	if !ok {
		record = newPhaseOneDraftRecord(handle.ID, title, originKind)
	}
	// Recovery republishes durable handles without source provenance. Once the
	// outbox has frozen a source, attaching that same handle must preserve it.
	record.LocalBytesRetained = true
	o.records[handle.ID] = record
	return o.persistLocked()
}

func (o *PhaseOneDraftOutbox) Snapshots() []PhaseOneDraftSnapshot {
	if o == nil {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	ids := make([]string, 0, len(o.records))
	for id := range o.records {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	result := make([]PhaseOneDraftSnapshot, 0, len(ids))
	for _, id := range ids {
		snapshot := snapshotPhaseOneDraft(o.records[id])
		_, snapshot.FallbackConfirmationAvailable = o.challenges[id]
		result = append(result, snapshot)
	}
	return result
}

func (o *PhaseOneDraftOutbox) Send(ctx context.Context, draftID string, route PhaseOneRoute, delivery PhaseOneDelivery, originKind PhaseOneOriginKind) (PhaseOneDraftSnapshot, error) {
	if o == nil || !validPhaseOneRoute(route) || !validPhaseOneDelivery(delivery) ||
		(originKind != PhaseOneMicrophone && originKind != PhaseOneFile) {
		return PhaseOneDraftSnapshot{}, ErrPhaseOneInvalidDraft
	}
	o.mu.Lock()
	record, ok := o.records[draftID]
	if !ok || record.Route != "" && record.Route != route || record.RequestedDelivery != "" && record.RequestedDelivery != delivery {
		o.mu.Unlock()
		return PhaseOneDraftSnapshot{}, ErrPhaseOneInvalidDraft
	}
	if o.active[draftID] {
		o.mu.Unlock()
		return PhaseOneDraftSnapshot{}, ErrPhaseOneDraftBusy
	}
	if record.OriginKind != originKind {
		o.mu.Unlock()
		return PhaseOneDraftSnapshot{}, ErrPhaseOneInvalidDraft
	}
	o.active[draftID] = true
	record.Route, record.RequestedDelivery, record.FailureCode = route, delivery, ""
	o.records[draftID] = record
	if err := o.persistLocked(); err != nil {
		delete(o.active, draftID)
		o.mu.Unlock()
		return PhaseOneDraftSnapshot{}, err
	}
	o.mu.Unlock()
	defer func() {
		o.mu.Lock()
		delete(o.active, draftID)
		o.mu.Unlock()
	}()

	if record.MediaID == "" {
		o.mu.Lock()
		handle, exists := o.handles[draftID]
		if !exists {
			o.mu.Unlock()
			return o.fail(draftID, record, "invalid_draft", ErrPhaseOneInvalidDraft)
		}
		record.State, record.Status = PhaseOneDraftUploading, "uploading"
		o.records[draftID] = record
		if err := o.persistLocked(); err != nil {
			o.mu.Unlock()
			return PhaseOneDraftSnapshot{}, err
		}
		o.mu.Unlock()

		confirmation, err := o.service.Upload(ctx, handle.Path, record.Title, record.UploadKey)
		if err != nil {
			return o.fail(draftID, record, phaseOneFailureCode(err), err)
		}
		record.MediaID, record.State, record.Status = confirmation.MediaID, PhaseOneDraftUploaded, "upload_confirmed"
		o.mu.Lock()
		o.records[draftID] = record
		if err := o.persistLocked(); err != nil {
			o.mu.Unlock()
			return PhaseOneDraftSnapshot{}, err
		}
		o.mu.Unlock()

		if err := o.store.ConfirmUploadAndDelete(handle); err != nil {
			return o.fail(draftID, record, "local_cleanup_failed", ErrPhaseOneLocalCleanup)
		}
		record.LocalBytesRetained = false
		o.mu.Lock()
		delete(o.handles, draftID)
		o.records[draftID] = record
		if err := o.persistLocked(); err != nil {
			o.mu.Unlock()
			return PhaseOneDraftSnapshot{}, err
		}
		o.mu.Unlock()
	}

	// A process may restart after persisting the server media ID but before a
	// successful local delete. Retry cleanup first and never perform a second
	// upload; transmission is not allowed to hide retained confirmed bytes.
	o.mu.Lock()
	staleHandle, hasStaleHandle := o.handles[draftID]
	o.mu.Unlock()
	if hasStaleHandle && record.LocalBytesRetained {
		if err := o.store.ConfirmUploadAndDelete(staleHandle); err != nil {
			return o.fail(draftID, record, "local_cleanup_failed", ErrPhaseOneLocalCleanup)
		}
		record.LocalBytesRetained = false
		o.mu.Lock()
		delete(o.handles, draftID)
		o.records[draftID] = record
		if err := o.persistLocked(); err != nil {
			o.mu.Unlock()
			return PhaseOneDraftSnapshot{}, err
		}
		o.mu.Unlock()
	}

	record.State, record.Status = PhaseOneDraftTransmitting, "requesting_acceptance"
	o.mu.Lock()
	o.records[draftID] = record
	if err := o.persistLocked(); err != nil {
		o.mu.Unlock()
		return PhaseOneDraftSnapshot{}, err
	}
	o.mu.Unlock()
	receipt, err := o.service.Transmit(ctx, record.MediaID, route, delivery, record.OriginKind, record.TransmissionKey, nil)
	if err != nil {
		o.rememberChallenge(draftID, err)
		return o.fail(draftID, record, phaseOneFailureCode(err), err)
	}
	record.TransmissionID, record.EffectiveDelivery = receipt.TransmissionID, receipt.EffectiveDelivery
	record.DowngradeReason, record.Status, record.State, record.FailureCode = receipt.DowngradeReason, receipt.Status, PhaseOneDraftAccepted, ""
	o.mu.Lock()
	o.records[draftID] = record
	err = o.persistLocked()
	snapshot := snapshotPhaseOneDraft(record)
	o.mu.Unlock()
	return snapshot, err
}

func (o *PhaseOneDraftOutbox) ConfirmFallback(ctx context.Context, draftID string, fallbackDelivery PhaseOneDelivery, originKind PhaseOneOriginKind) (PhaseOneDraftSnapshot, error) {
	if o == nil || fallbackDelivery != PhaseOneAfterCurrent || (originKind != PhaseOneMicrophone && originKind != PhaseOneFile) {
		return PhaseOneDraftSnapshot{}, ErrPhaseOneInvalidDraft
	}
	o.mu.Lock()
	record, ok := o.records[draftID]
	challenge, hasChallenge := o.challenges[draftID]
	if !ok || !hasChallenge || challenge.Delivery != fallbackDelivery || record.MediaID == "" || record.RequestedDelivery != PhaseOneInterrupt || record.OriginKind != originKind {
		o.mu.Unlock()
		return PhaseOneDraftSnapshot{}, ErrPhaseOneInvalidDraft
	}
	if o.active[draftID] {
		o.mu.Unlock()
		return PhaseOneDraftSnapshot{}, ErrPhaseOneDraftBusy
	}
	o.active[draftID] = true
	record.State, record.Status, record.FailureCode = PhaseOneDraftTransmitting, "confirming_fallback", ""
	o.records[draftID] = record
	if err := o.persistLocked(); err != nil {
		delete(o.active, draftID)
		o.mu.Unlock()
		return PhaseOneDraftSnapshot{}, err
	}
	o.mu.Unlock()
	defer func() {
		o.mu.Lock()
		delete(o.active, draftID)
		o.mu.Unlock()
	}()
	receipt, err := o.service.Transmit(ctx, record.MediaID, record.Route, record.RequestedDelivery, record.OriginKind, record.TransmissionKey, &challenge)
	if err != nil {
		o.rememberChallenge(draftID, err)
		return o.fail(draftID, record, phaseOneFailureCode(err), err)
	}
	record.TransmissionID, record.EffectiveDelivery = receipt.TransmissionID, receipt.EffectiveDelivery
	record.DowngradeReason, record.Status, record.State = receipt.DowngradeReason, receipt.Status, PhaseOneDraftAccepted
	o.mu.Lock()
	delete(o.challenges, draftID)
	o.records[draftID] = record
	err = o.persistLocked()
	snapshot := snapshotPhaseOneDraft(record)
	o.mu.Unlock()
	return snapshot, err
}

func (o *PhaseOneDraftOutbox) Delete(ctx context.Context, draftID string) error {
	if o == nil {
		return ErrPhaseOneInvalidDraft
	}
	o.mu.Lock()
	record, ok := o.records[draftID]
	if !ok {
		o.mu.Unlock()
		return ErrPhaseOneInvalidDraft
	}
	if o.active[draftID] {
		o.mu.Unlock()
		return ErrPhaseOneDraftBusy
	}
	o.active[draftID] = true
	handle, hasHandle := o.handles[draftID]
	o.mu.Unlock()
	defer func() {
		o.mu.Lock()
		delete(o.active, draftID)
		o.mu.Unlock()
	}()
	if record.MediaID != "" {
		if err := o.service.DeleteMedia(ctx, record.MediaID); err != nil {
			return ErrPhaseOneRemoteDelete
		}
	}
	if hasHandle {
		if err := o.store.ExplicitlyDelete(handle); err != nil {
			return ErrPhaseOneLocalCleanup
		}
	}
	o.mu.Lock()
	delete(o.handles, draftID)
	delete(o.records, draftID)
	delete(o.challenges, draftID)
	err := o.persistLocked()
	o.mu.Unlock()
	return err
}

func (o *PhaseOneDraftOutbox) rememberChallenge(draftID string, err error) {
	var client *PhaseOneClientError
	if !errors.As(err, &client) || client.Kind != PhaseOneRejected || client.Code != "requires_confirmation" || !validPhaseOneConfirmationToken(client.ConfirmationToken) {
		return
	}
	for _, alternative := range client.Alternatives {
		if alternative.Available && alternative.Delivery == PhaseOneAfterCurrent {
			o.mu.Lock()
			o.challenges[draftID] = PhaseOneFallbackConfirmation{Token: client.ConfirmationToken, Delivery: alternative.Delivery}
			o.mu.Unlock()
			return
		}
	}
}

func (o *PhaseOneDraftOutbox) fail(draftID string, record phaseOneDraftRecord, code string, cause error) (PhaseOneDraftSnapshot, error) {
	o.mu.Lock()
	if latest, ok := o.records[draftID]; ok {
		record = latest
	}
	record.State, record.FailureCode = PhaseOneDraftRetryableFailure, code
	o.records[draftID] = record
	_ = o.persistLocked()
	snapshot := snapshotPhaseOneDraft(record)
	o.mu.Unlock()
	return snapshot, cause
}

func (o *PhaseOneDraftOutbox) persistLocked() error {
	records := make([]phaseOneDraftRecord, 0, len(o.records))
	for _, record := range o.records {
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].DraftID < records[j].DraftID })
	return writePhaseOneDraftRecords(o.statePath, records)
}

func newPhaseOneDraftRecord(id, title string, originKind PhaseOneOriginKind) phaseOneDraftRecord {
	return phaseOneDraftRecord{
		DraftID: id, Title: title, OriginKind: originKind, UploadKey: "windows-upload-" + id,
		TransmissionKey: "windows-transmission-" + id, State: PhaseOneDraftRetained,
		Status: "ready_to_send", LocalBytesRetained: true,
	}
}

func snapshotPhaseOneDraft(record phaseOneDraftRecord) PhaseOneDraftSnapshot {
	return PhaseOneDraftSnapshot{
		DraftID: record.DraftID, Title: record.Title, OriginKind: record.OriginKind, State: record.State, Route: record.Route,
		RequestedDelivery: record.RequestedDelivery, EffectiveDelivery: record.EffectiveDelivery,
		DowngradeReason: record.DowngradeReason, Status: record.Status,
		FailureCode: record.FailureCode, LocalBytesRetained: record.LocalBytesRetained,
	}
}

func phaseOneFailureCode(err error) string {
	var client *PhaseOneClientError
	if errors.As(err, &client) {
		if client.Kind == PhaseOneRejected && client.Code != "" {
			return client.Code
		}
		switch client.Kind {
		case PhaseOneTransport:
			return "coordinator_unavailable"
		case PhaseOneRedirectRejected:
			return "redirect_rejected"
		case PhaseOneResponseTooLarge:
			return "response_too_large"
		case PhaseOneInvalidConfiguration:
			return "credential_unavailable"
		case PhaseOneInvalidRequest:
			return "invalid_request"
		default:
			return "invalid_response"
		}
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "coordinator_unavailable"
	}
	return "service_unavailable"
}

func loadPhaseOneDraftRecords(path string) ([]phaseOneDraftRecord, error) {
	raw, err := os.ReadFile(filepath.Clean(path))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil || len(raw) > 1<<20 {
		return nil, ErrPhaseOnePersistence
	}
	var envelope phaseOneDraftEnvelope
	if decodePhaseOneJSON(raw, &envelope) != nil || envelope.Version != 1 {
		return nil, ErrPhaseOnePersistence
	}
	seen := map[string]bool{}
	for _, record := range envelope.Records {
		if seen[record.DraftID] || !validPhaseOneDraftRecord(record) {
			return nil, ErrPhaseOnePersistence
		}
		seen[record.DraftID] = true
	}
	return envelope.Records, nil
}

func validPhaseOneDraftRecord(record phaseOneDraftRecord) bool {
	if !isCaptureMediaID(record.DraftID) || record.UploadKey != "windows-upload-"+record.DraftID ||
		record.TransmissionKey != "windows-transmission-"+record.DraftID || record.Title == "" || len([]byte(record.Title)) > 512 ||
		(record.OriginKind != PhaseOneMicrophone && record.OriginKind != PhaseOneFile) ||
		record.MediaID != "" && !validPhaseOnePublicID(record.MediaID, "m_") ||
		record.TransmissionID != "" && !validPhaseOnePublicID(record.TransmissionID, "tr_") ||
		record.TransmissionID != "" && record.MediaID == "" {
		return false
	}
	if record.Route != "" && !validPhaseOneRoute(record.Route) || record.RequestedDelivery != "" && !validPhaseOneDelivery(record.RequestedDelivery) ||
		record.EffectiveDelivery != "" && !validPhaseOneDelivery(record.EffectiveDelivery) {
		return false
	}
	switch record.State {
	case PhaseOneDraftRetained, PhaseOneDraftUploading, PhaseOneDraftUploaded, PhaseOneDraftTransmitting, PhaseOneDraftAccepted, PhaseOneDraftRetryableFailure:
		return true
	default:
		return false
	}
}

func writePhaseOneDraftRecords(path string, records []phaseOneDraftRecord) error {
	directory := filepath.Dir(filepath.Clean(path))
	if err := os.MkdirAll(directory, 0o700); err != nil || os.Chmod(directory, 0o700) != nil {
		return ErrPhaseOnePersistence
	}
	raw, err := json.Marshal(phaseOneDraftEnvelope{Version: 1, Records: records})
	if err != nil {
		return ErrPhaseOnePersistence
	}
	temporary, err := os.CreateTemp(directory, ".phase-one-outbox-*.tmp")
	if err != nil {
		return ErrPhaseOnePersistence
	}
	temporaryPath := temporary.Name()
	remove := true
	defer func() {
		if remove {
			_ = os.Remove(temporaryPath)
		}
	}()
	if temporary.Chmod(0o600) != nil {
		_ = temporary.Close()
		return ErrPhaseOnePersistence
	}
	if written, writeErr := temporary.Write(raw); writeErr != nil || written != len(raw) || temporary.Sync() != nil || temporary.Close() != nil {
		_ = temporary.Close()
		return ErrPhaseOnePersistence
	}
	if err := os.Rename(temporaryPath, filepath.Clean(path)); err != nil {
		return ErrPhaseOnePersistence
	}
	remove = false
	if parent, openErr := os.Open(directory); openErr == nil {
		_ = parent.Sync()
		_ = parent.Close()
	}
	return nil
}
