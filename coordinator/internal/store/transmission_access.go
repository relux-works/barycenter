package store

import (
	"database/sql"
	"errors"
)

type AuthorizedTransmissionView struct {
	Transmission       Transmission
	Targets            []TransmissionTarget
	TargetCount        int
	TargetStatusCounts map[TransmissionTargetStatus]int
	CanCancel          bool
}

type CancelTransmissionResult struct {
	Transmission  Transmission
	Changed       bool
	DisarmTargets []TransmissionTarget
}

func authorizeTransmissionActorTx(
	tx *sql.Tx,
	expectedActorID int64,
	bearer string,
) (ActorContext, error) {
	ctx, err := resolveTokenActorContext(tx, bearer)
	if errors.Is(err, ErrUnauthorized) || ctx.ActorID != expectedActorID {
		return ActorContext{}, ErrUnauthorized
	}
	if err != nil {
		if errors.Is(err, ErrInsufficientCapability) || errors.Is(err, ErrOrbitDisabled) {
			return ActorContext{}, ErrInsufficientCapability
		}
		return ActorContext{}, err
	}
	if !ctx.Capabilities.Has(CapabilityNode) || ctx.ActorID <= 0 || ctx.OrbitID <= 0 {
		return ActorContext{}, ErrInsufficientCapability
	}
	return ctx, nil
}

func senderCancellationAllowed(
	transmission Transmission,
	targets []TransmissionTarget,
) bool {
	if transmission.CancellationCause != "" || transmission.CompletedAt != 0 {
		return false
	}
	for _, target := range targets {
		if target.Status == TransmissionTargetPlaying ||
			target.Status == TransmissionTargetPlayed ||
			target.Status == TransmissionTargetCancelling {
			return false
		}
	}
	return true
}

func targetMatchesCurrentBindingTx(
	tx *sql.Tx,
	target TransmissionTarget,
) (bool, error) {
	pairedAt, err := resolveTransmissionTargetBindingTx(tx, CreateTransmissionTarget{
		OrbitID: target.OrbitID, ActorID: target.ActorID, Slot: target.Slot,
	})
	if errors.Is(err, ErrTransmissionTargetInvalid) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return pairedAt == target.BindingPairedAt, nil
}

// GetAuthorizedTransmission returns aggregate state to current source-orbit
// actors and exact snapshotted actors, while applying the contract's narrower
// per-target visibility to everyone except the creator/current source primary.
func (s *Store) GetAuthorizedTransmission(
	expectedActorID int64,
	bearer string,
	transmissionID string,
) (AuthorizedTransmissionView, error) {
	if expectedActorID <= 0 || bearer == "" ||
		!transmissionIDPattern.MatchString(transmissionID) {
		return AuthorizedTransmissionView{}, ErrTransmissionNotFound
	}
	tx, err := s.db.Begin()
	if err != nil {
		return AuthorizedTransmissionView{}, err
	}
	defer tx.Rollback()
	ctx, err := authorizeTransmissionActorTx(tx, expectedActorID, bearer)
	if err != nil {
		return AuthorizedTransmissionView{}, err
	}
	creation, err := loadTransmissionCreationTx(tx, transmissionID)
	if errors.Is(err, ErrTransmissionNotFound) {
		return AuthorizedTransmissionView{}, ErrTransmissionNotFound
	}
	if err != nil {
		return AuthorizedTransmissionView{}, err
	}
	transmission := creation.Transmission
	showAll := ctx.ActorID == transmission.SourceActorID ||
		(ctx.OrbitID == transmission.SourceOrbitID && ctx.Role == "primary")
	authorizedAggregate := ctx.OrbitID == transmission.SourceOrbitID
	visible := make([]TransmissionTarget, 0, len(creation.Targets))
	for _, target := range creation.Targets {
		if showAll {
			visible = append(visible, target)
			continue
		}
		if target.ActorID != ctx.ActorID {
			continue
		}
		matches, err := targetMatchesCurrentBindingTx(tx, target)
		if err != nil {
			return AuthorizedTransmissionView{}, err
		}
		if matches {
			authorizedAggregate = true
			visible = append(visible, target)
		}
	}
	if !authorizedAggregate {
		return AuthorizedTransmissionView{}, ErrTransmissionNotFound
	}
	view := AuthorizedTransmissionView{
		Transmission:       transmission,
		Targets:            visible,
		TargetCount:        len(creation.Targets),
		TargetStatusCounts: make(map[TransmissionTargetStatus]int),
		CanCancel: showAll && ctx.Capabilities.Has(CapabilityControl) &&
			senderCancellationAllowed(transmission, creation.Targets),
	}
	for _, target := range creation.Targets {
		view.TargetStatusCounts[target.Status]++
	}
	if err := tx.Commit(); err != nil {
		return AuthorizedTransmissionView{}, err
	}
	return view, nil
}

// CancelAuthorizedTransmission decides the sender-cancel/start race and
// changes the transmission cause plus every eligible target in one writer
// transaction. Scheduler code consumes DisarmTargets to emit generation-bound
// cancel_media messages; accepted rows need no acknowledgement.
func (s *Store) CancelAuthorizedTransmission(
	expectedActorID int64,
	bearer string,
	transmissionID string,
	now int64,
) (CancelTransmissionResult, error) {
	if expectedActorID <= 0 || bearer == "" || now <= 0 ||
		!transmissionIDPattern.MatchString(transmissionID) {
		return CancelTransmissionResult{}, ErrTransmissionNotFound
	}
	tx, err := s.db.Begin()
	if err != nil {
		return CancelTransmissionResult{}, err
	}
	defer tx.Rollback()
	ctx, err := authorizeTransmissionControlTx(tx, expectedActorID,
		CreateResolvedTransmissionParams{Bearer: bearer})
	if err != nil {
		return CancelTransmissionResult{}, err
	}
	creation, err := loadTransmissionCreationTx(tx, transmissionID)
	if errors.Is(err, ErrTransmissionNotFound) {
		return CancelTransmissionResult{}, ErrTransmissionNotFound
	}
	if err != nil {
		return CancelTransmissionResult{}, err
	}
	transmission := creation.Transmission
	if ctx.ActorID != transmission.SourceActorID &&
		(ctx.OrbitID != transmission.SourceOrbitID || ctx.Role != "primary") {
		return CancelTransmissionResult{}, ErrTransmissionNotFound
	}
	if transmission.CancellationCause == TransmissionReasonSenderCancelled {
		if err := tx.Commit(); err != nil {
			return CancelTransmissionResult{}, err
		}
		return CancelTransmissionResult{Transmission: transmission}, nil
	}
	if now < transmission.UpdatedAt {
		return CancelTransmissionResult{}, ErrTransmissionStateConflict
	}
	for _, target := range creation.Targets {
		if now < target.UpdatedAt {
			return CancelTransmissionResult{}, ErrTransmissionStateConflict
		}
	}
	if !senderCancellationAllowed(transmission, creation.Targets) {
		return CancelTransmissionResult{}, ErrTransmissionStateConflict
	}
	result, err := tx.Exec(`UPDATE transmissions
SET cancellation_cause = ?, revision = revision + 1, updated_at = ?
WHERE id = ? AND revision = ? AND cancellation_cause = '' AND completed_at = 0`,
		TransmissionReasonSenderCancelled, now, transmissionID, transmission.Revision,
	)
	if err != nil {
		return CancelTransmissionResult{}, err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		if err != nil {
			return CancelTransmissionResult{}, err
		}
		return CancelTransmissionResult{}, ErrTransmissionStateConflict
	}
	var disarm []TransmissionTarget
	for _, target := range creation.Targets {
		status := target.Status
		switch target.Status {
		case TransmissionTargetAccepted:
			status = TransmissionTargetCancelled
		case TransmissionTargetPreparing, TransmissionTargetReady,
			TransmissionTargetScheduled:
			status = TransmissionTargetCancelling
		default:
			continue
		}
		endedAt := target.EndedAt
		if status == TransmissionTargetCancelled && endedAt == 0 {
			endedAt = now
		}
		update, err := tx.Exec(`UPDATE transmission_targets
SET status = ?, reason_code = ?, revision = revision + 1,
    ended_at = ?, updated_at = ?
WHERE transmission_id = ? AND orbit_id = ? AND actor_id = ? AND slot = ?
  AND revision = ? AND generation = ?`, status,
			TransmissionReasonSenderCancelled, endedAt, now,
			transmissionID, target.OrbitID, target.ActorID, target.Slot,
			target.Revision, target.Generation,
		)
		if err != nil {
			return CancelTransmissionResult{}, err
		}
		if changed, err := update.RowsAffected(); err != nil || changed != 1 {
			if err != nil {
				return CancelTransmissionResult{}, err
			}
			return CancelTransmissionResult{}, ErrTransmissionStateConflict
		}
		if status == TransmissionTargetCancelling {
			target.Status = status
			target.ReasonCode = TransmissionReasonSenderCancelled
			target.Revision++
			target.UpdatedAt = now
			disarm = append(disarm, target)
		}
	}
	transmission, err = recomputeTransmissionTx(tx, transmissionID, now)
	if err != nil {
		return CancelTransmissionResult{}, err
	}
	if _, err := tx.Exec(`INSERT INTO automation_audit_events(
  event_kind, operation, owner_orbit_id, actor_id, execution_id,
  transmission_id, outcome, reason_code, terminal_at, created_at
)
SELECT 'control', 'automation.execution.cancel.v1', owner_orbit_id, ?, id, ?,
  'accepted', 'sender_cancelled', ?, ?
FROM automation_executions WHERE transmission_id = ?`, ctx.ActorID,
		transmissionID, now, now, transmissionID); err != nil {
		return CancelTransmissionResult{}, err
	}
	if err := s.checkpoint("transmission_sender_cancel_before_commit"); err != nil {
		return CancelTransmissionResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return CancelTransmissionResult{}, err
	}
	return CancelTransmissionResult{
		Transmission: transmission, Changed: true, DisarmTargets: disarm,
	}, nil
}
