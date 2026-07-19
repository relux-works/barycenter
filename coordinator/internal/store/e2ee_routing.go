package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"

	"relux.works/duet/coordinator/internal/e2eecontract"
)

var (
	ErrE2EERotationRequired  = errors.New("E2EE epoch rotation required")
	ErrE2EEUnsupportedTarget = errors.New("E2EE target contains an unsupported installation")
	ErrE2EEDeliveryNotFound  = errors.New("E2EE group event delivery not found")
)

type E2EEGroupMember struct {
	GroupID, DeviceID, ProtocolActorID, ActorMembershipRole string
	AirMembershipID, AirRole, State                         string
	ActorID, ActorMembershipJoinedAt, OrbitID               int64
	AirMembershipRevision, AddedEpoch, RemovedEpoch         int64
	Revision, UpdatedAt                                     int64
}

type E2EEAirSnapshot struct {
	AirID               string
	Digest              string
	Members             []E2EEGroupMember
	UnsupportedActorIDs []int64
}

type E2EERotationRequirement struct {
	GroupID, ObservedSnapshotDigest, RequiredSnapshotDigest    string
	ReasonCode, State                                          string
	BaseEpoch, SatisfiedEpoch, Revision, DetectedAt, UpdatedAt int64
}

type E2EEGroupEventDelivery struct {
	EventID, GroupID, RecipientDeviceID, EventDigest, Kind, State string
	Epoch, Revision, CreatedAt, AcknowledgedAt, RevokedAt         int64
	PublicPayload                                                 []byte
}

type E2EEGroupEventRouteResult struct {
	Group      E2EEGroup
	EventID    string
	Kind       string
	Epoch      int64
	Recipients []string
}

type E2EEGroupEventCursor struct {
	CreatedAt int64
	EventID   string
}

func e2eeGroupTx(tx *sql.Tx, groupID string) (E2EEGroup, error) {
	var value E2EEGroup
	err := tx.QueryRow(`SELECT id, air_id, target_snapshot_digest, current_epoch,
commit_digest, fork_state, revision, created_at, updated_at
FROM e2ee_groups WHERE id = ?`, groupID).Scan(&value.ID, &value.AirID,
		&value.TargetSnapshotDigest, &value.CurrentEpoch, &value.CommitDigest,
		&value.ForkState, &value.Revision, &value.CreatedAt, &value.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return E2EEGroup{}, ErrE2EENotFound
	}
	return value, err
}

func e2eeAirSnapshotTx(tx *sql.Tx, airID string) (E2EEAirSnapshot, error) {
	var airStatus string
	if err := tx.QueryRow(`SELECT status FROM airs WHERE public_id = ?`, airID).Scan(&airStatus); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return E2EEAirSnapshot{}, ErrE2EENotFound
		}
		return E2EEAirSnapshot{}, err
	}
	if airStatus == "dissolved" {
		return E2EEAirSnapshot{}, ErrE2EERevoked
	}
	rows, err := tx.Query(`SELECT a.id, m.orbit_id,
  COALESCE(d.device_id, ''), COALESCE(b.protocol_actor_id, ''),
  COALESCE(d.public_package_digest, ''), COALESCE(d.verification_digest, ''),
  m.role, m.joined_at, am.public_id, am.air_role, am.revision,
  EXISTS(SELECT 1 FROM e2ee_device_public_state anyd
    WHERE anyd.actor_id = a.id AND anyd.verification_state <> 'revoked'),
  (SELECT COUNT(*) FROM e2ee_device_public_state anyd WHERE anyd.actor_id = a.id)
FROM air_members am
JOIN memberships m ON m.orbit_id = am.orbit_id AND m.left_at IS NULL
JOIN actors a ON a.id = m.actor_id AND a.revoked_at IS NULL
  AND a.kind = 'app_installation'
LEFT JOIN e2ee_device_public_state d ON d.actor_id = a.id
  AND d.verification_state = 'verified' AND d.revoked_at = 0
LEFT JOIN e2ee_protocol_actor_bindings b ON b.device_id = d.device_id
  AND b.actor_id = a.id
WHERE am.air_id = ? AND am.status = 'joined'
ORDER BY a.id, d.device_id`, airID)
	if err != nil {
		return E2EEAirSnapshot{}, err
	}
	defer rows.Close()
	snapshot := E2EEAirSnapshot{AirID: airID}
	unsupported := map[int64]struct{}{}
	var canonical []string
	for rows.Next() {
		var actorID, orbitID, actorJoinedAt, airMembershipRevision int64
		var hasNonRevokedRegistration, registrations int
		var deviceID, protocolActorID, packageDigest, verificationDigest string
		var actorRole, airMembershipID, airRole string
		if err := rows.Scan(&actorID, &orbitID, &deviceID, &protocolActorID,
			&packageDigest, &verificationDigest, &actorRole, &actorJoinedAt,
			&airMembershipID, &airRole, &airMembershipRevision,
			&hasNonRevokedRegistration,
			&registrations); err != nil {
			return E2EEAirSnapshot{}, err
		}
		if deviceID == "" || protocolActorID == "" ||
			!validE2EEDigest(packageDigest) || !validE2EEDigest(verificationDigest) {
			if hasNonRevokedRegistration != 0 {
				unsupported[actorID] = struct{}{}
			} else if registrations == 0 {
				unsupported[actorID] = struct{}{}
			}
			continue
		}
		snapshot.Members = append(snapshot.Members, E2EEGroupMember{
			DeviceID: deviceID, ProtocolActorID: protocolActorID,
			ActorMembershipRole: actorRole, ActorMembershipJoinedAt: actorJoinedAt,
			ActorID: actorID, OrbitID: orbitID, AirMembershipID: airMembershipID,
			AirRole: airRole, AirMembershipRevision: airMembershipRevision,
			State: "current",
		})
		canonical = append(canonical, fmt.Sprintf(
			"%020d\x00%020d\x00%s\x00%020d\x00%s\x00%s\x00%020d\x00%s\x00%s\x00%s\x00%s",
			orbitID, actorID, actorRole, actorJoinedAt, airMembershipID, airRole,
			airMembershipRevision, protocolActorID, deviceID, packageDigest,
			verificationDigest))
	}
	if err := rows.Err(); err != nil {
		return E2EEAirSnapshot{}, err
	}
	for actorID := range unsupported {
		snapshot.UnsupportedActorIDs = append(snapshot.UnsupportedActorIDs, actorID)
	}
	sort.Slice(snapshot.UnsupportedActorIDs, func(i, j int) bool {
		return snapshot.UnsupportedActorIDs[i] < snapshot.UnsupportedActorIDs[j]
	})
	sort.Strings(canonical)
	material := "e2ee-target-snapshot.v1\n" + airID + "\n" + strings.Join(canonical, "\n")
	sum := sha256.Sum256([]byte(material))
	snapshot.Digest = hex.EncodeToString(sum[:])
	return snapshot, nil
}

func (s *Store) E2EEAirSnapshot(airID string) (E2EEAirSnapshot, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return E2EEAirSnapshot{}, err
	}
	defer tx.Rollback()
	snapshot, err := e2eeAirSnapshotTx(tx, airID)
	if err != nil {
		return E2EEAirSnapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return E2EEAirSnapshot{}, err
	}
	return snapshot, nil
}

func e2eeRoutingInitializedTx(tx *sql.Tx, groupID string) (bool, error) {
	var count int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM e2ee_group_members WHERE group_id = ?`,
		groupID).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func e2eeCurrentMembersTx(tx *sql.Tx, groupID string) ([]E2EEGroupMember, error) {
	rows, err := tx.Query(`SELECT group_id, device_id, actor_id, protocol_actor_id,
actor_membership_role, actor_membership_joined_at, orbit_id, air_membership_id,
air_role, air_membership_revision, state, added_epoch, removed_epoch, revision, updated_at
FROM e2ee_group_members WHERE group_id = ? AND state = 'current'
ORDER BY device_id`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var members []E2EEGroupMember
	for rows.Next() {
		var member E2EEGroupMember
		if err := rows.Scan(&member.GroupID, &member.DeviceID, &member.ActorID,
			&member.ProtocolActorID, &member.ActorMembershipRole,
			&member.ActorMembershipJoinedAt, &member.OrbitID,
			&member.AirMembershipID, &member.AirRole,
			&member.AirMembershipRevision, &member.State,
			&member.AddedEpoch, &member.RemovedEpoch, &member.Revision,
			&member.UpdatedAt); err != nil {
			return nil, err
		}
		members = append(members, member)
	}
	return members, rows.Err()
}

func memberSet(members []E2EEGroupMember) map[string]E2EEGroupMember {
	out := make(map[string]E2EEGroupMember, len(members))
	for _, member := range members {
		out[member.DeviceID] = member
	}
	return out
}

func sameE2EEMemberSet(left, right []E2EEGroupMember) bool {
	if len(left) != len(right) {
		return false
	}
	rightSet := memberSet(right)
	for _, member := range left {
		other, ok := rightSet[member.DeviceID]
		if !ok || member.ActorID != other.ActorID || member.OrbitID != other.OrbitID ||
			member.ProtocolActorID != other.ProtocolActorID ||
			member.ActorMembershipRole != other.ActorMembershipRole ||
			member.ActorMembershipJoinedAt != other.ActorMembershipJoinedAt ||
			member.AirMembershipID != other.AirMembershipID ||
			member.AirRole != other.AirRole ||
			member.AirMembershipRevision != other.AirMembershipRevision {
			return false
		}
	}
	return true
}

func sameE2EEMemberLineage(left, right E2EEGroupMember) bool {
	return left.DeviceID == right.DeviceID && left.ActorID == right.ActorID &&
		left.ProtocolActorID == right.ProtocolActorID && left.OrbitID == right.OrbitID &&
		left.ActorMembershipJoinedAt == right.ActorMembershipJoinedAt &&
		left.AirMembershipID == right.AirMembershipID
}

func e2eeRotationReasonTx(tx *sql.Tx, group E2EEGroup, current []E2EEGroupMember,
	snapshot E2EEAirSnapshot,
) (string, error) {
	oldSet, newSet := memberSet(current), memberSet(snapshot.Members)
	reason := ""
	for deviceID := range oldSet {
		if _, ok := newSet[deviceID]; ok {
			continue
		}
		var deviceState string
		var deviceRevoked int64
		var actorRevoked sql.NullInt64
		var activeMembership int
		err := tx.QueryRow(`SELECT d.verification_state, d.revoked_at, a.revoked_at,
  EXISTS(SELECT 1 FROM memberships m JOIN air_members am ON am.orbit_id = m.orbit_id
    WHERE m.actor_id = d.actor_id AND m.left_at IS NULL
      AND am.air_id = ? AND am.status = 'joined')
FROM e2ee_device_public_state d JOIN actors a ON a.id = d.actor_id
WHERE d.device_id = ?`, group.AirID, deviceID).Scan(&deviceState, &deviceRevoked,
			&actorRevoked, &activeMembership)
		if errors.Is(err, sql.ErrNoRows) {
			reason = "membership_change"
			continue
		}
		if err != nil {
			return "", err
		}
		if actorRevoked.Valid {
			return "actor_disable", nil
		}
		if deviceState == "revoked" || deviceRevoked > 0 {
			reason = "device_revoke"
			continue
		}
		if activeMembership == 0 && reason != "device_revoke" {
			reason = "air_leave"
		}
	}
	if reason != "" {
		return reason, nil
	}
	if len(snapshot.UnsupportedActorIDs) > 0 {
		return "unsupported_client", nil
	}
	for deviceID := range newSet {
		if _, ok := oldSet[deviceID]; !ok {
			return "air_join", nil
		}
	}
	return "membership_change", nil
}

func e2eeRotationRequirementTx(tx *sql.Tx, groupID string) (*E2EERotationRequirement, error) {
	var value E2EERotationRequirement
	err := tx.QueryRow(`SELECT group_id, observed_snapshot_digest,
required_snapshot_digest, reason_code, state, base_epoch, satisfied_epoch,
revision, detected_at, updated_at
FROM e2ee_rotation_requirements WHERE group_id = ?`, groupID).Scan(&value.GroupID,
		&value.ObservedSnapshotDigest, &value.RequiredSnapshotDigest,
		&value.ReasonCode, &value.State, &value.BaseEpoch, &value.SatisfiedEpoch,
		&value.Revision, &value.DetectedAt, &value.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &value, err
}

func recordE2EERotationTx(tx *sql.Tx, group E2EEGroup, snapshot E2EEAirSnapshot,
	reason string, now int64,
) (*E2EERotationRequirement, error) {
	current, err := e2eeRotationRequirementTx(tx, group.ID)
	if err != nil {
		return nil, err
	}
	if current != nil && current.State == "required" &&
		current.BaseEpoch == group.CurrentEpoch &&
		current.ObservedSnapshotDigest == group.TargetSnapshotDigest &&
		current.RequiredSnapshotDigest == snapshot.Digest && current.ReasonCode == reason {
		return current, nil
	}
	if _, err := tx.Exec(`INSERT INTO e2ee_rotation_requirements(
group_id, base_epoch, observed_snapshot_digest, required_snapshot_digest,
reason_code, state, satisfied_epoch, revision, detected_at, updated_at
) VALUES(?, ?, ?, ?, ?, 'required', 0, 1, ?, ?)
ON CONFLICT(group_id) DO UPDATE SET
  base_epoch = excluded.base_epoch,
  observed_snapshot_digest = excluded.observed_snapshot_digest,
  required_snapshot_digest = excluded.required_snapshot_digest,
  reason_code = excluded.reason_code,
  state = 'required', satisfied_epoch = 0,
  revision = e2ee_rotation_requirements.revision + 1,
  detected_at = excluded.detected_at, updated_at = excluded.updated_at`,
		group.ID, group.CurrentEpoch, group.TargetSnapshotDigest, snapshot.Digest,
		reason, now, now); err != nil {
		return nil, err
	}
	if err := appendE2EEAuditTx(tx, group.ID, "group", group.ID,
		"rotation.require", "accepted", reason, 0, "", group.CurrentEpoch,
		group.Revision, now); err != nil {
		return nil, err
	}
	return e2eeRotationRequirementTx(tx, group.ID)
}

func revokeRemovedE2EEDeliveriesTx(tx *sql.Tx, current, desired []E2EEGroupMember,
	now int64,
) error {
	desiredSet := memberSet(desired)
	for _, member := range current {
		if _, ok := desiredSet[member.DeviceID]; ok {
			continue
		}
		if _, err := tx.Exec(`UPDATE e2ee_group_event_deliveries
SET state = 'revoked', revision = revision + 1, revoked_at = ?
WHERE group_id = ? AND recipient_device_id = ? AND state = 'pending'`,
			now, member.GroupID, member.DeviceID); err != nil {
			return err
		}
	}
	return nil
}

func reconcileE2EERotationTx(tx *sql.Tx, group E2EEGroup, now int64) (*E2EERotationRequirement, error) {
	initialized, err := e2eeRoutingInitializedTx(tx, group.ID)
	if err != nil || !initialized {
		return nil, err
	}
	current, err := e2eeCurrentMembersTx(tx, group.ID)
	if err != nil {
		return nil, err
	}
	snapshot, err := e2eeAirSnapshotTx(tx, group.AirID)
	if err != nil {
		return nil, err
	}
	if len(snapshot.UnsupportedActorIDs) == 0 && snapshot.Digest == group.TargetSnapshotDigest &&
		sameE2EEMemberSet(current, snapshot.Members) {
		return e2eeRotationRequirementTx(tx, group.ID)
	}
	reason, err := e2eeRotationReasonTx(tx, group, current, snapshot)
	if err != nil {
		return nil, err
	}
	if err := revokeRemovedE2EEDeliveriesTx(tx, current, snapshot.Members, now); err != nil {
		return nil, err
	}
	return recordE2EERotationTx(tx, group, snapshot, reason, now)
}

func (s *Store) ReconcileE2EERotation(groupID string, now int64) (*E2EERotationRequirement, error) {
	if len(groupID) != 30 || now <= 0 {
		return nil, ErrE2EEInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	group, err := e2eeGroupTx(tx, groupID)
	if err != nil {
		return nil, err
	}
	requirement, err := reconcileE2EERotationTx(tx, group, now)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return requirement, nil
}

func (s *Store) GetE2EERotationRequirement(groupID string) (*E2EERotationRequirement, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	value, err := e2eeRotationRequirementTx(tx, groupID)
	if err != nil {
		return nil, err
	}
	if value == nil {
		return nil, ErrE2EENotFound
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return value, nil
}

func upsertE2EEGroupMembersTx(tx *sql.Tx, groupID string, members []E2EEGroupMember,
	epoch, now int64,
) error {
	desired := memberSet(members)
	current, err := e2eeCurrentMembersTx(tx, groupID)
	if err != nil {
		return err
	}
	for _, member := range current {
		if _, ok := desired[member.DeviceID]; ok {
			continue
		}
		if _, err := tx.Exec(`UPDATE e2ee_group_members
SET state = 'removed', removed_epoch = ?, revision = revision + 1, updated_at = ?
WHERE group_id = ? AND device_id = ? AND state = 'current'`, epoch, now,
			groupID, member.DeviceID); err != nil {
			return err
		}
	}
	for _, member := range members {
		if _, err := tx.Exec(`INSERT INTO e2ee_group_members(
group_id, device_id, actor_id, protocol_actor_id, actor_membership_role,
actor_membership_joined_at, orbit_id, air_membership_id, air_role,
air_membership_revision, state, added_epoch, removed_epoch, revision, updated_at
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'current', ?, 0, 1, ?)
ON CONFLICT(group_id, device_id) DO UPDATE SET
  actor_id = excluded.actor_id, protocol_actor_id = excluded.protocol_actor_id,
  actor_membership_role = excluded.actor_membership_role,
  actor_membership_joined_at = excluded.actor_membership_joined_at,
  orbit_id = excluded.orbit_id, air_membership_id = excluded.air_membership_id,
  air_role = excluded.air_role,
  air_membership_revision = excluded.air_membership_revision, state = 'current',
  added_epoch = CASE WHEN e2ee_group_members.state = 'removed'
    THEN excluded.added_epoch ELSE e2ee_group_members.added_epoch END,
  removed_epoch = 0, revision = e2ee_group_members.revision + 1,
  updated_at = excluded.updated_at`, groupID, member.DeviceID, member.ActorID,
			member.ProtocolActorID, member.ActorMembershipRole,
			member.ActorMembershipJoinedAt, member.OrbitID, member.AirMembershipID,
			member.AirRole, member.AirMembershipRevision, epoch, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) InitializeE2EEGroupRouting(groupID, authorDeviceID string, now int64) (E2EEGroup, error) {
	if len(groupID) != 30 || len(authorDeviceID) < 8 || now <= 0 {
		return E2EEGroup{}, ErrE2EEInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return E2EEGroup{}, err
	}
	defer tx.Rollback()
	group, err := e2eeGroupTx(tx, groupID)
	if err != nil {
		return E2EEGroup{}, err
	}
	initialized, err := e2eeRoutingInitializedTx(tx, groupID)
	if err != nil {
		return E2EEGroup{}, err
	}
	if initialized {
		return E2EEGroup{}, ErrE2EEConflict
	}
	snapshot, err := e2eeAirSnapshotTx(tx, group.AirID)
	if err != nil {
		return E2EEGroup{}, err
	}
	if len(snapshot.UnsupportedActorIDs) > 0 {
		return E2EEGroup{}, ErrE2EEUnsupportedTarget
	}
	if snapshot.Digest != group.TargetSnapshotDigest {
		return E2EEGroup{}, ErrE2EEStaleEpoch
	}
	authorized := false
	var authorActor int64
	for _, member := range snapshot.Members {
		if member.DeviceID == authorDeviceID {
			authorized, authorActor = true, member.ActorID
			break
		}
	}
	if !authorized {
		return E2EEGroup{}, ErrE2EEInvalid
	}
	if err := upsertE2EEGroupMembersTx(tx, groupID, snapshot.Members,
		group.CurrentEpoch, now); err != nil {
		return E2EEGroup{}, err
	}
	if err := appendE2EEAuditTx(tx, groupID, "group", groupID,
		"routing.initialize", "accepted", "", authorActor, authorDeviceID,
		group.CurrentEpoch, group.Revision, now); err != nil {
		return E2EEGroup{}, err
	}
	if err := tx.Commit(); err != nil {
		return E2EEGroup{}, err
	}
	return s.GetE2EEGroup(groupID)
}

func authorizedE2EEGroupMemberTx(tx *sql.Tx, group E2EEGroup, deviceID,
	protocolActorID string,
) (E2EEGroupMember, error) {
	var member E2EEGroupMember
	err := tx.QueryRow(`SELECT gm.group_id, gm.device_id, gm.actor_id,
gm.protocol_actor_id, gm.actor_membership_role, gm.actor_membership_joined_at,
gm.orbit_id, gm.air_membership_id, gm.air_role, gm.air_membership_revision,
gm.state, gm.added_epoch, gm.removed_epoch, gm.revision, gm.updated_at
FROM e2ee_group_members gm
JOIN e2ee_device_public_state d ON d.device_id = gm.device_id
  AND d.actor_id = gm.actor_id AND d.verification_state = 'verified' AND d.revoked_at = 0
JOIN actors a ON a.id = gm.actor_id AND a.revoked_at IS NULL
JOIN memberships m ON m.actor_id = gm.actor_id AND m.orbit_id = gm.orbit_id
  AND m.left_at IS NULL
JOIN air_members am ON am.air_id = ? AND am.orbit_id = gm.orbit_id
  AND am.public_id = gm.air_membership_id
  AND am.status = 'joined'
WHERE gm.group_id = ? AND gm.device_id = ? AND gm.protocol_actor_id = ?
  AND gm.state = 'current'`, group.AirID, group.ID, deviceID,
		protocolActorID).Scan(&member.GroupID, &member.DeviceID, &member.ActorID,
		&member.ProtocolActorID, &member.ActorMembershipRole,
		&member.ActorMembershipJoinedAt, &member.OrbitID, &member.AirMembershipID,
		&member.AirRole, &member.AirMembershipRevision, &member.State, &member.AddedEpoch,
		&member.RemovedEpoch, &member.Revision, &member.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return E2EEGroupMember{}, ErrE2EEInvalid
	}
	return member, err
}

func eventPayloadDigest(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func insertE2EEEventDeliveriesTx(tx *sql.Tx, eventID, groupID, eventDigest string,
	epoch, now int64, members []E2EEGroupMember,
) ([]string, error) {
	recipients := make([]string, 0, len(members))
	for _, member := range members {
		if _, err := tx.Exec(`INSERT INTO e2ee_group_event_deliveries(
event_id, group_id, recipient_device_id, event_digest, epoch, state,
revision, created_at, acknowledged_at, revoked_at
) VALUES(?, ?, ?, ?, ?, 'pending', 1, ?, 0, 0)`, eventID, groupID,
			member.DeviceID, eventDigest, epoch, now); err != nil {
			return nil, err
		}
		recipients = append(recipients, member.DeviceID)
	}
	sort.Strings(recipients)
	return recipients, nil
}

func persistE2EERejectedRouteTx(tx *sql.Tx, group E2EEGroup, eventID, deviceID,
	operation string, routeErr error, epoch, now int64,
) error {
	reason := string(e2eecontract.Code(routeErr))
	if reason == "" {
		switch {
		case errors.Is(routeErr, ErrE2EEUnsupportedTarget):
			reason = "unsupported_client"
		case errors.Is(routeErr, ErrE2EEInvalid):
			reason = "unauthorized_device"
		case errors.Is(routeErr, ErrE2EEReplay):
			reason = "replay"
		case errors.Is(routeErr, ErrE2EEForked):
			reason = "forked_epoch"
		case errors.Is(routeErr, ErrE2EEStaleEpoch):
			reason = "stale_epoch"
		case errors.Is(routeErr, ErrE2EERevoked):
			reason = "revoked"
		default:
			reason = "state_conflict"
		}
	}
	if len(reason) > 64 {
		reason = "state_conflict"
	}
	if eventID == "" {
		eventID = group.ID
	}
	if err := appendE2EEAuditTx(tx, group.ID, "public_event", eventID,
		operation, "rejected", reason, 0, deviceID, epoch, group.Revision, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) RouteE2EEProposal(config e2eecontract.Config, raw []byte,
	now int64,
) (E2EEGroupEventRouteResult, error) {
	if now <= 0 || !validE2EEPayload(raw) {
		return E2EEGroupEventRouteResult{}, ErrE2EEInvalid
	}
	proposal, err := e2eecontract.DecodeCoordinatorProposal(raw)
	if err != nil {
		return E2EEGroupEventRouteResult{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return E2EEGroupEventRouteResult{}, err
	}
	defer tx.Rollback()
	group, err := e2eeGroupTx(tx, proposal.GroupID)
	if err != nil {
		return E2EEGroupEventRouteResult{}, err
	}
	if group.ForkState != "clean" {
		routeErr := ErrE2EEForked
		if group.ForkState == "revoked" {
			routeErr = ErrE2EERevoked
		}
		if auditErr := persistE2EERejectedRouteTx(tx, group, proposal.EventID,
			proposal.DeviceID, "proposal.route", routeErr, int64(proposal.Epoch), now); auditErr != nil {
			return E2EEGroupEventRouteResult{}, auditErr
		}
		return E2EEGroupEventRouteResult{}, routeErr
	}
	state := e2eecontract.NewState(group.ID, group.AirID, group.TargetSnapshotDigest,
		uint64(group.CurrentEpoch), group.CommitDigest)
	if err := state.ValidateProposal(config, proposal); err != nil {
		if auditErr := persistE2EERejectedRouteTx(tx, group, proposal.EventID,
			proposal.DeviceID, "proposal.route", err, int64(proposal.Epoch), now); auditErr != nil {
			return E2EEGroupEventRouteResult{}, auditErr
		}
		return E2EEGroupEventRouteResult{}, err
	}
	if !validE2EEDigest(proposal.ProposalDigest) ||
		!validE2EEDigest(proposal.AuthenticatedDataDigest) {
		return E2EEGroupEventRouteResult{}, ErrE2EEInvalid
	}
	if _, err := authorizedE2EEGroupMemberTx(tx, group, proposal.DeviceID,
		proposal.ActorID); err != nil {
		if auditErr := persistE2EERejectedRouteTx(tx, group, proposal.EventID,
			proposal.DeviceID, "proposal.route", ErrE2EEInvalid, int64(proposal.Epoch), now); auditErr != nil {
			return E2EEGroupEventRouteResult{}, auditErr
		}
		return E2EEGroupEventRouteResult{}, ErrE2EEInvalid
	}
	current, err := e2eeCurrentMembersTx(tx, group.ID)
	if err != nil {
		return E2EEGroupEventRouteResult{}, err
	}
	snapshot, err := e2eeAirSnapshotTx(tx, group.AirID)
	if err != nil {
		return E2EEGroupEventRouteResult{}, err
	}
	if len(snapshot.UnsupportedActorIDs) > 0 {
		reason, reasonErr := e2eeRotationReasonTx(tx, group, current, snapshot)
		if reasonErr != nil {
			return E2EEGroupEventRouteResult{}, reasonErr
		}
		if err := revokeRemovedE2EEDeliveriesTx(tx, current, snapshot.Members, now); err != nil {
			return E2EEGroupEventRouteResult{}, err
		}
		if _, err := recordE2EERotationTx(tx, group, snapshot, reason, now); err != nil {
			return E2EEGroupEventRouteResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return E2EEGroupEventRouteResult{}, err
		}
		return E2EEGroupEventRouteResult{}, ErrE2EEUnsupportedTarget
	}
	if proposal.TargetSnapshotDigest != snapshot.Digest {
		routeErr := &e2eecontract.ValidationError{Code: e2eecontract.ErrForeignTarget}
		if auditErr := persistE2EERejectedRouteTx(tx, group, proposal.EventID,
			proposal.DeviceID, "proposal.route", routeErr, int64(proposal.Epoch), now); auditErr != nil {
			return E2EEGroupEventRouteResult{}, auditErr
		}
		return E2EEGroupEventRouteResult{}, routeErr
	}
	eventDigest := eventPayloadDigest(raw)
	var duplicate int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM e2ee_public_group_events
WHERE group_id = ? AND (id = ? OR event_digest = ?)`, group.ID,
		proposal.EventID, eventDigest).Scan(&duplicate); err != nil {
		return E2EEGroupEventRouteResult{}, err
	}
	if duplicate != 0 {
		if auditErr := persistE2EERejectedRouteTx(tx, group, proposal.EventID,
			proposal.DeviceID, "proposal.route", ErrE2EEReplay, int64(proposal.Epoch), now); auditErr != nil {
			return E2EEGroupEventRouteResult{}, auditErr
		}
		return E2EEGroupEventRouteResult{}, ErrE2EEReplay
	}
	if _, err := tx.Exec(`INSERT INTO e2ee_public_group_events(
id, group_id, kind, author_device_id, previous_epoch, epoch,
previous_commit_digest, event_digest, public_payload, state, reason_code,
created_at, updated_at
) VALUES(?, ?, 'proposal', ?, ?, ?, '', ?, ?, 'accepted', '', ?, ?)`,
		proposal.EventID, group.ID, proposal.DeviceID, proposal.PreviousEpoch,
		proposal.Epoch, eventDigest, raw, now, now); err != nil {
		return E2EEGroupEventRouteResult{}, ErrE2EEConflict
	}
	// Proposals go only to surviving members of the prior epoch. Newly added
	// devices receive the client-produced commit/welcome, never the proposal.
	desired := memberSet(snapshot.Members)
	var recipientsMembers []E2EEGroupMember
	for _, member := range current {
		if next, ok := desired[member.DeviceID]; ok && sameE2EEMemberLineage(member, next) {
			recipientsMembers = append(recipientsMembers, member)
		}
	}
	recipients, err := insertE2EEEventDeliveriesTx(tx, proposal.EventID, group.ID,
		eventDigest, int64(proposal.Epoch), now, recipientsMembers)
	if err != nil {
		return E2EEGroupEventRouteResult{}, err
	}
	if snapshot.Digest != group.TargetSnapshotDigest || !sameE2EEMemberSet(current, snapshot.Members) {
		reason, err := e2eeRotationReasonTx(tx, group, current, snapshot)
		if err != nil {
			return E2EEGroupEventRouteResult{}, err
		}
		if err := revokeRemovedE2EEDeliveriesTx(tx, current, snapshot.Members, now); err != nil {
			return E2EEGroupEventRouteResult{}, err
		}
		if _, err := recordE2EERotationTx(tx, group, snapshot, reason, now); err != nil {
			return E2EEGroupEventRouteResult{}, err
		}
	}
	if err := appendE2EEAuditTx(tx, group.ID, "public_event", proposal.EventID,
		"proposal.route", "accepted", "", 0, proposal.DeviceID,
		int64(proposal.Epoch), group.Revision, now); err != nil {
		return E2EEGroupEventRouteResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return E2EEGroupEventRouteResult{}, err
	}
	return E2EEGroupEventRouteResult{Group: group, EventID: proposal.EventID,
		Kind: "proposal", Epoch: int64(proposal.Epoch), Recipients: recipients}, nil
}

func (s *Store) RouteE2EECommit(config e2eecontract.Config, raw []byte,
	now int64,
) (E2EEGroupEventRouteResult, error) {
	if now <= 0 || !validE2EEPayload(raw) {
		return E2EEGroupEventRouteResult{}, ErrE2EEInvalid
	}
	commit, err := e2eecontract.DecodeCoordinatorCommit(raw)
	if err != nil {
		return E2EEGroupEventRouteResult{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return E2EEGroupEventRouteResult{}, err
	}
	defer tx.Rollback()
	group, err := e2eeGroupTx(tx, commit.GroupID)
	if err != nil {
		return E2EEGroupEventRouteResult{}, err
	}
	if group.ForkState != "clean" {
		routeErr := ErrE2EEForked
		if group.ForkState == "revoked" {
			routeErr = ErrE2EERevoked
		}
		if auditErr := persistE2EERejectedRouteTx(tx, group, commit.EventID,
			commit.DeviceID, "commit.route", routeErr, int64(commit.Epoch), now); auditErr != nil {
			return E2EEGroupEventRouteResult{}, auditErr
		}
		return E2EEGroupEventRouteResult{}, routeErr
	}
	state := e2eecontract.NewState(group.ID, group.AirID, group.TargetSnapshotDigest,
		uint64(group.CurrentEpoch), group.CommitDigest)
	if err := state.ApplyCommit(config, commit); err != nil {
		if e2eecontract.Code(err) == e2eecontract.ErrForkedEpoch &&
			validE2EEDigest(commit.PreviousCommitDigest) &&
			validE2EEDigest(commit.CommitDigest) &&
			validE2EEDigest(commit.TargetSnapshotDigest) &&
			validE2EEDigest(commit.AuthenticatedDataDigest) &&
			int64(commit.PreviousEpoch) == group.CurrentEpoch &&
			int64(commit.Epoch) == group.CurrentEpoch+1 &&
			commit.PreviousCommitDigest != group.CommitDigest {
			if _, updateErr := tx.Exec(`UPDATE e2ee_groups SET fork_state = 'forked',
revision = revision + 1, updated_at = ? WHERE id = ? AND revision = ? AND fork_state = 'clean'`,
				now, group.ID, group.Revision); updateErr != nil {
				return E2EEGroupEventRouteResult{}, updateErr
			}
			group.Revision++
		}
		if auditErr := persistE2EERejectedRouteTx(tx, group, commit.EventID,
			commit.DeviceID, "commit.route", err, int64(commit.Epoch), now); auditErr != nil {
			return E2EEGroupEventRouteResult{}, auditErr
		}
		return E2EEGroupEventRouteResult{}, err
	}
	if !validE2EEDigest(commit.PreviousCommitDigest) ||
		!validE2EEDigest(commit.CommitDigest) ||
		!validE2EEDigest(commit.TargetSnapshotDigest) ||
		!validE2EEDigest(commit.AuthenticatedDataDigest) {
		return E2EEGroupEventRouteResult{}, ErrE2EEInvalid
	}
	member, err := authorizedE2EEGroupMemberTx(tx, group, commit.DeviceID, commit.ActorID)
	if err != nil {
		if auditErr := persistE2EERejectedRouteTx(tx, group, commit.EventID,
			commit.DeviceID, "commit.route", ErrE2EEInvalid, int64(commit.Epoch), now); auditErr != nil {
			return E2EEGroupEventRouteResult{}, auditErr
		}
		return E2EEGroupEventRouteResult{}, ErrE2EEInvalid
	}
	current, err := e2eeCurrentMembersTx(tx, group.ID)
	if err != nil {
		return E2EEGroupEventRouteResult{}, err
	}
	snapshot, err := e2eeAirSnapshotTx(tx, group.AirID)
	if err != nil {
		return E2EEGroupEventRouteResult{}, err
	}
	if len(snapshot.UnsupportedActorIDs) > 0 {
		reason, reasonErr := e2eeRotationReasonTx(tx, group, current, snapshot)
		if reasonErr != nil {
			return E2EEGroupEventRouteResult{}, reasonErr
		}
		if err := revokeRemovedE2EEDeliveriesTx(tx, current, snapshot.Members, now); err != nil {
			return E2EEGroupEventRouteResult{}, err
		}
		if _, err := recordE2EERotationTx(tx, group, snapshot, reason, now); err != nil {
			return E2EEGroupEventRouteResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return E2EEGroupEventRouteResult{}, err
		}
		return E2EEGroupEventRouteResult{}, ErrE2EEUnsupportedTarget
	}
	if commit.TargetSnapshotDigest != snapshot.Digest {
		routeErr := &e2eecontract.ValidationError{Code: e2eecontract.ErrForeignTarget}
		if auditErr := persistE2EERejectedRouteTx(tx, group, commit.EventID,
			commit.DeviceID, "commit.route", routeErr, int64(commit.Epoch), now); auditErr != nil {
			return E2EEGroupEventRouteResult{}, auditErr
		}
		return E2EEGroupEventRouteResult{}, routeErr
	}
	if snapshot.Digest != group.TargetSnapshotDigest ||
		!sameE2EEMemberSet(current, snapshot.Members) {
		reason, err := e2eeRotationReasonTx(tx, group, current, snapshot)
		if err != nil {
			return E2EEGroupEventRouteResult{}, err
		}
		if err := revokeRemovedE2EEDeliveriesTx(tx, current, snapshot.Members, now); err != nil {
			return E2EEGroupEventRouteResult{}, err
		}
		if _, err := recordE2EERotationTx(tx, group, snapshot, reason, now); err != nil {
			return E2EEGroupEventRouteResult{}, err
		}
	}
	eventDigest := eventPayloadDigest(raw)
	var duplicate int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM e2ee_public_group_events
WHERE group_id = ? AND (id = ? OR event_digest = ?)`, group.ID,
		commit.EventID, eventDigest).Scan(&duplicate); err != nil {
		return E2EEGroupEventRouteResult{}, err
	}
	if duplicate != 0 {
		if auditErr := persistE2EERejectedRouteTx(tx, group, commit.EventID,
			commit.DeviceID, "commit.route", ErrE2EEReplay, int64(commit.Epoch), now); auditErr != nil {
			return E2EEGroupEventRouteResult{}, auditErr
		}
		return E2EEGroupEventRouteResult{}, ErrE2EEReplay
	}
	if _, err := tx.Exec(`INSERT INTO e2ee_public_group_events(
id, group_id, kind, author_device_id, previous_epoch, epoch,
previous_commit_digest, event_digest, public_payload, state, reason_code,
created_at, updated_at
) VALUES(?, ?, 'commit', ?, ?, ?, ?, ?, ?, 'accepted', '', ?, ?)`,
		commit.EventID, group.ID, commit.DeviceID, commit.PreviousEpoch, commit.Epoch,
		commit.PreviousCommitDigest, eventDigest, raw, now, now); err != nil {
		return E2EEGroupEventRouteResult{}, ErrE2EEConflict
	}
	result, err := tx.Exec(`UPDATE e2ee_groups SET current_epoch = ?, commit_digest = ?,
target_snapshot_digest = ?, revision = revision + 1, updated_at = ?
WHERE id = ? AND current_epoch = ? AND commit_digest = ? AND revision = ?
  AND fork_state = 'clean'`, commit.Epoch, commit.CommitDigest,
		commit.TargetSnapshotDigest, now, group.ID, commit.PreviousEpoch,
		commit.PreviousCommitDigest, group.Revision)
	if err != nil {
		return E2EEGroupEventRouteResult{}, err
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		if err != nil {
			return E2EEGroupEventRouteResult{}, err
		}
		return E2EEGroupEventRouteResult{}, ErrE2EEConflict
	}
	if err := revokeRemovedE2EEDeliveriesTx(tx, current, snapshot.Members, now); err != nil {
		return E2EEGroupEventRouteResult{}, err
	}
	if err := upsertE2EEGroupMembersTx(tx, group.ID, snapshot.Members,
		int64(commit.Epoch), now); err != nil {
		return E2EEGroupEventRouteResult{}, err
	}
	if requirement, err := e2eeRotationRequirementTx(tx, group.ID); err != nil {
		return E2EEGroupEventRouteResult{}, err
	} else if requirement != nil && requirement.State == "required" {
		rotationResult, err := tx.Exec(`UPDATE e2ee_rotation_requirements
SET state = 'satisfied', satisfied_epoch = ?, revision = revision + 1, updated_at = ?
WHERE group_id = ? AND state = 'required' AND base_epoch = ?
`, commit.Epoch, now, group.ID, group.CurrentEpoch)
		if err != nil {
			return E2EEGroupEventRouteResult{}, err
		}
		changed, err := rotationResult.RowsAffected()
		if err != nil || changed != 1 {
			if err != nil {
				return E2EEGroupEventRouteResult{}, err
			}
			return E2EEGroupEventRouteResult{}, ErrE2EEConflict
		}
		if err := appendE2EEAuditTx(tx, group.ID, "group", group.ID,
			"rotation.satisfy", "accepted", requirement.ReasonCode, member.ActorID,
			commit.DeviceID, int64(commit.Epoch), requirement.Revision+1, now); err != nil {
			return E2EEGroupEventRouteResult{}, err
		}
	}
	recipients, err := insertE2EEEventDeliveriesTx(tx, commit.EventID, group.ID,
		eventDigest, int64(commit.Epoch), now, snapshot.Members)
	if err != nil {
		return E2EEGroupEventRouteResult{}, err
	}
	if err := appendE2EEAuditTx(tx, group.ID, "public_event", commit.EventID,
		"commit.route", "accepted", "", member.ActorID, commit.DeviceID,
		int64(commit.Epoch), group.Revision+1, now); err != nil {
		return E2EEGroupEventRouteResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return E2EEGroupEventRouteResult{}, err
	}
	updated, err := s.GetE2EEGroup(group.ID)
	if err != nil {
		return E2EEGroupEventRouteResult{}, err
	}
	return E2EEGroupEventRouteResult{Group: updated, EventID: commit.EventID,
		Kind: "commit", Epoch: int64(commit.Epoch), Recipients: recipients}, nil
}

func (s *Store) PendingE2EEGroupEvents(deviceID string, after E2EEGroupEventCursor,
	limit int,
) ([]E2EEGroupEventDelivery, error) {
	if len(deviceID) < 8 || after.CreatedAt < 0 || limit <= 0 || limit > 100 ||
		(after.CreatedAt == 0 && after.EventID != "") ||
		(after.CreatedAt > 0 && len(after.EventID) < 8) {
		return nil, ErrE2EEInvalid
	}
	rows, err := s.db.Query(`SELECT d.event_id, d.group_id, d.recipient_device_id,
d.event_digest, e.kind, d.state, d.epoch, d.revision, d.created_at,
d.acknowledged_at, d.revoked_at, e.public_payload
FROM e2ee_group_event_deliveries d
JOIN e2ee_public_group_events e ON e.id = d.event_id AND e.group_id = d.group_id
JOIN e2ee_group_members gm ON gm.group_id = d.group_id
  AND gm.device_id = d.recipient_device_id AND gm.state = 'current'
JOIN e2ee_groups g ON g.id = d.group_id AND g.fork_state = 'clean'
JOIN e2ee_device_public_state p ON p.device_id = d.recipient_device_id
  AND p.verification_state = 'verified' AND p.revoked_at = 0
JOIN actors a ON a.id = p.actor_id AND a.revoked_at IS NULL
JOIN memberships m ON m.actor_id = a.id AND m.orbit_id = gm.orbit_id
  AND m.left_at IS NULL
JOIN air_members am ON am.air_id = g.air_id AND am.orbit_id = gm.orbit_id
  AND am.status = 'joined'
WHERE d.recipient_device_id = ? AND d.state = 'pending'
  AND (d.created_at > ? OR (d.created_at = ? AND d.event_id > ?))
ORDER BY d.created_at, d.event_id LIMIT ?`, deviceID, after.CreatedAt,
		after.CreatedAt, after.EventID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []E2EEGroupEventDelivery
	for rows.Next() {
		var delivery E2EEGroupEventDelivery
		if err := rows.Scan(&delivery.EventID, &delivery.GroupID,
			&delivery.RecipientDeviceID, &delivery.EventDigest, &delivery.Kind,
			&delivery.State, &delivery.Epoch, &delivery.Revision,
			&delivery.CreatedAt, &delivery.AcknowledgedAt, &delivery.RevokedAt,
			&delivery.PublicPayload); err != nil {
			return nil, err
		}
		out = append(out, delivery)
	}
	return out, rows.Err()
}

func (s *Store) AcknowledgeE2EEGroupEvent(deviceID, eventID, eventDigest string,
	expectedRevision, now int64,
) (E2EEGroupEventDelivery, error) {
	if len(deviceID) < 8 || len(eventID) < 8 || !validE2EEDigest(eventDigest) ||
		expectedRevision <= 0 || now <= 0 {
		return E2EEGroupEventDelivery{}, ErrE2EEInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return E2EEGroupEventDelivery{}, err
	}
	defer tx.Rollback()
	var delivery E2EEGroupEventDelivery
	err = tx.QueryRow(`SELECT d.event_id, d.group_id, d.recipient_device_id,
d.event_digest, e.kind, d.state, d.epoch, d.revision, d.created_at,
d.acknowledged_at, d.revoked_at, e.public_payload
FROM e2ee_group_event_deliveries d
JOIN e2ee_public_group_events e ON e.id = d.event_id
JOIN e2ee_group_members gm ON gm.group_id = d.group_id
  AND gm.device_id = d.recipient_device_id AND gm.state = 'current'
JOIN e2ee_groups g ON g.id = d.group_id
WHERE d.event_id = ? AND d.recipient_device_id = ? AND d.event_digest = ?
  AND d.state = 'pending'`, eventID, deviceID, eventDigest).Scan(&delivery.EventID,
		&delivery.GroupID, &delivery.RecipientDeviceID, &delivery.EventDigest,
		&delivery.Kind, &delivery.State, &delivery.Epoch, &delivery.Revision,
		&delivery.CreatedAt, &delivery.AcknowledgedAt, &delivery.RevokedAt,
		&delivery.PublicPayload)
	if errors.Is(err, sql.ErrNoRows) {
		return E2EEGroupEventDelivery{}, ErrE2EEDeliveryNotFound
	}
	if err != nil {
		return E2EEGroupEventDelivery{}, err
	}
	group, err := e2eeGroupTx(tx, delivery.GroupID)
	if err != nil {
		return E2EEGroupEventDelivery{}, err
	}
	var protocolActorID string
	if err := tx.QueryRow(`SELECT protocol_actor_id FROM e2ee_group_members
WHERE group_id = ? AND device_id = ?`, group.ID, deviceID).Scan(&protocolActorID); err != nil {
		return E2EEGroupEventDelivery{}, ErrE2EEDeliveryNotFound
	}
	member, err := authorizedE2EEGroupMemberTx(tx, group, deviceID, protocolActorID)
	if err != nil {
		return E2EEGroupEventDelivery{}, ErrE2EEInvalid
	}
	result, err := tx.Exec(`UPDATE e2ee_group_event_deliveries
SET state = 'acknowledged', revision = revision + 1, acknowledged_at = ?
WHERE event_id = ? AND recipient_device_id = ? AND state = 'pending'
  AND revision = ? AND event_digest = ?`, now, eventID, deviceID,
		expectedRevision, eventDigest)
	if err != nil {
		return E2EEGroupEventDelivery{}, err
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		if err != nil {
			return E2EEGroupEventDelivery{}, err
		}
		return E2EEGroupEventDelivery{}, ErrE2EEConflict
	}
	if err := appendE2EEAuditTx(tx, group.ID, "public_event", eventID,
		"delivery.acknowledge", "accepted", "", member.ActorID, deviceID,
		delivery.Epoch, expectedRevision+1, now); err != nil {
		return E2EEGroupEventDelivery{}, err
	}
	if err := tx.Commit(); err != nil {
		return E2EEGroupEventDelivery{}, err
	}
	delivery.State = "acknowledged"
	delivery.Revision++
	delivery.AcknowledgedAt = now
	return delivery, nil
}

func (s *Store) RevokeE2EEPublicDevice(deviceID string, expectedRevision,
	now int64,
) (E2EEPublicDevice, error) {
	if len(deviceID) < 8 || expectedRevision <= 0 || now <= 0 {
		return E2EEPublicDevice{}, ErrE2EEInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return E2EEPublicDevice{}, err
	}
	defer tx.Rollback()
	var actorID, revision int64
	var state string
	if err := tx.QueryRow(`SELECT actor_id, verification_state, revision
FROM e2ee_device_public_state WHERE device_id = ?`, deviceID).Scan(&actorID,
		&state, &revision); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return E2EEPublicDevice{}, ErrE2EENotFound
		}
		return E2EEPublicDevice{}, err
	}
	if state == "revoked" {
		return E2EEPublicDevice{}, ErrE2EERevoked
	}
	if revision != expectedRevision {
		return E2EEPublicDevice{}, ErrE2EEConflict
	}
	result, err := tx.Exec(`UPDATE e2ee_device_public_state
SET verification_state = 'revoked', revision = revision + 1,
updated_at = ?, revoked_at = ?
WHERE device_id = ? AND revision = ? AND verification_state <> 'revoked'`,
		now, now, deviceID, expectedRevision)
	if err != nil {
		return E2EEPublicDevice{}, err
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		if err != nil {
			return E2EEPublicDevice{}, err
		}
		return E2EEPublicDevice{}, ErrE2EEConflict
	}
	if err := appendE2EEAuditTx(tx, "", "device", deviceID,
		"device.public_state.revoke", "revoked", "device_revoke", actorID,
		deviceID, 0, expectedRevision+1, now); err != nil {
		return E2EEPublicDevice{}, err
	}
	if err := tx.Commit(); err != nil {
		return E2EEPublicDevice{}, err
	}
	return s.GetE2EEPublicDevice(deviceID)
}
