package store

import (
	"database/sql"
	"errors"
	"time"

	"relux.works/duet/coordinator/internal/ulid"
)

type ManualSoundboardTriggerParams struct {
	CueID        string
	Transmission CreateResolvedTransmissionParams
}

type ManualSoundboardTriggerResult struct {
	ExecutionID string
	ResolvedTransmissionCreation
}

func (s *Store) TriggerManualSoundboard(params ManualSoundboardTriggerParams) (ManualSoundboardTriggerResult, error) {
	if !savedCueIDPattern.MatchString(params.CueID) || params.Transmission.AcceptedAt <= 0 {
		return ManualSoundboardTriggerResult{}, ErrAutomationInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return ManualSoundboardTriggerResult{}, err
	}
	defer tx.Rollback()
	ctx, err := authorizeTransmissionControlTx(tx, params.Transmission.ExpectedActorID, params.Transmission)
	if err != nil {
		return ManualSoundboardTriggerResult{}, err
	}
	feature, err := scanAutomationFeatureState(tx.QueryRow(`SELECT `+automationFeatureColumns+`
FROM automation_feature_state WHERE owner_orbit_id = ?`, ctx.OrbitID))
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !feature.SoundboardEnabled) {
		return ManualSoundboardTriggerResult{}, ErrAutomationDisabled
	}
	if err != nil {
		return ManualSoundboardTriggerResult{}, err
	}
	cue, mediaItem, err := automationRuntimeCueTx(tx, params.CueID, ctx.OrbitID, params.Transmission.AcceptedAt)
	if err != nil {
		return ManualSoundboardTriggerResult{}, err
	}
	params.Transmission.MediaID = mediaItem.ID
	params.Transmission.OriginKind = automationTransmissionOrigin(mediaItem)
	if !validResolvedTransmissionParams(params.Transmission) {
		return ManualSoundboardTriggerResult{}, ErrTransmissionInvalid
	}
	resolved, err := s.createResolvedTransmissionTx(tx, ctx, params.Transmission)
	if err != nil {
		return ManualSoundboardTriggerResult{}, err
	}
	if resolved.Challenge != nil {
		if err := tx.Commit(); err != nil {
			return ManualSoundboardTriggerResult{}, err
		}
		return ManualSoundboardTriggerResult{ResolvedTransmissionCreation: resolved}, nil
	}
	var executionID string
	if resolved.Reused {
		if err := tx.QueryRow(`SELECT id FROM manual_soundboard_executions
WHERE transmission_id = ?`, resolved.Creation.Transmission.ID).Scan(&executionID); err != nil {
			return ManualSoundboardTriggerResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return ManualSoundboardTriggerResult{}, err
		}
		return ManualSoundboardTriggerResult{ExecutionID: executionID,
			ResolvedTransmissionCreation: resolved}, nil
	}
	executionID = ulid.NewManualSoundboardExecutionID(time.UnixMilli(params.Transmission.AcceptedAt))
	_, err = tx.Exec(`INSERT INTO manual_soundboard_executions(
  id, transmission_id, owner_orbit_id, actor_id, cue_id, cue_revision,
  cue_source_generation, feature_revision, audience_kind, delivery,
  resolved_target_count, created_at
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, executionID,
		resolved.Creation.Transmission.ID, ctx.OrbitID, ctx.ActorID, cue.ID,
		cue.Revision, cue.SourceGeneration, feature.Revision,
		params.Transmission.AudienceKind, resolved.Creation.Transmission.EffectiveDelivery,
		len(resolved.Creation.Targets), params.Transmission.AcceptedAt)
	if err != nil {
		return ManualSoundboardTriggerResult{}, err
	}
	if _, err := tx.Exec(`INSERT INTO automation_audit_events(
  event_kind, operation, owner_orbit_id, actor_id, execution_id,
  transmission_id, cue_id, cue_label, cue_revision, trigger_kind,
  audience_kind, resolved_target_count, outcome, accepted_at, terminal_at,
  created_at
) VALUES('trigger', 'automation.trigger.manual_soundboard.v1', ?, ?, ?, ?, ?, ?, ?,
  'manual_soundboard', ?, ?, 'accepted', ?, ?, ?)`, ctx.OrbitID, ctx.ActorID,
		executionID, resolved.Creation.Transmission.ID, cue.ID, cue.Title,
		cue.Revision, params.Transmission.AudienceKind, len(resolved.Creation.Targets),
		params.Transmission.AcceptedAt, params.Transmission.AcceptedAt,
		params.Transmission.AcceptedAt); err != nil {
		return ManualSoundboardTriggerResult{}, err
	}
	if err := s.checkpoint("manual_soundboard_trigger_before_commit"); err != nil {
		return ManualSoundboardTriggerResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return ManualSoundboardTriggerResult{}, err
	}
	return ManualSoundboardTriggerResult{ExecutionID: executionID,
		ResolvedTransmissionCreation: resolved}, nil
}
