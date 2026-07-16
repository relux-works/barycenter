package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"relux.works/duet/coordinator/internal/presentation"
	"relux.works/duet/coordinator/internal/store"
)

func TestTelegramRoutingChoicesMatchSharedPairwisePresentation(t *testing.T) {
	l, _ := newTestLoop(t)
	peer, err := l.st.CreateOrbit("Orion", 333)
	if err != nil {
		t.Fatal(err)
	}
	code, err := l.st.ProposeLink(1, 111)
	if err != nil {
		t.Fatal(err)
	}
	linkID, _, err := l.st.AcceptByCode(code, peer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.st.ActivateLink(linkID); err != nil {
		t.Fatal(err)
	}

	choices := l.telegramRoutingChoices(1)
	audiences := []store.TransmissionAudienceKind{
		store.TransmissionAudienceOwnBarycenter,
		store.TransmissionAudienceCurrentAir,
	}
	deliveries := []store.TransmissionDelivery{
		store.TransmissionDeliveryOverlay,
		store.TransmissionDeliveryInterrupt,
		store.TransmissionDeliveryAfterCurrent,
	}
	if len(choices) != len(audiences)*len(deliveries) {
		t.Fatalf("choices=%+v", choices)
	}
	index := 0
	for _, audience := range audiences {
		for _, delivery := range deliveries {
			choice := choices[index]
			index++
			audienceLabel := presentation.AudienceLabel(audience, "Orion")
			deliveryLabel := presentation.DeliveryLabel(delivery)
			wantRU := deliveryLabel.RU + " · " + audienceLabel.RU
			if choice.delivery != delivery || choice.audience != audience || choice.text != wantRU {
				t.Errorf("choice=%+v want delivery=%s audience=%s text=%q",
					choice, delivery, audience, wantRU)
			}
			if deliveryLabel.EN == "" || deliveryLabel.RU == "" ||
				audienceLabel.EN == "" || audienceLabel.RU == "" {
				t.Errorf("app label missing locale: delivery=%+v audience=%+v",
					deliveryLabel, audienceLabel)
			}
		}
	}
	pairwise := presentation.AudienceLabel(store.TransmissionAudienceCurrentAir, "Orion")
	if pairwise.EN != "Current Air with «Orion»" || pairwise.RU != "Текущий эфир с «Orion»" {
		t.Fatalf("pairwise label=%+v", pairwise)
	}
}

func TestTelegramTargetsInboxParityRegressionFixture(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "acceptance",
		"targets-inbox-parity-regressions-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var evidence struct {
		Contract string `json:"contract"`
		Scope    string `json:"scope"`
		Fixture  struct {
			CanonicalOutcomes   []string `json:"canonicalOutcomes"`
			TargetedTrackPolicy string   `json:"targetedTrackPolicy"`
			ManualReplay        bool     `json:"manualReplayRequired"`
			LateAutoplay        bool     `json:"lateAutoplayAllowed"`
			OpaquePrefixes      []string `json:"opaquePrefixesNeverRendered"`
		} `json:"sharedSurfaceFixture"`
	}
	if err := json.Unmarshal(raw, &evidence); err != nil {
		t.Fatal(err)
	}
	wantOutcomes := []string{
		"replay_accepted", "inbox_dismissed", "media_deleted", "report_received", "sender_blocked",
	}
	if evidence.Contract != "p2-targets-inbox-parity-regressions.v1" ||
		evidence.Scope != "repository-automated-only" ||
		strings.Join(evidence.Fixture.CanonicalOutcomes, ",") != strings.Join(wantOutcomes, ",") ||
		evidence.Fixture.TargetedTrackPolicy != "unsupported" ||
		!evidence.Fixture.ManualReplay || evidence.Fixture.LateAutoplay {
		t.Fatalf("Telegram parity fixture diverged: %+v", evidence)
	}
	for _, outcome := range []string{
		"replay_accepted", "media_deleted", "report_received", "sender_blocked",
	} {
		label := presentation.HistoryActionOutcomeLabel(outcome)
		if label.Key != "history.outcome."+outcome || label.EN == "" || label.RU == "" {
			t.Fatalf("Telegram outcome %q is not canonical: %+v", outcome, label)
		}
	}
	for _, prefix := range evidence.Fixture.OpaquePrefixes {
		for _, outcome := range evidence.Fixture.CanonicalOutcomes {
			label := presentation.HistoryActionOutcomeLabel(outcome)
			if strings.Contains(label.EN, prefix) || strings.Contains(label.RU, prefix) {
				t.Fatalf("Telegram label leaked opaque prefix %q: %+v", prefix, label)
			}
		}
	}
}
