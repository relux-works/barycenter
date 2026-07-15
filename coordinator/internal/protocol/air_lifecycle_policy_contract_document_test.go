package protocol

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type airLifecycleContract struct {
	SchemaVersion int    `json:"schema_version"`
	ContractID    string `json:"contract_id"`
	Statuses      struct {
		Air        []string `json:"air"`
		Membership []string `json:"membership"`
		Invite     []string `json:"invite"`
		AirRole    []string `json:"air_role"`
		Authority  []string `json:"authority"`
	} `json:"statuses"`
	Limits struct {
		Barycenters        int `json:"barycenters_per_air"`
		OnlinePulsars      int `json:"online_pulsars_per_active_air"`
		InviteEntropyBits  int `json:"invite_entropy_bits"`
		InviteTTLSeconds   int `json:"invite_ttl_seconds"`
		CallbackTTLSeconds int `json:"telegram_callback_ttl_seconds"`
	} `json:"limits"`
	Policies struct {
		Defaults map[string]string `json:"defaults"`
	} `json:"policies"`
	Routes []struct {
		Method    string `json:"method"`
		Path      string `json:"path"`
		Operation string `json:"operation"`
	} `json:"routes"`
	Aliases    map[string]string `json:"aliases"`
	Errors     []string          `json:"errors"`
	Invariants []string          `json:"invariants"`
}

func TestPhaseTwoAirLifecyclePolicyContractIsComplete(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..", "..")
	contractPath := filepath.Join(repositoryRoot, "docs", "analysis", "p2-air-lifecycle-policy-contract-v1.md")
	raw, err := os.ReadFile(contractPath)
	if err != nil {
		t.Fatalf("read Air contract: %v", err)
	}
	contract := string(raw)

	remaining := contract
	examples := 0
	for {
		start := strings.Index(remaining, "```json\n")
		if start < 0 {
			break
		}
		remaining = remaining[start+len("```json\n"):]
		end := strings.Index(remaining, "\n```")
		if end < 0 {
			t.Fatal("unterminated JSON example in Air contract")
		}
		if !json.Valid([]byte(remaining[:end])) {
			t.Fatalf("invalid JSON example %d:\n%s", examples+1, remaining[:end])
		}
		examples++
		remaining = remaining[end+len("\n```"):]
	}
	if examples != 12 {
		t.Fatalf("Air contract JSON example count = %d, want 12", examples)
	}

	required := []string{
		"A barycenter MAY have many saved `joined` Air memberships",
		"current primary of the joining barycenter",
		"parked rows, policy, queue",
		"accepted policy revision and authorization result",
		"GET /v1/airs",
		"POST /v1/air-invites/consume",
		"POST /v1/airs/{air_id}/join/confirm",
		"POST /v1/airs/{air_id}/activate",
		"POST /v1/airs/{air_id}/deactivate",
		"POST /v1/airs/{air_id}/ownership/transfer",
		"PUT /v1/airs/{air_id}/policy",
		"`/apart` removes only the caller",
		"do not hear an already-started or ended",
		"never a link controller plus Air",
		"Explicit target selection, target ACL",
		"EPIC-260714-th54l3",
	}
	for _, decision := range required {
		if !strings.Contains(contract, decision) {
			t.Errorf("Air contract lost required decision %q", decision)
		}
	}

	for _, entryPoint := range []string{"docs/protocol.md", "docs/spec-self-contained-audio.md"} {
		entry, readErr := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(entryPoint)))
		if readErr != nil {
			t.Errorf("read Air contract entry point %q: %v", entryPoint, readErr)
			continue
		}
		if !strings.Contains(string(entry), "p2-air-lifecycle-policy-contract-v1.md") {
			t.Errorf("%s does not link the frozen Air contract", entryPoint)
		}
	}
}

func TestPhaseTwoAirLifecyclePolicyExecutableSummary(t *testing.T) {
	path := filepath.Join("..", "..", "..", "protocol", "air-lifecycle-policy-v1.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read executable Air contract: %v", err)
	}
	var contract airLifecycleContract
	if err := json.Unmarshal(raw, &contract); err != nil {
		t.Fatalf("decode executable Air contract: %v", err)
	}
	if contract.SchemaVersion != 1 || contract.ContractID != "pulsar.air-lifecycle-policy.v1" {
		t.Fatalf("unexpected contract identity: version=%d id=%q", contract.SchemaVersion, contract.ContractID)
	}
	assertStrings := func(name string, got, want []string) {
		t.Helper()
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
	assertStrings("air statuses", contract.Statuses.Air, []string{"parked", "active", "dissolved"})
	assertStrings("membership statuses", contract.Statuses.Membership, []string{"pending_confirmation", "joined", "left"})
	assertStrings("invite statuses", contract.Statuses.Invite, []string{"open", "consumed", "expired", "withdrawn"})
	assertStrings("Air roles", contract.Statuses.AirRole, []string{"owner", "admin", "member"})
	assertStrings("authority statuses", contract.Statuses.Authority, []string{"links_authoritative", "airs_shadow", "airs_authoritative", "rollback_hold"})
	if contract.Limits.Barycenters != 8 || contract.Limits.OnlinePulsars != 20 ||
		contract.Limits.InviteEntropyBits != 256 || contract.Limits.InviteTTLSeconds != 900 ||
		contract.Limits.CallbackTTLSeconds != 900 {
		t.Errorf("unexpected Air limits: %+v", contract.Limits)
	}
	wantDefaults := map[string]string{
		"invite": "air_admin_primary", "overlay": "primary_companion",
		"queue": "primary_companion", "replace": "air_admin_primary",
	}
	for key, want := range wantDefaults {
		if got := contract.Policies.Defaults[key]; got != want {
			t.Errorf("policy default %s = %q, want %q", key, got, want)
		}
	}
	if len(contract.Routes) != 15 || len(contract.Aliases) != 5 || len(contract.Errors) != 18 || len(contract.Invariants) != 11 {
		t.Errorf("executable Air contract shape changed: routes=%d aliases=%d errors=%d invariants=%d",
			len(contract.Routes), len(contract.Aliases), len(contract.Errors), len(contract.Invariants))
	}
	seen := map[string]bool{}
	for _, route := range contract.Routes {
		key := route.Method + " " + route.Path
		if seen[key] || route.Operation == "" {
			t.Errorf("invalid or duplicate route: %+v", route)
		}
		seen[key] = true
	}
}
