package bot

// AttachmentFailureCode is the stable Telegram-facing vocabulary. Common
// ingest remains authoritative; this adapter only translates its proof into a
// non-disclosing user result.
type AttachmentFailureCode string

const (
	AttachmentDownloadFailed    AttachmentFailureCode = "telegram_download_failed"
	AttachmentTooLarge          AttachmentFailureCode = "telegram_media_too_large"
	AttachmentNotAudio          AttachmentFailureCode = "attachment_not_audio"
	AttachmentGroupUnsupported  AttachmentFailureCode = "media_group_not_supported_phase1"
	AttachmentTrackPhase2       AttachmentFailureCode = "track_not_available_phase1"
	AttachmentDecodeFailed      AttachmentFailureCode = "decode_failed"
	AttachmentDurationMismatch  AttachmentFailureCode = "duration_mismatch"
	AttachmentCanonicalTooLarge AttachmentFailureCode = "canonical_output_too_large"
)

// AttachmentFailureFromIngest maps proof produced by bounded common ingest.
// No Telegram metadata is accepted as proof of file type, duration or size.
func AttachmentFailureFromIngest(kind AttachmentKind, ingestCode string) AttachmentFailureCode {
	switch ingestCode {
	case "media_input_oversized":
		return AttachmentTooLarge
	case "media_duration_exceeded":
		return AttachmentTrackPhase2
	case "codec_profile_unavailable":
		return AttachmentTrackPhase2
	case "media_duration_mismatch", "media_duration_invalid":
		return AttachmentDurationMismatch
	case "canonical_output_oversized":
		return AttachmentCanonicalTooLarge
	case "media_signature_unsupported":
		if kind == AttachmentDocument {
			return AttachmentNotAudio
		}
		return AttachmentDecodeFailed
	case "media_input_unavailable", "media_input_unreadable":
		return AttachmentDownloadFailed
	default:
		return AttachmentDecodeFailed
	}
}

func AttachmentFailureText(code AttachmentFailureCode) string {
	switch code {
	case AttachmentTooLarge:
		return "Файл больше 20 МБ — не возьму. Размер проверен после ограниченной загрузки."
	case AttachmentNotAudio:
		return "В документе не найден поддерживаемый аудиопоток."
	case AttachmentGroupUnsupported:
		return "Альбомы и несколько вложений пока не поддерживаются. Отправь один аудиофайл."
	case AttachmentTrackPhase2:
		return "Этот файл требует режима трека, но потоковая доставка пока недоступна для production-профиля. Короткий клип до 3 минут можно отправить сейчас."
	case AttachmentDurationMismatch:
		return "Не удалось надёжно определить длительность аудио."
	case AttachmentCanonicalTooLarge:
		return "Нормализованный клип превышает допустимый размер."
	case AttachmentDownloadFailed:
		return "Не удалось безопасно скачать вложение из Telegram."
	default:
		return "Не удалось декодировать поддерживаемое аудио."
	}
}
