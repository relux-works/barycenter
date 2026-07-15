package store

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func installLegacyModerationTargetConstraint(t *testing.T, st *Store) {
	t.Helper()
	ctx := context.Background()
	conn, err := st.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatal(err)
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	legacyDDL := strings.Replace(moderationReportsSharedTargetDDL,
		"moderation_reports_shared_target", "moderation_reports_legacy", 1)
	legacyDDL = strings.Replace(legacyDDL,
		"CHECK(reporter_actor_id <> reported_actor_id),",
		"CHECK(reporter_actor_id <> reported_actor_id),\n  CHECK(target_actor_id = reporter_actor_id),", 1)
	if _, err := tx.Exec(legacyDDL); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO moderation_reports_legacy(` + moderationReportColumnsForRebuild + `)
SELECT ` + moderationReportColumnsForRebuild + ` FROM moderation_reports`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`DROP TABLE moderation_reports`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`ALTER TABLE moderation_reports_legacy RENAME TO moderation_reports`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(moderationReportsSharedTargetAuxDDL); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}
}

func TestModerationSharedTargetMigrationRollsBackAndPreservesEvidence(t *testing.T) {
	fixture := newModerationFixture(t)
	report := createFixtureReport(t, fixture)
	installLegacyModerationTargetConstraint(t, fixture.store)

	injected := errors.New("injected moderation report rebuild failure")
	fixture.store.testCheckpoint = func(name string) error {
		if name == "moderation_report_rebuild_before_commit" {
			return injected
		}
		return nil
	}
	if err := fixture.store.ensureModerationSharedTargets(); !errors.Is(err, injected) {
		t.Fatalf("migration failure=%v", err)
	}
	var tableSQL string
	if err := fixture.store.db.QueryRow(`SELECT sql FROM sqlite_master
WHERE type = 'table' AND name = 'moderation_reports'`).Scan(&tableSQL); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tableSQL, "CHECK(target_actor_id = reporter_actor_id)") {
		t.Fatalf("failed migration did not restore legacy table: %s", tableSQL)
	}
	var foreignKeys int
	if err := fixture.store.db.QueryRow(`PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil || foreignKeys != 1 {
		t.Fatalf("foreign_keys=%d err=%v", foreignKeys, err)
	}

	fixture.store.testCheckpoint = nil
	if err := fixture.store.ensureModerationSharedTargets(); err != nil {
		t.Fatal(err)
	}
	if err := fixture.store.db.QueryRow(`SELECT sql FROM sqlite_master
WHERE type = 'table' AND name = 'moderation_reports'`).Scan(&tableSQL); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(tableSQL, "CHECK(target_actor_id = reporter_actor_id)") {
		t.Fatalf("legacy target constraint survived migration: %s", tableSQL)
	}
	loaded, err := fixture.store.GetAuthorizedModerationReport(
		fixture.reporter.ActorID, fixture.reporter.ControlToken, report.ID,
	)
	if err != nil || loaded != report {
		t.Fatalf("migrated report=%+v want=%+v err=%v", loaded, report, err)
	}
	if _, err := fixture.store.db.Exec(`UPDATE moderation_reports SET target_slot = 'z' WHERE id = ?`, report.ID); err == nil ||
		!strings.Contains(err.Error(), "immutable") {
		t.Fatalf("migrated immutable trigger error=%v", err)
	}
	if err := foreignKeyCheck(fixture.store.db); err != nil {
		t.Fatal(err)
	}
}
