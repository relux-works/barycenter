package presentation

import (
	"regexp"
	"sort"
	"strings"

	"relux.works/duet/coordinator/internal/store"
)

// PhaseTwoSurfaceState is local presentation state. It is deliberately
// separate from coordinator business state so a stale snapshot is never
// presented as fresh authority for a mutation.
type PhaseTwoSurfaceState string

const (
	PhaseTwoLoading          PhaseTwoSurfaceState = "loading"
	PhaseTwoReady            PhaseTwoSurfaceState = "ready"
	PhaseTwoStale            PhaseTwoSurfaceState = "stale"
	PhaseTwoOffline          PhaseTwoSurfaceState = "offline"
	PhaseTwoCoordinatorError PhaseTwoSurfaceState = "coordinator_error"
)

type ActionPresentation struct {
	Action string `json:"action"`
	Label  Label  `json:"label"`
}

type TargetChoicePresentation struct {
	Label           Label    `json:"label"`
	CapabilityState Label    `json:"capability_state"`
	Capabilities    []string `json:"capabilities"`
}

type InboxItemPresentation struct {
	Sender            Label                `json:"sender"`
	Source            Label                `json:"source"`
	RequestedDelivery Label                `json:"requested_delivery"`
	EffectiveDelivery Label                `json:"effective_delivery"`
	Availability      Label                `json:"availability"`
	Receipt           Label                `json:"receipt"`
	Expiry            Label                `json:"expiry"`
	Actions           []ActionPresentation `json:"actions"`
}

type HistoryItemPresentation struct {
	Direction         Label                `json:"direction"`
	Sender            *Label               `json:"sender,omitempty"`
	Source            *Label               `json:"source,omitempty"`
	RequestedDelivery *Label               `json:"requested_delivery,omitempty"`
	EffectiveDelivery *Label               `json:"effective_delivery,omitempty"`
	Status            Label                `json:"status"`
	Actions           []ActionPresentation `json:"actions"`
}

type HistoryReceiptPresentation struct {
	Status Label `json:"status"`
}

var presentationEnum = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

func PhaseTwoSurfaceStateLabel(state PhaseTwoSurfaceState) Label {
	switch state {
	case PhaseTwoLoading, PhaseTwoReady, PhaseTwoStale, PhaseTwoOffline, PhaseTwoCoordinatorError:
		return label("surface." + string(state))
	default:
		return label("surface.coordinator_error")
	}
}

func ContentPolicyStateLabel(state string) Label {
	switch state {
	case "current", "required", "stale":
		return label("content_policy." + state)
	default:
		return label("content_policy.required")
	}
}

func TargetedTrackPolicyLabel(policy string) Label {
	switch policy {
	case "clip", "queue", "replace", "unsupported":
		return label("targeted_track." + policy)
	default:
		return label("targeted_track.unsupported")
	}
}

func CapabilityStateLabel(state string) Label {
	switch state {
	case "known", "mixed", "unknown":
		return label("capability_state." + state)
	default:
		return label("capability_state.unknown")
	}
}

func InboxAvailabilityLabel(availability string) Label {
	switch availability {
	case "available", "dismissed", "replayed", "unavailable", "expired":
		return label("inbox.availability." + availability)
	default:
		return label("inbox.availability.unavailable")
	}
}

// ActionLabel is the generic command label. Report reason selection remains a
// separate finite step and continues to use HistoryActionLabel.
func ActionLabel(action string) Label {
	switch action {
	case "cancel", "delete", "replay", "dismiss", "report", "block_actor", "block_orbit", "unblock":
		return label("action." + action)
	default:
		return label("action.unsupported")
	}
}

// PresentActions preserves a bounded unknown enum explicitly, but never
// invents a command or echoes arbitrary text. The wire action remains the
// command capability; the label is presentation only.
func PresentActions(actions []string) []ActionPresentation {
	result := make([]ActionPresentation, 0, len(actions))
	seen := make(map[string]bool, len(actions))
	for _, action := range actions {
		if !presentationEnum.MatchString(action) || seen[action] {
			continue
		}
		seen[action] = true
		result = append(result, ActionPresentation{Action: action, Label: ActionLabel(action)})
	}
	return result
}

func canonicalCapabilities(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if !presentationEnum.MatchString(value) || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func PresentTargetChoice(metadata TargetMetadata, capabilityState string, capabilities []string) TargetChoicePresentation {
	return TargetChoicePresentation{
		Label: TargetLabel(metadata), CapabilityState: CapabilityStateLabel(capabilityState),
		Capabilities: canonicalCapabilities(capabilities),
	}
}

func PresentInboxItem(
	senderName, sourceName string,
	requested, effective store.TransmissionDelivery,
	availability, status string,
	reason store.TransmissionReason,
	actions []string,
) InboxItemPresentation {
	return InboxItemPresentation{
		Sender: SenderLabel(senderName), Source: OriginLabel(sourceName),
		RequestedDelivery: DeliveryLabel(requested), EffectiveDelivery: DeliveryLabel(effective),
		Availability: InboxAvailabilityLabel(availability), Receipt: ReceiptLabel(status, reason),
		Expiry: label("inbox.expiry"), Actions: PresentActions(actions),
	}
}

func PresentHistoryItem(
	direction store.HistoryDirection,
	senderName, sourceName string,
	requested, effective string,
	status string,
	reason store.TransmissionReason,
	actions []string,
) HistoryItemPresentation {
	result := HistoryItemPresentation{
		Direction: HistoryDirectionLabel(direction), Status: ReceiptLabel(status, reason),
		Actions: PresentActions(actions),
	}
	if strings.TrimSpace(senderName) != "" {
		value := SenderLabel(senderName)
		result.Sender = &value
	}
	if strings.TrimSpace(sourceName) != "" {
		value := OriginLabel(sourceName)
		result.Source = &value
	}
	if requested != "" {
		value := DeliveryLabel(store.TransmissionDelivery(requested))
		result.RequestedDelivery = &value
	}
	if effective != "" {
		value := DeliveryLabel(store.TransmissionDelivery(effective))
		result.EffectiveDelivery = &value
	}
	return result
}

func PresentHistoryReceipt(status string, reason store.TransmissionReason) HistoryReceiptPresentation {
	return HistoryReceiptPresentation{Status: ReceiptLabel(status, reason)}
}

// CommandAllowed performs the final presentation-layer capability check. It
// does not replace server authorization; it prevents a platform UI from
// manufacturing an action that was absent from the latest ready projection.
func CommandAllowed(state PhaseTwoSurfaceState, actions []ActionPresentation, requested string) bool {
	if state != PhaseTwoReady || !presentationEnum.MatchString(requested) {
		return false
	}
	for _, action := range actions {
		if action.Action == requested && ActionLabel(requested).Key != "action.unsupported" {
			return true
		}
	}
	return false
}

// OpaqueSelectionAllowed verifies only current opaque reference membership.
// Identity, membership and capability authority remain server-side.
func OpaqueSelectionAllowed(current []string, requested []string) bool {
	if len(requested) == 0 || len(requested) > 64 {
		return false
	}
	allowed := make(map[string]bool, len(current))
	for _, reference := range current {
		if strings.HasPrefix(reference, "trf_") && len(reference) == 47 {
			allowed[reference] = true
		}
	}
	seen := make(map[string]bool, len(requested))
	for _, reference := range requested {
		if !allowed[reference] || seen[reference] {
			return false
		}
		seen[reference] = true
	}
	return true
}
