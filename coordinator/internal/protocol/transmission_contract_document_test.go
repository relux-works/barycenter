package protocol

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPhaseOneTransmissionContractExamplesAndDecisions(t *testing.T) {
	path := filepath.Join(
		"..", "..", "..", "docs", "analysis", "p1-transmission-contract-v1.md",
	)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read transmission contract: %v", err)
	}
	contract := string(raw)

	const jsonFence = "```json\n"
	remaining := contract
	examples := 0
	for {
		start := strings.Index(remaining, jsonFence)
		if start < 0 {
			break
		}
		remaining = remaining[start+len(jsonFence):]
		end := strings.Index(remaining, "\n```")
		if end < 0 {
			t.Fatal("unterminated JSON example in transmission contract")
		}
		example := remaining[:end]
		if !json.Valid([]byte(example)) {
			t.Fatalf("invalid JSON example %d:\n%s", examples+1, example)
		}
		examples++
		remaining = remaining[end+len("\n```"):]
	}
	if examples != 23 {
		t.Fatalf("JSON example count = %d, want 23; update the guard intentionally", examples)
	}

	required := []string{
		"Media IDs use the existing `m_` plus 26-character ULID identifier",
		"POST /v1/transmissions",
		"GET /v1/transmissions/{id}",
		"POST /v1/transmissions/{id}/cancel",
		"prepare_deadline_at = min(expires_at, barrier_opened_at + 3000 ms)",
		"T_coord_ms = decision_now_ms + lead_ms",
		"start_deadline_coord_ms = T_coord_ms + 100",
		"mandatory_target_missing_overlay_capability",
		"requires_confirmation",
		"sender_confirmed_overlay_fallback",
		"cancelled/media_deleted",
		"resume the captured main position exactly once",
		"There is no `force`, `priority`, `emergency` or bypass field",
		"coordinator-owned",
	}
	for _, decision := range required {
		if !strings.Contains(contract, decision) {
			t.Errorf("transmission contract lost required decision %q", decision)
		}
	}
}

func TestPhaseOneTransmissionRolloutHandoffIsComplete(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..", "..")
	handoffPath := filepath.Join(
		repositoryRoot, "docs", "analysis", "p1-transmission-rollout-handoff.md",
	)
	raw, err := os.ReadFile(handoffPath)
	if err != nil {
		t.Fatalf("read transmission rollout handoff: %v", err)
	}
	handoff := string(raw)

	requiredDecisions := []string{
		"POST /v1/transmissions",
		"GET /v1/transmissions/{id}",
		"POST /v1/transmissions/{id}/cancel",
		"mandatory_target_missing_overlay_capability",
		"409 requires_confirmation",
		"There is never a per-target protocol split",
		"Only `media_ended(reason=completed)` proves `played`",
		"missed_offline | missed_dnd | missed_not_ready | blocked",
		"prepare_media",
		"play_media_at",
		"cancel_media",
		"coordinator **first**",
		"SELECT COUNT(*) AS nonterminal_transmissions",
		"WHERE completed_at = 0",
		"Do not manually rewrite transmission, target or scheduler status rows",
		"There is no transmission-wide runtime feature flag",
		"EPIC-260714-th54l3",
	}
	for _, decision := range requiredDecisions {
		if !strings.Contains(handoff, decision) {
			t.Errorf("transmission rollout handoff lost required decision %q", decision)
		}
	}

	requiredLinks := []string{
		"docs/analysis/p1-transmission-contract-v1.md",
		"docs/analysis/p1-transmission-http-resolution.md",
		"docs/analysis/p1-clip-transmission-wire-contract.md",
		"docs/analysis/p1-transmission-store-target-snapshots.md",
		"docs/protocol.md",
		"docs/diagrams/p1-transmission-protocol-components.puml",
		"docs/diagrams/p1-transmission-scheduler-sequence.puml",
		".task-board/.resources/TASK-260712-26ip33/macos-transmission-client-hooks-outcome.md",
		".task-board/.resources/TASK-260712-2bbz13/windows-transmission-client-hooks-outcome.md",
		".task-board/.resources/TASK-260712-31vvjt/overlay-controller-scheduler-outcome.md",
		".task-board/.resources/TASK-260712-2qc27p/transmission-regression-evidence.md",
		".planning/260714_045154_epic-260714-th54l3.md",
	}
	for _, target := range requiredLinks {
		if _, err := os.Stat(filepath.Join(repositoryRoot, filepath.FromSlash(target))); err != nil {
			t.Errorf("required handoff target %q is unavailable: %v", target, err)
		}
		linkFromHandoff, err := filepath.Rel(filepath.Dir(handoffPath), filepath.Join(repositoryRoot, filepath.FromSlash(target)))
		if err != nil {
			t.Fatalf("resolve handoff link for %q: %v", target, err)
		}
		if !strings.Contains(handoff, "("+filepath.ToSlash(linkFromHandoff)+")") {
			t.Errorf("transmission rollout handoff does not link required target %q", target)
		}
	}

	for _, entryPoint := range []string{
		"docs/protocol.md",
		"docs/runbook.md",
	} {
		raw, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(entryPoint)))
		if err != nil {
			t.Errorf("read transmission handoff entry point %q: %v", entryPoint, err)
			continue
		}
		if !strings.Contains(string(raw), "p1-transmission-rollout-handoff.md") {
			t.Errorf("%s does not link the stable transmission rollout handoff", entryPoint)
		}
	}
}

func TestPhaseOneTransmissionGoldenSetIsComplete(t *testing.T) {
	required := []string{
		TypePrepareMedia,
		TypePlayMediaAt,
		TypeCancelMedia,
		TypeMediaReady,
		TypeMediaStarted,
		TypeMediaEnded,
		TypeMediaFailed,
		TypeMediaCancelled,
		TypeSetDND,
		TypePresenceUpdate,
	}
	for _, messageType := range required {
		path := filepath.Join(goldenDir(t), messageType+".json")
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("read required transmission golden %q: %v", messageType, err)
			continue
		}
		var envelope Envelope
		if err := json.Unmarshal(raw, &envelope); err != nil {
			t.Errorf("decode required transmission golden %q: %v", messageType, err)
			continue
		}
		if envelope.Type != messageType || !KnownType(messageType) {
			t.Errorf("required transmission type %q is not registered: envelope=%q",
				messageType, envelope.Type)
			continue
		}
		if _, err := DecodePayloadStrict(envelope); err != nil {
			t.Errorf("strict decode required transmission golden %q: %v", messageType, err)
		}
	}
}
