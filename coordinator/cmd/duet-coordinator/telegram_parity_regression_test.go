package main

import (
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
