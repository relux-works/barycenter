package store

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func readySavedCueMedia(t *testing.T, st *Store, owner OnboardingCredentials, now, expiresAt int64, suffix string, size, duration int64) MediaItem {
	t.Helper()
	item, err := st.CreateMediaItem(CreateMediaItemParams{
		OwnerOrbitID: owner.OrbitID, ActorID: owner.ActorID,
		Kind: MediaKindAudioClip, Source: MediaSourceApp,
		Title: "saved-cue-" + suffix, CreatedAt: now, ExpiresAt: expiresAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	op, err := st.StageMediaPublication(item.ID, item.Revision, now+1)
	if err != nil {
		t.Fatal(err)
	}
	sha := strings.Repeat(suffix[:1], 64)
	ready, err := st.CompleteMediaPublication(op.ID, op.Revision, MediaPublication{
		MIME: "audio/wav", Codec: "pcm_s16le", DurationMS: duration,
		SizeBytes: size, SHA256: sha,
		LoudnessJSON: `{"input_i":"-20.0","output_i":"-14.0"}`,
	}, now+2)
	if err != nil {
		t.Fatal(err)
	}
	return ready
}

func createMediaSavedCue(t *testing.T, st *Store, owner OnboardingCredentials, media MediaItem, title string, now int64) SavedCue {
	t.Helper()
	cue, reused, err := st.CreateSavedCue(SavedCueMutationParams{
		ExpectedActorID: owner.ActorID, Bearer: owner.ControlToken,
		Title: title, MediaID: media.ID, OccurredAt: now,
	})
	if err != nil || reused {
		t.Fatalf("create cue=%+v reused=%v err=%v", cue, reused, err)
	}
	return cue
}

func TestSavedCuePinsCanonicalMediaAndDeleteReleasesExpiredSource(t *testing.T) {
	st, owner := newMediaIngestTestStore(t)
	now := time.Now().UnixMilli()
	media := readySavedCueMedia(t, st, owner, now, now+10, "a", 4096, 1000)
	cue := createMediaSavedCue(t, st, owner, media, "Door bell", now+3)

	items, err := st.ExpiredMediaItems(now+20, 10)
	if err != nil || len(items) != 0 {
		t.Fatalf("pinned expiry items=%+v err=%v", items, err)
	}
	backlog, err := st.MediaLifecycleBacklog(now + 20)
	if err != nil || backlog.ExpirableMedia != 0 {
		t.Fatalf("pinned backlog=%+v err=%v", backlog, err)
	}
	if _, err := st.ExpireMediaItem(media.ID, media.Revision, now+20); !errors.Is(err, ErrMediaStateConflict) {
		t.Fatalf("direct pinned expiry error=%v", err)
	}

	deleted, err := st.DeleteSavedCue(owner.ActorID, owner.ControlToken, cue.ID, cue.Revision, now+20)
	if err != nil || deleted.State != SavedCueDeleted || deleted.SourceGeneration != 2 {
		t.Fatalf("deleted cue=%+v err=%v", deleted, err)
	}
	stored, err := st.GetMediaItem(media.ID)
	if err != nil || stored == nil || stored.Status != MediaStatusExpired || stored.StorageKey != "" {
		t.Fatalf("released source=%+v err=%v", stored, err)
	}
	usage, err := st.SavedCueUsage(owner.OrbitID)
	if err != nil || usage.Count != 0 || usage.Bytes != 0 {
		t.Fatalf("usage after delete=%+v err=%v", usage, err)
	}
	pending, err := st.PendingSavedCueRevocations(10)
	if err != nil || len(pending) != 1 || pending[0].Reason != "cue_deleted" || pending[0].InvalidatedGeneration != 1 {
		t.Fatalf("pending=%+v err=%v", pending, err)
	}
	if pending[0].PendingAction != "cancel" || pending[0].ActiveAction != "fade_stop" ||
		pending[0].InterruptedMainAction != "resume_once" {
		t.Fatalf("unsafe revocation actions=%+v", pending[0])
	}
}

func TestSavedCueDedupeRenameReplaceAndGenerationOrdering(t *testing.T) {
	st, owner := newMediaIngestTestStore(t)
	now := time.Now().UnixMilli()
	one := readySavedCueMedia(t, st, owner, now, now+100_000, "b", 1000, 500)
	two := readySavedCueMedia(t, st, owner, now+10, now+100_000, "c", 2000, 700)
	cue := createMediaSavedCue(t, st, owner, one, "One", now+20)

	reused, deduped, err := st.CreateSavedCue(SavedCueMutationParams{
		ExpectedActorID: owner.ActorID, Bearer: owner.ControlToken,
		Title: "ignored", MediaID: one.ID, OccurredAt: now + 21,
	})
	if err != nil || !deduped || reused.ID != cue.ID {
		t.Fatalf("dedupe=%+v reused=%v err=%v", reused, deduped, err)
	}
	renamed, err := st.RenameSavedCue(owner.ActorID, owner.ControlToken, cue.ID, "Two", cue.Revision, now+22)
	if err != nil || renamed.Revision != 2 || renamed.SourceGeneration != 1 || renamed.Title != "Two" {
		t.Fatalf("renamed=%+v err=%v", renamed, err)
	}
	replaced, err := st.ReplaceSavedCue(SavedCueMutationParams{
		ExpectedActorID: owner.ActorID, Bearer: owner.ControlToken,
		MediaID: two.ID, OccurredAt: now + 23,
	}, cue.ID, renamed.Revision)
	if err != nil || replaced.Revision != 3 || replaced.SourceGeneration != 2 || replaced.MediaID != two.ID {
		t.Fatalf("replaced=%+v err=%v", replaced, err)
	}
	usage, err := st.SavedCueUsage(owner.OrbitID)
	if err != nil || usage != (SavedCueUsage{Count: 1, Bytes: 2000}) {
		t.Fatalf("usage=%+v err=%v", usage, err)
	}
	pending, err := st.PendingSavedCueRevocations(10)
	if err != nil || len(pending) != 1 || pending[0].Reason != "cue_replaced" || pending[0].InvalidatedGeneration != 1 {
		t.Fatalf("pending=%+v err=%v", pending, err)
	}
	injected := errors.New("revocation completion interrupted")
	st.testCheckpoint = func(name string) error {
		if name == "saved_cue_revocation_complete_before_commit" {
			return injected
		}
		return nil
	}
	if _, err := st.CompleteSavedCueRevocation(cue.ID, 1, now+24); !errors.Is(err, injected) {
		t.Fatalf("injected completion error=%v", err)
	}
	st.testCheckpoint = nil
	if stillPending, err := st.PendingSavedCueRevocations(10); err != nil || len(stillPending) != 1 {
		t.Fatalf("completion rollback pending=%+v err=%v", stillPending, err)
	}
	done, err := st.CompleteSavedCueRevocation(cue.ID, 1, now+24)
	if err != nil || done.State != "done" || done.CompletedAt != now+24 {
		t.Fatalf("completed=%+v err=%v", done, err)
	}
}

func TestSavedCueRejectsForeignUnreadyWrongClassOversizeAndNodeCredential(t *testing.T) {
	st, owner := newMediaIngestTestStore(t)
	foreign, err := st.CreateSelfServiceOrbit("foreign cue")
	if err != nil {
		t.Fatal(err)
	}
	acceptCurrentContentPolicy(t, st, foreign, time.Now().UnixMilli())
	now := time.Now().UnixMilli()
	valid := readySavedCueMedia(t, st, owner, now, now+100_000, "d", 1024, 500)
	oversize := readySavedCueMedia(t, st, owner, now+10, now+100_000, "e", SavedCueMaxItemBytes+1, 500)
	expired := readySavedCueMedia(t, st, owner, now+20, now+35, "f", 1024, 500)
	unready, err := st.CreateMediaItem(CreateMediaItemParams{
		OwnerOrbitID: owner.OrbitID, ActorID: owner.ActorID, Kind: MediaKindAudioClip,
		Source: MediaSourceApp, Title: "unready", CreatedAt: now + 30, ExpiresAt: now + 100_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	voice := readyLifecycleMedia(t, st, owner, now+40, now+100_000)

	cases := []struct {
		name  string
		actor int64
		token string
		media string
		want  error
	}{
		{"foreign", foreign.ActorID, foreign.ControlToken, valid.ID, ErrSavedCueNotFound},
		{"unready", owner.ActorID, owner.ControlToken, unready.ID, ErrSavedCueInvalid},
		{"voice", owner.ActorID, owner.ControlToken, voice.ID, ErrSavedCueInvalid},
		{"oversize", owner.ActorID, owner.ControlToken, oversize.ID, ErrSavedCueInvalid},
		{"expired", owner.ActorID, owner.ControlToken, expired.ID, ErrSavedCueInvalid},
		{"node", owner.ActorID, owner.NodeToken, valid.ID, ErrInsufficientCapability},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := st.CreateSavedCue(SavedCueMutationParams{
				ExpectedActorID: tc.actor, Bearer: tc.token, Title: "cue",
				MediaID: tc.media, OccurredAt: now + 100 + int64(i),
			})
			if !errors.Is(err, tc.want) {
				t.Fatalf("error=%v want=%v", err, tc.want)
			}
		})
	}
}

func TestSavedCueBuiltinIsHashPinnedAndDoesNotCreateMedia(t *testing.T) {
	st, owner := newMediaIngestTestStore(t)
	now := time.Now().UnixMilli()
	var mediaBefore int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM media_items`).Scan(&mediaBefore); err != nil {
		t.Fatal(err)
	}
	cue, reused, err := st.CreateSavedCue(SavedCueMutationParams{
		ExpectedActorID: owner.ActorID, Bearer: owner.ControlToken, Title: "Recording",
		BuiltinAssetID: BuiltinRecordingCueAssetID, BuiltinSHA256: BuiltinRecordingCueSHA256,
		OccurredAt: now,
	})
	if err != nil || reused || cue.SourceKind != SavedCueSourceBuiltin || cue.MediaID != "" ||
		cue.SourceBytes != BuiltinRecordingCueBytes || cue.SourceDurationMS != BuiltinRecordingCueDuration {
		t.Fatalf("builtin cue=%+v reused=%v err=%v", cue, reused, err)
	}
	var mediaAfter int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM media_items`).Scan(&mediaAfter); err != nil || mediaAfter != mediaBefore {
		t.Fatalf("parallel media before=%d after=%d err=%v", mediaBefore, mediaAfter, err)
	}

	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "assets", "audio", "pulsar-recording-cue.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		AssetID string `json:"asset_id"`
		SHA256  string `json:"sha256"`
		Bytes   int64  `json:"bytes"`
		Format  struct {
			DurationMS int64 `json:"duration_ms"`
		} `json:"format"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.AssetID != BuiltinRecordingCueAssetID || manifest.SHA256 != BuiltinRecordingCueSHA256 ||
		manifest.Bytes != BuiltinRecordingCueBytes || manifest.Format.DurationMS != BuiltinRecordingCueDuration {
		t.Fatalf("binary/asset manifest drift: %+v", manifest)
	}
	wav, err := os.ReadFile(filepath.Join("..", "..", "..", "assets", "audio", "pulsar-recording-cue.wav"))
	if err != nil {
		t.Fatal(err)
	}
	if digest := fmt.Sprintf("%x", sha256.Sum256(wav)); digest != BuiltinRecordingCueSHA256 || int64(len(wav)) != BuiltinRecordingCueBytes {
		t.Fatalf("binary/asset bytes drift: digest=%s bytes=%d", digest, len(wav))
	}
	if _, _, err := st.CreateSavedCue(SavedCueMutationParams{
		ExpectedActorID: owner.ActorID, Bearer: owner.ControlToken, Title: "bad",
		BuiltinAssetID: BuiltinRecordingCueAssetID, BuiltinSHA256: strings.Repeat("0", 64),
		OccurredAt: now + 1,
	}); !errors.Is(err, ErrSavedCueInvalid) {
		t.Fatalf("unsupported builtin error=%v", err)
	}
}

func TestSavedCueModerationAndStartupReconciliationRevokeFutureUse(t *testing.T) {
	t.Run("source media", func(t *testing.T) {
		st, owner := newMediaIngestTestStore(t)
		now := time.Now().UnixMilli()
		media := readySavedCueMedia(t, st, owner, now, now+100_000, "e", 1024, 500)
		cue := createMediaSavedCue(t, st, owner, media, "media", now+3)
		if _, err := st.DeleteMediaForModeration(media.ID, now+4); err != nil {
			t.Fatal(err)
		}
		var state, reason string
		if err := st.db.QueryRow(`SELECT state, revoke_reason FROM saved_cues WHERE id = ?`, cue.ID).Scan(&state, &reason); err != nil ||
			state != string(SavedCueSourceRevoked) || reason != "source_media_deleted" {
			t.Fatalf("state=%s reason=%s err=%v", state, reason, err)
		}
	})

	t.Run("source actor", func(t *testing.T) {
		st, owner := newMediaIngestTestStore(t)
		now := time.Now().UnixMilli()
		media := readySavedCueMedia(t, st, owner, now, now+100_000, "f", 1024, 500)
		cue := createMediaSavedCue(t, st, owner, media, "actor", now+3)
		if _, err := st.DisableActorForModeration(owner.ActorID, now+4); err != nil {
			t.Fatal(err)
		}
		var state, reason string
		if err := st.db.QueryRow(`SELECT state, revoke_reason FROM saved_cues WHERE id = ?`, cue.ID).Scan(&state, &reason); err != nil ||
			state != string(SavedCueSourceRevoked) || reason != "source_actor_disabled" {
			t.Fatalf("state=%s reason=%s err=%v", state, reason, err)
		}
	})

	t.Run("orbit", func(t *testing.T) {
		st, owner := newMediaIngestTestStore(t)
		now := time.Now().UnixMilli()
		cue, _, err := st.CreateSavedCue(SavedCueMutationParams{
			ExpectedActorID: owner.ActorID, Bearer: owner.ControlToken, Title: "builtin",
			BuiltinAssetID: BuiltinRecordingCueAssetID, BuiltinSHA256: BuiltinRecordingCueSHA256,
			OccurredAt: now,
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := st.DisableOrbitForModeration(owner.OrbitID, now+1); err != nil {
			t.Fatal(err)
		}
		var reason string
		if err := st.db.QueryRow(`SELECT revoke_reason FROM saved_cues WHERE id = ?`, cue.ID).Scan(&reason); err != nil || reason != "owner_orbit_disabled" {
			t.Fatalf("reason=%s err=%v", reason, err)
		}
	})

	t.Run("reconcile stale source", func(t *testing.T) {
		st, owner := newMediaIngestTestStore(t)
		now := time.Now().UnixMilli()
		media := readySavedCueMedia(t, st, owner, now, now+100_000, "1", 1024, 500)
		cue := createMediaSavedCue(t, st, owner, media, "stale", now+3)
		if _, err := st.db.Exec(`UPDATE media_items SET status = 'deleted', storage_key = '',
revision = revision + 1, updated_at = ?, deleted_at = ? WHERE id = ?`, now+4, now+4, media.ID); err != nil {
			t.Fatal(err)
		}
		if err := st.ReconcileSavedCues(now + 5); err != nil {
			t.Fatal(err)
		}
		var state string
		if err := st.db.QueryRow(`SELECT state FROM saved_cues WHERE id = ?`, cue.ID).Scan(&state); err != nil || state != string(SavedCueSourceRevoked) {
			t.Fatalf("state=%s err=%v", state, err)
		}
	})
}

func TestSavedCueMutationRollbackLeavesPinAndAccountingExact(t *testing.T) {
	st, owner := newMediaIngestTestStore(t)
	now := time.Now().UnixMilli()
	media := readySavedCueMedia(t, st, owner, now, now+100_000, "2", 1024, 500)
	injected := errors.New("commit interrupted")
	st.testCheckpoint = func(name string) error {
		if name == "saved_cue_create_before_commit" {
			return injected
		}
		return nil
	}
	_, _, err := st.CreateSavedCue(SavedCueMutationParams{
		ExpectedActorID: owner.ActorID, Bearer: owner.ControlToken,
		Title: "rollback", MediaID: media.ID, OccurredAt: now + 3,
	})
	if !errors.Is(err, injected) {
		t.Fatalf("error=%v", err)
	}
	st.testCheckpoint = nil
	usage, err := st.SavedCueUsage(owner.OrbitID)
	if err != nil || usage != (SavedCueUsage{}) {
		t.Fatalf("usage=%+v err=%v", usage, err)
	}
	var audits, revocations int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM saved_cue_audit_events`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM saved_cue_revocations`).Scan(&revocations); err != nil {
		t.Fatal(err)
	}
	if audits != 0 || revocations != 0 {
		t.Fatalf("rollback leaked audits=%d revocations=%d", audits, revocations)
	}
}

func TestSavedCueQuotasAreTransactional(t *testing.T) {
	t.Run("count", func(t *testing.T) {
		st, owner := newMediaIngestTestStore(t)
		now := time.Now().UnixMilli()
		for i := 0; i < SavedCueMaxCount+1; i++ {
			media := readySavedCueMedia(t, st, owner, now+int64(i*10), now+1_000_000,
				"a", 128, 100)
			_, _, err := st.CreateSavedCue(SavedCueMutationParams{
				ExpectedActorID: owner.ActorID, Bearer: owner.ControlToken,
				Title: "quota", MediaID: media.ID, OccurredAt: now + int64(i*10) + 3,
			})
			if i < SavedCueMaxCount && err != nil {
				t.Fatalf("create %d: %v", i, err)
			}
			if i == SavedCueMaxCount && !errors.Is(err, ErrSavedCueQuotaExceeded) {
				t.Fatalf("overflow error=%v", err)
			}
		}
		usage, err := st.SavedCueUsage(owner.OrbitID)
		if err != nil || usage.Count != SavedCueMaxCount || usage.Bytes != SavedCueMaxCount*128 {
			t.Fatalf("usage=%+v err=%v", usage, err)
		}
	})

	t.Run("bytes", func(t *testing.T) {
		st, owner := newMediaIngestTestStore(t)
		now := time.Now().UnixMilli()
		itemBytes := int64(9 << 20)
		for i := 0; i < 6; i++ {
			media := readySavedCueMedia(t, st, owner, now+int64(i*10), now+1_000_000,
				"b", itemBytes, 100)
			_, _, err := st.CreateSavedCue(SavedCueMutationParams{
				ExpectedActorID: owner.ActorID, Bearer: owner.ControlToken,
				Title: "byte quota", MediaID: media.ID, OccurredAt: now + int64(i*10) + 3,
			})
			if i < 5 && err != nil {
				t.Fatalf("create %d: %v", i, err)
			}
			if i == 5 && !errors.Is(err, ErrSavedCueQuotaExceeded) {
				t.Fatalf("overflow error=%v", err)
			}
		}
		usage, err := st.SavedCueUsage(owner.OrbitID)
		if err != nil || usage.Count != 5 || usage.Bytes != 5*itemBytes {
			t.Fatalf("usage=%+v err=%v", usage, err)
		}
	})
}

func TestSavedCueRequiresCurrentContentPolicy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "saved-cue-policy.db")
	st, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	owner, err := st.CreateSelfServiceOrbit("policy gate")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	media := readySavedCueMedia(t, st, owner, now, now+100_000, "b", 128, 100)
	_, _, err = st.CreateSavedCue(SavedCueMutationParams{
		ExpectedActorID: owner.ActorID, Bearer: owner.ControlToken,
		Title: "policy", MediaID: media.ID, OccurredAt: now + 3,
	})
	if !errors.Is(err, ErrContentPolicyAcceptanceRequired) {
		t.Fatalf("error=%v", err)
	}
}
