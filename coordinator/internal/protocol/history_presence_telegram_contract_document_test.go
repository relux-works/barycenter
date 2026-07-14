package protocol

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPhaseOneHistoryPresenceTelegramContractExamplesAndDecisions(t *testing.T) {
	path := filepath.Join("..", "..", "..", "docs", "analysis",
		"p1-history-presence-telegram-contract-v1.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read history/presence/Telegram contract: %v", err)
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
			t.Fatal("unterminated JSON example in history/presence/Telegram contract")
		}
		example := remaining[:end]
		if !json.Valid([]byte(example)) {
			t.Fatalf("invalid JSON example %d:\n%s", examples+1, example)
		}
		examples++
		remaining = remaining[end+len("\n```"):]
	}
	if examples != 8 {
		t.Fatalf("JSON example count = %d, want 8; update the guard intentionally", examples)
	}

	required := []string{
		"GET /v1/history?view=all&limit=30&cursor=opaque",
		"GET /v1/history/{history_item_id}",
		"GET /v1/presence",
		"PUT /v1/presence/dnd/local",
		"PUT /v1/presence/dnd/orbit",
		"POST /v1/blocks",
		"DELETE /v1/blocks/{block_id}",
		"block-before-DND-before-offline precedence",
		"Phase 1 has no offline inbox",
		"below Telegram's 64-byte limit",
		"stores only a keyed token hash",
		"15-minute expiry",
		"callback query IDs are deduplicated for 24 hours",
		"There is no decision window",
		"new coordinator `accepted_at`",
		"one audible transmission, never both",
		"No request or callback has",
		"DND bypass",
		"missed_dnd/local_dnd",
		"missed_dnd/orbit_dnd",
		"cancelled/dnd_enabled",
		"blocked/actor_blocked",
		"blocked/orbit_blocked",
		"cancelled/sender_blocked",
		"`scope=actor` requires the `ar_` reference",
		"`scope=orbit` requires the `or_` reference",
		"track_not_available_phase1",
	}
	for _, decision := range required {
		if !strings.Contains(contract, decision) {
			t.Errorf("history/presence/Telegram contract lost decision %q", decision)
		}
	}

	callbackData := "tg1_" + strings.Repeat("A", 32)
	if len(callbackData) != 36 || len(callbackData) > 64 {
		t.Fatalf("callback_data bytes = %d, want 36 and <=64", len(callbackData))
	}

	for _, entryPoint := range []string{
		"docs/protocol.md",
		"docs/spec-self-contained-audio.md",
		"docs/analysis/p1-transmission-contract-v1.md",
	} {
		entryRaw, readErr := os.ReadFile(filepath.Join("..", "..", "..", filepath.FromSlash(entryPoint)))
		if readErr != nil {
			t.Errorf("read contract entry point %q: %v", entryPoint, readErr)
			continue
		}
		if !strings.Contains(string(entryRaw), "p1-history-presence-telegram-contract-v1.md") {
			t.Errorf("%s does not link the frozen history/presence/Telegram contract", entryPoint)
		}
	}
}
