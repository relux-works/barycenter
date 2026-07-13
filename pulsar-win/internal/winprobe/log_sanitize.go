package winprobe

import (
	"reflect"
	"regexp"
	"strings"
)

const redactedLogValue = "[redacted]"
const maxLogSanitizeDepth = 16

var (
	windowsPathPattern  = regexp.MustCompile(`(?i)(^|[\s("'=:\[])(([a-z]:[\\/])|(\\\\[^\\\s]+[\\/]))`)
	posixPathPattern    = regexp.MustCompile(`(^|[\s("'=\[])/`)
	localFileURIPattern = regexp.MustCompile(`(?i)\bfile:/+`)
	credentialPattern   = regexp.MustCompile(`(?i)(bearer\s+|(?:access[_-]?)?token(?:\s*[=:]\s*|\s+)|secret(?:\s*[=:]\s*|\s+)|api[_-]?key(?:\s*[=:]\s*|\s+)|password(?:\s*[=:]\s*|\s+)|passwd(?:\s*[=:]\s*|\s+)|credential(?:\s*[=:]\s*|\s+)|authorization(?:\s*[=:]\s*|\s+))[^\s,;]+`)
	mmDeviceIDPattern   = regexp.MustCompile(`(?i)^\{[0-9]+\.[0-9]+\.[0-9]+\.[0-9a-f]+\}\.\{[0-9a-f-]+\}$`)
)

// SanitizeLogEvent applies the ordinary-log privacy boundary immediately before
// serialization. It clones nested fields so callers retain their own values.
// Generated/session IDs, hashes, sizes, and device identities remain usable;
// local paths, original picker names, credentials, and payload-like fields do
// not cross the boundary.
func SanitizeLogEvent(event LogEvent) LogEvent {
	event.Action = sanitizeLogText(event.Action)
	event.SelectedAPIPath = sanitizeLogText(event.SelectedAPIPath)
	event.DeviceID = sanitizeTopLevelDeviceID(event.DeviceID)
	event.DeviceName = sanitizeLogText(event.DeviceName)
	if event.Scenario == ScenarioPicker {
		event.DeviceName = ""
	}
	event.PermissionStatus = sanitizeLogText(event.PermissionStatus)
	event.FailureCause = sanitizeLogText(event.FailureCause)
	if event.Fields != nil {
		event.Fields = sanitizeLogMapDepth(event.Fields, 0)
	}
	return event
}

func sanitizeTopLevelDeviceID(value string) string {
	// DeviceID is populated only from the trusted Windows default-device or
	// enumeration result, never from the picker. Preserve the two recognized
	// Windows audio identity forms even though the device-interface prefix looks
	// like UNC. All other values still cross the ordinary credential/path guard.
	upper := strings.ToUpper(value)
	if strings.HasPrefix(upper, `\\?\SWD#MMDEVAPI#`) || mmDeviceIDPattern.MatchString(value) {
		if credentialPattern.MatchString(value) {
			return redactedLogValue
		}
		return value
	}
	return sanitizeLogText(value)
}

func sanitizeLogMap(fields map[string]any) map[string]any {
	return sanitizeLogMapDepth(fields, 0)
}

func sanitizeLogMapDepth(fields map[string]any, depth int) map[string]any {
	clean := make(map[string]any, len(fields))
	for key, value := range fields {
		if sensitiveLogKey(key) {
			clean[key] = redactedLogValue
			continue
		}
		clean[key] = sanitizeLogValueDepth(value, depth+1)
	}
	return clean
}

func sanitizeLogValue(value any) any {
	return sanitizeLogValueDepth(value, 0)
}

func sanitizeLogValueDepth(value any, depth int) any {
	if depth >= maxLogSanitizeDepth {
		return redactedLogValue
	}
	switch typed := value.(type) {
	case string:
		return sanitizeLogText(typed)
	case error:
		return sanitizeLogText(typed.Error())
	case map[string]any:
		return sanitizeLogMapDepth(typed, depth+1)
	case []string:
		clean := make([]string, len(typed))
		for index := range typed {
			clean[index] = sanitizeLogText(typed[index])
		}
		return clean
	case []any:
		clean := make([]any, len(typed))
		for index := range typed {
			clean[index] = sanitizeLogValueDepth(typed[index], depth+1)
		}
		return clean
	}

	reflected := reflect.ValueOf(value)
	if reflected.IsValid() && (reflected.Kind() == reflect.Pointer || reflected.Kind() == reflect.Interface) {
		if reflected.IsNil() {
			return nil
		}
		return sanitizeLogValueDepth(reflected.Elem().Interface(), depth+1)
	}
	if reflected.IsValid() && reflected.Kind() == reflect.String {
		return sanitizeLogText(reflected.String())
	}
	if reflected.IsValid() && reflected.Kind() == reflect.Map && reflected.Type().Key().Kind() == reflect.String {
		clean := make(map[string]any, reflected.Len())
		iterator := reflected.MapRange()
		for iterator.Next() {
			key := iterator.Key().String()
			if sensitiveLogKey(key) {
				clean[key] = redactedLogValue
			} else {
				clean[key] = sanitizeLogValueDepth(iterator.Value().Interface(), depth+1)
			}
		}
		return clean
	}
	if reflected.IsValid() && reflected.Kind() == reflect.Struct {
		clean := make(map[string]any, reflected.NumField())
		typeInfo := reflected.Type()
		for index := 0; index < reflected.NumField(); index++ {
			fieldInfo := typeInfo.Field(index)
			if fieldInfo.PkgPath != "" {
				continue
			}
			if sensitiveLogKey(fieldInfo.Name) {
				clean[fieldInfo.Name] = redactedLogValue
			} else {
				clean[fieldInfo.Name] = sanitizeLogValueDepth(reflected.Field(index).Interface(), depth+1)
			}
		}
		return clean
	}
	if reflected.IsValid() && (reflected.Kind() == reflect.Slice || reflected.Kind() == reflect.Array) {
		clean := make([]any, reflected.Len())
		for index := 0; index < reflected.Len(); index++ {
			clean[index] = sanitizeLogValueDepth(reflected.Index(index).Interface(), depth+1)
		}
		return clean
	}
	return value
}

func sensitiveLogKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "_", ""), "-", ""))
	for _, fragment := range []string{"path", "filename", "originalname", "username", "userprofile", "token", "secret", "auth", "credential", "apikey", "password", "passwd", "cookie", "audiocontent", "recordingcontent", "pcmpayload", "samples"} {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return false
}

func sanitizeLogText(value string) string {
	// Redact the whole value when it contains an absolute path. Trying to keep a
	// prefix is unsafe because paths can contain spaces and original filenames;
	// partial replacement can leak the tail.
	if windowsPathPattern.MatchString(value) || posixPathPattern.MatchString(value) || localFileURIPattern.MatchString(value) {
		return redactedLogValue
	}
	if credentialPattern.MatchString(value) {
		return redactedLogValue
	}
	return value
}
