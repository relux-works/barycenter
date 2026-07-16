package store

import (
	"database/sql"
	"errors"
)

const (
	TransmissionPrepareBarrierMS  = int64(3000)
	TransmissionRTTFreshMS        = int64(12000)
	TransmissionMaxRTTMS          = int64(10000)
	TransmissionStartWindowMS     = int64(100)
	TransmissionCancelAckMS       = int64(2000)
	TransmissionEndReceiptGraceMS = int64(2000)
)

var ErrTransmissionNotFIFOHead = errors.New("transmission is not the playback-domain FIFO head")

// TransmissionSchedulerState contains only mutable runtime timestamps. The
// accepted transmission and target snapshots remain immutable in their
// original tables.
type TransmissionSchedulerState struct {
	TransmissionID       string
	BarrierOpenedAt      int64
	PrepareDeadlineAt    int64
	DecisionAt           int64
	TCoordMS             int64
	StartDeadlineCoordMS int64
	LegacyElementID      string
	Revision             int64
	UpdatedAt            int64
}

type TransmissionSchedulerWork struct {
	Transmission Transmission
	Targets      []TransmissionTarget
	Media        MediaItem
	Scheduler    TransmissionSchedulerState
}

// TransmissionRuntimeTarget is an untrusted point-in-time projection of the
// authenticated WebSocket registry. Store methods compare its credential
// witness to the immutable binding before using liveness or capabilities.
type TransmissionRuntimeTarget struct {
	OrbitID              int64
	Slot                 string
	Connected            bool
	LastSeenAt           int64
	CredentialTokenHash  string
	MediaClipCapable     bool
	OverlayCapable       bool
	InterruptCapable     bool
	InterruptResumeReady bool
	RTTMS                int64
	RTTSampledAt         int64
}

type OpenTransmissionBarrierResult struct {
	Work           TransmissionSchedulerWork
	PrepareTargets []TransmissionTarget
	Opened         bool
	Changed        bool
}

type DecideTransmissionBarrierResult struct {
	Work             TransmissionSchedulerWork
	ScheduledTargets []TransmissionTarget
	DisarmTargets    []TransmissionTarget
	Decided          bool
	Changed          bool
}

type ClaimLegacyTransmissionResult struct {
	Work    TransmissionSchedulerWork
	Targets []TransmissionTarget
	Changed bool
}

type RecheckTransmissionRuntimeResult struct {
	Work          TransmissionSchedulerWork
	DisarmTargets []TransmissionTarget
	Changed       bool
}

type ExpireTransmissionRuntimeResult struct {
	Work          TransmissionSchedulerWork
	DisarmTargets []TransmissionTarget
	Changed       bool
	NextDue       int64
}

func scanTransmissionSchedulerState(row sqlScanner) (TransmissionSchedulerState, error) {
	var state TransmissionSchedulerState
	err := row.Scan(
		&state.TransmissionID, &state.BarrierOpenedAt,
		&state.PrepareDeadlineAt, &state.DecisionAt, &state.TCoordMS,
		&state.StartDeadlineCoordMS, &state.LegacyElementID,
		&state.Revision, &state.UpdatedAt,
	)
	return state, err
}

const transmissionSchedulerColumns = `transmission_id, barrier_opened_at,
prepare_deadline_at, decision_at, t_coord_ms, start_deadline_coord_ms,
legacy_element_id, revision, updated_at`

func loadTransmissionSchedulerWorkTx(
	tx *sql.Tx,
	transmissionID string,
) (TransmissionSchedulerWork, error) {
	transmission, err := scanTransmission(tx.QueryRow(
		`SELECT `+transmissionColumns+` FROM transmissions WHERE id = ?`, transmissionID,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return TransmissionSchedulerWork{}, ErrTransmissionNotFound
	}
	if err != nil {
		return TransmissionSchedulerWork{}, err
	}
	state, err := scanTransmissionSchedulerState(tx.QueryRow(
		`SELECT `+transmissionSchedulerColumns+`
FROM transmission_scheduler_state WHERE transmission_id = ?`, transmissionID,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return TransmissionSchedulerWork{}, ErrTransmissionStateConflict
	}
	if err != nil {
		return TransmissionSchedulerWork{}, err
	}
	mediaItem, err := scanMediaItem(tx.QueryRow(
		`SELECT `+mediaItemColumns+` FROM media_items WHERE id = ?`, transmission.MediaID,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return TransmissionSchedulerWork{}, ErrTransmissionMediaNotFound
	}
	if err != nil {
		return TransmissionSchedulerWork{}, err
	}
	rows, err := tx.Query(`SELECT `+transmissionTargetColumns+`
FROM transmission_targets WHERE transmission_id = ? ORDER BY orbit_id, slot`, transmissionID)
	if err != nil {
		return TransmissionSchedulerWork{}, err
	}
	var targets []TransmissionTarget
	for rows.Next() {
		target, scanErr := scanTransmissionTarget(rows)
		if scanErr != nil {
			rows.Close()
			return TransmissionSchedulerWork{}, scanErr
		}
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return TransmissionSchedulerWork{}, err
	}
	if err := rows.Close(); err != nil {
		return TransmissionSchedulerWork{}, err
	}
	if len(targets) == 0 {
		return TransmissionSchedulerWork{}, ErrTransmissionTargetInvalid
	}
	return TransmissionSchedulerWork{
		Transmission: transmission, Targets: targets, Media: mediaItem, Scheduler: state,
	}, nil
}

func (s *Store) GetTransmissionSchedulerWork(
	transmissionID string,
) (TransmissionSchedulerWork, error) {
	if !transmissionIDPattern.MatchString(transmissionID) {
		return TransmissionSchedulerWork{}, ErrTransmissionNotFound
	}
	tx, err := s.db.Begin()
	if err != nil {
		return TransmissionSchedulerWork{}, err
	}
	defer tx.Rollback()
	work, err := loadTransmissionSchedulerWorkTx(tx, transmissionID)
	if err != nil {
		return TransmissionSchedulerWork{}, err
	}
	if err := tx.Commit(); err != nil {
		return TransmissionSchedulerWork{}, err
	}
	return work, nil
}

// ListTransmissionSchedulerWork returns nonterminal work in the one trusted
// global order. The runtime chooses only the first overlay/interrupt row for
// each persisted playback domain; after-current rows are handed to the legacy
// Session FSM independently.
func (s *Store) ListTransmissionSchedulerWork(limit int) ([]TransmissionSchedulerWork, error) {
	if limit <= 0 || limit > 1000 {
		return nil, ErrTransmissionInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT id FROM transmissions
WHERE completed_at = 0 ORDER BY accepted_at, id LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	work := make([]TransmissionSchedulerWork, 0, len(ids))
	for _, id := range ids {
		item, err := loadTransmissionSchedulerWorkTx(tx, id)
		if err != nil {
			return nil, err
		}
		work = append(work, item)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return work, nil
}

func runtimeTargetMap(
	runtime []TransmissionRuntimeTarget,
) (map[string]TransmissionRuntimeTarget, error) {
	result := make(map[string]TransmissionRuntimeTarget, len(runtime))
	for _, target := range runtime {
		if target.OrbitID <= 0 || !transmissionSlotPattern.MatchString(target.Slot) ||
			target.LastSeenAt < 0 || target.RTTSampledAt < 0 ||
			(target.CredentialTokenHash != "" &&
				!transmissionDigestPattern.MatchString(target.CredentialTokenHash)) {
			return nil, ErrTransmissionInvalid
		}
		key := transmissionTargetKey(target.OrbitID, target.Slot)
		if _, exists := result[key]; exists {
			return nil, ErrTransmissionInvalid
		}
		result[key] = target
	}
	return result, nil
}

func targetRuntimeBindingMatchesTx(
	tx *sql.Tx,
	target TransmissionTarget,
	runtime TransmissionRuntimeTarget,
) (bool, error) {
	var pairedAt int64
	var tokenHash string
	err := tx.QueryRow(`SELECT ic.slot_paired_at, ic.binding_token_hash
FROM installation_credentials ic
JOIN actors a ON a.id = ic.actor_id AND a.revoked_at IS NULL
JOIN memberships m ON m.actor_id = ic.actor_id
  AND m.orbit_id = ic.slot_orbit_id AND m.left_at IS NULL
JOIN orbits o ON o.id = m.orbit_id AND o.status = 'active'
JOIN slots sl ON sl.orbit_id = ic.slot_orbit_id AND sl.slot = ic.slot_name
  AND sl.revoked_at IS NULL AND sl.token_hash = ic.binding_token_hash
  AND COALESCE(sl.paired_at, 0) = ic.slot_paired_at
WHERE ic.slot_orbit_id = ? AND ic.actor_id = ? AND ic.slot_name = ?`,
		target.OrbitID, target.ActorID, target.Slot,
	).Scan(&pairedAt, &tokenHash)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return pairedAt == target.BindingPairedAt &&
		runtime.CredentialTokenHash != "" && runtime.CredentialTokenHash == tokenHash, nil
}

func targetDeliveryCapable(
	delivery TransmissionDelivery,
	target TransmissionTarget,
	runtime TransmissionRuntimeTarget,
) (bool, TransmissionReason) {
	if !target.MediaClipCapable || !runtime.MediaClipCapable {
		return false, TransmissionReasonCapabilityLost
	}
	switch delivery {
	case TransmissionDeliveryOverlay:
		if !target.OverlayCapable || !runtime.OverlayCapable {
			return false, TransmissionReasonCapabilityLost
		}
	case TransmissionDeliveryInterrupt:
		if !target.InterruptCapable || !target.InterruptResumeReady ||
			!runtime.InterruptCapable || !runtime.InterruptResumeReady {
			return false, TransmissionReasonInterruptCapabilityLost
		}
	default:
		return false, TransmissionReasonCapabilityLost
	}
	return true, ""
}

type runtimeTargetDecision struct {
	status TransmissionTargetStatus
	reason TransmissionReason
	rttMS  int64
}

func evaluateRuntimeTargetTx(
	tx *sql.Tx,
	transmission Transmission,
	media MediaItem,
	target TransmissionTarget,
	runtime map[string]TransmissionRuntimeTarget,
	now int64,
	requireRTT bool,
) (runtimeTargetDecision, error) {
	if transmission.AirID != "" {
		var stillActive int
		err := tx.QueryRow(`SELECT EXISTS(
  SELECT 1 FROM air_members m
  JOIN air_active_pointers p ON p.orbit_id = m.orbit_id AND p.air_id = m.air_id
  JOIN airs a ON a.public_id = m.air_id AND a.status <> 'dissolved'
  WHERE m.air_id = ? AND m.orbit_id = ? AND m.status = 'joined'
)`, transmission.AirID, target.OrbitID).Scan(&stillActive)
		if err != nil {
			return runtimeTargetDecision{}, err
		}
		if stillActive == 0 {
			return runtimeTargetDecision{
				status: TransmissionTargetCancelled,
				reason: TransmissionReasonApproachLeft,
			}, nil
		}
	}
	projected, exists := runtime[transmissionTargetKey(target.OrbitID, target.Slot)]
	if !exists {
		return runtimeTargetDecision{
			status: TransmissionTargetMissedOffline,
			reason: TransmissionReasonOfflineBeforePrepare,
		}, nil
	}
	bindingMatches, err := targetRuntimeBindingMatchesTx(tx, target, projected)
	if err != nil {
		return runtimeTargetDecision{}, err
	}
	if !bindingMatches {
		return runtimeTargetDecision{
			status: TransmissionTargetCancelled,
			reason: TransmissionReasonTargetRevoked,
		}, nil
	}
	block, err := transmissionBlockDecisionTx(
		tx, target.OrbitID, target.ActorID,
		transmission.SourceOrbitID, transmission.SourceActorID,
	)
	if err != nil {
		return runtimeTargetDecision{}, err
	}
	if block.Blocked {
		return runtimeTargetDecision{status: TransmissionTargetBlocked, reason: block.Reason}, nil
	}
	dnd, err := effectiveDNDTx(tx, resolvedTransmissionTarget{
		OrbitID: target.OrbitID, ActorID: target.ActorID, Slot: target.Slot,
	}, now)
	if err != nil {
		return runtimeTargetDecision{}, err
	}
	localThisPulsar := transmission.AudienceKind == TransmissionAudienceThisPulsar &&
		target.OrbitID == transmission.SourceOrbitID &&
		target.ActorID == transmission.SourceActorID &&
		target.Slot == transmission.SourceSlot
	dndSuppresses := !localThisPulsar &&
		(dnd.Mode == DNDMutedUntil ||
			(dnd.Mode == DNDMessagesOnly && media.Kind == MediaKindBuiltinCue))
	if dndSuppresses {
		return runtimeTargetDecision{status: TransmissionTargetMissedDND, reason: dnd.Reason}, nil
	}
	if !projected.Connected || projected.LastSeenAt <= 0 ||
		projected.LastSeenAt > now || now-projected.LastSeenAt > TransmissionRTTFreshMS {
		reason := TransmissionReasonOfflineBeforePrepare
		if target.Status == TransmissionTargetReady ||
			target.Status == TransmissionTargetScheduled {
			reason = TransmissionReasonOfflineBeforeStart
		}
		return runtimeTargetDecision{status: TransmissionTargetMissedOffline, reason: reason}, nil
	}
	if capable, reason := targetDeliveryCapable(
		transmission.EffectiveDelivery, target, projected,
	); !capable {
		return runtimeTargetDecision{status: TransmissionTargetFailed, reason: reason}, nil
	}
	if requireRTT && (projected.RTTSampledAt <= 0 || projected.RTTSampledAt > now ||
		now-projected.RTTSampledAt > TransmissionRTTFreshMS || projected.RTTMS < 0 ||
		projected.RTTMS > TransmissionMaxRTTMS) {
		return runtimeTargetDecision{
			status: TransmissionTargetFailed,
			reason: TransmissionReasonClockUnsynchronized,
		}, nil
	}
	return runtimeTargetDecision{rttMS: projected.RTTMS}, nil
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func schedulerWorkEventTime(work TransmissionSchedulerWork, now int64) int64 {
	eventAt := maxInt64(now, work.Transmission.UpdatedAt)
	eventAt = maxInt64(eventAt, work.Scheduler.UpdatedAt)
	for _, target := range work.Targets {
		eventAt = maxInt64(eventAt, target.UpdatedAt)
	}
	return eventAt
}

func setTransmissionTargetStatusTx(
	tx *sql.Tx,
	target TransmissionTarget,
	status TransmissionTargetStatus,
	reason TransmissionReason,
	now int64,
	receipt bool,
) (TransmissionTarget, bool, error) {
	if target.Status == status && target.ReasonCode == reason {
		return target, false, nil
	}
	if now < target.UpdatedAt || !validTransmissionTargetReason(status, reason) ||
		!validTransmissionTargetTransition(target.Status, status) {
		return TransmissionTarget{}, false, ErrTransmissionStateConflict
	}
	readyAt, scheduledAt := target.ReadyAt, target.ScheduledAt
	startedAt, endedAt := target.StartedAt, target.EndedAt
	switch status {
	case TransmissionTargetReady:
		if readyAt == 0 {
			readyAt = now
		}
	case TransmissionTargetScheduled:
		if scheduledAt == 0 {
			scheduledAt = now
		}
	case TransmissionTargetPlaying:
		if startedAt == 0 {
			startedAt = now
		}
	default:
		if terminalTransmissionTargetStatus(status) && endedAt == 0 {
			endedAt = now
		}
	}
	lastReceiptAt := target.LastReceiptAt
	if receipt {
		lastReceiptAt = now
	}
	result, err := tx.Exec(`UPDATE transmission_targets
SET status = ?, reason_code = ?, revision = revision + 1,
    ready_at = ?, scheduled_at = ?, started_at = ?, ended_at = ?,
    last_receipt_at = ?, updated_at = ?
WHERE transmission_id = ? AND orbit_id = ? AND actor_id = ? AND slot = ?
  AND revision = ? AND generation = ?`, status, reason, readyAt, scheduledAt,
		startedAt, endedAt, lastReceiptAt, now, target.TransmissionID,
		target.OrbitID, target.ActorID, target.Slot, target.Revision, target.Generation)
	if err != nil {
		return TransmissionTarget{}, false, err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		if err != nil {
			return TransmissionTarget{}, false, err
		}
		return TransmissionTarget{}, false, ErrTransmissionStateConflict
	}
	target.Status, target.ReasonCode = status, reason
	target.Revision++
	target.ReadyAt, target.ScheduledAt = readyAt, scheduledAt
	target.StartedAt, target.EndedAt = startedAt, endedAt
	target.LastReceiptAt, target.UpdatedAt = lastReceiptAt, now
	return target, true, nil
}

func schedulerFIFOHeadTx(tx *sql.Tx, transmission Transmission) (bool, error) {
	var older int
	err := tx.QueryRow(`SELECT COUNT(*) FROM transmissions
WHERE playback_domain_kind = ? AND playback_domain_id = ?
  AND effective_delivery IN ('overlay', 'interrupt') AND completed_at = 0
  AND (accepted_at < ? OR (accepted_at = ? AND id < ?))`,
		transmission.PlaybackDomainKind, transmission.PlaybackDomainID,
		transmission.AcceptedAt, transmission.AcceptedAt, transmission.ID,
	).Scan(&older)
	return older == 0, err
}

func (s *Store) OpenTransmissionBarrier(
	transmissionID string,
	now int64,
	runtime []TransmissionRuntimeTarget,
) (OpenTransmissionBarrierResult, error) {
	if !transmissionIDPattern.MatchString(transmissionID) || now <= 0 {
		return OpenTransmissionBarrierResult{}, ErrTransmissionInvalid
	}
	runtimeByTarget, err := runtimeTargetMap(runtime)
	if err != nil {
		return OpenTransmissionBarrierResult{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return OpenTransmissionBarrierResult{}, err
	}
	defer tx.Rollback()
	work, err := loadTransmissionSchedulerWorkTx(tx, transmissionID)
	if err != nil {
		return OpenTransmissionBarrierResult{}, err
	}
	if work.Transmission.CompletedAt != 0 || work.Transmission.CancellationCause != "" ||
		(work.Transmission.EffectiveDelivery != TransmissionDeliveryOverlay &&
			work.Transmission.EffectiveDelivery != TransmissionDeliveryInterrupt) {
		return OpenTransmissionBarrierResult{}, ErrTransmissionStateConflict
	}
	head, err := schedulerFIFOHeadTx(tx, work.Transmission)
	if err != nil {
		return OpenTransmissionBarrierResult{}, err
	}
	if !head {
		return OpenTransmissionBarrierResult{}, ErrTransmissionNotFIFOHead
	}
	if work.Media.Status != MediaStatusReady || work.Media.StorageKey == "" ||
		work.Media.ExpiresAt <= now || work.Transmission.ExpiresAt <= now {
		return OpenTransmissionBarrierResult{}, ErrTransmissionMediaNotReady
	}
	opened := false
	changed := false
	if work.Scheduler.BarrierOpenedAt == 0 {
		deadline := now + TransmissionPrepareBarrierMS
		if work.Transmission.ExpiresAt < deadline {
			deadline = work.Transmission.ExpiresAt
		}
		result, err := tx.Exec(`UPDATE transmission_scheduler_state
SET barrier_opened_at = ?, prepare_deadline_at = ?,
    revision = revision + 1, updated_at = ?
WHERE transmission_id = ? AND revision = ? AND barrier_opened_at = 0`,
			now, deadline, now, transmissionID, work.Scheduler.Revision)
		if err != nil {
			return OpenTransmissionBarrierResult{}, err
		}
		if count, err := result.RowsAffected(); err != nil || count != 1 {
			if err != nil {
				return OpenTransmissionBarrierResult{}, err
			}
			return OpenTransmissionBarrierResult{}, ErrTransmissionStateConflict
		}
		work.Scheduler.BarrierOpenedAt = now
		work.Scheduler.PrepareDeadlineAt = deadline
		work.Scheduler.Revision++
		work.Scheduler.UpdatedAt = now
		opened, changed = true, true
	}
	prepareTargets := make([]TransmissionTarget, 0, len(work.Targets))
	for i, target := range work.Targets {
		if target.Status == TransmissionTargetPreparing {
			prepareTargets = append(prepareTargets, target)
			continue
		}
		if target.Status != TransmissionTargetAccepted {
			continue
		}
		decision, err := evaluateRuntimeTargetTx(
			tx, work.Transmission, work.Media, target, runtimeByTarget, now, false,
		)
		if err != nil {
			return OpenTransmissionBarrierResult{}, err
		}
		status, reason := TransmissionTargetPreparing, TransmissionReason("")
		if decision.status != "" {
			status, reason = decision.status, decision.reason
		}
		updated, didChange, err := setTransmissionTargetStatusTx(
			tx, target, status, reason, maxInt64(now, target.UpdatedAt), false,
		)
		if err != nil {
			return OpenTransmissionBarrierResult{}, err
		}
		work.Targets[i] = updated
		changed = changed || didChange
		if updated.Status == TransmissionTargetPreparing {
			prepareTargets = append(prepareTargets, updated)
		}
	}
	work.Transmission, err = recomputeTransmissionTx(tx, transmissionID, now)
	if err != nil {
		return OpenTransmissionBarrierResult{}, err
	}
	if err := s.checkpoint("transmission_barrier_open_before_commit"); err != nil {
		return OpenTransmissionBarrierResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return OpenTransmissionBarrierResult{}, err
	}
	return OpenTransmissionBarrierResult{
		Work: work, PrepareTargets: prepareTargets, Opened: opened, Changed: changed,
	}, nil
}

func (s *Store) DecideTransmissionBarrier(
	transmissionID string,
	now int64,
	runtime []TransmissionRuntimeTarget,
) (DecideTransmissionBarrierResult, error) {
	if !transmissionIDPattern.MatchString(transmissionID) || now <= 0 {
		return DecideTransmissionBarrierResult{}, ErrTransmissionInvalid
	}
	runtimeByTarget, err := runtimeTargetMap(runtime)
	if err != nil {
		return DecideTransmissionBarrierResult{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return DecideTransmissionBarrierResult{}, err
	}
	defer tx.Rollback()
	work, err := loadTransmissionSchedulerWorkTx(tx, transmissionID)
	if err != nil {
		return DecideTransmissionBarrierResult{}, err
	}
	if work.Scheduler.BarrierOpenedAt == 0 || work.Transmission.CompletedAt != 0 ||
		work.Transmission.CancellationCause != "" {
		return DecideTransmissionBarrierResult{}, ErrTransmissionStateConflict
	}
	if work.Scheduler.DecisionAt != 0 {
		var scheduled []TransmissionTarget
		for _, target := range work.Targets {
			if target.Status == TransmissionTargetScheduled {
				scheduled = append(scheduled, target)
			}
		}
		if err := tx.Commit(); err != nil {
			return DecideTransmissionBarrierResult{}, err
		}
		return DecideTransmissionBarrierResult{
			Work: work, ScheduledTargets: scheduled, Decided: true,
		}, nil
	}
	pending := false
	for _, target := range work.Targets {
		if target.Status == TransmissionTargetPreparing {
			pending = true
			break
		}
	}
	if pending && now < work.Scheduler.PrepareDeadlineAt {
		if err := tx.Commit(); err != nil {
			return DecideTransmissionBarrierResult{}, err
		}
		return DecideTransmissionBarrierResult{Work: work}, nil
	}
	changed := false
	decisionAt := now
	if decisionAt < work.Scheduler.UpdatedAt {
		decisionAt = work.Scheduler.UpdatedAt
	}
	for i, target := range work.Targets {
		if target.Status != TransmissionTargetPreparing {
			continue
		}
		occurredAt := decisionAt
		if work.Scheduler.PrepareDeadlineAt > 0 &&
			work.Scheduler.PrepareDeadlineAt < occurredAt {
			occurredAt = work.Scheduler.PrepareDeadlineAt
		}
		occurredAt = maxInt64(occurredAt, target.UpdatedAt)
		updated, didChange, err := setTransmissionTargetStatusTx(
			tx, target, TransmissionTargetMissedNotReady,
			TransmissionReasonPrepareDeadline, occurredAt, false,
		)
		if err != nil {
			return DecideTransmissionBarrierResult{}, err
		}
		work.Targets[i] = updated
		changed = changed || didChange
	}
	maxRTT := int64(0)
	readyIndexes := make([]int, 0, len(work.Targets))
	for i, target := range work.Targets {
		if target.Status != TransmissionTargetReady {
			continue
		}
		decision, err := evaluateRuntimeTargetTx(
			tx, work.Transmission, work.Media, target, runtimeByTarget, decisionAt, true,
		)
		if err != nil {
			return DecideTransmissionBarrierResult{}, err
		}
		if decision.status != "" {
			updated, didChange, err := setTransmissionTargetStatusTx(
				tx, target, decision.status, decision.reason,
				maxInt64(decisionAt, target.UpdatedAt), false,
			)
			if err != nil {
				return DecideTransmissionBarrierResult{}, err
			}
			work.Targets[i] = updated
			changed = changed || didChange
			continue
		}
		if decision.rttMS > maxRTT {
			maxRTT = decision.rttMS
		}
		readyIndexes = append(readyIndexes, i)
	}
	leadMS := maxInt64(2*maxRTT+250, 500)
	tCoordMS, startDeadline := int64(0), int64(0)
	var disarm []TransmissionTarget
	if len(readyIndexes) > 0 && decisionAt+leadMS >= work.Transmission.ExpiresAt {
		result, err := tx.Exec(`UPDATE transmissions
SET cancellation_cause = ?, revision = revision + 1, updated_at = ?
WHERE id = ? AND revision = ? AND cancellation_cause = '' AND completed_at = 0`,
			TransmissionReasonDeliveryExpired, decisionAt,
			transmissionID, work.Transmission.Revision,
		)
		if err != nil {
			return DecideTransmissionBarrierResult{}, err
		}
		if count, err := result.RowsAffected(); err != nil || count != 1 {
			if err != nil {
				return DecideTransmissionBarrierResult{}, err
			}
			return DecideTransmissionBarrierResult{}, ErrTransmissionStateConflict
		}
		work.Transmission.CancellationCause = TransmissionReasonDeliveryExpired
		work.Transmission.Revision++
		work.Transmission.UpdatedAt = decisionAt
		for _, index := range readyIndexes {
			target := work.Targets[index]
			updated, didChange, err := setTransmissionTargetStatusTx(
				tx, target, TransmissionTargetExpired, TransmissionReasonDeliveryExpired,
				maxInt64(decisionAt, target.UpdatedAt), false,
			)
			if err != nil {
				return DecideTransmissionBarrierResult{}, err
			}
			work.Targets[index] = updated
			changed = changed || didChange
			if didChange {
				disarm = append(disarm, updated)
			}
		}
		readyIndexes = nil
	}
	if len(readyIndexes) > 0 {
		tCoordMS = decisionAt + leadMS
		startDeadline = tCoordMS + TransmissionStartWindowMS
		for _, index := range readyIndexes {
			target := work.Targets[index]
			updated, didChange, err := setTransmissionTargetStatusTx(
				tx, target, TransmissionTargetScheduled, "",
				maxInt64(decisionAt, target.UpdatedAt), false,
			)
			if err != nil {
				return DecideTransmissionBarrierResult{}, err
			}
			work.Targets[index] = updated
			changed = changed || didChange
		}
	}
	result, err := tx.Exec(`UPDATE transmission_scheduler_state
SET decision_at = ?, t_coord_ms = ?, start_deadline_coord_ms = ?,
    revision = revision + 1, updated_at = ?
WHERE transmission_id = ? AND revision = ? AND decision_at = 0`,
		decisionAt, tCoordMS, startDeadline, decisionAt,
		transmissionID, work.Scheduler.Revision)
	if err != nil {
		return DecideTransmissionBarrierResult{}, err
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		if err != nil {
			return DecideTransmissionBarrierResult{}, err
		}
		return DecideTransmissionBarrierResult{}, ErrTransmissionStateConflict
	}
	work.Scheduler.DecisionAt = decisionAt
	work.Scheduler.TCoordMS = tCoordMS
	work.Scheduler.StartDeadlineCoordMS = startDeadline
	work.Scheduler.Revision++
	work.Scheduler.UpdatedAt = decisionAt
	changed = true
	work.Transmission, err = recomputeTransmissionTx(tx, transmissionID, decisionAt)
	if err != nil {
		return DecideTransmissionBarrierResult{}, err
	}
	if err := s.checkpoint("transmission_barrier_decide_before_commit"); err != nil {
		return DecideTransmissionBarrierResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return DecideTransmissionBarrierResult{}, err
	}
	var scheduled []TransmissionTarget
	for _, target := range work.Targets {
		if target.Status == TransmissionTargetScheduled {
			scheduled = append(scheduled, target)
		}
	}
	return DecideTransmissionBarrierResult{
		Work: work, ScheduledTargets: scheduled,
		DisarmTargets: disarm, Decided: true, Changed: changed,
	}, nil
}

// RecheckTransmissionRuntime applies policy and binding changes between the
// durable schedule decision and T. It also converts active block, DND and
// revocation changes into generation-bound fade-stop work. Main-program
// pause/skip state is deliberately absent from this repository boundary.
func (s *Store) RecheckTransmissionRuntime(
	transmissionID string,
	now int64,
	runtime []TransmissionRuntimeTarget,
) (RecheckTransmissionRuntimeResult, error) {
	if !transmissionIDPattern.MatchString(transmissionID) || now <= 0 {
		return RecheckTransmissionRuntimeResult{}, ErrTransmissionInvalid
	}
	runtimeByTarget, err := runtimeTargetMap(runtime)
	if err != nil {
		return RecheckTransmissionRuntimeResult{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return RecheckTransmissionRuntimeResult{}, err
	}
	defer tx.Rollback()
	work, err := loadTransmissionSchedulerWorkTx(tx, transmissionID)
	if err != nil {
		return RecheckTransmissionRuntimeResult{}, err
	}
	if work.Transmission.CompletedAt != 0 ||
		(work.Transmission.CancellationCause != "" &&
			work.Transmission.CancellationCause != TransmissionReasonDeliveryExpired) {
		if err := tx.Commit(); err != nil {
			return RecheckTransmissionRuntimeResult{}, err
		}
		return RecheckTransmissionRuntimeResult{Work: work}, nil
	}
	eventAt := schedulerWorkEventTime(work, now)
	changed := false
	var disarm []TransmissionTarget
	for i, target := range work.Targets {
		if target.Status != TransmissionTargetPreparing &&
			target.Status != TransmissionTargetReady &&
			target.Status != TransmissionTargetScheduled &&
			target.Status != TransmissionTargetPlaying {
			continue
		}
		// Once the hard start window has elapsed, stale-play reconciliation
		// owns a scheduled row. A reconnect must not turn it into a different
		// terminal policy result and thereby obscure the late-start boundary.
		if target.Status == TransmissionTargetScheduled &&
			work.Scheduler.StartDeadlineCoordMS > 0 &&
			now > work.Scheduler.StartDeadlineCoordMS {
			continue
		}
		decision, err := evaluateRuntimeTargetTx(
			tx, work.Transmission, work.Media, target, runtimeByTarget, now, false,
		)
		if err != nil {
			return RecheckTransmissionRuntimeResult{}, err
		}
		if decision.status == "" {
			continue
		}
		status, reason := decision.status, decision.reason
		if target.Status == TransmissionTargetPlaying {
			switch decision.status {
			case TransmissionTargetCancelled:
				status = TransmissionTargetCancelling
				if decision.reason != TransmissionReasonApproachLeft {
					reason = TransmissionReasonTargetRevoked
				}
			case TransmissionTargetBlocked:
				status, reason = TransmissionTargetCancelling, TransmissionReasonSenderBlocked
			case TransmissionTargetMissedDND:
				status, reason = TransmissionTargetCancelling, TransmissionReasonDNDEnabled
			default:
				// A transient connection or capability projection cannot prove
				// that already-started audio stopped. The bounded end receipt
				// watchdog closes it without allowing an overlapping FIFO head.
				continue
			}
		}
		updated, didChange, err := setTransmissionTargetStatusTx(
			tx, target, status, reason, maxInt64(eventAt, target.UpdatedAt), false,
		)
		if err != nil {
			return RecheckTransmissionRuntimeResult{}, err
		}
		work.Targets[i] = updated
		changed = changed || didChange
		if didChange {
			disarm = append(disarm, updated)
		}
	}
	if changed {
		work.Transmission, err = recomputeTransmissionTx(tx, transmissionID, eventAt)
		if err != nil {
			return RecheckTransmissionRuntimeResult{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return RecheckTransmissionRuntimeResult{}, err
	}
	return RecheckTransmissionRuntimeResult{
		Work: work, DisarmTargets: disarm, Changed: changed,
	}, nil
}

// ExpireTransmissionRuntime closes stale scheduled starts and unacknowledged
// cancellations. It is exact-retry idempotent and returns the nearest future
// coordinator deadline needed by the runtime.
func (s *Store) ExpireTransmissionRuntime(
	transmissionID string,
	now int64,
) (ExpireTransmissionRuntimeResult, error) {
	if !transmissionIDPattern.MatchString(transmissionID) || now <= 0 {
		return ExpireTransmissionRuntimeResult{}, ErrTransmissionInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return ExpireTransmissionRuntimeResult{}, err
	}
	defer tx.Rollback()
	work, err := loadTransmissionSchedulerWorkTx(tx, transmissionID)
	if err != nil {
		return ExpireTransmissionRuntimeResult{}, err
	}
	eventAt := schedulerWorkEventTime(work, now)
	changed, nextDue := false, int64(0)
	var disarm []TransmissionTarget
	for i, target := range work.Targets {
		status, reason, due := TransmissionTargetStatus(""), TransmissionReason(""), int64(0)
		switch target.Status {
		case TransmissionTargetScheduled:
			due = work.Scheduler.StartDeadlineCoordMS
			status, reason = TransmissionTargetFailed, TransmissionReasonStalePlay
		case TransmissionTargetPlaying:
			startedAt := maxInt64(target.StartedAt, work.Scheduler.TCoordMS)
			due = startedAt + work.Media.DurationMS + TransmissionEndReceiptGraceMS
			status, reason = TransmissionTargetFailed, TransmissionReasonInternalError
		case TransmissionTargetCancelling:
			due = target.UpdatedAt + TransmissionCancelAckMS
			status, reason = TransmissionTargetFailed, TransmissionReasonCancelUnacknowledged
		}
		if due == 0 {
			continue
		}
		if now <= due {
			if nextDue == 0 || due < nextDue {
				nextDue = due
			}
			continue
		}
		updated, didChange, err := setTransmissionTargetStatusTx(
			tx, target, status, reason, maxInt64(eventAt, target.UpdatedAt), false,
		)
		if err != nil {
			return ExpireTransmissionRuntimeResult{}, err
		}
		work.Targets[i] = updated
		changed = changed || didChange
		if didChange && target.Status != TransmissionTargetCancelling {
			disarm = append(disarm, updated)
		}
	}
	if changed {
		work.Transmission, err = recomputeTransmissionTx(tx, transmissionID, eventAt)
		if err != nil {
			return ExpireTransmissionRuntimeResult{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return ExpireTransmissionRuntimeResult{}, err
	}
	return ExpireTransmissionRuntimeResult{
		Work: work, DisarmTargets: disarm, Changed: changed, NextDue: nextDue,
	}, nil
}

// ExpireTransmissionDelivery applies the immutable speak-now expiry. Work
// which never started becomes terminal immediately, while any prepared node
// still receives a same-generation disarm. A playing row may finish normally;
// its bounded end-receipt watchdog remains armed separately.
func (s *Store) ExpireTransmissionDelivery(
	transmissionID string,
	now int64,
) (CancelTransmissionResult, error) {
	if !transmissionIDPattern.MatchString(transmissionID) || now <= 0 {
		return CancelTransmissionResult{}, ErrTransmissionInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return CancelTransmissionResult{}, err
	}
	defer tx.Rollback()
	work, err := loadTransmissionSchedulerWorkTx(tx, transmissionID)
	if err != nil {
		return CancelTransmissionResult{}, err
	}
	if work.Transmission.CompletedAt != 0 || now < work.Transmission.ExpiresAt ||
		(work.Transmission.CancellationCause != "" &&
			work.Transmission.CancellationCause != TransmissionReasonDeliveryExpired) {
		if err := tx.Commit(); err != nil {
			return CancelTransmissionResult{}, err
		}
		return CancelTransmissionResult{Transmission: work.Transmission}, nil
	}
	eventAt := schedulerWorkEventTime(work, now)
	changed := false
	if work.Transmission.CancellationCause == "" {
		result, err := tx.Exec(`UPDATE transmissions
SET cancellation_cause = ?, revision = revision + 1, updated_at = ?
WHERE id = ? AND revision = ? AND cancellation_cause = '' AND completed_at = 0`,
			TransmissionReasonDeliveryExpired, eventAt, transmissionID,
			work.Transmission.Revision,
		)
		if err != nil {
			return CancelTransmissionResult{}, err
		}
		if count, err := result.RowsAffected(); err != nil || count != 1 {
			if err != nil {
				return CancelTransmissionResult{}, err
			}
			return CancelTransmissionResult{}, ErrTransmissionStateConflict
		}
		work.Transmission.CancellationCause = TransmissionReasonDeliveryExpired
		work.Transmission.Revision++
		work.Transmission.UpdatedAt = eventAt
		changed = true
	}
	var disarm []TransmissionTarget
	for i, target := range work.Targets {
		needsDisarm := false
		switch target.Status {
		case TransmissionTargetAccepted:
		case TransmissionTargetPreparing, TransmissionTargetReady,
			TransmissionTargetScheduled:
			needsDisarm = true
		default:
			continue
		}
		updated, didChange, err := setTransmissionTargetStatusTx(
			tx, target, TransmissionTargetExpired, TransmissionReasonDeliveryExpired,
			maxInt64(eventAt, target.UpdatedAt), false,
		)
		if err != nil {
			return CancelTransmissionResult{}, err
		}
		work.Targets[i] = updated
		changed = changed || didChange
		if needsDisarm && didChange {
			disarm = append(disarm, updated)
		}
	}
	work.Transmission, err = recomputeTransmissionTx(tx, transmissionID, eventAt)
	if err != nil {
		return CancelTransmissionResult{}, err
	}
	if err := s.checkpoint("transmission_delivery_expire_before_commit"); err != nil {
		return CancelTransmissionResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return CancelTransmissionResult{}, err
	}
	return CancelTransmissionResult{
		Transmission: work.Transmission, Changed: changed, DisarmTargets: disarm,
	}, nil
}

// ReconcileTransmissionSchedulerRestart runs before the HTTP listener accepts
// new work. Future schedules retain their exact generation and T; prepared or
// active work whose node state cannot be proven is cancelled, and past
// schedules become stale without creating a fresh acceptance timestamp.
func (s *Store) ReconcileTransmissionSchedulerRestart(
	now int64,
) ([]CancelTransmissionResult, error) {
	if now <= 0 {
		return nil, ErrTransmissionInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT id FROM transmissions
WHERE completed_at = 0 AND effective_delivery IN ('overlay', 'interrupt')
ORDER BY accepted_at, id`)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	results := make([]CancelTransmissionResult, 0)
	for _, id := range ids {
		work, err := loadTransmissionSchedulerWorkTx(tx, id)
		if err != nil {
			return nil, err
		}
		unsafe := false
		for _, target := range work.Targets {
			if target.Status == TransmissionTargetPreparing ||
				target.Status == TransmissionTargetReady ||
				target.Status == TransmissionTargetPlaying {
				unsafe = true
				break
			}
		}
		eventAt := schedulerWorkEventTime(work, now)
		if unsafe {
			result, err := cancelTransmissionWorkTx(
				tx, work, TransmissionReasonCoordinatorRestarted, eventAt, true,
				func(TransmissionTarget) bool { return true },
			)
			if err != nil {
				return nil, err
			}
			if result.Changed || len(result.DisarmTargets) > 0 {
				results = append(results, result)
			}
			continue
		}
		changed := false
		var disarm []TransmissionTarget
		for i, target := range work.Targets {
			switch target.Status {
			case TransmissionTargetScheduled:
				if work.Scheduler.StartDeadlineCoordMS >= now {
					continue
				}
				updated, didChange, err := setTransmissionTargetStatusTx(
					tx, target, TransmissionTargetFailed, TransmissionReasonStalePlay,
					maxInt64(eventAt, target.UpdatedAt), false,
				)
				if err != nil {
					return nil, err
				}
				work.Targets[i] = updated
				changed = changed || didChange
				if didChange {
					disarm = append(disarm, updated)
				}
			case TransmissionTargetCancelling:
				disarm = append(disarm, target)
			}
		}
		if changed {
			work.Transmission, err = recomputeTransmissionTx(tx, id, eventAt)
			if err != nil {
				return nil, err
			}
		}
		if changed || len(disarm) > 0 {
			results = append(results, CancelTransmissionResult{
				Transmission: work.Transmission, Changed: changed, DisarmTargets: disarm,
			})
		}
	}
	if err := s.checkpoint("transmission_restart_reconcile_before_commit"); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return results, nil
}

func (s *Store) ClaimLegacyTransmission(
	transmissionID string,
	elementID string,
	now int64,
) (ClaimLegacyTransmissionResult, error) {
	if !transmissionIDPattern.MatchString(transmissionID) || elementID == "" ||
		len(elementID) > 64 || now <= 0 {
		return ClaimLegacyTransmissionResult{}, ErrTransmissionInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return ClaimLegacyTransmissionResult{}, err
	}
	defer tx.Rollback()
	work, err := loadTransmissionSchedulerWorkTx(tx, transmissionID)
	if err != nil {
		return ClaimLegacyTransmissionResult{}, err
	}
	if work.Transmission.EffectiveDelivery != TransmissionDeliveryAfterCurrent ||
		work.Transmission.CompletedAt != 0 || work.Transmission.CancellationCause != "" {
		return ClaimLegacyTransmissionResult{}, ErrTransmissionStateConflict
	}
	if work.Scheduler.LegacyElementID != "" {
		if work.Scheduler.LegacyElementID != elementID {
			return ClaimLegacyTransmissionResult{}, ErrTransmissionStateConflict
		}
		if err := tx.Commit(); err != nil {
			return ClaimLegacyTransmissionResult{}, err
		}
		var scheduled []TransmissionTarget
		for _, target := range work.Targets {
			if target.Status == TransmissionTargetScheduled {
				scheduled = append(scheduled, target)
			}
		}
		return ClaimLegacyTransmissionResult{Work: work, Targets: scheduled}, nil
	}
	changed := false
	for i, target := range work.Targets {
		if target.Status != TransmissionTargetAccepted {
			continue
		}
		updated, didChange, err := setTransmissionTargetStatusTx(
			tx, target, TransmissionTargetScheduled, "",
			maxInt64(now, target.UpdatedAt), false,
		)
		if err != nil {
			return ClaimLegacyTransmissionResult{}, err
		}
		work.Targets[i] = updated
		changed = changed || didChange
	}
	result, err := tx.Exec(`UPDATE transmission_scheduler_state
SET legacy_element_id = ?, revision = revision + 1, updated_at = ?
WHERE transmission_id = ? AND revision = ? AND legacy_element_id = ''`,
		elementID, now, transmissionID, work.Scheduler.Revision)
	if err != nil {
		return ClaimLegacyTransmissionResult{}, err
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		if err != nil {
			return ClaimLegacyTransmissionResult{}, err
		}
		return ClaimLegacyTransmissionResult{}, ErrTransmissionStateConflict
	}
	work.Scheduler.LegacyElementID = elementID
	work.Scheduler.Revision++
	work.Scheduler.UpdatedAt = now
	work.Transmission, err = recomputeTransmissionTx(tx, transmissionID, now)
	if err != nil {
		return ClaimLegacyTransmissionResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return ClaimLegacyTransmissionResult{}, err
	}
	var scheduled []TransmissionTarget
	for _, target := range work.Targets {
		if target.Status == TransmissionTargetScheduled {
			scheduled = append(scheduled, target)
		}
	}
	return ClaimLegacyTransmissionResult{
		Work: work, Targets: scheduled, Changed: true,
	}, nil
}

// TransmissionTargetForReceipt resolves only the immutable target addressed
// by the exact authenticated socket generation. A revoked slot keeps its
// binding digest long enough for that socket to acknowledge a cancellation;
// re-pairing changes both the digest and paired_at, so a replacement occupant
// cannot mutate the predecessor transmission.
func (s *Store) TransmissionTargetForReceipt(
	transmissionID string,
	orbitID int64,
	slot string,
	credentialTokenHash string,
) (*TransmissionTarget, error) {
	if !transmissionIDPattern.MatchString(transmissionID) || orbitID <= 0 ||
		!transmissionSlotPattern.MatchString(slot) ||
		!transmissionDigestPattern.MatchString(credentialTokenHash) {
		return nil, ErrTransmissionNotFound
	}
	target, err := scanTransmissionTarget(s.db.QueryRow(
		`SELECT tt.transmission_id, tt.orbit_id, tt.actor_id, tt.slot,
tt.binding_paired_at, tt.capability_set_hash, tt.resolved_at_ms, tt.online_at_acceptance,
tt.media_clip_capable, tt.overlay_capable, tt.interrupt_capable, tt.interrupt_resume_ready,
tt.status, tt.reason_code, tt.generation, tt.revision, tt.ready_at,
tt.scheduled_at, tt.started_at, tt.ended_at, tt.last_receipt_at, tt.updated_at
FROM transmission_targets tt
JOIN slots sl ON sl.orbit_id = tt.orbit_id AND sl.slot = tt.slot
  AND COALESCE(sl.paired_at, 0) = tt.binding_paired_at
  AND sl.token_hash = ?
WHERE tt.transmission_id = ? AND tt.orbit_id = ? AND tt.slot = ?`,
		credentialTokenHash, transmissionID, orbitID, slot,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &target, nil
}

// CurrentInstallationTarget resolves the actor currently bound to an
// authenticated orbit/slot address. The hub has already authenticated the
// socket; this repository lookup prevents a replaced slot from mutating the
// predecessor actor's DND row.
func (s *Store) CurrentInstallationTarget(
	orbitID int64,
	slot string,
) (*MediaTargetIdentity, error) {
	if orbitID <= 0 || !transmissionSlotPattern.MatchString(slot) {
		return nil, ErrTransmissionTargetInvalid
	}
	var target MediaTargetIdentity
	err := s.db.QueryRow(`SELECT ic.slot_orbit_id, ic.actor_id, ic.slot_name
FROM installation_credentials ic
JOIN actors a ON a.id = ic.actor_id AND a.revoked_at IS NULL
JOIN memberships m ON m.actor_id = ic.actor_id
  AND m.orbit_id = ic.slot_orbit_id AND m.left_at IS NULL
JOIN orbits o ON o.id = m.orbit_id AND o.status = 'active'
JOIN slots sl ON sl.orbit_id = ic.slot_orbit_id AND sl.slot = ic.slot_name
  AND sl.revoked_at IS NULL AND sl.token_hash = ic.binding_token_hash
  AND COALESCE(sl.paired_at, 0) = ic.slot_paired_at
WHERE ic.slot_orbit_id = ? AND ic.slot_name = ?`, orbitID, slot).Scan(
		&target.OrbitID, &target.ActorID, &target.Slot,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &target, nil
}

// CurrentInstallationTargetForSocket additionally proves that the caller is
// the still-current authenticated slot generation. A socket that remains open
// across revoke/re-pair cannot mutate the replacement installation's DND row.
func (s *Store) CurrentInstallationTargetForSocket(
	orbitID int64,
	slot string,
	credentialTokenHash string,
) (*MediaTargetIdentity, error) {
	if orbitID <= 0 || !transmissionSlotPattern.MatchString(slot) ||
		!transmissionDigestPattern.MatchString(credentialTokenHash) {
		return nil, ErrTransmissionTargetInvalid
	}
	var target MediaTargetIdentity
	err := s.db.QueryRow(`SELECT ic.slot_orbit_id, ic.actor_id, ic.slot_name
FROM installation_credentials ic
JOIN actors a ON a.id = ic.actor_id AND a.revoked_at IS NULL
JOIN memberships m ON m.actor_id = ic.actor_id
  AND m.orbit_id = ic.slot_orbit_id AND m.left_at IS NULL
JOIN orbits o ON o.id = m.orbit_id AND o.status = 'active'
JOIN slots sl ON sl.orbit_id = ic.slot_orbit_id AND sl.slot = ic.slot_name
  AND sl.revoked_at IS NULL AND sl.token_hash = ic.binding_token_hash
  AND COALESCE(sl.paired_at, 0) = ic.slot_paired_at
WHERE ic.slot_orbit_id = ? AND ic.slot_name = ?
  AND ic.binding_token_hash = ?`, orbitID, slot, credentialTokenHash).Scan(
		&target.OrbitID, &target.ActorID, &target.Slot,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &target, nil
}

func cancellationReasonForScheduler(reason TransmissionReason) bool {
	switch reason {
	case TransmissionReasonMediaDeleted, TransmissionReasonMediaExpired,
		TransmissionReasonModerationDisabled, TransmissionReasonApproachLeft,
		TransmissionReasonApproachApart, TransmissionReasonTargetRevoked,
		TransmissionReasonDNDEnabled, TransmissionReasonSenderBlocked,
		TransmissionReasonReported,
		TransmissionReasonCoordinatorRestarted,
		TransmissionReasonAutomationDisabled,
		TransmissionReasonPrincipalRevoked,
		TransmissionReasonScheduleDisabled:
		return true
	default:
		return false
	}
}

func cancelTransmissionWorkTx(
	tx *sql.Tx,
	work TransmissionSchedulerWork,
	reason TransmissionReason,
	now int64,
	wholeTransmission bool,
	match func(TransmissionTarget) bool,
) (CancelTransmissionResult, error) {
	if work.Transmission.CompletedAt != 0 {
		return CancelTransmissionResult{Transmission: work.Transmission}, nil
	}
	now = schedulerWorkEventTime(work, now)
	changed := false
	if wholeTransmission {
		if work.Transmission.CancellationCause != "" &&
			work.Transmission.CancellationCause != reason {
			return CancelTransmissionResult{Transmission: work.Transmission}, nil
		}
		if work.Transmission.CancellationCause == "" {
			result, err := tx.Exec(`UPDATE transmissions
SET cancellation_cause = ?, revision = revision + 1, updated_at = ?
WHERE id = ? AND revision = ? AND cancellation_cause = '' AND completed_at = 0`,
				reason, now, work.Transmission.ID, work.Transmission.Revision)
			if err != nil {
				return CancelTransmissionResult{}, err
			}
			if count, err := result.RowsAffected(); err != nil || count != 1 {
				if err != nil {
					return CancelTransmissionResult{}, err
				}
				return CancelTransmissionResult{}, ErrTransmissionStateConflict
			}
			work.Transmission.CancellationCause = reason
			work.Transmission.Revision++
			work.Transmission.UpdatedAt = now
			changed = true
		}
	}
	var disarm []TransmissionTarget
	for i, target := range work.Targets {
		if !match(target) {
			continue
		}
		status := TransmissionTargetStatus("")
		switch target.Status {
		case TransmissionTargetAccepted:
			status = TransmissionTargetCancelled
		case TransmissionTargetPreparing, TransmissionTargetReady,
			TransmissionTargetScheduled, TransmissionTargetPlaying:
			status = TransmissionTargetCancelling
		case TransmissionTargetCancelling:
			disarm = append(disarm, target)
			continue
		default:
			continue
		}
		updated, didChange, err := setTransmissionTargetStatusTx(
			tx, target, status, reason, maxInt64(now, target.UpdatedAt), false,
		)
		if err != nil {
			return CancelTransmissionResult{}, err
		}
		work.Targets[i] = updated
		changed = changed || didChange
		if updated.Status == TransmissionTargetCancelling {
			disarm = append(disarm, updated)
		}
	}
	transmission, err := recomputeTransmissionTx(tx, work.Transmission.ID, now)
	if err != nil {
		return CancelTransmissionResult{}, err
	}
	return CancelTransmissionResult{
		Transmission: transmission, Changed: changed, DisarmTargets: disarm,
	}, nil
}

func cancelSchedulerWork(
	s *Store,
	ids []string,
	reason TransmissionReason,
	now int64,
	wholeTransmission bool,
	match func(TransmissionTarget) bool,
) ([]CancelTransmissionResult, error) {
	if now <= 0 || !cancellationReasonForScheduler(reason) {
		return nil, ErrTransmissionInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	results := make([]CancelTransmissionResult, 0, len(ids))
	for _, id := range ids {
		work, err := loadTransmissionSchedulerWorkTx(tx, id)
		if err != nil {
			return nil, err
		}
		result, err := cancelTransmissionWorkTx(
			tx, work, reason, now, wholeTransmission, match,
		)
		if err != nil {
			return nil, err
		}
		if result.Changed || len(result.DisarmTargets) > 0 {
			results = append(results, result)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return results, nil
}

func schedulerTransmissionIDs(
	s *Store,
	query string,
	args ...any,
) ([]string, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) CancelTransmissionsForMedia(
	mediaID string,
	reason TransmissionReason,
	now int64,
) ([]CancelTransmissionResult, error) {
	if mediaID == "" || len(mediaID) > 128 {
		return nil, ErrTransmissionInvalid
	}
	// The lifecycle sink is shared with predecessor legacy media IDs. Those
	// cannot be referenced by the generic transmission FK and are therefore a
	// valid no-op while the Session FSM performs its compatibility cleanup.
	if !mediaItemIDPattern.MatchString(mediaID) {
		return nil, nil
	}
	ids, err := schedulerTransmissionIDs(s, `SELECT id FROM transmissions
WHERE media_id = ? AND completed_at = 0 ORDER BY accepted_at, id`, mediaID)
	if err != nil {
		return nil, err
	}
	return cancelSchedulerWork(
		s, ids, reason, now, true, func(TransmissionTarget) bool { return true },
	)
}

func (s *Store) CancelTransmissionsForSourceActor(
	actorID int64,
	reason TransmissionReason,
	now int64,
) ([]CancelTransmissionResult, error) {
	if actorID <= 0 {
		return nil, ErrTransmissionInvalid
	}
	ids, err := schedulerTransmissionIDs(s, `SELECT id FROM transmissions
WHERE source_actor_id = ? AND completed_at = 0 ORDER BY accepted_at, id`, actorID)
	if err != nil {
		return nil, err
	}
	return cancelSchedulerWork(
		s, ids, reason, now, true, func(TransmissionTarget) bool { return true },
	)
}

func (s *Store) CancelTransmissionsForSourceOrbit(
	orbitID int64,
	reason TransmissionReason,
	now int64,
) ([]CancelTransmissionResult, error) {
	if orbitID <= 0 {
		return nil, ErrTransmissionInvalid
	}
	ids, err := schedulerTransmissionIDs(s, `SELECT id FROM transmissions
WHERE source_orbit_id = ? AND completed_at = 0 ORDER BY accepted_at, id`, orbitID)
	if err != nil {
		return nil, err
	}
	return cancelSchedulerWork(
		s, ids, reason, now, true, func(TransmissionTarget) bool { return true },
	)
}

func (s *Store) CancelTransmissionPlaybackDomain(
	domainKind PlaybackDomainKind,
	domainID int64,
	reason TransmissionReason,
	now int64,
) ([]CancelTransmissionResult, error) {
	if (domainKind != PlaybackDomainOrbit && domainKind != PlaybackDomainApproach) ||
		domainID <= 0 {
		return nil, ErrTransmissionInvalid
	}
	ids, err := schedulerTransmissionIDs(s, `SELECT id FROM transmissions
WHERE playback_domain_kind = ? AND playback_domain_id = ? AND completed_at = 0
ORDER BY accepted_at, id`, domainKind, domainID)
	if err != nil {
		return nil, err
	}
	match := func(TransmissionTarget) bool { return true }
	if domainKind == PlaybackDomainApproach &&
		(reason == TransmissionReasonApproachApart || reason == TransmissionReasonApproachLeft) {
		// A domain split cannot retroactively move accepted work into two orbit
		// controllers. Non-started rows are disarmed; audio which durably won T
		// is allowed to reach its ordinary end receipt in the old controller.
		match = func(target TransmissionTarget) bool {
			return target.Status != TransmissionTargetPlaying
		}
	}
	return cancelSchedulerWork(
		s, ids, reason, now, true, match,
	)
}

func (s *Store) CancelTransmissionNode(
	orbitID, actorID int64,
	slot string,
	reason TransmissionReason,
	now int64,
) ([]CancelTransmissionResult, error) {
	if orbitID <= 0 || actorID <= 0 || !transmissionSlotPattern.MatchString(slot) {
		return nil, ErrTransmissionInvalid
	}
	ids, err := schedulerTransmissionIDs(s, `SELECT t.id
FROM transmissions t
JOIN transmission_targets tt ON tt.transmission_id = t.id
WHERE tt.orbit_id = ? AND tt.actor_id = ? AND tt.slot = ?
  AND t.completed_at = 0 ORDER BY t.accepted_at, t.id`, orbitID, actorID, slot)
	if err != nil {
		return nil, err
	}
	return cancelSchedulerWork(
		s, ids, reason, now, false,
		func(target TransmissionTarget) bool {
			return target.OrbitID == orbitID && target.ActorID == actorID && target.Slot == slot
		},
	)
}

// CancelTransmissionsFromSourceActorToNode is the scheduler enforcement seam
// for a recipient's canonical actor block. It disarms only the blocked
// sender's accepted work for the exact recipient generation; unrelated media
// queued for the same node is preserved.
func (s *Store) CancelTransmissionsFromSourceActorToNode(
	sourceActorID, recipientOrbitID, recipientActorID int64,
	recipientSlot string,
	reason TransmissionReason,
	now int64,
) ([]CancelTransmissionResult, error) {
	if sourceActorID <= 0 || recipientOrbitID <= 0 || recipientActorID <= 0 ||
		!transmissionSlotPattern.MatchString(recipientSlot) {
		return nil, ErrTransmissionInvalid
	}
	ids, err := schedulerTransmissionIDs(s, `SELECT t.id
FROM transmissions t
JOIN transmission_targets tt ON tt.transmission_id = t.id
WHERE t.source_actor_id = ? AND tt.orbit_id = ? AND tt.actor_id = ?
  AND tt.slot = ? AND t.completed_at = 0
ORDER BY t.accepted_at, t.id`, sourceActorID, recipientOrbitID,
		recipientActorID, recipientSlot)
	if err != nil {
		return nil, err
	}
	return cancelSchedulerWork(
		s, ids, reason, now, false,
		func(target TransmissionTarget) bool {
			return target.OrbitID == recipientOrbitID &&
				target.ActorID == recipientActorID && target.Slot == recipientSlot
		},
	)
}

// CancelTransmissionsForMediaToActor is the report-local scheduler seam. It
// disarms only the reported media for the reporting actor; evidence targets
// owned by a companion and every other recipient keep their independent state.
func (s *Store) CancelTransmissionsForMediaToActor(
	mediaID string,
	recipientActorID int64,
	reason TransmissionReason,
	now int64,
) ([]CancelTransmissionResult, error) {
	if !mediaItemIDPattern.MatchString(mediaID) || recipientActorID <= 0 {
		return nil, ErrTransmissionInvalid
	}
	ids, err := schedulerTransmissionIDs(s, `SELECT t.id
FROM transmissions t
JOIN transmission_targets tt ON tt.transmission_id = t.id
WHERE t.media_id = ? AND tt.actor_id = ? AND t.completed_at = 0
ORDER BY t.accepted_at, t.id`, mediaID, recipientActorID)
	if err != nil {
		return nil, err
	}
	return cancelSchedulerWork(
		s, ids, reason, now, false,
		func(target TransmissionTarget) bool {
			return target.ActorID == recipientActorID
		},
	)
}

// CancelTransmissionsFromSourceOrbitToNode is the orbit-block counterpart of
// CancelTransmissionsFromSourceActorToNode. Unblocking deliberately has no
// inverse operation: previously disarmed work is never resurrected.
func (s *Store) CancelTransmissionsFromSourceOrbitToNode(
	sourceOrbitID, recipientOrbitID, recipientActorID int64,
	recipientSlot string,
	reason TransmissionReason,
	now int64,
) ([]CancelTransmissionResult, error) {
	if sourceOrbitID <= 0 || recipientOrbitID <= 0 || recipientActorID <= 0 ||
		!transmissionSlotPattern.MatchString(recipientSlot) {
		return nil, ErrTransmissionInvalid
	}
	ids, err := schedulerTransmissionIDs(s, `SELECT t.id
FROM transmissions t
JOIN transmission_targets tt ON tt.transmission_id = t.id
WHERE t.source_orbit_id = ? AND tt.orbit_id = ? AND tt.actor_id = ?
  AND tt.slot = ? AND t.completed_at = 0
ORDER BY t.accepted_at, t.id`, sourceOrbitID, recipientOrbitID,
		recipientActorID, recipientSlot)
	if err != nil {
		return nil, err
	}
	return cancelSchedulerWork(
		s, ids, reason, now, false,
		func(target TransmissionTarget) bool {
			return target.OrbitID == recipientOrbitID &&
				target.ActorID == recipientActorID && target.Slot == recipientSlot
		},
	)
}
