package moderationops

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validOperations() Operations {
	var operations Operations
	operations.SchemaVersion = 1
	operations.TaskID = "TASK-260712-3t9nr8"
	operations.Approval.State = "approved"
	operations.Approval.ApprovedBy = "Ivan Oparin"
	operations.Approval.ApprovedAt = "2026-07-14T17:52:59+04:00"
	operations.Mail.Domain = "barycenter.live"
	operations.Mail.Support = "support@barycenter.live"
	operations.Mail.Moderation = "moderator@barycenter.live"
	operations.Mail.Urgent = "moderation-urgent@barycenter.live"
	operations.Mail.DeliveryState = DeliveryExternalAction
	operations.Mail.DNSCheckedAt = "2026-07-14T20:28:36+04:00"
	operations.Mail.ExternalTaskID = "TASK-260714-200ib8"
	operations.Mail.ProposedDefault = "Cloudflare Email Routing"
	operations.Rotation.Primary = "Ivan Oparin"
	operations.Rotation.Backup = "Ivan Oparin"
	operations.Rotation.Escalation = "Ivan Oparin"
	operations.Rotation.Coverage = "Monday-Friday 10:00-19:00 GMT+4"
	operations.Rotation.NormalTarget = "2 business days"
	operations.Rotation.UrgentTarget = "24 hours"
	operations.Rotation.CredentialReview = "quarterly and on role change"
	operations.ControlPlane.QueueRoute = "/v1/moderation/reports"
	operations.ControlPlane.EvidenceRoute = "/v1/moderation/reports/{report_id}/evidence"
	operations.ControlPlane.DecisionRoute = "/v1/moderation/reports/{report_id}/decision"
	operations.ControlPlane.AuditRoute = "/v1/moderation/reports/{report_id}/audit"
	operations.ControlPlane.Actions = []string{"no_action", "delete_media", "disable_actor", "disable_orbit"}
	operations.Safety.EvidenceRetentionDays = 30
	operations.RunbookPath = "docs/moderation-operations-runbook.md"
	return operations
}

func TestValidateHonestExternalMailState(t *testing.T) {
	operations := validOperations()
	if err := operations.Validate(); err != nil {
		t.Fatal(err)
	}
	operations.Mail.DeliveryState = DeliveryReady
	if err := operations.Validate(); err == nil || !strings.Contains(err.Error(), "MX") {
		t.Fatalf("ready without MX error=%v", err)
	}
	operations = validOperations()
	operations.Safety.EmailAudio = true
	if err := operations.Validate(); err == nil || !strings.Contains(err.Error(), "safety") {
		t.Fatalf("unsafe email error=%v", err)
	}
}

func TestCheckedInOperationsAndRunbook(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	operations, err := Load(filepath.Join(root, "docs", "compliance", "moderation-operations.json"))
	if err != nil {
		t.Fatal(err)
	}
	if operations.Mail.DeliveryState != DeliveryExternalAction {
		t.Fatalf("mail delivery=%q", operations.Mail.DeliveryState)
	}
	if err := ValidateRunbook(filepath.Join(root, "docs", "moderation-operations-runbook.md")); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRunbookMarkers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runbook.md")
	if err := os.WriteFile(path, []byte("TASK-260712-3t9nr8"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRunbook(path); err == nil || !strings.Contains(err.Error(), "support@") {
		t.Fatalf("incomplete runbook error=%v", err)
	}
}

type testResolver struct{ records []*net.MX }

func (resolver testResolver) LookupMX(context.Context, string) ([]*net.MX, error) {
	return resolver.records, nil
}

func TestVerifyReadyMailIsFailClosed(t *testing.T) {
	operations := validOperations()
	if err := VerifyReadyMail(context.Background(), testResolver{}, operations); err == nil ||
		!strings.Contains(err.Error(), "TASK-260714-200ib8") {
		t.Fatalf("external state error=%v", err)
	}
	operations.Mail.DeliveryState = DeliveryReady
	operations.Mail.ObservedMX = []string{"mx.example"}
	operations.Mail.ExternalTaskID = ""
	if err := VerifyReadyMail(context.Background(), testResolver{}, operations); err == nil ||
		!strings.Contains(err.Error(), "no MX") {
		t.Fatalf("empty live MX error=%v", err)
	}
	if err := VerifyReadyMail(context.Background(), testResolver{records: []*net.MX{{Host: "mx.example."}}}, operations); err != nil {
		t.Fatal(err)
	}
}
