// Package presentation owns transport-neutral, privacy-safe user-facing
// labels shared by the app HTTP views and Telegram adapter.
package presentation

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"relux.works/duet/coordinator/internal/store"
)

type Locale string

const (
	English Locale = "en"
	Russian Locale = "ru"
)

type Label struct {
	Key string `json:"key"`
	EN  string `json:"en"`
	RU  string `json:"ru"`
}

func (label Label) Text(locale Locale) string {
	if locale == Russian {
		return label.RU
	}
	return label.EN
}

type textPair struct{ en, ru string }

var templates = map[string]textPair{
	"sender.named":                         {"%s", "%s"},
	"sender.unknown":                       {"Unknown sender", "Неизвестный отправитель"},
	"member.unknown":                       {"Unknown member", "Неизвестный участник"},
	"origin.named":                         {"From «%s»", "Из «%s»"},
	"origin.unknown":                       {"Unknown Barycenter", "Неизвестный Барицентр"},
	"target.orbit":                         {"«%s»", "«%s»"},
	"target.pulsar":                        {"«%s», Pulsar %s", "«%s», Пульсар %s"},
	"target.pulsar_unknown":                {"«%s», unknown Pulsar", "«%s», неизвестный Пульсар"},
	"target.unknown":                       {"Unknown recipient", "Неизвестный получатель"},
	"audience.this_pulsar":                 {"This Pulsar", "Этот Пульсар"},
	"audience.own_barycenter":              {"My Barycenter", "Мой Барицентр"},
	"audience.current_air":                 {"Current Air", "Текущий эфир"},
	"audience.current_air_named":           {"Current Air with «%s»", "Текущий эфир с «%s»"},
	"audience.explicit":                    {"Selected recipients", "Выбранные получатели"},
	"audience.unknown":                     {"Recipients", "Получатели"},
	"origin.included":                      {"Include this Pulsar", "Включая этот Пульсар"},
	"origin.excluded":                      {"Do not play on this Pulsar", "Не воспроизводить на этом Пульсаре"},
	"delivery.overlay":                     {"Overlay", "Поверх эфира"},
	"delivery.interrupt":                   {"Interrupt and resume", "Прервать и продолжить"},
	"delivery.after_current":               {"After current", "После текущего"},
	"delivery.unknown":                     {"Delivery unavailable", "Способ доставки недоступен"},
	"confirmation.interrupt_required":      {"Interrupt is unavailable for every recipient. Choose a fallback.", "Прерывание недоступно для всех получателей. Выберите замену."},
	"confirmation.choose_overlay":          {"Play as overlay", "Воспроизвести поверх"},
	"confirmation.choose_after_current":    {"Play after current", "Воспроизвести после текущего"},
	"confirmation.expired":                 {"This choice has expired", "Срок этого выбора истёк"},
	"confirmation.too_late":                {"Playback already started", "Воспроизведение уже началось"},
	"downgrade.missing_overlay_capability": {"Overlay is unavailable for all recipients; queued after current.", "Режим поверх эфира недоступен для всех получателей; поставлено после текущего."},
	"downgrade.confirmed_overlay":          {"Interrupt was replaced with overlay by the sender.", "Отправитель заменил прерывание режимом поверх эфира."},
	"downgrade.confirmed_after_current":    {"Interrupt was queued after current by the sender.", "Отправитель поставил сообщение после текущего вместо прерывания."},
	"downgrade.unknown":                    {"Delivery mode changed", "Способ доставки изменён"},
}

var rawIdentifier = regexp.MustCompile(`(?i)^(?:[a-z]|\d+|\d+:[a-z]|[a-z]@\d+|(?:actor|orbit|node|telegram|tg)[_:\- ]?\d+|(?:tr|m|hi|bl|ar|or)_[a-z0-9_-]+)$`)

func label(key string, args ...any) Label {
	template, ok := templates[key]
	if !ok {
		template = templates["target.unknown"]
		key = "target.unknown"
	}
	return Label{Key: key, EN: fmt.Sprintf(template.en, args...), RU: fmt.Sprintf(template.ru, args...)}
}

func safeHumanText(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if value == "" || rawIdentifier.MatchString(value) {
		return ""
	}
	if utf8.RuneCountInString(value) <= 120 {
		return value
	}
	runes := []rune(value)
	return string(runes[:120])
}

func SenderLabel(displayName string) Label {
	if name := safeHumanText(displayName); name != "" {
		return label("sender.named", name)
	}
	return label("sender.unknown")
}

func MemberLabel(displayName string) Label {
	if name := safeHumanText(displayName); name != "" {
		return label("sender.named", name)
	}
	return label("member.unknown")
}

func OriginLabel(orbitTitle string) Label {
	if title := safeHumanText(orbitTitle); title != "" {
		return label("origin.named", title)
	}
	return label("origin.unknown")
}

type TargetMetadata struct {
	OrbitTitle    string
	Slot          string
	MultipleSlots bool
}

func TargetLabel(metadata TargetMetadata) Label {
	title := safeHumanText(metadata.OrbitTitle)
	if title == "" {
		return label("target.unknown")
	}
	if !metadata.MultipleSlots {
		return label("target.orbit", title)
	}
	slot := strings.ToUpper(strings.TrimSpace(metadata.Slot))
	if len(slot) == 1 && slot[0] >= 'A' && slot[0] <= 'Z' {
		return label("target.pulsar", title, slot)
	}
	return label("target.pulsar_unknown", title)
}

func AudienceLabel(kind store.TransmissionAudienceKind, peerOrbitTitle string) Label {
	switch kind {
	case store.TransmissionAudienceThisPulsar:
		return label("audience.this_pulsar")
	case store.TransmissionAudienceOwnBarycenter:
		return label("audience.own_barycenter")
	case store.TransmissionAudienceCurrentAir:
		if title := safeHumanText(peerOrbitTitle); title != "" {
			return label("audience.current_air_named", title)
		}
		return label("audience.current_air")
	case store.TransmissionAudienceExplicit:
		return label("audience.explicit")
	default:
		return label("audience.unknown")
	}
}

func IncludeOriginLabel(include bool) Label {
	if include {
		return label("origin.included")
	}
	return label("origin.excluded")
}

func DeliveryLabel(delivery store.TransmissionDelivery) Label {
	switch delivery {
	case store.TransmissionDeliveryOverlay:
		return label("delivery.overlay")
	case store.TransmissionDeliveryInterrupt:
		return label("delivery.interrupt")
	case store.TransmissionDeliveryAfterCurrent:
		return label("delivery.after_current")
	default:
		return label("delivery.unknown")
	}
}

const (
	DowngradeMissingOverlayCapability = "mandatory_target_missing_overlay_capability"
	DowngradeConfirmedOverlay         = "sender_confirmed_overlay_fallback"
	DowngradeConfirmedAfterCurrent    = "sender_confirmed_after_current_fallback"
)

func DowngradeLabel(reason string) Label {
	switch reason {
	case DowngradeMissingOverlayCapability:
		return label("downgrade.missing_overlay_capability")
	case DowngradeConfirmedOverlay:
		return label("downgrade.confirmed_overlay")
	case DowngradeConfirmedAfterCurrent:
		return label("downgrade.confirmed_after_current")
	default:
		return label("downgrade.unknown")
	}
}

type DeliveryPresentation struct {
	Requested Label  `json:"requested"`
	Effective Label  `json:"effective"`
	Changed   bool   `json:"changed"`
	Notice    *Label `json:"notice,omitempty"`
}

func PresentDelivery(requested, effective store.TransmissionDelivery, downgradeReason string) DeliveryPresentation {
	result := DeliveryPresentation{
		Requested: DeliveryLabel(requested), Effective: DeliveryLabel(effective), Changed: requested != effective,
	}
	if result.Changed || downgradeReason != "" {
		notice := DowngradeLabel(downgradeReason)
		result.Notice = &notice
	}
	return result
}

func ConfirmationLabel(code string) Label {
	switch code {
	case "interrupt_required":
		return label("confirmation.interrupt_required")
	case "choose_overlay":
		return label("confirmation.choose_overlay")
	case "choose_after_current":
		return label("confirmation.choose_after_current")
	case "expired":
		return label("confirmation.expired")
	case "too_late":
		return label("confirmation.too_late")
	default:
		return label("confirmation.expired")
	}
}

var statusText = map[string]textPair{
	"accepted":         {"Accepted", "Принято"},
	"processing":       {"Processing", "Обрабатывается"},
	"ready":            {"Ready", "Готово"},
	"preparing":        {"Preparing recipients", "Получатели готовятся"},
	"scheduled":        {"Scheduled", "Запланировано"},
	"playing":          {"Playing", "Воспроизводится"},
	"cancelling":       {"Stopping", "Останавливается"},
	"played":           {"Played", "Воспроизведено"},
	"partial":          {"Partially delivered", "Доставлено частично"},
	"failed":           {"Delivery failed", "Ошибка доставки"},
	"cancelled":        {"Cancelled", "Отменено"},
	"expired":          {"Expired", "Истекло"},
	"missed_offline":   {"Recipient was offline", "Получатель был офлайн"},
	"missed_dnd":       {"Suppressed by Do Not Disturb", "Отклонено режимом «Не беспокоить»"},
	"missed_not_ready": {"Recipient was not ready", "Получатель не успел подготовиться"},
	"blocked":          {"Blocked", "Заблокировано"},
	"error":            {"Processing failed", "Ошибка обработки"},
}

var reasonText = map[store.TransmissionReason]textPair{
	store.TransmissionReasonCompleted:               {"Played completely", "Воспроизведено полностью"},
	store.TransmissionReasonPartialDelivery:         {"Some recipients did not complete playback", "Некоторые получатели не завершили воспроизведение"},
	store.TransmissionReasonNoEligibleTargets:       {"No eligible recipients", "Нет доступных получателей"},
	store.TransmissionReasonNoReadyTargets:          {"No recipient became ready", "Ни один получатель не подготовился"},
	store.TransmissionReasonAllTargetsFailed:        {"Delivery failed for every recipient", "Доставка не удалась всем получателям"},
	store.TransmissionReasonOfflineAtAcceptance:     {"Offline when accepted", "Был офлайн при принятии"},
	store.TransmissionReasonOfflineBeforePrepare:    {"Went offline before preparation", "Перешёл офлайн до подготовки"},
	store.TransmissionReasonOfflineBeforeStart:      {"Went offline before playback", "Перешёл офлайн до воспроизведения"},
	store.TransmissionReasonLocalDND:                {"Local Do Not Disturb", "Локальный режим «Не беспокоить»"},
	store.TransmissionReasonOrbitDND:                {"Barycenter Do Not Disturb", "Режим «Не беспокоить» Барицентра"},
	store.TransmissionReasonPrepareDeadline:         {"Preparation deadline missed", "Пропущен срок подготовки"},
	store.TransmissionReasonActorBlocked:            {"Sender is blocked", "Отправитель заблокирован"},
	store.TransmissionReasonOrbitBlocked:            {"Sender's Barycenter is blocked", "Барицентр отправителя заблокирован"},
	store.TransmissionReasonMediaDownloadFailed:     {"Media download failed", "Не удалось загрузить медиа"},
	store.TransmissionReasonMediaAuthFailed:         {"Media access was rejected", "Доступ к медиа отклонён"},
	store.TransmissionReasonMediaExpired:            {"Media expired", "Срок медиа истёк"},
	store.TransmissionReasonHashMismatch:            {"Media integrity check failed", "Проверка целостности медиа не пройдена"},
	store.TransmissionReasonDecodeFailed:            {"Audio decoding failed", "Не удалось декодировать аудио"},
	store.TransmissionReasonDurationMismatch:        {"Audio duration did not match", "Длительность аудио не совпала"},
	store.TransmissionReasonClockUnsynchronized:     {"Recipient clock is not synchronized", "Часы получателя не синхронизированы"},
	store.TransmissionReasonStalePlay:               {"Scheduled playback became stale", "Время запланированного воспроизведения прошло"},
	store.TransmissionReasonDeviceUnavailable:       {"Audio device is unavailable", "Аудиоустройство недоступно"},
	store.TransmissionReasonAudioGraphFailed:        {"Audio playback could not start", "Не удалось запустить аудиовоспроизведение"},
	store.TransmissionReasonConnectionLost:          {"Connection was lost", "Соединение потеряно"},
	store.TransmissionReasonCapabilityLost:          {"Required playback capability was lost", "Требуемая возможность воспроизведения потеряна"},
	store.TransmissionReasonInterruptCapabilityLost: {"Exact interrupt/resume became unavailable", "Точное прерывание и продолжение стало недоступно"},
	store.TransmissionReasonCancelUnacknowledged:    {"Recipient did not confirm cancellation", "Получатель не подтвердил отмену"},
	store.TransmissionReasonInternalError:           {"Internal delivery error", "Внутренняя ошибка доставки"},
	store.TransmissionReasonSenderCancelled:         {"Cancelled by sender", "Отменено отправителем"},
	store.TransmissionReasonMediaDeleted:            {"Media was deleted", "Медиа удалено"},
	store.TransmissionReasonModerationDisabled:      {"Media was disabled", "Медиа отключено"},
	store.TransmissionReasonApproachLeft:            {"Barycenter left the shared Air", "Барицентр покинул общий эфир"},
	store.TransmissionReasonApproachApart:           {"Shared Air ended", "Общий эфир завершён"},
	store.TransmissionReasonTargetRevoked:           {"Recipient access was revoked", "Доступ получателя отозван"},
	store.TransmissionReasonDNDEnabled:              {"Do Not Disturb was enabled", "Включён режим «Не беспокоить»"},
	store.TransmissionReasonSenderBlocked:           {"Sender was blocked during playback", "Отправитель заблокирован во время воспроизведения"},
	store.TransmissionReasonCoordinatorRestarted:    {"Delivery stopped after coordinator restart", "Доставка остановлена после перезапуска координатора"},
	store.TransmissionReasonDeliveryExpired:         {"Delivery window expired", "Срок доставки истёк"},
}

func ReceiptLabel(status string, reason store.TransmissionReason) Label {
	if pair, ok := reasonText[reason]; ok {
		return Label{Key: "receipt.reason." + string(reason), EN: pair.en, RU: pair.ru}
	}
	if pair, ok := statusText[status]; ok {
		return Label{Key: "receipt.status." + status, EN: pair.en, RU: pair.ru}
	}
	return Label{Key: "receipt.unknown", EN: "Status unavailable", RU: "Статус недоступен"}
}

func StaticLabels() []Label {
	labels := make([]Label, 0, len(templates)+len(statusText)+len(reasonText))
	for key, pair := range templates {
		if !strings.Contains(pair.en, "%") && !strings.Contains(pair.ru, "%") {
			labels = append(labels, Label{Key: key, EN: pair.en, RU: pair.ru})
		}
	}
	for status, pair := range statusText {
		labels = append(labels, Label{Key: "receipt.status." + status, EN: pair.en, RU: pair.ru})
	}
	for reason, pair := range reasonText {
		labels = append(labels, Label{Key: "receipt.reason." + string(reason), EN: pair.en, RU: pair.ru})
	}
	sort.Slice(labels, func(i, j int) bool { return labels[i].Key < labels[j].Key })
	return labels
}
