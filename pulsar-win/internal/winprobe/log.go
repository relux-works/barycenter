package winprobe

import (
	"encoding/json"
	"io"
	"sync"
	"time"
)

type LogEvent struct {
	Timestamp        time.Time      `json:"timestamp"`
	Scenario         ProbeScenario  `json:"scenario"`
	Result           ProbeResult    `json:"result"`
	Action           string         `json:"action"`
	SelectedAPIPath  string         `json:"selectedApiPath,omitempty"`
	DeviceID         string         `json:"deviceId,omitempty"`
	DeviceName       string         `json:"deviceName,omitempty"`
	PermissionStatus string         `json:"permissionStatus,omitempty"`
	HResult          string         `json:"hresult,omitempty"`
	FailureCause     string         `json:"failureCause,omitempty"`
	WindowVisible    *bool          `json:"windowVisible,omitempty"`
	HotkeyRegistered *bool          `json:"hotkeyRegistered,omitempty"`
	Fields           map[string]any `json:"fields,omitempty"`
}

type JSONLogger struct {
	mu sync.Mutex
	w  io.Writer
}

func NewJSONLogger(w io.Writer) *JSONLogger {
	return &JSONLogger{w: w}
}

func (l *JSONLogger) Log(event LogEvent) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	enc := json.NewEncoder(l.w)
	return enc.Encode(event)
}
