package store

import (
	"errors"
	"strings"
	"testing"
)

func readyE2EEReportObject(t *testing.T, f e2eeRoutingFixture, source string) E2EEProtectedObject {
	t.Helper()
	chunk := []byte("opaque-ciphertext-for-report:" + source)
	object := stageRoutedCiphertextObject(t, f, source, chunk)
	putRoutedCiphertextChunks(t, f, object, chunk)
	ready, err := f.store.FinalizeE2EEProtectedObject(object.ID, object.Revision, f.now+120)
	if err != nil {
		t.Fatal(err)
	}
	return ready
}

func createE2EEReportFixture(t *testing.T, f e2eeRoutingFixture, object E2EEProtectedObject, at int64) E2EEModerationReport {
	t.Helper()
	created, err := f.store.CreateE2EEModerationReport(CreateE2EEModerationReportParams{
		ProtectedObjectID: object.ID, ReporterActorID: f.peer.ActorID,
		ReporterDeviceID: f.peerDevice, Reason: ModerationReasonHarassment,
		Statement: "recipient-supplied context", CreatedAt: at,
	})
	if err != nil {
		t.Fatal(err)
	}
	return created.Report
}

func e2eeEvidenceParams(report E2EEModerationReport, at int64) CreateE2EEReportEvidenceParams {
	atRest := strings.Repeat("8", 64)
	return CreateE2EEReportEvidenceParams{
		ReportID: report.ID, ProtectedObjectID: report.ProtectedObjectID,
		ReporterActorID: report.ReporterActorID, ReporterDeviceID: report.ReporterDeviceID,
		ConsentVersion: "report-evidence-consent-v1", ConsentDigest: strings.Repeat("6", 64),
		AuthenticatedEvidenceDigest: strings.Repeat("7", 64),
		EncryptedEvidenceRef:        "evidence/v1/" + atRest, AtRestCiphertextDigest: atRest,
		EvidenceMIME: E2EEReportEvidenceMIME, EvidenceSizeBytes: 4096,
		ExpectedReportRevision: report.Revision, ConsentConfirmedAt: at,
		CreatedAt: at + 1, RetentionExpiresAt: at + e2eeReportRetention.Milliseconds(),
	}
}

func fullE2EEModerator(t *testing.T, st *Store, at int64) ModerationOperatorCredential {
	t.Helper()
	operator, err := st.ProvisionModerationOperator("E2EE moderation reviewer",
		ModerationOperatorCapabilities{List: true, Evidence: true, Decide: true}, at)
	if err != nil {
		t.Fatal(err)
	}
	return operator
}

func TestE2EEReportIsMetadataOnlyWithoutExplicitEvidenceConsent(t *testing.T) {
	f := newE2EERoutingFixture(t)
	ready := readyE2EEReportObject(t, f, "report_metadata_only_source_01")
	revoked, err := f.store.RevokeE2EEProtectedObject(ready.ID, ready.Revision, f.now+121)
	if err != nil {
		t.Fatal(err)
	}
	report := createE2EEReportFixture(t, f, revoked, f.now+122)
	if report.EvidenceState != "metadata_only" || report.Status != "open" ||
		report.CiphertextDigest != ready.CiphertextDigest {
		t.Fatalf("metadata-only report=%+v", report)
	}
	for _, table := range []string{
		"e2ee_report_evidence_consents",
		"e2ee_report_evidence_metadata",
		"e2ee_report_evidence_state",
	} {
		var count int
		if err := f.store.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("pre-consent table=%s count=%d err=%v", table, count, err)
		}
	}
	if _, err := f.store.AttachE2EEReportEvidence(e2eeEvidenceParams(report, f.now+123)); !errors.Is(err, ErrE2EERevoked) {
		t.Fatalf("revoked object evidence export error=%v", err)
	}
	var plaintextMatches int
	if err := f.store.db.QueryRow(`SELECT COUNT(*) FROM e2ee_moderation_reports
WHERE statement LIKE '%opaque-ciphertext-for-report%' OR statement LIKE '%evidence/v1/%'`).Scan(&plaintextMatches); err != nil || plaintextMatches != 0 {
		t.Fatalf("pre-consent plaintext-like rows=%d err=%v", plaintextMatches, err)
	}
}

func TestE2EEReportEvidenceConsentAccessAuditDeleteExpiryAndRevocation(t *testing.T) {
	f := newE2EERoutingFixture(t)
	ready := readyE2EEReportObject(t, f, "report_evidence_access_source_1")
	report := createE2EEReportFixture(t, f, ready, f.now+122)
	params := e2eeEvidenceParams(report, f.now+123)

	invalid := params
	invalid.ReporterDeviceID = f.ownerDevice
	if _, err := f.store.AttachE2EEReportEvidence(invalid); !errors.Is(err, ErrE2EEConflict) {
		t.Fatalf("wrong reporter evidence error=%v", err)
	}
	created, err := f.store.AttachE2EEReportEvidence(params)
	if err != nil || created.Reused || created.Evidence.Status != "active" ||
		created.Evidence.ReporterDeviceID != f.peerDevice ||
		created.Evidence.EncryptedEvidenceRef != params.EncryptedEvidenceRef {
		t.Fatalf("created evidence=%+v err=%v", created, err)
	}
	if replay, err := f.store.AttachE2EEReportEvidence(params); err != nil || !replay.Reused {
		t.Fatalf("evidence replay=%+v err=%v", replay, err)
	}
	operator := fullE2EEModerator(t, f.store, f.now+125)
	evidence, err := f.store.AuthorizeE2EEReportEvidence(
		operator.Operator.ID, operator.Token, report.ID, f.now+126)
	if err != nil || evidence.ID != created.Evidence.ID {
		t.Fatalf("authorized evidence=%+v err=%v", evidence, err)
	}
	audit, err := f.store.ListE2EEReportAuditEvents(
		operator.Operator.ID, operator.Token, report.ID, 20)
	if err != nil || len(audit) != 4 || audit[0].EventType != "report.created" ||
		audit[1].EventType != "evidence.consent_recorded" ||
		audit[2].EventType != "evidence.created" || audit[3].EventType != "evidence.read" {
		t.Fatalf("evidence audit=%+v err=%v", audit, err)
	}
	if _, err := f.store.db.Exec(`UPDATE e2ee_report_audit_events SET created_at = created_at + 1`); err == nil ||
		!strings.Contains(err.Error(), "append-only") {
		t.Fatalf("audit tamper error=%v", err)
	}
	deleted, err := f.store.DeleteE2EEReportEvidence(
		operator.Operator.ID, operator.Token, report.ID, f.now+127)
	if err != nil || !deleted.Changed || deleted.Evidence.Status != "deleted" {
		t.Fatalf("deleted evidence=%+v err=%v", deleted, err)
	}
	if replay, err := f.store.DeleteE2EEReportEvidence(
		operator.Operator.ID, operator.Token, report.ID, f.now+128); err != nil || replay.Changed {
		t.Fatalf("delete replay=%+v err=%v", replay, err)
	}
	if _, err := f.store.AuthorizeE2EEReportEvidence(
		operator.Operator.ID, operator.Token, report.ID, f.now+129); !errors.Is(err, ErrModerationEvidenceExpired) {
		t.Fatalf("deleted evidence read error=%v", err)
	}
	if changed, err := f.store.RevokeModerationOperator(operator.Operator.ID, f.now+130); err != nil || !changed {
		t.Fatalf("operator revoke changed=%v err=%v", changed, err)
	}
	if _, err := f.store.ListE2EEModerationReports(
		operator.Operator.ID, operator.Token, "open", 10); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("revoked operator list error=%v", err)
	}

	expiry := newE2EERoutingFixture(t)
	expiryReady := readyE2EEReportObject(t, expiry, "report_evidence_expiry_src_01")
	expiryReport := createE2EEReportFixture(t, expiry, expiryReady, expiry.now+122)
	expiryParams := e2eeEvidenceParams(expiryReport, expiry.now+123)
	expiryParams.RetentionExpiresAt = expiryParams.CreatedAt + 2
	if _, err := expiry.store.AttachE2EEReportEvidence(expiryParams); err != nil {
		t.Fatal(err)
	}
	expired, err := expiry.store.ExpireE2EEReportEvidence(expiryParams.RetentionExpiresAt, 10)
	if err != nil || len(expired) != 1 || expired[0].Status != "expired" {
		t.Fatalf("expired evidence=%+v err=%v", expired, err)
	}
}

func TestE2EEModerationDecisionUsesCanonicalOpaqueDeleteAndSurvivesRestart(t *testing.T) {
	f := newE2EERoutingFixture(t)
	ready := readyE2EEReportObject(t, f, "report_decision_delete_source_1")
	report := createE2EEReportFixture(t, f, ready, f.now+122)
	operator := fullE2EEModerator(t, f.store, f.now+123)

	if _, err := f.store.DeleteE2EEProtectedObjectForModeration(
		operator.Operator.ID, operator.Token, report.ID, ready.Revision, f.now+124,
	); !errors.Is(err, ErrModerationDecisionConflict) {
		t.Fatalf("delete without decision error=%v", err)
	}
	request, err := f.store.BeginE2EEModerationDecision(
		operator.Operator.ID, operator.Token, report.ID, ModerationActionDeleteMedia, f.now+125)
	if err != nil || request.Applied || request.Reused {
		t.Fatalf("decision request=%+v err=%v", request, err)
	}
	deleted, err := f.store.DeleteE2EEProtectedObjectForModeration(
		operator.Operator.ID, operator.Token, report.ID, ready.Revision, f.now+126)
	if err != nil || deleted.Status != "deleted" || deleted.Revision != ready.Revision+1 {
		t.Fatalf("moderation delete=%+v err=%v", deleted, err)
	}
	var chunks int
	if err := f.store.db.QueryRow(`SELECT COUNT(*) FROM e2ee_protected_object_chunks
WHERE protected_object_id = ?`, ready.ID).Scan(&chunks); err != nil || chunks != 0 {
		t.Fatalf("remaining chunks=%d err=%v", chunks, err)
	}
	decision, err := f.store.CompleteE2EEModerationDecision(request.Decision.ID, f.now+127)
	if err != nil || decision.State != "applied" {
		t.Fatalf("completed decision=%+v err=%v", decision, err)
	}
	if err := f.store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenWithOptions(f.path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	f.store = reopened
	t.Cleanup(func() { _ = reopened.Close() })
	replay, err := reopened.BeginE2EEModerationDecision(
		operator.Operator.ID, operator.Token, report.ID, ModerationActionDeleteMedia, f.now+128)
	if err != nil || !replay.Reused || !replay.Applied {
		t.Fatalf("restart decision replay=%+v err=%v", replay, err)
	}
}

func TestE2EEReportAndEvidenceCheckpointRollbackIsAtomic(t *testing.T) {
	f := newE2EERoutingFixture(t)
	ready := readyE2EEReportObject(t, f, "report_checkpoint_source_0001")
	injected := errors.New("injected report checkpoint")
	f.store.testCheckpoint = func(name string) error {
		if name == "e2ee_report_metadata_before_commit" {
			return injected
		}
		return nil
	}
	if _, err := f.store.CreateE2EEModerationReport(CreateE2EEModerationReportParams{
		ProtectedObjectID: ready.ID, ReporterActorID: f.peer.ActorID,
		ReporterDeviceID: f.peerDevice, Reason: ModerationReasonOther,
		CreatedAt: f.now + 122,
	}); !errors.Is(err, injected) {
		t.Fatalf("report checkpoint error=%v", err)
	}
	var reports, audits int
	if err := f.store.db.QueryRow(`SELECT COUNT(*) FROM e2ee_moderation_reports`).Scan(&reports); err != nil {
		t.Fatal(err)
	}
	if err := f.store.db.QueryRow(`SELECT COUNT(*) FROM e2ee_report_audit_events`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if reports != 0 || audits != 0 {
		t.Fatalf("rolled-back reports=%d audits=%d", reports, audits)
	}
	f.store.testCheckpoint = nil
	report := createE2EEReportFixture(t, f, ready, f.now+123)
	f.store.testCheckpoint = func(name string) error {
		if name == "e2ee_report_evidence_before_commit" {
			return injected
		}
		return nil
	}
	if _, err := f.store.AttachE2EEReportEvidence(e2eeEvidenceParams(report, f.now+124)); !errors.Is(err, injected) {
		t.Fatalf("evidence checkpoint error=%v", err)
	}
	for _, table := range []string{
		"e2ee_report_evidence_consents", "e2ee_report_evidence_metadata",
		"e2ee_report_evidence_state",
	} {
		var count int
		if err := f.store.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("rolled-back table=%s count=%d err=%v", table, count, err)
		}
	}
}
