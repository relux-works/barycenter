package store

import (
	"database/sql"
	"encoding/hex"
	"errors"
	"sort"

	"relux.works/duet/coordinator/internal/e2eecontract"
	"relux.works/duet/coordinator/internal/protocol"
)

const (
	e2eeOpaqueLiveBurstFrames     int64 = 8
	e2eeOpaqueLiveMaxTerminalRows       = 1000
)

var (
	ErrE2EELiveBusy         = errors.New("E2EE opaque live target busy")
	ErrE2EELiveNotReady     = errors.New("E2EE opaque live target unavailable")
	ErrE2EELiveRateExceeded = errors.New("E2EE opaque live rate exceeded")
)

type StartE2EEOpaqueLiveParams struct {
	SessionID, GroupID, AuthorDeviceID, TargetSnapshotDigest string
	HeaderDigest                                             string
	Epoch, Generation, StartedAt, ExpiresAt                  int64
	OpaqueHeader                                             []byte
}

type E2EEOpaqueLiveSession struct {
	SessionID, GroupID, AuthorDeviceID, TargetSnapshotDigest string
	HeaderDigest, State, TerminalReason, LastFrameDigest     string
	Epoch, Generation, LastSequence, LastCaptureUS           int64
	RateTokensMilli, RateAt, RelayedBytes, Revision          int64
	StartedAt, ExpiresAt, UpdatedAt, EndedAt                 int64
	OpaqueHeader                                             []byte
}

type E2EEOpaqueLiveRelay struct {
	SessionID          string
	Sequence           int64
	RecipientDeviceIDs []string
	OpaqueFrame        []byte
}

func scanE2EEOpaqueLiveSession(scanner sqlScanner) (E2EEOpaqueLiveSession, error) {
	var value E2EEOpaqueLiveSession
	err := scanner.Scan(&value.SessionID, &value.GroupID, &value.AuthorDeviceID,
		&value.TargetSnapshotDigest, &value.HeaderDigest, &value.State,
		&value.TerminalReason, &value.LastFrameDigest, &value.Epoch,
		&value.Generation, &value.LastSequence, &value.LastCaptureUS,
		&value.RateTokensMilli, &value.RateAt, &value.RelayedBytes,
		&value.Revision, &value.StartedAt, &value.ExpiresAt, &value.UpdatedAt,
		&value.EndedAt, &value.OpaqueHeader)
	if err == nil {
		value.OpaqueHeader = append([]byte(nil), value.OpaqueHeader...)
	}
	return value, err
}

const e2eeOpaqueLiveColumns = `session_id, group_id, author_device_id,
target_snapshot_digest, header_digest, state, terminal_reason, last_frame_digest,
epoch, generation, last_sequence, last_capture_us, rate_tokens_milli, rate_at,
relayed_bytes, revision, started_at, expires_at, updated_at, ended_at, opaque_header`

func (s *Store) StartE2EEOpaqueLiveSession(params StartE2EEOpaqueLiveParams) (E2EEOpaqueLiveSession, []string, error) {
	if _, err := protocol.ParseLivePTTSessionID(params.SessionID); err != nil ||
		len(params.GroupID) != 30 || len(params.AuthorDeviceID) < 8 ||
		params.Epoch <= 0 || params.Generation <= 0 || params.StartedAt <= 0 ||
		params.ExpiresAt <= params.StartedAt ||
		params.ExpiresAt-params.StartedAt > protocol.LivePTTMaxDurationMS ||
		!validE2EEDigest(params.TargetSnapshotDigest) ||
		!validE2EEDigest(params.HeaderDigest) || len(params.OpaqueHeader) == 0 ||
		len(params.OpaqueHeader) > 4096 ||
		!payloadDigestMatches(params.OpaqueHeader, params.HeaderDigest) {
		return E2EEOpaqueLiveSession{}, nil, ErrE2EEInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return E2EEOpaqueLiveSession{}, nil, err
	}
	defer tx.Rollback()
	group, err := e2eeGroupTx(tx, params.GroupID)
	if err != nil {
		return E2EEOpaqueLiveSession{}, nil, err
	}
	requirement, err := reconcileE2EERotationTx(tx, group, params.StartedAt)
	if err != nil {
		return E2EEOpaqueLiveSession{}, nil, err
	}
	if requirement != nil && requirement.State == "required" {
		return E2EEOpaqueLiveSession{}, nil, ErrE2EERotationRequired
	}
	if group.ForkState != "clean" || group.CurrentEpoch != params.Epoch ||
		group.TargetSnapshotDigest != params.TargetSnapshotDigest {
		return E2EEOpaqueLiveSession{}, nil, ErrE2EEStaleEpoch
	}
	var protocolActorID string
	if err := tx.QueryRow(`SELECT protocol_actor_id FROM e2ee_protocol_actor_bindings
WHERE device_id = ?`, params.AuthorDeviceID).Scan(&protocolActorID); err != nil {
		return E2EEOpaqueLiveSession{}, nil, ErrE2EEInvalid
	}
	if _, err := authorizedE2EEGroupMemberTx(tx, group, params.AuthorDeviceID,
		protocolActorID); err != nil {
		return E2EEOpaqueLiveSession{}, nil, ErrE2EEInvalid
	}
	var latestGeneration int64
	if err := tx.QueryRow(`SELECT COALESCE(MAX(generation), 0)
FROM e2ee_opaque_live_sessions WHERE group_id = ? AND author_device_id = ?`,
		group.ID, params.AuthorDeviceID).Scan(&latestGeneration); err != nil {
		return E2EEOpaqueLiveSession{}, nil, err
	}
	if params.Generation <= latestGeneration {
		return E2EEOpaqueLiveSession{}, nil, ErrE2EEReplay
	}
	var active int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM e2ee_opaque_live_sessions
WHERE group_id = ? AND state = 'active'`, group.ID).Scan(&active); err != nil {
		return E2EEOpaqueLiveSession{}, nil, err
	}
	if active != 0 {
		return E2EEOpaqueLiveSession{}, nil, ErrE2EELiveBusy
	}
	members, err := e2eeCurrentMembersTx(tx, group.ID)
	if err != nil {
		return E2EEOpaqueLiveSession{}, nil, err
	}
	var recipients []string
	for _, member := range members {
		if member.DeviceID != params.AuthorDeviceID {
			recipients = append(recipients, member.DeviceID)
		}
	}
	if len(recipients) == 0 || len(recipients) > protocol.LivePTTMaxTargets {
		return E2EEOpaqueLiveSession{}, nil, ErrE2EELiveNotReady
	}
	if _, err := tx.Exec(`INSERT INTO e2ee_opaque_live_sessions(
session_id, group_id, author_device_id, epoch, generation,
target_snapshot_digest, header_digest, opaque_header, state, terminal_reason,
last_sequence, last_capture_us, last_frame_digest, rate_tokens_milli, rate_at,
relayed_bytes, revision, started_at, expires_at, updated_at, ended_at
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, 'active', '', 0, 0, '', ?, ?, 0, 1, ?, ?, ?, 0)`,
		params.SessionID, group.ID, params.AuthorDeviceID, params.Epoch,
		params.Generation, params.TargetSnapshotDigest, params.HeaderDigest,
		params.OpaqueHeader, e2eeOpaqueLiveBurstFrames*1000, params.StartedAt,
		params.StartedAt, params.ExpiresAt, params.StartedAt); err != nil {
		return E2EEOpaqueLiveSession{}, nil, ErrE2EEConflict
	}
	for _, member := range members {
		if member.DeviceID == params.AuthorDeviceID {
			continue
		}
		if _, err := tx.Exec(`INSERT INTO e2ee_opaque_live_recipients(
session_id, recipient_device_id, actor_id, protocol_actor_id,
actor_membership_joined_at, air_membership_id, air_membership_revision,
state, terminal_reason, revision, created_at, updated_at, ended_at
) VALUES(?, ?, ?, ?, ?, ?, ?, 'active', '', 1, ?, ?, 0)`, params.SessionID,
			member.DeviceID, member.ActorID, member.ProtocolActorID,
			member.ActorMembershipJoinedAt, member.AirMembershipID,
			member.AirMembershipRevision, params.StartedAt, params.StartedAt); err != nil {
			return E2EEOpaqueLiveSession{}, nil, err
		}
	}
	if err := appendE2EEAuditTx(tx, group.ID, "public_event", params.SessionID,
		"opaque_live.start", "accepted", "", 0, params.AuthorDeviceID,
		params.Epoch, 1, params.StartedAt); err != nil {
		return E2EEOpaqueLiveSession{}, nil, err
	}
	value, err := scanE2EEOpaqueLiveSession(tx.QueryRow(
		`SELECT `+e2eeOpaqueLiveColumns+` FROM e2ee_opaque_live_sessions WHERE session_id = ?`,
		params.SessionID))
	if err != nil {
		return E2EEOpaqueLiveSession{}, nil, err
	}
	if err := tx.Commit(); err != nil {
		return E2EEOpaqueLiveSession{}, nil, err
	}
	sort.Strings(recipients)
	return value, recipients, nil
}

func terminalE2EEOpaqueLiveTx(tx *sql.Tx, session E2EEOpaqueLiveSession, reason string, now int64) error {
	if !validateOpaqueReason(reason) || now <= 0 {
		return ErrE2EEInvalid
	}
	if session.State == "terminal" {
		return nil
	}
	if _, err := tx.Exec(`UPDATE e2ee_opaque_live_recipients
SET state = 'terminal', terminal_reason = ?, revision = revision + 1,
updated_at = ?, ended_at = ? WHERE session_id = ? AND state = 'active'`,
		reason, now, now, session.SessionID); err != nil {
		return err
	}
	result, err := tx.Exec(`UPDATE e2ee_opaque_live_sessions
SET state = 'terminal', terminal_reason = ?, revision = revision + 1,
updated_at = ?, ended_at = ? WHERE session_id = ? AND state = 'active'`,
		reason, now, now, session.SessionID)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return ErrE2EEConflict
	}
	return appendE2EEAuditTx(tx, session.GroupID, "public_event", session.SessionID,
		"opaque_live.terminal", "revoked", reason, 0, session.AuthorDeviceID,
		session.Epoch, session.Revision+1, now)
}

func (s *Store) RelayE2EEOpaqueLiveFrame(sessionID, authorDeviceID string, raw []byte, now int64) (E2EEOpaqueLiveRelay, error) {
	frame, err := e2eecontract.DecodeOpaqueLiveFrame(raw)
	if err != nil || now <= 0 || len(authorDeviceID) < 8 ||
		hex.EncodeToString(frame.SessionID[:]) != sessionID {
		return E2EEOpaqueLiveRelay{}, ErrE2EEInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return E2EEOpaqueLiveRelay{}, err
	}
	defer tx.Rollback()
	session, err := scanE2EEOpaqueLiveSession(tx.QueryRow(
		`SELECT `+e2eeOpaqueLiveColumns+` FROM e2ee_opaque_live_sessions WHERE session_id = ?`, sessionID))
	if errors.Is(err, sql.ErrNoRows) {
		return E2EEOpaqueLiveRelay{}, ErrE2EENotFound
	}
	if err != nil {
		return E2EEOpaqueLiveRelay{}, err
	}
	if session.State != "active" || session.AuthorDeviceID != authorDeviceID {
		return E2EEOpaqueLiveRelay{}, ErrE2EERevoked
	}
	if now >= session.ExpiresAt {
		if err := terminalE2EEOpaqueLiveTx(tx, session, "timeout", now); err != nil {
			return E2EEOpaqueLiveRelay{}, err
		}
		if err := tx.Commit(); err != nil {
			return E2EEOpaqueLiveRelay{}, err
		}
		return E2EEOpaqueLiveRelay{}, ErrE2EERevoked
	}
	group, err := e2eeGroupTx(tx, session.GroupID)
	if err != nil {
		return E2EEOpaqueLiveRelay{}, err
	}
	requirement, err := reconcileE2EERotationTx(tx, group, now)
	if err != nil {
		return E2EEOpaqueLiveRelay{}, err
	}
	if requirement != nil && requirement.State == "required" {
		if err := terminalE2EEOpaqueLiveTx(tx, session, "membership_changed", now); err != nil {
			return E2EEOpaqueLiveRelay{}, err
		}
		if err := tx.Commit(); err != nil {
			return E2EEOpaqueLiveRelay{}, err
		}
		return E2EEOpaqueLiveRelay{}, ErrE2EERotationRequired
	}
	if group.ForkState != "clean" || group.CurrentEpoch != session.Epoch ||
		group.TargetSnapshotDigest != session.TargetSnapshotDigest {
		return E2EEOpaqueLiveRelay{}, ErrE2EEStaleEpoch
	}
	if int64(frame.Epoch) != session.Epoch || int64(frame.Generation) != session.Generation ||
		frame.TargetSnapshotDigest != session.TargetSnapshotDigest {
		return E2EEOpaqueLiveRelay{}, ErrE2EEStaleEpoch
	}
	digest := e2eeFrameDigest(raw)
	if int64(frame.Sequence) <= session.LastSequence {
		if int64(frame.Sequence) != session.LastSequence || digest != session.LastFrameDigest {
			return E2EEOpaqueLiveRelay{}, ErrE2EEReplay
		}
		if err := tx.Commit(); err != nil {
			return E2EEOpaqueLiveRelay{}, err
		}
		return E2EEOpaqueLiveRelay{SessionID: session.SessionID,
			Sequence: int64(frame.Sequence)}, nil
	}
	if session.LastSequence == 0 {
		if frame.Sequence != 1 {
			return E2EEOpaqueLiveRelay{}, ErrE2EEInvalid
		}
	} else {
		gap := int64(frame.Sequence) - session.LastSequence
		if gap <= 0 || gap > int64(e2eecontract.OpaqueLiveMaxGapFrames) ||
			int64(frame.CaptureMonotonicUS) <= session.LastCaptureUS ||
			int64(frame.CaptureMonotonicUS)-session.LastCaptureUS !=
				gap*e2eecontract.OpaqueLiveFrameMS*1000 {
			return E2EEOpaqueLiveRelay{}, ErrE2EEInvalid
		}
	}
	if frame.Sequence > uint32(e2eecontract.OpaqueLiveMaxDurationMS/e2eecontract.OpaqueLiveFrameMS) {
		return E2EEOpaqueLiveRelay{}, ErrE2EEInvalid
	}
	rateAt := session.RateAt
	if now < rateAt {
		now = rateAt
	}
	tokens := session.RateTokensMilli + (now-rateAt)*protocol.LivePTTMaxFramesPerSecond
	if tokens > e2eeOpaqueLiveBurstFrames*1000 {
		tokens = e2eeOpaqueLiveBurstFrames * 1000
	}
	if tokens < 1000 {
		return E2EEOpaqueLiveRelay{}, ErrE2EELiveRateExceeded
	}
	tokens -= 1000
	rows, err := tx.Query(`SELECT r.recipient_device_id
FROM e2ee_opaque_live_recipients r
JOIN e2ee_group_members gm ON gm.group_id = ? AND gm.device_id = r.recipient_device_id
  AND gm.state = 'current' AND gm.actor_id = r.actor_id
  AND gm.protocol_actor_id = r.protocol_actor_id
  AND gm.actor_membership_joined_at = r.actor_membership_joined_at
  AND gm.air_membership_id = r.air_membership_id
  AND gm.air_membership_revision = r.air_membership_revision
JOIN e2ee_device_public_state d ON d.device_id = r.recipient_device_id
  AND d.verification_state = 'verified' AND d.revoked_at = 0
JOIN actors a ON a.id = r.actor_id AND a.revoked_at IS NULL
WHERE r.session_id = ? AND r.state = 'active'
ORDER BY r.recipient_device_id`, session.GroupID, session.SessionID)
	if err != nil {
		return E2EEOpaqueLiveRelay{}, err
	}
	var recipients []string
	for rows.Next() {
		var deviceID string
		if err := rows.Scan(&deviceID); err != nil {
			rows.Close()
			return E2EEOpaqueLiveRelay{}, err
		}
		recipients = append(recipients, deviceID)
	}
	if err := rows.Close(); err != nil {
		return E2EEOpaqueLiveRelay{}, err
	}
	if len(recipients) == 0 {
		if err := terminalE2EEOpaqueLiveTx(tx, session, "no_targets", now); err != nil {
			return E2EEOpaqueLiveRelay{}, err
		}
		if err := tx.Commit(); err != nil {
			return E2EEOpaqueLiveRelay{}, err
		}
		return E2EEOpaqueLiveRelay{}, ErrE2EELiveNotReady
	}
	state, reason, endedAt := "active", "", int64(0)
	if frame.Flags&e2eecontract.OpaqueLiveFlagEnd != 0 {
		state, reason, endedAt = "terminal", "sender_end", now
	}
	result, err := tx.Exec(`UPDATE e2ee_opaque_live_sessions
SET state = ?, terminal_reason = ?, last_sequence = ?, last_capture_us = ?,
last_frame_digest = ?, rate_tokens_milli = ?, rate_at = ?,
relayed_bytes = relayed_bytes + ?, revision = revision + 1,
updated_at = ?, ended_at = ?
WHERE session_id = ? AND state = 'active' AND revision = ?`, state, reason,
		frame.Sequence, frame.CaptureMonotonicUS, digest, tokens, now, len(raw),
		now, endedAt, session.SessionID, session.Revision)
	if err != nil {
		return E2EEOpaqueLiveRelay{}, err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		if err != nil {
			return E2EEOpaqueLiveRelay{}, err
		}
		return E2EEOpaqueLiveRelay{}, ErrE2EEConflict
	}
	if frame.Flags&e2eecontract.OpaqueLiveFlagEnd != 0 {
		if _, err := tx.Exec(`UPDATE e2ee_opaque_live_recipients
SET state = 'terminal', terminal_reason = 'sender_end', revision = revision + 1,
updated_at = ?, ended_at = ? WHERE session_id = ? AND state = 'active'`,
			now, now, session.SessionID); err != nil {
			return E2EEOpaqueLiveRelay{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return E2EEOpaqueLiveRelay{}, err
	}
	return E2EEOpaqueLiveRelay{SessionID: session.SessionID,
		Sequence: int64(frame.Sequence), RecipientDeviceIDs: recipients,
		OpaqueFrame: append([]byte(nil), raw...)}, nil
}

func (s *Store) MarkE2EEOpaqueLiveRecipientUnavailable(sessionID, recipientDeviceID, reason string, now int64) error {
	allowed := map[string]bool{"backpressure": true, "blocked": true, "dnd": true,
		"policy": true, "revoked": true, "unsupported": true}
	if _, err := protocol.ParseLivePTTSessionID(sessionID); err != nil ||
		len(recipientDeviceID) < 8 || !allowed[reason] || now <= 0 {
		return ErrE2EEInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`UPDATE e2ee_opaque_live_recipients
SET state = 'terminal', terminal_reason = ?, revision = revision + 1,
updated_at = ?, ended_at = ?
WHERE session_id = ? AND recipient_device_id = ? AND state = 'active'`,
		reason, now, now, sessionID, recipientDeviceID)
	if err != nil {
		return err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		if err != nil {
			return err
		}
		return ErrE2EELiveNotReady
	}
	var active int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM e2ee_opaque_live_recipients
WHERE session_id = ? AND state = 'active'`, sessionID).Scan(&active); err != nil {
		return err
	}
	if active == 0 {
		session, err := scanE2EEOpaqueLiveSession(tx.QueryRow(
			`SELECT `+e2eeOpaqueLiveColumns+` FROM e2ee_opaque_live_sessions WHERE session_id = ?`,
			sessionID))
		if err != nil {
			return err
		}
		if err := terminalE2EEOpaqueLiveTx(tx, session, "no_targets", now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) RecordE2EEOpaqueLiveReceipt(sessionID, recipientDeviceID string,
	eventSequence int64, state string, now int64,
) error {
	allowed := map[string]bool{"accepted": true, "audible_started": true,
		"ended": true, "failed": true, "rejected": true, "unsupported": true}
	if _, err := protocol.ParseLivePTTSessionID(sessionID); err != nil ||
		len(recipientDeviceID) < 8 || eventSequence <= 0 || eventSequence > 32 ||
		!allowed[state] || now <= 0 {
		return ErrE2EEInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var groupID string
	var lastSequence int64
	var lastState string
	err = tx.QueryRow(`SELECT s.group_id, r.last_event_sequence, r.last_receipt_state
FROM e2ee_opaque_live_recipients r
JOIN e2ee_opaque_live_sessions s ON s.session_id = r.session_id
JOIN e2ee_group_members gm ON gm.group_id = s.group_id
  AND gm.device_id = r.recipient_device_id AND gm.state = 'current'
  AND gm.actor_id = r.actor_id AND gm.protocol_actor_id = r.protocol_actor_id
  AND gm.actor_membership_joined_at = r.actor_membership_joined_at
  AND gm.air_membership_id = r.air_membership_id
  AND gm.air_membership_revision = r.air_membership_revision
JOIN e2ee_device_public_state d ON d.device_id = r.recipient_device_id
  AND d.verification_state = 'verified' AND d.revoked_at = 0
WHERE r.session_id = ? AND r.recipient_device_id = ?`, sessionID,
		recipientDeviceID).Scan(&groupID, &lastSequence, &lastState)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrE2EEInvalid
	}
	if err != nil {
		return err
	}
	if eventSequence < lastSequence {
		return ErrE2EEReplay
	}
	if eventSequence == lastSequence {
		if state != lastState {
			return ErrE2EEConflict
		}
		return tx.Commit()
	}
	result, err := tx.Exec(`UPDATE e2ee_opaque_live_recipients
SET last_event_sequence = ?, last_receipt_state = ?, last_receipt_at = ?,
revision = revision + 1, updated_at = CASE WHEN updated_at > ? THEN updated_at ELSE ? END
WHERE session_id = ? AND recipient_device_id = ? AND last_event_sequence = ?`,
		eventSequence, state, now, now, now, sessionID, recipientDeviceID, lastSequence)
	if err != nil {
		return err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		if err != nil {
			return err
		}
		return ErrE2EEConflict
	}
	if err := appendE2EEAuditTx(tx, groupID, "public_event", sessionID,
		"opaque_live.receipt", "accepted", state, 0, recipientDeviceID, 0,
		eventSequence, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ReconcileE2EEOpaqueLiveRestart(now int64) (int64, error) {
	if now <= 0 {
		return 0, ErrE2EEInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE e2ee_opaque_live_recipients
SET state = 'terminal', terminal_reason = 'coordinator_restart',
revision = revision + 1,
updated_at = CASE WHEN updated_at > ? THEN updated_at ELSE ? END,
ended_at = CASE WHEN updated_at > ? THEN updated_at ELSE ? END
WHERE state = 'active'`, now, now, now, now); err != nil {
		return 0, err
	}
	result, err := tx.Exec(`UPDATE e2ee_opaque_live_sessions
SET state = 'terminal', terminal_reason = 'coordinator_restart',
revision = revision + 1,
updated_at = CASE WHEN updated_at > ? THEN updated_at ELSE ? END,
ended_at = CASE WHEN updated_at > ? THEN updated_at ELSE ? END
WHERE state = 'active'`, now, now, now, now)
	if err != nil {
		return 0, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return changed, nil
}

func (s *Store) PruneE2EEOpaqueLiveSessions(cutoff int64, limit int) (int64, error) {
	if cutoff <= 0 || limit <= 0 || limit > e2eeOpaqueLiveMaxTerminalRows {
		return 0, ErrE2EEInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT session_id FROM e2ee_opaque_live_sessions
WHERE state = 'terminal' AND ended_at < ? ORDER BY ended_at, session_id LIMIT ?`,
		cutoff, limit)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	for _, id := range ids {
		if _, err := tx.Exec(`DELETE FROM e2ee_opaque_live_recipients WHERE session_id = ?`, id); err != nil {
			return 0, err
		}
		if _, err := tx.Exec(`DELETE FROM e2ee_opaque_live_sessions WHERE session_id = ?`, id); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return int64(len(ids)), nil
}
