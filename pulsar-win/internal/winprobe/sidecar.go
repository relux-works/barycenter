package winprobe

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

const (
	SidecarVersion = 1
	MaxSidecarSize = 4096
)

type ReasonRecord struct {
	Version     int           `json:"version"`
	SessionID   string        `json:"sessionId"`
	Reason      CaptureReason `json:"reason"`
	ReasonName  string        `json:"reasonName"`
	HResult     HResult       `json:"hresult"`
	TimestampMS int64         `json:"timestampMs"`
}

type reasonRecordWire struct {
	Version     *int           `json:"version"`
	SessionID   *string        `json:"sessionId"`
	Reason      *CaptureReason `json:"reason"`
	ReasonName  *string        `json:"reasonName"`
	HResult     *int64         `json:"hresult"`
	TimestampMS *int64         `json:"timestampMs"`
}

func ParseReasonRecord(data []byte, expectedSessionID string) (ReasonRecord, error) {
	if len(data) == 0 || len(data) > MaxSidecarSize {
		return ReasonRecord{}, fmt.Errorf("sidecar size %d outside 1..%d", len(data), MaxSidecarSize)
	}
	if err := rejectDuplicateKeys(data); err != nil {
		return ReasonRecord{}, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var wire reasonRecordWire
	if err := dec.Decode(&wire); err != nil {
		return ReasonRecord{}, fmt.Errorf("decode sidecar: %w", err)
	}
	if err := requireJSONEOF(dec); err != nil {
		return ReasonRecord{}, err
	}
	if wire.Version == nil || wire.SessionID == nil || wire.Reason == nil || wire.ReasonName == nil || wire.HResult == nil || wire.TimestampMS == nil {
		return ReasonRecord{}, fmt.Errorf("sidecar missing required field")
	}
	if *wire.HResult < -1<<31 || *wire.HResult > 1<<31-1 {
		return ReasonRecord{}, fmt.Errorf("hresult %d outside signed int32", *wire.HResult)
	}
	record := ReasonRecord{
		Version:     *wire.Version,
		SessionID:   *wire.SessionID,
		Reason:      *wire.Reason,
		ReasonName:  *wire.ReasonName,
		HResult:     HResult(*wire.HResult),
		TimestampMS: *wire.TimestampMS,
	}
	if err := record.Validate(expectedSessionID); err != nil {
		return ReasonRecord{}, err
	}
	return record, nil
}

func (r ReasonRecord) Validate(expectedSessionID string) error {
	if r.Version != SidecarVersion {
		return fmt.Errorf("sidecar version %d, want %d", r.Version, SidecarVersion)
	}
	if r.SessionID == "" || r.SessionID != expectedSessionID {
		return fmt.Errorf("sidecar session %q does not match %q", r.SessionID, expectedSessionID)
	}
	if r.Reason < ReasonUserStop || r.Reason > ReasonDiscontinuity {
		return fmt.Errorf("unknown capture reason %d", r.Reason)
	}
	if r.ReasonName != r.Reason.String() {
		return fmt.Errorf("reason name %q does not match %q", r.ReasonName, r.Reason.String())
	}
	if r.TimestampMS <= 0 {
		return fmt.Errorf("timestampMs must be positive")
	}
	if !reasonHResultCompatible(r.Reason, r.HResult) {
		return fmt.Errorf("reason %s is incompatible with hresult %s", r.Reason, r.HResult.Hex())
	}
	return nil
}

func reasonHResultCompatible(reason CaptureReason, hr HResult) bool {
	raw := uint32(hr)
	switch reason {
	case ReasonUserStop, ReasonShutdown, ReasonSuspend, ReasonLock:
		return raw == 0
	case ReasonPermissionRevoke:
		return raw == 0x80070005
	case ReasonDeviceLost:
		return raw == 0x88890004
	case ReasonCancel:
		return raw == 0x800704c7
	case ReasonOverflow:
		return raw == 0x8007006f
	case ReasonFormatError:
		return raw == 0x80070057
	case ReasonDiscontinuity:
		return raw == 0x8007000d
	case ReasonWasapiError:
		if !hr.Failed() {
			return false
		}
		switch raw {
		case 0x80070005, 0x88890004, 0x800704c7, 0x8007006f, 0x8007000d:
			return false
		default:
			return true
		}
	default:
		return false
	}
}

func rejectDuplicateKeys(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	var walk func(json.Token) error
	walk = func(token json.Token) error {
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := make(map[string]struct{})
			for dec.More() {
				keyToken, err := dec.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok {
					return fmt.Errorf("object key is not a string")
				}
				if _, duplicate := seen[key]; duplicate {
					return fmt.Errorf("duplicate key %q", key)
				}
				seen[key] = struct{}{}
				value, err := dec.Token()
				if err != nil {
					return err
				}
				if err := walk(value); err != nil {
					return err
				}
			}
			end, err := dec.Token()
			if err != nil || end != json.Delim('}') {
				return fmt.Errorf("unterminated object")
			}
		case '[':
			for dec.More() {
				value, err := dec.Token()
				if err != nil {
					return err
				}
				if err := walk(value); err != nil {
					return err
				}
			}
			end, err := dec.Token()
			if err != nil || end != json.Delim(']') {
				return fmt.Errorf("unterminated array")
			}
		default:
			return fmt.Errorf("unexpected delimiter %q", delim)
		}
		return nil
	}
	root, err := dec.Token()
	if err != nil {
		return err
	}
	if root != json.Delim('{') {
		return fmt.Errorf("sidecar root must be an object")
	}
	if err := walk(root); err != nil {
		return err
	}
	return requireJSONEOF(dec)
}

func requireJSONEOF(dec *json.Decoder) error {
	if _, err := dec.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing JSON content")
		}
		return fmt.Errorf("trailing JSON content: %w", err)
	}
	return nil
}

func PromotableReason(reason CaptureReason) bool {
	switch reason {
	case ReasonUserStop, ReasonDeviceLost, ReasonShutdown, ReasonSuspend, ReasonLock:
		return true
	default:
		return false
	}
}
