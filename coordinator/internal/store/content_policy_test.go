package store

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func acceptCurrentContentPolicy(
	t *testing.T,
	st *Store,
	credentials OnboardingCredentials,
	now int64,
) ContentPolicyGrant {
	t.Helper()
	grant, err := st.AcceptContentPolicy(AcceptContentPolicyParams{
		ExpectedActorID: credentials.ActorID,
		Identity: Identity{
			Kind:  IdentityBearer,
			Token: credentials.ControlToken,
		},
		Version:    CurrentContentPolicyVersion,
		PolicyHash: CurrentContentPolicyHash,
		Locale:     ContentPolicyLocaleEN,
		AcceptedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	return grant
}

func openContentPolicyTestStore(t *testing.T) (*Store, OnboardingCredentials) {
	t.Helper()
	st, err := OpenWithOptions(filepath.Join(t.TempDir(), "content-policy.db"), Options{
		SelfServiceOnboarding: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	credentials, err := st.CreateSelfServiceOrbit("Content policy test")
	if err != nil {
		t.Fatal(err)
	}
	return st, credentials
}

func TestCurrentContentPolicyPublishesApprovedEquivalentLocales(t *testing.T) {
	en, err := CurrentContentPolicy(ContentPolicyLocaleEN)
	if err != nil {
		t.Fatal(err)
	}
	ru, err := CurrentContentPolicy(ContentPolicyLocaleRU)
	if err != nil {
		t.Fatal(err)
	}
	for _, manifest := range []ContentPolicyManifest{en, ru} {
		if manifest.Version != "1.0" || manifest.Hash != CurrentContentPolicyHash ||
			manifest.TermsURL != "https://barycenter.live/legal/terms" ||
			manifest.ContentGuidelinesURL != "https://barycenter.live/legal/content-guidelines" ||
			manifest.ControllingLanguage != "en" || manifest.EffectiveAt != ContentPolicyEffectiveAt {
			t.Fatalf("manifest=%+v", manifest)
		}
		if !strings.Contains(manifest.RightsText, "not") &&
			!strings.Contains(manifest.RightsText, "не ") {
			t.Fatalf("rights disclaimer missing: %q", manifest.RightsText)
		}
	}
	if en.LocaleHash != ContentPolicyENHash || ru.LocaleHash != ContentPolicyRUHash ||
		en.RightsText == ru.RightsText || en.ConsentText == ru.ConsentText {
		t.Fatalf("en=%+v ru=%+v", en, ru)
	}
	if _, err := CurrentContentPolicy("ka"); !errors.Is(err, ErrContentPolicyInvalid) {
		t.Fatalf("unsupported locale error=%v", err)
	}
}

func TestContentPolicyGrantAuthorizationLifecycleAndAudit(t *testing.T) {
	st, credentials := openContentPolicyTestStore(t)
	now := time.Now().UnixMilli()
	if _, err := st.RequireCurrentContentPolicy(credentials.ActorID, Identity{
		Kind: IdentityBearer, Token: credentials.ControlToken,
	}); !errors.Is(err, ErrContentPolicyAcceptanceRequired) {
		t.Fatalf("missing grant error=%v", err)
	}
	if _, err := st.AcceptContentPolicy(AcceptContentPolicyParams{
		ExpectedActorID: credentials.ActorID,
		Identity:        Identity{Kind: IdentityBearer, Token: credentials.NodeToken},
		Version:         CurrentContentPolicyVersion, PolicyHash: CurrentContentPolicyHash,
		Locale: ContentPolicyLocaleEN, AcceptedAt: now,
	}); !errors.Is(err, ErrInsufficientCapability) {
		t.Fatalf("node acceptance error=%v", err)
	}
	if _, err := st.AcceptContentPolicy(AcceptContentPolicyParams{
		ExpectedActorID: credentials.ActorID + 1,
		Identity:        Identity{Kind: IdentityBearer, Token: credentials.ControlToken},
		Version:         CurrentContentPolicyVersion, PolicyHash: CurrentContentPolicyHash,
		Locale: ContentPolicyLocaleEN, AcceptedAt: now,
	}); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("actor mismatch error=%v", err)
	}
	for name, mutate := range map[string]func(*AcceptContentPolicyParams){
		"version": func(p *AcceptContentPolicyParams) { p.Version = "0.9" },
		"hash":    func(p *AcceptContentPolicyParams) { p.PolicyHash = strings.Repeat("0", 64) },
		"locale":  func(p *AcceptContentPolicyParams) { p.Locale = "ka" },
	} {
		t.Run(name, func(t *testing.T) {
			params := AcceptContentPolicyParams{
				ExpectedActorID: credentials.ActorID,
				Identity:        Identity{Kind: IdentityBearer, Token: credentials.ControlToken},
				Version:         CurrentContentPolicyVersion, PolicyHash: CurrentContentPolicyHash,
				Locale: ContentPolicyLocaleEN, AcceptedAt: now,
			}
			mutate(&params)
			if _, err := st.AcceptContentPolicy(params); !errors.Is(err, ErrContentPolicyInvalid) {
				t.Fatalf("invalid request error=%v", err)
			}
		})
	}

	accepted := acceptCurrentContentPolicy(t, st, credentials, now)
	if !accepted.Current || !accepted.TermsAccepted || accepted.AcceptedVia != "control" ||
		accepted.AcceptedAt != now || accepted.Revision != 1 {
		t.Fatalf("accepted=%+v", accepted)
	}
	revoked, err := st.RevokeContentPolicy(credentials.ActorID, Identity{
		Kind: IdentityBearer, Token: credentials.ControlToken,
	}, ContentPolicyLocaleEN, now+1)
	if err != nil || revoked.Current || revoked.RevokedAt != now+1 || revoked.Revision != 2 {
		t.Fatalf("revoked=%+v err=%v", revoked, err)
	}
	if _, err := st.RequireCurrentContentPolicy(credentials.ActorID, Identity{
		Kind: IdentityBearer, Token: credentials.ControlToken,
	}); !errors.Is(err, ErrContentPolicyAcceptanceRequired) {
		t.Fatalf("revoked grant error=%v", err)
	}
	reaccepted := acceptCurrentContentPolicy(t, st, credentials, now+2)
	if !reaccepted.Current || reaccepted.Revision != 3 || reaccepted.AcceptedAt != now+2 {
		t.Fatalf("reaccepted=%+v", reaccepted)
	}

	var accepts, revokes int
	if err := st.db.QueryRow(`SELECT
  SUM(CASE WHEN event = 'accepted' THEN 1 ELSE 0 END),
  SUM(CASE WHEN event = 'revoked' THEN 1 ELSE 0 END)
FROM content_policy_audit WHERE actor_id = ?`, credentials.ActorID).Scan(
		&accepts, &revokes); err != nil {
		t.Fatal(err)
	}
	if accepts != 2 || revokes != 1 {
		t.Fatalf("audit accepts=%d revokes=%d", accepts, revokes)
	}
	var leakedColumns int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('content_policy_acceptances')
WHERE lower(name) LIKE '%content%' OR lower(name) LIKE '%filename%' OR lower(name) LIKE '%transport_id%'`).Scan(&leakedColumns); err != nil {
		t.Fatal(err)
	}
	if leakedColumns != 0 {
		t.Fatalf("consent schema contains %d forbidden metadata column(s)", leakedColumns)
	}
}

func TestContentPolicyMaterialChangeAndRateLimitFailClosed(t *testing.T) {
	st, credentials := openContentPolicyTestStore(t)
	now := time.Now().UnixMilli()
	acceptCurrentContentPolicy(t, st, credentials, now)
	if _, err := st.db.Exec(`UPDATE content_policy_acceptances SET policy_hash = ?
WHERE actor_id = ? AND policy_version = ?`, strings.Repeat("0", 64),
		credentials.ActorID, CurrentContentPolicyVersion); err != nil {
		t.Fatal(err)
	}
	if _, err := st.RequireCurrentContentPolicy(credentials.ActorID, Identity{
		Kind: IdentityBearer, Token: credentials.ControlToken,
	}); !errors.Is(err, ErrContentPolicyAcceptanceRequired) {
		t.Fatalf("stale hash error=%v", err)
	}

	other, err := st.CreateSelfServiceOrbit("Rate limited policy actor")
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < contentPolicyMutationLimit; index++ {
		acceptCurrentContentPolicy(t, st, other, now+int64(index))
	}
	_, err = st.AcceptContentPolicy(AcceptContentPolicyParams{
		ExpectedActorID: other.ActorID,
		Identity:        Identity{Kind: IdentityBearer, Token: other.ControlToken},
		Version:         CurrentContentPolicyVersion, PolicyHash: CurrentContentPolicyHash,
		Locale: ContentPolicyLocaleRU, AcceptedAt: now + contentPolicyMutationLimit,
	})
	var limited *ContentPolicyRateLimitError
	if !errors.As(err, &limited) || limited.RetryAfter <= 0 {
		t.Fatalf("rate limit error=%v", err)
	}
}

func TestContentPolicyAcceptsLinkedTelegramActorContext(t *testing.T) {
	st, owner := openContentPolicyTestStore(t)
	link, err := st.IssueTelegramLink(owner.ActorID, owner.ControlToken, "companion")
	if err != nil {
		t.Fatal(err)
	}
	const telegramUserID int64 = 95550123
	linked, err := st.ConsumeTelegramLink(
		telegramUserID, "Policy Telegram", "private", link.Code,
	)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	grant, err := st.AcceptContentPolicy(AcceptContentPolicyParams{
		ExpectedActorID: linked.ActorID,
		Identity:        Identity{Kind: IdentityTelegram, TelegramUserID: telegramUserID},
		Version:         CurrentContentPolicyVersion, PolicyHash: CurrentContentPolicyHash,
		Locale: ContentPolicyLocaleRU, AcceptedAt: now,
	})
	if err != nil || !grant.Current || grant.AcceptedVia != "telegram" ||
		grant.Locale != ContentPolicyLocaleRU || grant.OrbitID != owner.OrbitID {
		t.Fatalf("telegram grant=%+v err=%v", grant, err)
	}
}

func TestContentPolicyGatesNewUploadAndTransmissionWithoutChangingAcceptedMedia(t *testing.T) {
	st, credentials := openContentPolicyTestStore(t)
	now := time.Now().UnixMilli()
	upload := authorizedUploadParams(credentials, now, "policy-gated-upload-0001", 128)
	if _, err := st.CreateAuthorizedMediaUpload(
		credentials.ActorID, credentials.ControlToken, upload, permissiveMediaUploadQuota(),
	); !errors.Is(err, ErrContentPolicyAcceptanceRequired) {
		t.Fatalf("ungated upload error=%v", err)
	}
	acceptCurrentContentPolicy(t, st, credentials, now+1)
	if _, err := st.CreateAuthorizedMediaUpload(
		credentials.ActorID, credentials.ControlToken, upload, permissiveMediaUploadQuota(),
	); err != nil {
		t.Fatal(err)
	}

	media := readyLifecycleMedia(t, st, credentials, now+2,
		now+int64((7*24*time.Hour)/time.Millisecond))
	if _, err := st.RevokeContentPolicy(credentials.ActorID, Identity{
		Kind: IdentityBearer, Token: credentials.ControlToken,
	}, ContentPolicyLocaleEN, now+5); err != nil {
		t.Fatal(err)
	}
	params := resolvedTransmissionParams(credentials, media, now+6)
	params.Availability = []TransmissionTargetAvailability{
		fullTransmissionAvailability(credentials, params.AcceptedAt),
	}
	if _, err := st.CreateResolvedTransmission(params); !errors.Is(err, ErrContentPolicyAcceptanceRequired) {
		t.Fatalf("ungated transmission error=%v", err)
	}
	stored, err := st.GetMediaItem(media.ID)
	if err != nil || stored == nil || stored.Status != MediaStatusReady || stored.ID != media.ID {
		t.Fatalf("accepted media changed after policy revoke: %+v err=%v", stored, err)
	}
	acceptCurrentContentPolicy(t, st, credentials, now+7)
	if _, err := st.CreateResolvedTransmission(params); err != nil {
		t.Fatal(err)
	}
}
