package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	// ErrTelegramLinkInvalid deliberately covers malformed, unknown, expired,
	// invalidated, consumed, revoked, and race-loser credentials. The adapter
	// must not expose a more specific secret-facing failure.
	ErrTelegramLinkInvalid = errors.New("telegram link credential is not valid")
	// Conflict errors are the two contract-approved non-secret disclosures.
	ErrTelegramAlreadyLinkedSameOrbit = errors.New("telegram account is already linked to this orbit")
	ErrTelegramMemberOfOtherOrbit     = errors.New("telegram account belongs to a different orbit")
	ErrTelegramLinkRateLimited        = errors.New("too many telegram link attempts")
)

const (
	telegramLinkAttemptLimit  = 10
	telegramLinkAttemptWindow = 15 * time.Minute
	telegramLinkLimiterCap    = 10_000
)

// ConsumeTelegramLinkResult is returned only after actor resolution,
// membership creation, legacy dual-write, and audit commit together.
type ConsumeTelegramLinkResult struct {
	OrbitID int64
	ActorID int64
	Role    string
}

type telegramLinkAttempt struct {
	// attempts retains exactly the newest N attempts needed to decide whether
	// the next reservation fits in the rolling window. Rejected attempts are
	// retained too, so sustained rejection advances the admission boundary.
	attempts []int64
	lastUsed uint64
}

// telegramLinkAttemptLimiter atomically reserves every syntactically valid
// attempt. Telegram user IDs come from the authenticated Bot API transport;
// the cap still bounds state if many real accounts spray the bot.
type telegramLinkAttemptLimiter struct {
	mu      sync.Mutex
	entries map[int64]telegramLinkAttempt
	clock   uint64
}

func newTelegramLinkAttemptLimiter() *telegramLinkAttemptLimiter {
	return &telegramLinkAttemptLimiter{entries: make(map[int64]telegramLinkAttempt)}
}

func (l *telegramLinkAttemptLimiter) reserve(userID, now int64) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.clock++
	entry, ok := l.entries[userID]
	if !ok && len(l.entries) >= telegramLinkLimiterCap {
		var oldestID int64
		var oldest uint64 = ^uint64(0)
		for id, candidate := range l.entries {
			if candidate.lastUsed < oldest {
				oldestID, oldest = id, candidate.lastUsed
			}
		}
		delete(l.entries, oldestID)
	}

	cutoff := now - telegramLinkAttemptWindow.Milliseconds()
	firstLive := 0
	for firstLive < len(entry.attempts) && entry.attempts[firstLive] <= cutoff {
		firstLive++
	}
	if firstLive > 0 {
		copy(entry.attempts, entry.attempts[firstLive:])
		entry.attempts = entry.attempts[:len(entry.attempts)-firstLive]
	}

	allowed := len(entry.attempts) < telegramLinkAttemptLimit
	if len(entry.attempts) < telegramLinkAttemptLimit {
		entry.attempts = append(entry.attempts, now)
	} else {
		// The oldest retained attempt is no longer needed once this attempt,
		// allowed or rejected, becomes the newest boundary. Keeping exactly N
		// timestamps is sufficient for an exact rolling N-attempt decision.
		copy(entry.attempts, entry.attempts[1:])
		entry.attempts[len(entry.attempts)-1] = now
	}
	entry.lastUsed = l.clock
	l.entries[userID] = entry
	return allowed
}

// ConsumeTelegramLink is callable only from the trusted in-process Telegram
// adapter. telegramUserID and chatType are derived from the authenticated Bot
// API Update, never from public HTTP input.
func (s *Store) ConsumeTelegramLink(telegramUserID int64, displayName, chatType, linkCode string) (ConsumeTelegramLinkResult, error) {
	return s.consumeTelegramLinkAt(telegramUserID, displayName, chatType, linkCode, time.Now().UnixMilli())
}

func (s *Store) consumeTelegramLinkAt(telegramUserID int64, displayName, chatType, linkCode string, now int64) (ConsumeTelegramLinkResult, error) {
	if !s.selfServiceOnboarding {
		return ConsumeTelegramLinkResult{}, ErrSelfServiceOnboardingDisabled
	}
	if telegramUserID <= 0 || chatType != "private" {
		return ConsumeTelegramLinkResult{}, ErrTelegramLinkInvalid
	}
	canonical, err := normalizeHumanSecret(linkCode)
	if err != nil {
		return ConsumeTelegramLinkResult{}, ErrTelegramLinkInvalid
	}
	if s.telegramLinkAttempts == nil {
		return ConsumeTelegramLinkResult{}, errors.New("telegram link attempt limiter is not initialized")
	}
	if !s.telegramLinkAttempts.reserve(telegramUserID, now) {
		if err := s.RecordRateLimitAudit(
			RateLimitTelegramLinkConsumeTelegram,
			strconv.FormatInt(telegramUserID, 10),
			RateLimitAuditScope{},
		); err != nil {
			return ConsumeTelegramLinkResult{}, fmt.Errorf("record telegram link rate-limit audit: %w", err)
		}
		return ConsumeTelegramLinkResult{}, ErrTelegramLinkRateLimited
	}

	if err := s.checkpoint("telegram_link_transaction_attempting"); err != nil {
		return ConsumeTelegramLinkResult{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return ConsumeTelegramLinkResult{}, err
	}
	defer tx.Rollback()

	codeHash := hashToken(canonical)
	var issuerActorID, orbitID int64
	var desiredRole, storedCodeHash string
	var expiresAt int64
	var invalidatedAt, consumedAt sql.NullInt64
	var issuerActor, issuerRevoked, issuerMember, issuerLeft, targetOrbit sql.NullInt64
	var issuerRole, orbitStatus sql.NullString
	found := true
	err = tx.QueryRow(`SELECT
  c.issuer_actor_id,
  c.orbit_id,
  c.desired_role,
  c.code_hash,
  c.expires_at,
  c.invalidated_at,
  c.consumed_at,
  a.id,
  a.revoked_at,
  m.actor_id,
  m.role,
  m.left_at,
  o.id,
  o.status
FROM telegram_link_codes c
LEFT JOIN actors a ON a.id = c.issuer_actor_id
LEFT JOIN memberships m
  ON m.actor_id = c.issuer_actor_id AND m.orbit_id = c.orbit_id
LEFT JOIN orbits o ON o.id = c.orbit_id
WHERE c.code_hash = ?`, codeHash).Scan(
		&issuerActorID,
		&orbitID,
		&desiredRole,
		&storedCodeHash,
		&expiresAt,
		&invalidatedAt,
		&consumedAt,
		&issuerActor,
		&issuerRevoked,
		&issuerMember,
		&issuerRole,
		&issuerLeft,
		&targetOrbit,
		&orbitStatus,
	)
	if errors.Is(err, sql.ErrNoRows) {
		found = false
		storedCodeHash = dummyTelegramLinkCodeHash
	} else if err != nil {
		return ConsumeTelegramLinkResult{}, err
	}
	if err := s.checkpoint("telegram_link_preflight_read"); err != nil {
		return ConsumeTelegramLinkResult{}, err
	}
	if err := s.checkpoint("telegram_link_preflight_hash_compare"); err != nil {
		return ConsumeTelegramLinkResult{}, err
	}
	hashMatches := constantTimeHashEqual(canonical, storedCodeHash)
	valid := found && hashMatches &&
		!invalidatedAt.Valid && !consumedAt.Valid && expiresAt > now &&
		issuerActor.Valid && issuerActor.Int64 == issuerActorID && !issuerRevoked.Valid &&
		issuerMember.Valid && issuerMember.Int64 == issuerActorID && issuerRole.Valid && !issuerLeft.Valid &&
		targetOrbit.Valid && targetOrbit.Int64 == orbitID && orbitStatus.Valid && orbitStatus.String == "active" &&
		(issuerRole.String == "primary" || issuerRole.String == "companion") &&
		(desiredRole == "companion" || desiredRole == "satellite")
	if !valid {
		return ConsumeTelegramLinkResult{}, ErrTelegramLinkInvalid
	}
	if err := s.checkpoint("telegram_link_after_lookup"); err != nil {
		return ConsumeTelegramLinkResult{}, err
	}

	externalRef := strconv.FormatInt(telegramUserID, 10)
	name := sanitizeTelegramDisplayName(displayName)
	var actorID int64
	var actorRevoked sql.NullInt64
	err = tx.QueryRow(`SELECT id, revoked_at FROM actors
WHERE kind = 'telegram_user' AND external_ref = ?`, externalRef).Scan(&actorID, &actorRevoked)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		res, insertErr := tx.Exec(`INSERT INTO actors(kind, display_name, external_ref, created_at)
VALUES('telegram_user', ?, ?, ?)`, name, externalRef, now)
		if insertErr != nil {
			return ConsumeTelegramLinkResult{}, insertErr
		}
		actorID, err = res.LastInsertId()
		if err != nil {
			return ConsumeTelegramLinkResult{}, err
		}
	case err != nil:
		return ConsumeTelegramLinkResult{}, err
	case actorRevoked.Valid:
		return ConsumeTelegramLinkResult{}, ErrTelegramLinkInvalid
	default:
		if _, err := tx.Exec(`UPDATE actors SET display_name = ? WHERE id = ?`, name, actorID); err != nil {
			return ConsumeTelegramLinkResult{}, err
		}
	}

	var activeOrbit int64
	err = tx.QueryRow(`SELECT orbit_id FROM memberships
WHERE actor_id = ? AND left_at IS NULL`, actorID).Scan(&activeOrbit)
	if err == nil {
		if activeOrbit == orbitID {
			return ConsumeTelegramLinkResult{}, ErrTelegramAlreadyLinkedSameOrbit
		}
		return ConsumeTelegramLinkResult{}, ErrTelegramMemberOfOtherOrbit
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return ConsumeTelegramLinkResult{}, err
	}

	var legacyOrbit int64
	err = tx.QueryRow(`SELECT orbit_id FROM members WHERE tg_user_id = ?`, telegramUserID).Scan(&legacyOrbit)
	if err == nil {
		if legacyOrbit == orbitID {
			return ConsumeTelegramLinkResult{}, ErrTelegramAlreadyLinkedSameOrbit
		}
		return ConsumeTelegramLinkResult{}, ErrTelegramMemberOfOtherOrbit
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return ConsumeTelegramLinkResult{}, err
	}

	res, err := tx.Exec(`UPDATE telegram_link_codes
SET consumed_at = ?, consuming_actor_id = ?
WHERE code_hash = ?
  AND consumed_at IS NULL
  AND invalidated_at IS NULL
  AND expires_at > ?`, now, actorID, codeHash, now)
	if err != nil {
		return ConsumeTelegramLinkResult{}, err
	}
	if n, err := res.RowsAffected(); err != nil {
		return ConsumeTelegramLinkResult{}, err
	} else if n != 1 {
		return ConsumeTelegramLinkResult{}, ErrTelegramLinkInvalid
	}

	if _, err := tx.Exec(`INSERT INTO memberships(orbit_id, actor_id, role, joined_at, left_at)
VALUES(?, ?, ?, ?, NULL)
ON CONFLICT(orbit_id, actor_id) DO UPDATE SET
  role = excluded.role,
  joined_at = excluded.joined_at,
  left_at = NULL`, orbitID, actorID, desiredRole, now); err != nil {
		return ConsumeTelegramLinkResult{}, ErrTelegramLinkInvalid
	}
	if _, err := tx.Exec(`INSERT INTO members(orbit_id, tg_user_id, role, joined_at, display_name)
VALUES(?, ?, ?, ?, ?)
ON CONFLICT(orbit_id, tg_user_id) DO UPDATE SET
  role = excluded.role,
  joined_at = excluded.joined_at,
  display_name = excluded.display_name`, orbitID, telegramUserID, desiredRole, now, name); err != nil {
		return ConsumeTelegramLinkResult{}, ErrTelegramLinkInvalid
	}
	if _, err := tx.Exec(`INSERT INTO audit_events(orbit_id, actor_id, type, created_at)
VALUES(?, ?, 'telegram_link.consumed', ?)`, orbitID, actorID, now); err != nil {
		return ConsumeTelegramLinkResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return ConsumeTelegramLinkResult{}, err
	}
	return ConsumeTelegramLinkResult{OrbitID: orbitID, ActorID: actorID, Role: desiredRole}, nil
}

var dummyTelegramLinkCodeHash = hashToken(strings.Repeat("\x00", 64))

func sanitizeTelegramDisplayName(name string) string {
	if !utf8.ValidString(name) {
		return ""
	}
	name = strings.TrimSpace(name)
	var b strings.Builder
	b.Grow(len(name))
	count := 0
	for _, r := range name {
		if unicode.IsControl(r) || count >= 128 {
			continue
		}
		b.WriteRune(r)
		count++
	}
	result := strings.TrimSpace(b.String())
	return result
}
