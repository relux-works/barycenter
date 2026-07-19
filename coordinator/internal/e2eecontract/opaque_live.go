package e2eecontract

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	OpaqueLiveFrameHeaderBytes          = 84
	OpaqueLiveMaxCiphertextBytes        = 512
	OpaqueLiveMaxMessageBytes           = OpaqueLiveFrameHeaderBytes + OpaqueLiveMaxCiphertextBytes
	OpaqueLiveFrameMS                   = 20
	OpaqueLiveMaxGapFrames       uint32 = 8
	OpaqueLiveMaxDurationMS             = 300000
)

const (
	OpaqueLiveFlagStart byte = 1 << iota
	OpaqueLiveFlagEnd
	opaqueLiveAllowedFlags = OpaqueLiveFlagStart | OpaqueLiveFlagEnd
)

type OpaqueLiveFrame struct {
	Flags                byte
	SessionID            [16]byte
	Epoch, Generation    uint64
	Sequence             uint32
	CaptureMonotonicUS   uint64
	TargetSnapshotDigest string
	Ciphertext           []byte
}

func EncodeOpaqueLiveFrame(frame OpaqueLiveFrame) ([]byte, error) {
	if err := validateOpaqueLiveFrame(frame); err != nil {
		return nil, err
	}
	target, _ := hex.DecodeString(frame.TargetSnapshotDigest)
	result := make([]byte, OpaqueLiveFrameHeaderBytes+len(frame.Ciphertext))
	copy(result[0:2], []byte("BE"))
	result[2], result[3] = 1, frame.Flags
	copy(result[4:20], frame.SessionID[:])
	binary.BigEndian.PutUint64(result[20:28], frame.Epoch)
	binary.BigEndian.PutUint64(result[28:36], frame.Generation)
	binary.BigEndian.PutUint32(result[36:40], frame.Sequence)
	binary.BigEndian.PutUint64(result[40:48], frame.CaptureMonotonicUS)
	copy(result[48:80], target)
	binary.BigEndian.PutUint16(result[80:82], uint16(len(frame.Ciphertext)))
	binary.BigEndian.PutUint16(result[82:84], 0)
	copy(result[84:], frame.Ciphertext)
	return result, nil
}

func DecodeOpaqueLiveFrame(raw []byte) (OpaqueLiveFrame, error) {
	if len(raw) < OpaqueLiveFrameHeaderBytes || len(raw) > OpaqueLiveMaxMessageBytes ||
		string(raw[0:2]) != "BE" || raw[2] != 1 || binary.BigEndian.Uint16(raw[82:84]) != 0 {
		return OpaqueLiveFrame{}, fmt.Errorf("invalid opaque live frame header")
	}
	size := int(binary.BigEndian.Uint16(raw[80:82]))
	if size < 1 || size > OpaqueLiveMaxCiphertextBytes || len(raw) != OpaqueLiveFrameHeaderBytes+size {
		return OpaqueLiveFrame{}, fmt.Errorf("invalid opaque live ciphertext length")
	}
	var frame OpaqueLiveFrame
	frame.Flags = raw[3]
	copy(frame.SessionID[:], raw[4:20])
	frame.Epoch = binary.BigEndian.Uint64(raw[20:28])
	frame.Generation = binary.BigEndian.Uint64(raw[28:36])
	frame.Sequence = binary.BigEndian.Uint32(raw[36:40])
	frame.CaptureMonotonicUS = binary.BigEndian.Uint64(raw[40:48])
	frame.TargetSnapshotDigest = hex.EncodeToString(raw[48:80])
	frame.Ciphertext = append([]byte(nil), raw[84:]...)
	if err := validateOpaqueLiveFrame(frame); err != nil {
		return OpaqueLiveFrame{}, err
	}
	return frame, nil
}

func validateOpaqueLiveFrame(frame OpaqueLiveFrame) error {
	if frame.Flags&^opaqueLiveAllowedFlags != 0 || frame.SessionID == ([16]byte{}) ||
		frame.Epoch == 0 || frame.Generation == 0 || frame.Sequence == 0 ||
		frame.CaptureMonotonicUS == 0 || len(frame.Ciphertext) < 1 ||
		len(frame.Ciphertext) > OpaqueLiveMaxCiphertextBytes ||
		len(frame.TargetSnapshotDigest) != 64 ||
		strings.Trim(frame.TargetSnapshotDigest, "0123456789abcdef") != "" {
		return fmt.Errorf("invalid opaque live frame binding")
	}
	if (frame.Sequence == 1) != (frame.Flags&OpaqueLiveFlagStart != 0) ||
		frame.Sequence > uint32(OpaqueLiveMaxDurationMS/OpaqueLiveFrameMS) {
		return fmt.Errorf("invalid opaque live sequence")
	}
	return nil
}
