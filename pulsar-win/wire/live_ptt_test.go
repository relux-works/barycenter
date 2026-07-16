package protocol

import (
	"encoding/hex"
	"testing"
)

func TestLivePTTWindowsMirrorRejectsMalformedAndStaleFrames(t *testing.T) {
	raw, err := hex.DecodeString("4250010500112233445566778899aabbccddeeff0000000100000000000f42400003140101010000f8fffe")
	if err != nil {
		t.Fatal(err)
	}
	frame, err := DecodeLivePTTBinaryFrame(raw)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := EncodeLivePTTBinaryFrame(frame)
	if err != nil || hex.EncodeToString(encoded) != hex.EncodeToString(raw) {
		t.Fatalf("round trip: %v", err)
	}
	guard := NewLivePTTFrameGuard(frame.SessionID, 7)
	if guard.Accept(frame) != LivePTTFrameApply || guard.Accept(frame) != LivePTTFrameDuplicate {
		t.Fatal("generation guard decisions differ")
	}
	bad := append([]byte(nil), raw...)
	bad[34] = 10
	if _, err := DecodeLivePTTBinaryFrame(bad); err == nil {
		t.Fatal("wrong codec profile accepted")
	}
}
