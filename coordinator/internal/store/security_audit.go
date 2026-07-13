package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const rateLimitSubjectDomain = "barycenter/rate-limit-subject/v1:"

// RateLimitAuditClass is the stable, typed identity of one frozen Phase 1
// limiter. The raw limiter subject is never persisted.
type RateLimitAuditClass string

const (
	RateLimitCreateSourceIP              RateLimitAuditClass = "create/source-ip"
	RateLimitCreateInstallationAttempt   RateLimitAuditClass = "create/installation-attempt"
	RateLimitInviteConsumeSourceIP       RateLimitAuditClass = "invite-consume/source-ip"
	RateLimitRecoveryConsumeSourceIP     RateLimitAuditClass = "recovery-consume/source-ip"
	RateLimitRecoveryConsumeRecoveryID   RateLimitAuditClass = "recovery-consume/recovery-id"
	RateLimitRecoveryRotateActor         RateLimitAuditClass = "recovery-rotate/actor"
	RateLimitTelegramLinkIssueActor      RateLimitAuditClass = "telegram-link-issue/actor"
	RateLimitTelegramLinkConsumeTelegram RateLimitAuditClass = "telegram-link-consume/telegram-user"
)

// RateLimitAuditScope carries only real resolved identity coordinates. Nil
// fields represent a pre-identity limiter; fabricated/sentinel identifiers are
// rejected by RecordRateLimitAudit.
type RateLimitAuditScope struct {
	OrbitID *int64
	ActorID *int64
}

func validRateLimitAuditClass(class RateLimitAuditClass) bool {
	switch class {
	case RateLimitCreateSourceIP,
		RateLimitCreateInstallationAttempt,
		RateLimitInviteConsumeSourceIP,
		RateLimitRecoveryConsumeSourceIP,
		RateLimitRecoveryConsumeRecoveryID,
		RateLimitRecoveryRotateActor,
		RateLimitTelegramLinkIssueActor,
		RateLimitTelegramLinkConsumeTelegram:
		return true
	default:
		return false
	}
}

func rateLimitSubjectDigest(class RateLimitAuditClass, subject string) string {
	return hashToken(rateLimitSubjectDomain + string(class) + ":" + subject)
}

func rateLimitClassRequiresActorScope(class RateLimitAuditClass) bool {
	switch class {
	case RateLimitRecoveryRotateActor, RateLimitTelegramLinkIssueActor:
		return true
	default:
		return false
	}
}

// RecordRateLimitAudit durably records one already-reserved rate-limit
// rejection. It deliberately returns persistence errors: callers must not emit
// a 429 response unless this insert succeeds.
func (s *Store) RecordRateLimitAudit(class RateLimitAuditClass, subject string, scope RateLimitAuditScope) error {
	if !s.selfServiceOnboarding {
		return ErrSelfServiceOnboardingDisabled
	}
	if !validRateLimitAuditClass(class) || subject == "" {
		return errors.New("invalid rate-limit audit input")
	}
	if scope.OrbitID != nil && *scope.OrbitID <= 0 {
		return errors.New("invalid rate-limit audit orbit scope")
	}
	if scope.ActorID != nil && *scope.ActorID <= 0 {
		return errors.New("invalid rate-limit audit actor scope")
	}
	if (scope.OrbitID == nil) != (scope.ActorID == nil) {
		return errors.New("rate-limit audit scope must be fully scoped or unscoped")
	}
	requiresScope := rateLimitClassRequiresActorScope(class)
	if requiresScope != (scope.ActorID != nil) {
		return errors.New("rate-limit audit scope does not match limiter class")
	}
	if !requiresScope {
		_, err := s.db.Exec(`INSERT INTO rate_limit_audit_events
  (event_type, limiter_class, subject_digest, orbit_id, actor_id, created_at)
VALUES('security.rate_limited', ?, ?, NULL, NULL, ?)`,
			string(class), rateLimitSubjectDigest(class, subject), time.Now().UnixMilli())
		return err
	}
	res, err := s.db.Exec(`INSERT INTO rate_limit_audit_events
  (event_type, limiter_class, subject_digest, orbit_id, actor_id, created_at)
SELECT 'security.rate_limited', ?, ?, ?, ?, ?
WHERE EXISTS (
  SELECT 1
  FROM actors a
  JOIN memberships m ON m.actor_id = a.id AND m.orbit_id = ?
  JOIN orbits o ON o.id = m.orbit_id
  WHERE a.id = ?
)`, string(class), rateLimitSubjectDigest(class, subject), *scope.OrbitID, *scope.ActorID,
		time.Now().UnixMilli(), *scope.OrbitID, *scope.ActorID)
	if err != nil {
		return err
	}
	inserted, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if inserted != 1 {
		return errors.New("rate-limit audit scope does not identify an actor membership")
	}
	return nil
}

func insertRecoveryRotationAuditTx(tx *sql.Tx, orbitID, actorID int64, oldRecoveryID sql.NullString, newRecoveryID string, now int64) error {
	res, err := tx.Exec(`INSERT INTO audit_events(orbit_id, actor_id, type, created_at)
VALUES(?, ?, 'recovery.rotated', ?)`, orbitID, actorID, now)
	if err != nil {
		return err
	}
	auditID, err := res.LastInsertId()
	if err != nil {
		return err
	}
	var old any
	if oldRecoveryID.Valid {
		old = oldRecoveryID.String
	}
	if _, err := tx.Exec(`INSERT INTO recovery_rotation_audit_details
  (audit_event_id, old_recovery_id, new_recovery_id)
VALUES(?, ?, ?)`, auditID, old, newRecoveryID); err != nil {
		return fmt.Errorf("insert recovery rotation audit detail: %w", err)
	}
	return nil
}
