package winprobe

import (
	"fmt"
	"strings"
	"testing"
)

type nestedPrivateEvidence struct {
	OriginalFilename string
	Password         string
	Details          []string
	Result           string
}

type cyclicPrivateEvidence struct {
	Next     *cyclicPrivateEvidence
	Password string
}

func TestSanitizeLogEventRecursivelyRemovesPrivateValues(t *testing.T) {
	t.Parallel()
	event := SanitizeLogEvent(LogEvent{
		Scenario:     ScenarioPicker,
		Result:       ResultFail,
		Action:       "picker_result",
		DeviceName:   "original-recording.wav",
		FailureCause: `open C:\Users\alice\My Music\Secret Recording.wav: denied`,
		Fields: map[string]any{
			"path":             "/Users/alice/Music/private.wav",
			"originalFilename": "private.wav",
			"username":         "alice",
			"token":            "secret-token",
			"authCredential":   "credential-value",
			"auth":             "opaque-auth-value",
			"passwd":           "passwd-value",
			"audioContent":     []float32{0.1, 0.2},
			"nested": map[string]any{
				"cleanupError":  `remove /Volumes/External Drive/Probe Audio/capture.partial: denied`,
				"authorization": "Bearer abc.def.ghi",
				"typedMap":      map[string]string{"userProfile": "/mnt/user data/alice", "result": "pass"},
				"typedSlice":    []string{"/data/private clips/clip.wav", "safe-result"},
				"typedStruct": &nestedPrivateEvidence{
					OriginalFilename: "voice memo.wav",
					Password:         "password-value",
					Details:          []string{`\\server\private share\secret clip.wav`, "safe-detail"},
					Result:           "pass",
				},
			},
			"sessionId": "capture-42",
			"sha256":    "abc123",
			"bytes":     uint64(128),
		},
	})
	serialized := fmt.Sprintf("%+v", event)
	for _, prohibited := range []string{"original-recording.wav", "private.wav", "Secret Recording.wav", `C:\Users\alice`, "/Users/alice", "/Volumes/External Drive", "/mnt/user data", "/data/private clips", `\\server\private share`, "voice memo.wav", "password-value", "passwd-value", "credential-value", "opaque-auth-value", "secret-token", "abc.def.ghi", "0.1", "alice"} {
		if strings.Contains(serialized, prohibited) {
			t.Errorf("sanitized event retained %q: %s", prohibited, serialized)
		}
	}
	for _, retained := range []string{"capture-42", "abc123", "128", "safe-result", "safe-detail"} {
		if !strings.Contains(serialized, retained) {
			t.Errorf("sanitized event lost allowed evidence %q: %s", retained, serialized)
		}
	}
}

func TestSanitizeLogEventKeepsCaptureDeviceIdentity(t *testing.T) {
	t.Parallel()
	deviceInterfaceID := `\\?\SWD#MMDEVAPI#{0.0.1.00000000}.{1c403ff5-79ef-4c5b-a4fb-6fecc6c83a5a}#{2eef81be-33fa-4800-9670-1cd474972c3f}`
	mmDeviceID := `{0.0.1.00000000}.{1c403ff5-79ef-4c5b-a4fb-6fecc6c83a5a}`
	for _, deviceID := range []string{"device-id-42", deviceInterfaceID, mmDeviceID} {
		event := SanitizeLogEvent(LogEvent{Scenario: ScenarioCapture, DeviceID: deviceID, DeviceName: "USB microphone"})
		if event.DeviceID != deviceID || event.DeviceName != "USB microphone" {
			t.Errorf("device evidence was over-redacted: %+v", event)
		}
	}
	for _, privateValue := range []string{`C:\Users\alice\private.wav`, `\\server\share\private.wav`, "token token-value", "authorization Bearer auth-value"} {
		event := SanitizeLogEvent(LogEvent{Scenario: ScenarioCapture, DeviceID: privateValue})
		if event.DeviceID != redactedLogValue {
			t.Errorf("untrusted top-level DeviceID = %q, want redaction for %q", event.DeviceID, privateValue)
		}
	}
}

func TestSanitizeLogEventDirectPathCredentialAndCycleCases(t *testing.T) {
	t.Parallel()
	cycle := &cyclicPrivateEvidence{Password: "cycle-password"}
	cycle.Next = cycle
	tests := []struct {
		name                 string
		event                LogEvent
		leaks                []string
		failureFullyRedacted bool
	}{
		{name: "drive path with spaces", event: LogEvent{FailureCause: `open C:\Users\alice\My Music\Secret Recording.wav: denied`}, leaks: []string{`C:\Users\alice`, "Secret Recording.wav"}},
		{name: "normal UNC path", event: LogEvent{FailureCause: `remove \\server\private share\secret clip.wav: denied`}, leaks: []string{`\\server\private share`, "secret clip.wav"}},
		{name: "POSIX path after whitespace", event: LogEvent{FailureCause: `remove /srv/private clips/secret clip.wav: denied`}, leaks: []string{"/srv/private clips", "secret clip.wav"}},
		{name: "root-level POSIX file before punctuation", event: LogEvent{FailureCause: `remove /secret.wav: denied`}, leaks: []string{"/secret.wav", "secret.wav"}, failureFullyRedacted: true},
		{name: "local file URI", event: LogEvent{FailureCause: `open file:///Users/alice/private.wav: denied`}, leaks: []string{"file:///Users/alice", "private.wav"}, failureFullyRedacted: true},
		{name: "credentials in ordinary values", event: LogEvent{Fields: map[string]any{"note": "token=token-value", "message": "authorization=Bearer auth-value", "login": "password=password-value"}}, leaks: []string{"token-value", "auth-value", "password-value"}},
		{name: "whitespace credentials in ordinary values", event: LogEvent{FailureCause: "query failed: token token-value", Fields: map[string]any{"message": "authorization Bearer auth-value"}}, leaks: []string{"token-value", "auth-value"}, failureFullyRedacted: true},
		{name: "cyclic typed pointer", event: LogEvent{Fields: map[string]any{"cycle": cycle}}, leaks: []string{"cycle-password"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clean := SanitizeLogEvent(tc.event)
			serialized := fmt.Sprintf("%+v", clean)
			for _, leak := range tc.leaks {
				if strings.Contains(serialized, leak) {
					t.Errorf("sanitized event retained %q: %s", leak, serialized)
				}
			}
			if tc.failureFullyRedacted && clean.FailureCause != redactedLogValue {
				t.Errorf("FailureCause = %q, want whole-value redaction", clean.FailureCause)
			}
			if tc.name == "whitespace credentials in ordinary values" && clean.Fields["message"] != redactedLogValue {
				t.Errorf("ordinary authorization field = %q, want whole-value redaction", clean.Fields["message"])
			}
		})
	}
}
