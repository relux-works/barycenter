package store

import (
	"database/sql"
	"errors"
	"time"
)

const (
	CurrentContentPolicyVersion = "1.0"
	CurrentContentPolicyHash    = "a4d59ec7e9bfd8aeb2ec5d84356517580bde8df4540e6a2162f9206cd7ecd30e"
	ContentPolicyENHash         = "a25d1b46b530fb64f18224618701f67ed80ace9ce5c1b1cfb1a7c3d70a1988ca"
	ContentPolicyRUHash         = "4726df3e447674a5d6a87a34a5f05363c53604a80da3c1d6e69a3f9c05f41082"
	ContentPolicyEffectiveAt    = int64(1783972800000) // 2026-07-14T00:00:00+04:00
	ContentPolicyTermsURL       = "https://barycenter.live/legal/terms"
	ContentPolicyGuidelinesURL  = "https://barycenter.live/legal/content-guidelines"
	contentPolicyMutationLimit  = 10
	contentPolicyMutationWindow = time.Hour
)

var (
	ErrContentPolicyAcceptanceRequired = errors.New("current content policy acceptance is required")
	ErrContentPolicyInvalid            = errors.New("content policy request is invalid")
)

type ContentPolicyRateLimitError struct{ RetryAfter time.Duration }

func (e *ContentPolicyRateLimitError) Error() string { return "content policy mutation rate limited" }

type ContentPolicyLocale string

const (
	ContentPolicyLocaleEN ContentPolicyLocale = "en"
	ContentPolicyLocaleRU ContentPolicyLocale = "ru"
)

type ContentPolicyManifest struct {
	Version              string
	Hash                 string
	Locale               ContentPolicyLocale
	LocaleHash           string
	EffectiveAt          int64
	TermsURL             string
	ContentGuidelinesURL string
	Title                string
	RightsText           string
	ConsentText          string
	ControllingLanguage  string
}

type ContentPolicyGrant struct {
	ActorID       int64
	OrbitID       int64
	Version       string
	PolicyHash    string
	Locale        ContentPolicyLocale
	AcceptedVia   string
	AcceptedAt    int64
	RevokedAt     int64
	Revision      int64
	Current       bool
	TermsAccepted bool
}

type AcceptContentPolicyParams struct {
	ExpectedActorID int64
	Identity        Identity
	Version         string
	PolicyHash      string
	Locale          ContentPolicyLocale
	AcceptedAt      int64
}

const contentPolicySchema = `
CREATE TABLE IF NOT EXISTS content_policy_versions (
  version TEXT PRIMARY KEY CHECK(length(version) BETWEEN 1 AND 32),
  policy_hash TEXT NOT NULL UNIQUE CHECK(
    length(policy_hash) = 64 AND policy_hash NOT GLOB '*[^0-9a-f]*'
  ),
  en_hash TEXT NOT NULL CHECK(length(en_hash) = 64 AND en_hash NOT GLOB '*[^0-9a-f]*'),
  ru_hash TEXT NOT NULL CHECK(length(ru_hash) = 64 AND ru_hash NOT GLOB '*[^0-9a-f]*'),
  effective_at INTEGER NOT NULL CHECK(effective_at > 0),
  material INTEGER NOT NULL CHECK(material IN (0, 1)),
  current INTEGER NOT NULL CHECK(current IN (0, 1)),
  created_at INTEGER NOT NULL CHECK(created_at > 0)
);
CREATE UNIQUE INDEX IF NOT EXISTS content_policy_one_current
  ON content_policy_versions(current) WHERE current = 1;

CREATE TABLE IF NOT EXISTS content_policy_acceptances (
  actor_id INTEGER NOT NULL CHECK(actor_id > 0),
  orbit_id INTEGER NOT NULL CHECK(orbit_id > 0),
  policy_version TEXT NOT NULL REFERENCES content_policy_versions(version),
  policy_hash TEXT NOT NULL CHECK(
    length(policy_hash) = 64 AND policy_hash NOT GLOB '*[^0-9a-f]*'
  ),
  locale TEXT NOT NULL CHECK(locale IN ('en', 'ru')),
  accepted_via TEXT NOT NULL CHECK(accepted_via IN ('control', 'telegram')),
  terms_accepted INTEGER NOT NULL CHECK(terms_accepted IN (0, 1)),
  accepted_at INTEGER NOT NULL CHECK(accepted_at > 0),
  revoked_at INTEGER NOT NULL DEFAULT 0 CHECK(revoked_at >= 0),
  revision INTEGER NOT NULL DEFAULT 1 CHECK(revision > 0),
  PRIMARY KEY(actor_id, policy_version),
  CHECK(revoked_at = 0 OR revoked_at >= accepted_at)
);
CREATE INDEX IF NOT EXISTS content_policy_acceptance_current
  ON content_policy_acceptances(actor_id, orbit_id, policy_version, revoked_at);

CREATE TABLE IF NOT EXISTS content_policy_audit (
  id INTEGER PRIMARY KEY,
  actor_id INTEGER NOT NULL CHECK(actor_id > 0),
  orbit_id INTEGER NOT NULL CHECK(orbit_id > 0),
  policy_version TEXT NOT NULL,
  policy_hash TEXT NOT NULL CHECK(
    length(policy_hash) = 64 AND policy_hash NOT GLOB '*[^0-9a-f]*'
  ),
  locale TEXT NOT NULL CHECK(locale IN ('en', 'ru')),
  event TEXT NOT NULL CHECK(event IN ('accepted', 'revoked')),
  transport TEXT NOT NULL CHECK(transport IN ('control', 'telegram')),
  occurred_at INTEGER NOT NULL CHECK(occurred_at > 0)
);
CREATE INDEX IF NOT EXISTS content_policy_audit_actor_time
  ON content_policy_audit(actor_id, occurred_at DESC, id DESC);
`

func (s *Store) initContentPolicySchema() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(contentPolicySchema); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE content_policy_versions SET current = 0
WHERE current = 1 AND version <> ?`, CurrentContentPolicyVersion); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO content_policy_versions(
  version, policy_hash, en_hash, ru_hash, effective_at, material, current, created_at
) VALUES(?, ?, ?, ?, ?, 1, 1, ?)
ON CONFLICT(version) DO UPDATE SET
  current = 1
WHERE policy_hash = excluded.policy_hash AND en_hash = excluded.en_hash
  AND ru_hash = excluded.ru_hash AND effective_at = excluded.effective_at`,
		CurrentContentPolicyVersion, CurrentContentPolicyHash, ContentPolicyENHash,
		ContentPolicyRUHash, ContentPolicyEffectiveAt, ContentPolicyEffectiveAt); err != nil {
		return err
	}
	var version, hash string
	if err := tx.QueryRow(`SELECT version, policy_hash FROM content_policy_versions
WHERE current = 1`).Scan(&version, &hash); err != nil {
		return err
	}
	if version != CurrentContentPolicyVersion || hash != CurrentContentPolicyHash {
		return errors.New("content policy current version differs from approved binary manifest")
	}
	if err := foreignKeyCheck(tx); err != nil {
		return err
	}
	if err := s.checkpoint("content_policy_ddl_before_commit"); err != nil {
		return err
	}
	return tx.Commit()
}

func validContentPolicyLocale(locale ContentPolicyLocale) bool {
	return locale == ContentPolicyLocaleEN || locale == ContentPolicyLocaleRU
}

func CurrentContentPolicy(locale ContentPolicyLocale) (ContentPolicyManifest, error) {
	manifest := ContentPolicyManifest{
		Version: CurrentContentPolicyVersion, Hash: CurrentContentPolicyHash,
		Locale: locale, EffectiveAt: ContentPolicyEffectiveAt,
		TermsURL: ContentPolicyTermsURL, ContentGuidelinesURL: ContentPolicyGuidelinesURL,
		ControllingLanguage: "en",
	}
	switch locale {
	case ContentPolicyLocaleEN:
		manifest.LocaleHash = ContentPolicyENHash
		manifest.Title = "Upload and sharing rights"
		manifest.RightsText = "Upload, record, or send only content you created or have the rights, permissions, and recording consents to process and share with every selected recipient. Acceptance does not prove ownership or replace those rights."
		manifest.ConsentText = "I accept the current Pulsar Terms and Content Guidelines. Each file upload will separately ask me to confirm the rights reminder."
	case ContentPolicyLocaleRU:
		manifest.LocaleHash = ContentPolicyRUHash
		manifest.Title = "Права на загрузку и передачу"
		manifest.RightsText = "Загружайте, записывайте и отправляйте только материал, который вы создали либо на обработку и передачу которого каждому выбранному получателю у вас есть права, разрешения и согласия на запись. Принятие не доказывает право собственности и не заменяет такие права."
		manifest.ConsentText = "Я принимаю текущие Условия Pulsar и Правила содержимого. При каждой загрузке файла я отдельно подтвержу напоминание о правах."
	default:
		return ContentPolicyManifest{}, ErrContentPolicyInvalid
	}
	return manifest, nil
}

func contentPolicyTransport(identity Identity) string {
	if identity.Kind == IdentityTelegram {
		return "telegram"
	}
	return "control"
}

func authorizeContentPolicyActorTx(
	tx *sql.Tx, expectedActorID int64, identity Identity,
) (ActorContext, error) {
	ctx, err := resolveActorContext(tx, identity)
	if err != nil || ctx.ActorID != expectedActorID {
		if err == nil {
			err = ErrUnauthorized
		}
		return ActorContext{}, err
	}
	if !ctx.Capabilities.Has(CapabilityControl) && !ctx.Capabilities.Has(CapabilityTelegram) {
		return ActorContext{}, ErrInsufficientCapability
	}
	return ctx, nil
}

func contentPolicyMutationRateTx(tx *sql.Tx, actorID, now int64) error {
	cutoff := now - contentPolicyMutationWindow.Milliseconds()
	var count int
	var oldest int64
	if err := tx.QueryRow(`SELECT COUNT(*), COALESCE(MIN(occurred_at), 0)
FROM content_policy_audit WHERE actor_id = ? AND occurred_at > ?`,
		actorID, cutoff).Scan(&count, &oldest); err != nil {
		return err
	}
	if count < contentPolicyMutationLimit {
		return nil
	}
	retry := time.Duration(oldest+contentPolicyMutationWindow.Milliseconds()-now) * time.Millisecond
	if retry < time.Second {
		retry = time.Second
	}
	return &ContentPolicyRateLimitError{RetryAfter: retry}
}

func (s *Store) AcceptContentPolicy(params AcceptContentPolicyParams) (ContentPolicyGrant, error) {
	if params.ExpectedActorID <= 0 || params.AcceptedAt <= 0 ||
		params.Version != CurrentContentPolicyVersion ||
		params.PolicyHash != CurrentContentPolicyHash ||
		!validContentPolicyLocale(params.Locale) {
		return ContentPolicyGrant{}, ErrContentPolicyInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return ContentPolicyGrant{}, err
	}
	defer tx.Rollback()
	ctx, err := authorizeContentPolicyActorTx(tx, params.ExpectedActorID, params.Identity)
	if err != nil {
		return ContentPolicyGrant{}, err
	}
	if err := contentPolicyMutationRateTx(tx, ctx.ActorID, params.AcceptedAt); err != nil {
		return ContentPolicyGrant{}, err
	}
	transport := contentPolicyTransport(params.Identity)
	if _, err := tx.Exec(`INSERT INTO content_policy_acceptances(
  actor_id, orbit_id, policy_version, policy_hash, locale, accepted_via,
  terms_accepted, accepted_at, revoked_at, revision
) VALUES(?, ?, ?, ?, ?, ?, 1, ?, 0, 1)
ON CONFLICT(actor_id, policy_version) DO UPDATE SET
  orbit_id = excluded.orbit_id, policy_hash = excluded.policy_hash,
  locale = excluded.locale, accepted_via = excluded.accepted_via,
  terms_accepted = 1, accepted_at = excluded.accepted_at, revoked_at = 0,
  revision = content_policy_acceptances.revision + 1`, ctx.ActorID, ctx.OrbitID,
		params.Version, params.PolicyHash, params.Locale, transport,
		params.AcceptedAt); err != nil {
		return ContentPolicyGrant{}, err
	}
	if _, err := tx.Exec(`INSERT INTO content_policy_audit(
  actor_id, orbit_id, policy_version, policy_hash, locale, event, transport, occurred_at
) VALUES(?, ?, ?, ?, ?, 'accepted', ?, ?)`, ctx.ActorID, ctx.OrbitID,
		params.Version, params.PolicyHash, params.Locale, transport,
		params.AcceptedAt); err != nil {
		return ContentPolicyGrant{}, err
	}
	grant, err := contentPolicyGrantTx(tx, ctx.ActorID, ctx.OrbitID)
	if err != nil {
		return ContentPolicyGrant{}, err
	}
	if err := s.checkpoint("content_policy_accept_before_commit"); err != nil {
		return ContentPolicyGrant{}, err
	}
	if err := tx.Commit(); err != nil {
		return ContentPolicyGrant{}, err
	}
	return grant, nil
}

func contentPolicyGrantTx(tx *sql.Tx, actorID, orbitID int64) (ContentPolicyGrant, error) {
	var grant ContentPolicyGrant
	var termsAccepted int
	err := tx.QueryRow(`SELECT actor_id, orbit_id, policy_version, policy_hash,
       locale, accepted_via, accepted_at, revoked_at, revision, terms_accepted
FROM content_policy_acceptances
WHERE actor_id = ? AND orbit_id = ? AND policy_version = ?`, actorID, orbitID,
		CurrentContentPolicyVersion).Scan(&grant.ActorID, &grant.OrbitID,
		&grant.Version, &grant.PolicyHash, &grant.Locale, &grant.AcceptedVia,
		&grant.AcceptedAt, &grant.RevokedAt, &grant.Revision, &termsAccepted)
	if errors.Is(err, sql.ErrNoRows) {
		return ContentPolicyGrant{}, ErrContentPolicyAcceptanceRequired
	}
	if err != nil {
		return ContentPolicyGrant{}, err
	}
	grant.TermsAccepted = termsAccepted != 0
	grant.Current = grant.Version == CurrentContentPolicyVersion &&
		grant.PolicyHash == CurrentContentPolicyHash && grant.RevokedAt == 0 &&
		grant.TermsAccepted
	if !grant.Current {
		return ContentPolicyGrant{}, ErrContentPolicyAcceptanceRequired
	}
	return grant, nil
}

func requireCurrentContentPolicyTx(tx *sql.Tx, ctx ActorContext) error {
	_, err := contentPolicyGrantTx(tx, ctx.ActorID, ctx.OrbitID)
	return err
}

func (s *Store) RequireCurrentContentPolicy(
	expectedActorID int64, identity Identity,
) (ContentPolicyGrant, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return ContentPolicyGrant{}, err
	}
	defer tx.Rollback()
	ctx, err := authorizeContentPolicyActorTx(tx, expectedActorID, identity)
	if err != nil {
		return ContentPolicyGrant{}, err
	}
	grant, err := contentPolicyGrantTx(tx, ctx.ActorID, ctx.OrbitID)
	if err != nil {
		return ContentPolicyGrant{}, err
	}
	if err := tx.Commit(); err != nil {
		return ContentPolicyGrant{}, err
	}
	return grant, nil
}

func (s *Store) RevokeContentPolicy(
	expectedActorID int64, identity Identity, locale ContentPolicyLocale, now int64,
) (ContentPolicyGrant, error) {
	if expectedActorID <= 0 || now <= 0 || !validContentPolicyLocale(locale) {
		return ContentPolicyGrant{}, ErrContentPolicyInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return ContentPolicyGrant{}, err
	}
	defer tx.Rollback()
	ctx, err := authorizeContentPolicyActorTx(tx, expectedActorID, identity)
	if err != nil {
		return ContentPolicyGrant{}, err
	}
	if err := contentPolicyMutationRateTx(tx, ctx.ActorID, now); err != nil {
		return ContentPolicyGrant{}, err
	}
	result, err := tx.Exec(`UPDATE content_policy_acceptances
SET revoked_at = ?, revision = revision + 1
WHERE actor_id = ? AND orbit_id = ? AND policy_version = ? AND revoked_at = 0`,
		now, ctx.ActorID, ctx.OrbitID, CurrentContentPolicyVersion)
	if err != nil {
		return ContentPolicyGrant{}, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return ContentPolicyGrant{}, err
	}
	if changed == 0 {
		return ContentPolicyGrant{}, ErrContentPolicyAcceptanceRequired
	}
	transport := contentPolicyTransport(identity)
	if _, err := tx.Exec(`INSERT INTO content_policy_audit(
  actor_id, orbit_id, policy_version, policy_hash, locale, event, transport, occurred_at
) VALUES(?, ?, ?, ?, ?, 'revoked', ?, ?)`, ctx.ActorID, ctx.OrbitID,
		CurrentContentPolicyVersion, CurrentContentPolicyHash, locale, transport,
		now); err != nil {
		return ContentPolicyGrant{}, err
	}
	var grant ContentPolicyGrant
	var terms int
	if err := tx.QueryRow(`SELECT actor_id, orbit_id, policy_version, policy_hash,
 locale, accepted_via, accepted_at, revoked_at, revision, terms_accepted
FROM content_policy_acceptances WHERE actor_id = ? AND policy_version = ?`,
		ctx.ActorID, CurrentContentPolicyVersion).Scan(&grant.ActorID,
		&grant.OrbitID, &grant.Version, &grant.PolicyHash, &grant.Locale,
		&grant.AcceptedVia, &grant.AcceptedAt, &grant.RevokedAt,
		&grant.Revision, &terms); err != nil {
		return ContentPolicyGrant{}, err
	}
	grant.TermsAccepted = terms != 0
	if err := s.checkpoint("content_policy_revoke_before_commit"); err != nil {
		return ContentPolicyGrant{}, err
	}
	if err := tx.Commit(); err != nil {
		return ContentPolicyGrant{}, err
	}
	return grant, nil
}
