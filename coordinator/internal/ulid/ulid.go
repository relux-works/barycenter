// Package ulid generates ULID strings (48-bit ms timestamp + 80-bit random,
// Crockford base32) for protocol message and element ids. No external deps.
package ulid

import (
	"crypto/rand"
	"time"
)

const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// New returns a 26-char ULID for the given time.
func New(t time.Time) string {
	var b [16]byte
	ms := uint64(t.UnixMilli())
	b[0] = byte(ms >> 40)
	b[1] = byte(ms >> 32)
	b[2] = byte(ms >> 24)
	b[3] = byte(ms >> 16)
	b[4] = byte(ms >> 8)
	b[5] = byte(ms)
	if _, err := rand.Read(b[6:]); err != nil {
		panic(err) // crypto/rand failure is unrecoverable
	}
	return encode(b)
}

// FromEntropy returns a ULID with caller-supplied 80-bit entropy. It exists
// for deterministic migration identifiers: the same immutable legacy row must
// map to the same public ID even after a rolled-back migration attempt.
// Runtime-created IDs must continue to use New and crypto/rand.
func FromEntropy(t time.Time, entropy [10]byte) string {
	var b [16]byte
	ms := uint64(t.UnixMilli())
	b[0] = byte(ms >> 40)
	b[1] = byte(ms >> 32)
	b[2] = byte(ms >> 24)
	b[3] = byte(ms >> 16)
	b[4] = byte(ms >> 8)
	b[5] = byte(ms)
	copy(b[6:], entropy[:])
	return encode(b)
}

func encode(b [16]byte) string {
	dst := make([]byte, 26)
	dst[0] = crockford[(b[0]&224)>>5]
	dst[1] = crockford[b[0]&31]
	dst[2] = crockford[(b[1]&248)>>3]
	dst[3] = crockford[((b[1]&7)<<2)|((b[2]&192)>>6)]
	dst[4] = crockford[(b[2]&62)>>1]
	dst[5] = crockford[((b[2]&1)<<4)|((b[3]&240)>>4)]
	dst[6] = crockford[((b[3]&15)<<1)|((b[4]&128)>>7)]
	dst[7] = crockford[(b[4]&124)>>2]
	dst[8] = crockford[((b[4]&3)<<3)|((b[5]&224)>>5)]
	dst[9] = crockford[b[5]&31]
	dst[10] = crockford[(b[6]&248)>>3]
	dst[11] = crockford[((b[6]&7)<<2)|((b[7]&192)>>6)]
	dst[12] = crockford[(b[7]&62)>>1]
	dst[13] = crockford[((b[7]&1)<<4)|((b[8]&240)>>4)]
	dst[14] = crockford[((b[8]&15)<<1)|((b[9]&128)>>7)]
	dst[15] = crockford[(b[9]&124)>>2]
	dst[16] = crockford[((b[9]&3)<<3)|((b[10]&224)>>5)]
	dst[17] = crockford[b[10]&31]
	dst[18] = crockford[(b[11]&248)>>3]
	dst[19] = crockford[((b[11]&7)<<2)|((b[12]&192)>>6)]
	dst[20] = crockford[(b[12]&62)>>1]
	dst[21] = crockford[((b[12]&1)<<4)|((b[13]&240)>>4)]
	dst[22] = crockford[((b[13]&15)<<1)|((b[14]&128)>>7)]
	dst[23] = crockford[(b[14]&124)>>2]
	dst[24] = crockford[((b[14]&3)<<3)|((b[15]&224)>>5)]
	dst[25] = crockford[b[15]&31]
	return string(dst)
}

// NewMessageID returns "msg_<ULID>".
func NewMessageID(t time.Time) string { return "msg_" + New(t) }

// NewElementID returns "el_<ULID>".
func NewElementID(t time.Time) string { return "el_" + New(t) }

// NewMediaID returns "m_<ULID>".
func NewMediaID(t time.Time) string { return "m_" + New(t) }

// NewTransmissionID returns "tr_<ULID>".
func NewTransmissionID(t time.Time) string { return "tr_" + New(t) }

// NewInboxID returns "ib_<ULID>".
func NewInboxID(t time.Time) string { return "ib_" + New(t) }

// NewUploadSessionID returns "up_<ULID>".
func NewUploadSessionID(t time.Time) string { return "up_" + New(t) }

// NewStorageOperationID returns "sop_<ULID>".
func NewStorageOperationID(t time.Time) string { return "sop_" + New(t) }

// NewModerationReportID returns "rp_<ULID>".
func NewModerationReportID(t time.Time) string { return "rp_" + New(t) }

// NewModerationOperatorID returns "op_<ULID>".
func NewModerationOperatorID(t time.Time) string { return "op_" + New(t) }

// NewModerationDecisionID returns "md_<ULID>".
func NewModerationDecisionID(t time.Time) string { return "md_" + New(t) }

// NewCTID returns "ct_<ULID>" — a canonical track id (spec-providers §2).
func NewCTID(t time.Time) string { return "ct_" + New(t) }
