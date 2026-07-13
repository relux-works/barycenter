package winprobe

import (
	"encoding/json"
	"strings"
	"testing"
)

func hresultRaw(raw uint32) HResult { return HResult(int32(raw)) }

func TestReasonRecordCompatibilityTable(t *testing.T) {
	t.Parallel()
	tests := []struct {
		reason CaptureReason
		hr     HResult
	}{
		{ReasonUserStop, 0},
		{ReasonPermissionRevoke, hresultRaw(0x80070005)},
		{ReasonDeviceLost, hresultRaw(0x88890004)},
		{ReasonShutdown, 0},
		{ReasonSuspend, 0},
		{ReasonLock, 0},
		{ReasonCancel, hresultRaw(0x800704c7)},
		{ReasonOverflow, hresultRaw(0x8007006f)},
		{ReasonWasapiError, hresultRaw(0x88890010)},
		{ReasonFormatError, hresultRaw(0x80070057)},
		{ReasonDiscontinuity, hresultRaw(0x8007000d)},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.reason.String(), func(t *testing.T) {
			t.Parallel()
			record := ReasonRecord{Version: 1, SessionID: "session", Reason: tc.reason, ReasonName: tc.reason.String(), HResult: tc.hr, TimestampMS: 1}
			raw, err := json.Marshal(record)
			if err != nil {
				t.Fatal(err)
			}
			got, err := ParseReasonRecord(raw, "session")
			if err != nil {
				t.Fatalf("ParseReasonRecord() error = %v", err)
			}
			if got != record {
				t.Fatalf("ParseReasonRecord() = %#v, want %#v", got, record)
			}
		})
	}
}

func TestReasonRecordRejectsCorruptionFailClosed(t *testing.T) {
	t.Parallel()
	valid := `{"version":1,"sessionId":"s","reason":0,"reasonName":"user_stop","hresult":0,"timestampMs":1}`
	tests := []struct {
		name string
		raw  string
	}{
		{name: "top duplicate first safe", raw: `{"version":1,"sessionId":"s","reason":0,"reason":1,"reasonName":"user_stop","hresult":0,"timestampMs":1}`},
		{name: "top duplicate last safe", raw: `{"version":1,"sessionId":"s","reason":1,"reason":0,"reasonName":"user_stop","hresult":0,"timestampMs":1}`},
		{name: "nested duplicate", raw: `{"version":1,"sessionId":"s","reason":0,"reasonName":"user_stop","hresult":0,"timestampMs":1,"x":{"a":1,"a":2}}`},
		{name: "array duplicate", raw: `{"version":1,"sessionId":"s","reason":0,"reasonName":"user_stop","hresult":0,"timestampMs":1,"x":[{"a":1,"a":2}]}`},
		{name: "unknown", raw: strings.TrimSuffix(valid, "}") + `,"unknown":1}`},
		{name: "missing version", raw: `{"sessionId":"s","reason":0,"reasonName":"user_stop","hresult":0,"timestampMs":1}`},
		{name: "missing session", raw: `{"version":1,"reason":0,"reasonName":"user_stop","hresult":0,"timestampMs":1}`},
		{name: "missing reason", raw: `{"version":1,"sessionId":"s","reasonName":"user_stop","hresult":0,"timestampMs":1}`},
		{name: "missing reason name", raw: `{"version":1,"sessionId":"s","reason":0,"hresult":0,"timestampMs":1}`},
		{name: "missing hresult", raw: `{"version":1,"sessionId":"s","reason":0,"reasonName":"user_stop","timestampMs":1}`},
		{name: "missing timestamp", raw: `{"version":1,"sessionId":"s","reason":0,"reasonName":"user_stop","hresult":0}`},
		{name: "version mismatch", raw: `{"version":2,"sessionId":"s","reason":0,"reasonName":"user_stop","hresult":0,"timestampMs":1}`},
		{name: "unknown reason", raw: `{"version":1,"sessionId":"s","reason":11,"reasonName":"reason_11","hresult":0,"timestampMs":1}`},
		{name: "nonpositive timestamp", raw: `{"version":1,"sessionId":"s","reason":0,"reasonName":"user_stop","hresult":0,"timestampMs":0}`},
		{name: "name mismatch", raw: `{"version":1,"sessionId":"s","reason":0,"reasonName":"permission_revoke","hresult":0,"timestampMs":1}`},
		{name: "zero cancel", raw: `{"version":1,"sessionId":"s","reason":6,"reasonName":"cancel","hresult":0,"timestampMs":1}`},
		{name: "incompatible", raw: `{"version":1,"sessionId":"s","reason":1,"reasonName":"permission_revoke","hresult":-2147023729,"timestampMs":1}`},
		{name: "out of int32", raw: `{"version":1,"sessionId":"s","reason":0,"reasonName":"user_stop","hresult":4294967296,"timestampMs":1}`},
		{name: "concatenated", raw: valid + valid},
		{name: "trailing", raw: valid + " nope"},
		{name: "root array", raw: `[]`},
		{name: "empty", raw: ``},
		{name: "oversized", raw: `{"version":1,"padding":"` + strings.Repeat("x", MaxSidecarSize) + `"}`},
		{name: "wrong session", raw: valid},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			expected := "s"
			if tc.name == "wrong session" {
				expected = "different"
			}
			if _, err := ParseReasonRecord([]byte(tc.raw), expected); err == nil {
				t.Fatalf("ParseReasonRecord(%s) unexpectedly succeeded", tc.raw)
			}
		})
	}
}

func TestReasonRecordAcceptsWhitespaceEOF(t *testing.T) {
	t.Parallel()
	raw := []byte(" \n\t{\"version\":1,\"sessionId\":\"s\",\"reason\":0,\"reasonName\":\"user_stop\",\"hresult\":0,\"timestampMs\":1}\r\n")
	if _, err := ParseReasonRecord(raw, "s"); err != nil {
		t.Fatalf("ParseReasonRecord() error = %v", err)
	}
}
