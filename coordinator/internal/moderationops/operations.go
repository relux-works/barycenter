// Package moderationops validates the human moderation operating contract.
package moderationops

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/mail"
	"os"
	"slices"
	"strings"
)

const (
	DeliveryReady          = "ready"
	DeliveryExternalAction = "external_action_required"
)

type Operations struct {
	SchemaVersion int    `json:"schema_version"`
	TaskID        string `json:"task_id"`
	Approval      struct {
		State      string `json:"state"`
		ApprovedBy string `json:"approved_by"`
		ApprovedAt string `json:"approved_at"`
	} `json:"approval"`
	Mail struct {
		Domain          string   `json:"domain"`
		Support         string   `json:"support"`
		Moderation      string   `json:"moderation"`
		Urgent          string   `json:"urgent"`
		DeliveryState   string   `json:"delivery_state"`
		DNSCheckedAt    string   `json:"dns_checked_at"`
		ObservedMX      []string `json:"observed_mx"`
		ExternalTaskID  string   `json:"external_task_id"`
		ProposedDefault string   `json:"proposed_default"`
	} `json:"mail"`
	Rotation struct {
		Primary          string `json:"primary"`
		Backup           string `json:"backup"`
		Escalation       string `json:"escalation"`
		Coverage         string `json:"coverage"`
		NormalTarget     string `json:"normal_target"`
		UrgentTarget     string `json:"urgent_target"`
		CredentialReview string `json:"credential_review"`
	} `json:"rotation"`
	ControlPlane struct {
		QueueRoute    string   `json:"queue_route"`
		EvidenceRoute string   `json:"evidence_route"`
		DecisionRoute string   `json:"decision_route"`
		AuditRoute    string   `json:"audit_route"`
		Actions       []string `json:"actions"`
	} `json:"control_plane"`
	Safety struct {
		EvidenceRetentionDays     int  `json:"evidence_retention_days"`
		EmailAudio                bool `json:"email_audio"`
		LogSensitiveAudio         bool `json:"log_sensitive_audio"`
		ReporterThirdPartyData    bool `json:"reporter_third_party_data"`
		PhysicalDeleteRecoverable bool `json:"physical_delete_recoverable"`
	} `json:"safety"`
	RunbookPath string `json:"runbook_path"`
}

func Load(path string) (Operations, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Operations{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var operations Operations
	if err := decoder.Decode(&operations); err != nil {
		return Operations{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("additional JSON value")
		}
		return Operations{}, fmt.Errorf("moderation operations has trailing JSON: %w", err)
	}
	if err := operations.Validate(); err != nil {
		return Operations{}, err
	}
	return operations, nil
}

func (operations Operations) Validate() error {
	if operations.SchemaVersion != 1 || operations.TaskID != "TASK-260712-3t9nr8" ||
		operations.Approval.State != "approved" ||
		strings.TrimSpace(operations.Approval.ApprovedBy) == "" ||
		strings.TrimSpace(operations.Approval.ApprovedAt) == "" {
		return errors.New("moderation operations approval is incomplete")
	}
	if operations.Mail.Domain != "barycenter.live" ||
		!validMailbox(operations.Mail.Support, "support", operations.Mail.Domain) ||
		!validMailbox(operations.Mail.Moderation, "moderator", operations.Mail.Domain) ||
		!validMailbox(operations.Mail.Urgent, "moderation-urgent", operations.Mail.Domain) {
		return errors.New("moderation operations mailboxes are invalid")
	}
	switch operations.Mail.DeliveryState {
	case DeliveryReady:
		if len(operations.Mail.ObservedMX) == 0 {
			return errors.New("ready mail delivery lacks observed MX evidence")
		}
	case DeliveryExternalAction:
		if len(operations.Mail.ObservedMX) != 0 ||
			operations.Mail.ExternalTaskID != "TASK-260714-200ib8" ||
			strings.TrimSpace(operations.Mail.ProposedDefault) == "" {
			return errors.New("external mail action is not honestly tracked")
		}
	default:
		return errors.New("unknown mail delivery state")
	}
	if strings.TrimSpace(operations.Mail.DNSCheckedAt) == "" ||
		strings.TrimSpace(operations.Rotation.Primary) == "" ||
		strings.TrimSpace(operations.Rotation.Backup) == "" ||
		strings.TrimSpace(operations.Rotation.Escalation) == "" ||
		operations.Rotation.Coverage != "Monday-Friday 10:00-19:00 GMT+4" ||
		operations.Rotation.NormalTarget != "2 business days" ||
		operations.Rotation.UrgentTarget != "24 hours" ||
		strings.TrimSpace(operations.Rotation.CredentialReview) == "" {
		return errors.New("moderation rotation or response contract is incomplete")
	}
	wantActions := []string{"no_action", "delete_media", "disable_actor", "disable_orbit"}
	if operations.ControlPlane.QueueRoute != "/v1/moderation/reports" ||
		operations.ControlPlane.EvidenceRoute != "/v1/moderation/reports/{report_id}/evidence" ||
		operations.ControlPlane.DecisionRoute != "/v1/moderation/reports/{report_id}/decision" ||
		operations.ControlPlane.AuditRoute != "/v1/moderation/reports/{report_id}/audit" ||
		!slices.Equal(operations.ControlPlane.Actions, wantActions) {
		return errors.New("moderation control-plane contract is incomplete")
	}
	if operations.Safety.EvidenceRetentionDays != 30 || operations.Safety.EmailAudio ||
		operations.Safety.LogSensitiveAudio || operations.Safety.ReporterThirdPartyData ||
		operations.Safety.PhysicalDeleteRecoverable ||
		operations.RunbookPath != "docs/moderation-operations-runbook.md" {
		return errors.New("moderation safety contract is invalid")
	}
	return nil
}

func validMailbox(raw, local, domain string) bool {
	address, err := mail.ParseAddress(raw)
	return err == nil && address.Address == local+"@"+domain
}

func ValidateRunbook(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	text := string(data)
	markers := []string{
		"TASK-260712-3t9nr8", "support@barycenter.live",
		"moderator@barycenter.live", "moderation-urgent@barycenter.live",
		"Monday-Friday 10:00-19:00 GMT+4", "2 business days", "24 hours",
		"/v1/moderation/reports?status=open", "/evidence", "/decision", "/audit",
		"no_action", "delete_media", "disable_actor", "disable_orbit",
		"Do not email audio", "physically deleted audio is not recoverable",
		"TASK-260714-200ib8", "Microsoft",
	}
	for _, marker := range markers {
		if !strings.Contains(text, marker) {
			return fmt.Errorf("moderation runbook lacks %q", marker)
		}
	}
	return nil
}

type mxResolver interface {
	LookupMX(context.Context, string) ([]*net.MX, error)
}

func VerifyReadyMail(ctx context.Context, resolver mxResolver, operations Operations) error {
	if operations.Mail.DeliveryState != DeliveryReady {
		return fmt.Errorf("mail delivery is %s; complete %s", operations.Mail.DeliveryState,
			operations.Mail.ExternalTaskID)
	}
	records, err := resolver.LookupMX(ctx, operations.Mail.Domain)
	if err != nil {
		return fmt.Errorf("lookup MX: %w", err)
	}
	if len(records) == 0 {
		return errors.New("mail delivery is marked ready but live DNS has no MX records")
	}
	return nil
}
