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
