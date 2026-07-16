package protocol

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

type livePTTBinaryVectors struct {
	ValidFrames []struct {
		Name               string `json:"name"`
		Hex                string `json:"hex"`
		Sequence           uint32 `json:"sequence"`
		CaptureMonotonicUS uint64 `json:"captureMonotonicUS"`
		Flags              byte   `json:"flags"`
	} `json:"validFrames"`
}

func livePTTRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func TestLivePTTBinaryVectorsAndGenerationGuard(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(livePTTRoot(t), "protocol", "live-ptt-binary-vectors-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var vectors livePTTBinaryVectors
	if err := json.Unmarshal(raw, &vectors); err != nil {
		t.Fatal(err)
	}
	if len(vectors.ValidFrames) != 3 {
		t.Fatalf("valid vector count = %d", len(vectors.ValidFrames))
	}
	session, err := ParseLivePTTSessionID("00112233445566778899aabbccddeeff")
	if err != nil {
		t.Fatal(err)
	}
	guard := NewLivePTTFrameGuard(session, 7)
	for index, vector := range vectors.ValidFrames {
		bytes, err := hex.DecodeString(vector.Hex)
		if err != nil {
			t.Fatal(err)
		}
		frame, err := DecodeLivePTTBinaryFrame(bytes)
		if err != nil {
			t.Fatalf("%s: %v", vector.Name, err)
		}
		if frame.Sequence != vector.Sequence || frame.CaptureMonotonicUS != vector.CaptureMonotonicUS || frame.Flags != vector.Flags {
			t.Fatalf("%s metadata mismatch", vector.Name)
		}
		reencoded, err := EncodeLivePTTBinaryFrame(frame)
		if err != nil || hex.EncodeToString(reencoded) != vector.Hex {
			t.Fatalf("%s round trip: %v", vector.Name, err)
		}
		if got := guard.Accept(frame); got != LivePTTFrameApply {
			t.Fatalf("%s decision = %s", vector.Name, got)
		}
		if index == 0 && guard.Accept(frame) != LivePTTFrameDuplicate {
			t.Fatal("exact duplicate was not idempotent")
		}
	}
	last, _ := DecodeLivePTTBinaryFrame(mustDecodeHex(t, vectors.ValidFrames[2].Hex))
	if guard.Accept(last) != LivePTTFrameStale {
		t.Fatal("terminal session accepted another frame")
	}

	bad := mustDecodeHex(t, vectors.ValidFrames[0].Hex)
	for _, mutation := range []func([]byte) []byte{
		func(b []byte) []byte { return b[:39] },
		func(b []byte) []byte { b[2] = 2; return b },
		func(b []byte) []byte { b[3] = 132; return b },
		func(b []byte) []byte { b[34] = 10; return b },
		func(b []byte) []byte { b[39] = 1; return b },
		func(b []byte) []byte { b[20] = 0; b[21] = 0; b[22] = 0; b[23] = 0; return b },
		func(b []byte) []byte { b[33] = 4; return b },
	} {
		copyBytes := append([]byte(nil), bad...)
		if _, err := DecodeLivePTTBinaryFrame(mutation(copyBytes)); err == nil {
			t.Fatal("invalid mutation accepted")
		}
	}
}

func mustDecodeHex(t *testing.T, value string) []byte {
	t.Helper()
	result, err := hex.DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestLivePTTPayloadValidators(t *testing.T) {
	start := LivePTTStartPayload{SessionID: "00112233445566778899aabbccddeeff", Generation: 7, SenderActorID: 11, SenderOrbitID: 22, SenderNodeID: "node-a", TargetSnapshot: "lts1.opaque", TargetSHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", TargetCount: 2, PlaybackDomain: "air", PlaybackDomainID: 33, CodecProfile: LivePTTCodecProfile, FrameMS: 20, MaxPayloadBytes: 400, JitterBufferMS: 60, StartedAtCoordMS: 1000, AcceptDeadlineCoordMS: 2500, MaxDurationMS: 300000, MixedVersionPolicy: LivePTTMixedVersionReceipts, LateJoinPolicy: LivePTTLateJoinPolicy, CaptureAuthority: LivePTTCaptureAuthority}
	if err := ValidateLivePTTStartPayload(start); err != nil {
		t.Fatal(err)
	}
	start.CaptureAuthority = "remote_request"
	if ValidateLivePTTStartPayload(start) == nil {
		t.Fatal("remote capture authority accepted")
	}
	if _, err := ParseLivePTTSessionID("00000000000000000000000000000000"); err == nil {
		t.Fatal("zero session accepted")
	}
	if err := ValidateLivePTTAcceptPayload(LivePTTAcceptPayload{SessionID: "00112233445566778899aabbccddeeff", Generation: 7, EventSequence: 1, AcceptedAtCoordMS: 1, LiveEdgeSequence: 1, BufferFrames: 3}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateLivePTTRejectPayload(LivePTTRejectPayload{SessionID: "00112233445566778899aabbccddeeff", Generation: 7, EventSequence: 1, Code: "unsupported", RejectedAtCoordMS: 1}); err != nil {
		t.Fatal(err)
	}
}
