package automation

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"math"
)

const (
	builtinCueSampleRate = 48_000
	builtinCueFrames     = 7_680
	builtinCueChannels   = 1
	builtinCueBits       = 16
	BuiltinCueSHA256     = "479b1a9d605ac12454e3449e129991b7ce8599251506ca54a93be0b6144730fd"
)

// BuiltinRecordingCueWAV regenerates the reviewed system cue from source. It
// intentionally contains no sample or capture dependency, so coordinator-side
// automation can publish the same exact bytes used by both packaged clients.
func BuiltinRecordingCueWAV() ([]byte, error) {
	payload := make([]byte, 44+builtinCueFrames*builtinCueChannels*(builtinCueBits/8))
	copy(payload[0:4], "RIFF")
	binary.LittleEndian.PutUint32(payload[4:8], uint32(len(payload)-8))
	copy(payload[8:12], "WAVE")
	copy(payload[12:16], "fmt ")
	binary.LittleEndian.PutUint32(payload[16:20], 16)
	binary.LittleEndian.PutUint16(payload[20:22], 1)
	binary.LittleEndian.PutUint16(payload[22:24], builtinCueChannels)
	binary.LittleEndian.PutUint32(payload[24:28], builtinCueSampleRate)
	binary.LittleEndian.PutUint32(payload[28:32], builtinCueSampleRate*builtinCueChannels*(builtinCueBits/8))
	binary.LittleEndian.PutUint16(payload[32:34], builtinCueChannels*(builtinCueBits/8))
	binary.LittleEndian.PutUint16(payload[34:36], builtinCueBits)
	copy(payload[36:40], "data")
	binary.LittleEndian.PutUint32(payload[40:44], uint32(len(payload)-44))
	for frame := 0; frame < builtinCueFrames; frame++ {
		t := float64(frame) / builtinCueSampleRate
		envelope := 1.0
		switch {
		case frame < 480:
			x := float64(frame) / 480
			envelope = 0.5 - 0.5*math.Cos(math.Pi*x)
		case frame >= builtinCueFrames-960:
			x := float64(builtinCueFrames-1-frame) / 960
			envelope = 0.5 - 0.5*math.Cos(math.Pi*math.Max(0, x))
		}
		wave := 0.18*math.Sin(2*math.Pi*880*t) +
			0.07*math.Sin(2*math.Pi*1320*t+math.Pi/7)
		sample := int16(math.Round(32767 * envelope * wave))
		binary.LittleEndian.PutUint16(payload[44+frame*2:], uint16(sample))
	}
	digest := sha256.Sum256(payload)
	if hex.EncodeToString(digest[:]) != BuiltinCueSHA256 {
		return nil, errors.New("builtin recording cue digest drift")
	}
	return payload, nil
}
