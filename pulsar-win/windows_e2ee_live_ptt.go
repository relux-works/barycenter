package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"runtime"
	"sync"

	protocol "relux.works/duet/pulsar-win/wire"
)

const (
	windowsE2EELiveOpaqueHeaderBytes             = 84
	windowsE2EELiveMaximumCiphertextBytes        = 512
	windowsE2EELiveMaximumMessageBytes           = windowsE2EELiveOpaqueHeaderBytes + windowsE2EELiveMaximumCiphertextBytes
	windowsE2EELiveMaximumNonceTokenBytes        = 256
	windowsE2EELiveMaximumSequence        uint32 = protocol.LivePTTMaxDurationMS / protocol.LivePTTFrameMS
)

var (
	ErrWindowsE2EELiveInvalidContext          = errors.New("windows_e2ee_live_invalid_context")
	ErrWindowsE2EELiveInvalidFrame            = errors.New("windows_e2ee_live_invalid_frame")
	ErrWindowsE2EELiveProviderNotApproved     = errors.New("windows_e2ee_live_provider_not_approved")
	ErrWindowsE2EELiveProviderFailure         = errors.New("windows_e2ee_live_provider_failure")
	ErrWindowsE2EELiveMalformedProviderOutput = errors.New("windows_e2ee_live_malformed_provider_output")
	ErrWindowsE2EELiveAuthenticationFailed    = errors.New("windows_e2ee_live_authentication_failed")
	ErrWindowsE2EELiveReplay                  = errors.New("windows_e2ee_live_replay")
	ErrWindowsE2EELiveNonceReuse              = errors.New("windows_e2ee_live_nonce_reuse")
	ErrWindowsE2EELiveStaleEpoch              = errors.New("windows_e2ee_live_stale_epoch")
	ErrWindowsE2EELiveSenderRemoved           = errors.New("windows_e2ee_live_sender_removed")
	ErrWindowsE2EELiveMembershipChanged       = errors.New("windows_e2ee_live_membership_changed")
	ErrWindowsE2EELiveTerminal                = errors.New("windows_e2ee_live_terminal")
)

// WindowsE2EEOpaqueLiveFrame mirrors the accepted coordinator BE envelope.
// It is kept outside wire/ because that package is the byte-frozen plaintext
// protocol mirror and must not advertise a protected runtime capability.
type WindowsE2EEOpaqueLiveFrame struct {
	Flags                byte
	SessionID            [16]byte
	Epoch                uint64
	Generation           uint64
	Sequence             uint32
	CaptureMonotonicUS   uint64
	TargetSnapshotDigest string
	Ciphertext           []byte
}

func (f WindowsE2EEOpaqueLiveFrame) Encode() ([]byte, error) {
	if err := validateWindowsE2EEOpaqueLiveFrame(f); err != nil {
		return nil, err
	}
	target, _ := hex.DecodeString(f.TargetSnapshotDigest)
	result := make([]byte, windowsE2EELiveOpaqueHeaderBytes+len(f.Ciphertext))
	copy(result[0:2], []byte("BE"))
	result[2], result[3] = 1, f.Flags
	copy(result[4:20], f.SessionID[:])
	binary.BigEndian.PutUint64(result[20:28], f.Epoch)
	binary.BigEndian.PutUint64(result[28:36], f.Generation)
	binary.BigEndian.PutUint32(result[36:40], f.Sequence)
	binary.BigEndian.PutUint64(result[40:48], f.CaptureMonotonicUS)
	copy(result[48:80], target)
	binary.BigEndian.PutUint16(result[80:82], uint16(len(f.Ciphertext)))
	binary.BigEndian.PutUint16(result[82:84], 0)
	copy(result[84:], f.Ciphertext)
	return result, nil
}

func DecodeWindowsE2EEOpaqueLiveFrame(raw []byte) (WindowsE2EEOpaqueLiveFrame, error) {
	if len(raw) < windowsE2EELiveOpaqueHeaderBytes || len(raw) > windowsE2EELiveMaximumMessageBytes ||
		string(raw[0:2]) != "BE" || raw[2] != 1 || binary.BigEndian.Uint16(raw[82:84]) != 0 {
		return WindowsE2EEOpaqueLiveFrame{}, ErrWindowsE2EELiveInvalidFrame
	}
	size := int(binary.BigEndian.Uint16(raw[80:82]))
	if size < 1 || size > windowsE2EELiveMaximumCiphertextBytes || len(raw) != windowsE2EELiveOpaqueHeaderBytes+size {
		return WindowsE2EEOpaqueLiveFrame{}, ErrWindowsE2EELiveInvalidFrame
	}
	var frame WindowsE2EEOpaqueLiveFrame
	frame.Flags = raw[3]
	copy(frame.SessionID[:], raw[4:20])
	frame.Epoch = binary.BigEndian.Uint64(raw[20:28])
	frame.Generation = binary.BigEndian.Uint64(raw[28:36])
	frame.Sequence = binary.BigEndian.Uint32(raw[36:40])
	frame.CaptureMonotonicUS = binary.BigEndian.Uint64(raw[40:48])
	frame.TargetSnapshotDigest = hex.EncodeToString(raw[48:80])
	frame.Ciphertext = append([]byte(nil), raw[84:]...)
	if err := validateWindowsE2EEOpaqueLiveFrame(frame); err != nil {
		return WindowsE2EEOpaqueLiveFrame{}, err
	}
	return frame, nil
}

func validateWindowsE2EEOpaqueLiveFrame(frame WindowsE2EEOpaqueLiveFrame) error {
	if frame.Flags&^(protocol.LivePTTFlagStart|protocol.LivePTTFlagEnd) != 0 ||
		frame.SessionID == ([16]byte{}) || frame.Epoch == 0 || frame.Generation == 0 ||
		frame.Sequence == 0 || frame.Sequence > windowsE2EELiveMaximumSequence ||
		frame.CaptureMonotonicUS == 0 || len(frame.Ciphertext) == 0 ||
		len(frame.Ciphertext) > windowsE2EELiveMaximumCiphertextBytes ||
		!validWindowsE2EEDigest(frame.TargetSnapshotDigest) ||
		(frame.Sequence == 1) != (frame.Flags&protocol.LivePTTFlagStart != 0) {
		return ErrWindowsE2EELiveInvalidFrame
	}
	return nil
}

type WindowsE2EELiveSessionContext struct {
	GroupID               string
	AuthorDeviceID        string
	Epoch                 uint64
	CommitDigest          string
	SessionID             [16]byte
	Generation            uint64
	SenderActorID         int64
	SenderOrbitID         int64
	SenderNodeID          string
	TargetSnapshotDigest  string
	PlaybackDomain        string
	PlaybackDomainID      int64
	CodecProfile          string
	FrameMS               int
	MaximumPlaintextBytes int
	JitterBufferMS        int
	MaximumDurationMS     int64
}

func NewWindowsE2EELiveSessionContext(groupID, authorDeviceID string, epoch uint64, commitDigest string, start protocol.LivePTTStartPayload) (WindowsE2EELiveSessionContext, error) {
	sessionID, err := protocol.ParseLivePTTSessionID(start.SessionID)
	if err != nil || protocol.ValidateLivePTTStartPayload(start) != nil ||
		!validWindowsE2EEIdentifier(groupID, 8, 128) ||
		!validWindowsE2EEIdentifier(authorDeviceID, 8, 128) || epoch == 0 ||
		!validWindowsE2EEDigest(commitDigest) || start.Generation <= 0 {
		return WindowsE2EELiveSessionContext{}, ErrWindowsE2EELiveInvalidContext
	}
	return WindowsE2EELiveSessionContext{
		GroupID: groupID, AuthorDeviceID: authorDeviceID, Epoch: epoch,
		CommitDigest: commitDigest, SessionID: sessionID, Generation: uint64(start.Generation),
		SenderActorID: start.SenderActorID, SenderOrbitID: start.SenderOrbitID,
		SenderNodeID: start.SenderNodeID, TargetSnapshotDigest: start.TargetSHA256,
		PlaybackDomain: start.PlaybackDomain, PlaybackDomainID: start.PlaybackDomainID,
		CodecProfile: start.CodecProfile, FrameMS: start.FrameMS,
		MaximumPlaintextBytes: start.MaxPayloadBytes, JitterBufferMS: start.JitterBufferMS,
		MaximumDurationMS: start.MaxDurationMS,
	}, nil
}

type WindowsE2EELiveAuthorizationSnapshot struct {
	GroupID                   string
	Epoch                     uint64
	CommitDigest              string
	TargetSnapshotDigest      string
	AuthorizedSenderDeviceIDs map[string]struct{}
}

type WindowsE2EELiveAuthorizationChecker interface {
	CurrentAuthorization() WindowsE2EELiveAuthorizationSnapshot
}

type WindowsE2EELiveSealedPayload struct {
	NonceToken     []byte
	WireCiphertext []byte
}

type WindowsE2EELiveOpenedPayload struct {
	NonceToken []byte
	Plaintext  []byte
}

type WindowsE2EELiveCryptographicSession interface {
	ProductionApproved() bool
	Seal(plaintext []byte, sequence uint32, authenticatedData []byte) (WindowsE2EELiveSealedPayload, error)
	Open(wireCiphertext []byte, sequence uint32, authenticatedData []byte) (WindowsE2EELiveOpenedPayload, error)
	Destroy()
}

type WindowsE2EELiveSessionDeriver interface {
	ProductionApproved() bool
	Derive(context WindowsE2EELiveSessionContext, identity *WindowsE2EEDeviceIdentityLease, groupState *WindowsE2EEGroupStateLease) (WindowsE2EELiveCryptographicSession, error)
}

type WindowsE2EELiveOutgoingRequest struct {
	GroupID                      string
	AuthorDeviceID               string
	ExpectedGroupRevision        uint64
	ExpectedTargetSnapshotDigest string
	NowMS                        int64
}

type WindowsE2EELiveIncomingRequest struct {
	GroupID                    string
	LocalDeviceID              string
	AuthorDeviceID             string
	Epoch                      uint64
	ExpectedLocalGroupRevision uint64
	Start                      protocol.LivePTTStartPayload
}

type WindowsE2EELiveOutgoingPreparation struct {
	Start       protocol.LivePTTStartPayload
	Reservation WindowsE2EESendReservation
	Channel     *WindowsE2EELiveFrameChannel
}

// WindowsE2EELiveSessionFactory is the production-dark Windows E2EE live-PTT
// boundary. It cannot select a provider, suite, runtime or capability.
type WindowsE2EELiveSessionFactory struct {
	keyState      *WindowsE2EEKeyStateRepository
	derivation    WindowsE2EELiveSessionDeriver
	authorization WindowsE2EELiveAuthorizationChecker
	fixtureMode   bool
}

func NewWindowsE2EELiveSessionFactory(keyState *WindowsE2EEKeyStateRepository, derivation WindowsE2EELiveSessionDeriver, authorization WindowsE2EELiveAuthorizationChecker) (*WindowsE2EELiveSessionFactory, error) {
	if keyState == nil || derivation == nil || authorization == nil || !derivation.ProductionApproved() {
		return nil, ErrWindowsE2EELiveProviderNotApproved
	}
	return &WindowsE2EELiveSessionFactory{keyState: keyState, derivation: derivation, authorization: authorization}, nil
}

func newWindowsE2EELiveSessionFactoryForAudit(keyState *WindowsE2EEKeyStateRepository, derivation WindowsE2EELiveSessionDeriver, authorization WindowsE2EELiveAuthorizationChecker) (*WindowsE2EELiveSessionFactory, error) {
	if keyState == nil || derivation == nil || authorization == nil {
		return nil, ErrWindowsE2EELiveInvalidContext
	}
	return &WindowsE2EELiveSessionFactory{keyState: keyState, derivation: derivation, authorization: authorization, fixtureMode: true}, nil
}

func (f *WindowsE2EELiveSessionFactory) PrepareOutgoing(request WindowsE2EELiveOutgoingRequest, makeStart func(WindowsE2EESendReservation) (protocol.LivePTTStartPayload, error)) (WindowsE2EELiveOutgoingPreparation, error) {
	if f == nil || makeStart == nil || request.NowMS <= 0 {
		return WindowsE2EELiveOutgoingPreparation{}, ErrWindowsE2EELiveInvalidContext
	}
	identity, err := f.keyState.LoadDeviceIdentity(request.AuthorDeviceID)
	if err != nil {
		return WindowsE2EELiveOutgoingPreparation{}, ErrWindowsE2EELiveStaleEpoch
	}
	defer identity.Destroy()
	initial, err := f.keyState.LoadGroupState(identity.Metadata.InstallationID, request.GroupID)
	if err != nil {
		return WindowsE2EELiveOutgoingPreparation{}, ErrWindowsE2EELiveStaleEpoch
	}
	defer initial.Destroy()
	if initial.Metadata.Revision != request.ExpectedGroupRevision ||
		initial.Metadata.TargetSnapshotDigest != request.ExpectedTargetSnapshotDigest {
		return WindowsE2EELiveOutgoingPreparation{}, ErrWindowsE2EELiveMembershipChanged
	}
	reservation, err := f.keyState.ReserveSendGeneration(identity.Metadata.InstallationID, request.GroupID, "live_ptt", initial.Metadata.Revision, request.NowMS)
	if err != nil {
		return WindowsE2EELiveOutgoingPreparation{}, ErrWindowsE2EELiveStaleEpoch
	}
	current, err := f.keyState.LoadGroupState(identity.Metadata.InstallationID, request.GroupID)
	if err != nil {
		return WindowsE2EELiveOutgoingPreparation{}, ErrWindowsE2EELiveStaleEpoch
	}
	defer current.Destroy()
	if current.Metadata.Revision != reservation.Revision || current.Metadata.Epoch != reservation.Epoch ||
		current.Metadata.TargetSnapshotDigest != request.ExpectedTargetSnapshotDigest ||
		current.Metadata.CommitDigest != initial.Metadata.CommitDigest {
		return WindowsE2EELiveOutgoingPreparation{}, ErrWindowsE2EELiveMembershipChanged
	}
	start, err := makeStart(reservation)
	if err != nil || start.Generation <= 0 || uint64(start.Generation) != reservation.Generation ||
		start.TargetSHA256 != request.ExpectedTargetSnapshotDigest {
		return WindowsE2EELiveOutgoingPreparation{}, ErrWindowsE2EELiveInvalidContext
	}
	context, err := NewWindowsE2EELiveSessionContext(request.GroupID, request.AuthorDeviceID, reservation.Epoch, current.Metadata.CommitDigest, start)
	if err != nil {
		return WindowsE2EELiveOutgoingPreparation{}, err
	}
	crypto, err := f.derivation.Derive(context, identity, current)
	if err != nil || crypto == nil {
		return WindowsE2EELiveOutgoingPreparation{}, ErrWindowsE2EELiveProviderFailure
	}
	channel, err := f.newChannel(context, crypto)
	if err != nil {
		return WindowsE2EELiveOutgoingPreparation{}, err
	}
	return WindowsE2EELiveOutgoingPreparation{Start: start, Reservation: reservation, Channel: channel}, nil
}

func (f *WindowsE2EELiveSessionFactory) PrepareIncoming(request WindowsE2EELiveIncomingRequest) (*WindowsE2EELiveFrameChannel, error) {
	if f == nil {
		return nil, ErrWindowsE2EELiveInvalidContext
	}
	identity, err := f.keyState.LoadDeviceIdentity(request.LocalDeviceID)
	if err != nil {
		return nil, ErrWindowsE2EELiveStaleEpoch
	}
	defer identity.Destroy()
	group, err := f.keyState.LoadGroupState(identity.Metadata.InstallationID, request.GroupID)
	if err != nil {
		return nil, ErrWindowsE2EELiveStaleEpoch
	}
	defer group.Destroy()
	if group.Metadata.Epoch != request.Epoch || group.Metadata.Revision != request.ExpectedLocalGroupRevision ||
		group.Metadata.TargetSnapshotDigest != request.Start.TargetSHA256 {
		return nil, ErrWindowsE2EELiveStaleEpoch
	}
	context, err := NewWindowsE2EELiveSessionContext(request.GroupID, request.AuthorDeviceID, request.Epoch, group.Metadata.CommitDigest, request.Start)
	if err != nil {
		return nil, err
	}
	crypto, err := f.derivation.Derive(context, identity, group)
	if err != nil || crypto == nil {
		return nil, ErrWindowsE2EELiveProviderFailure
	}
	return f.newChannel(context, crypto)
}

func (f *WindowsE2EELiveSessionFactory) newChannel(context WindowsE2EELiveSessionContext, crypto WindowsE2EELiveCryptographicSession) (*WindowsE2EELiveFrameChannel, error) {
	if f.fixtureMode {
		return newWindowsE2EELiveFrameChannelForAudit(context, crypto, f.authorization)
	}
	return NewWindowsE2EELiveFrameChannel(context, crypto, f.authorization)
}

type WindowsE2EELiveFrameChannel struct {
	mu                    sync.Mutex
	context               WindowsE2EELiveSessionContext
	crypto                WindowsE2EELiveCryptographicSession
	authorization         WindowsE2EELiveAuthorizationChecker
	terminal              bool
	cryptoDestroyed       bool
	outgoingSequence      uint32
	outgoingCaptureBaseUS uint64
	outgoingNonces        map[string]struct{}
	lastPlaintextFrame    *protocol.LivePTTBinaryFrame
	lastOpaqueFrame       *WindowsE2EEOpaqueLiveFrame
	incomingHighest       uint32
	incomingCaptureBaseUS uint64
	incomingSequences     map[uint32]struct{}
	incomingNonces        map[string]struct{}
}

func NewWindowsE2EELiveFrameChannel(context WindowsE2EELiveSessionContext, crypto WindowsE2EELiveCryptographicSession, authorization WindowsE2EELiveAuthorizationChecker) (*WindowsE2EELiveFrameChannel, error) {
	if crypto == nil || authorization == nil || !crypto.ProductionApproved() {
		if crypto != nil {
			crypto.Destroy()
		}
		return nil, ErrWindowsE2EELiveProviderNotApproved
	}
	return newWindowsE2EELiveFrameChannel(context, crypto, authorization)
}

func newWindowsE2EELiveFrameChannelForAudit(context WindowsE2EELiveSessionContext, crypto WindowsE2EELiveCryptographicSession, authorization WindowsE2EELiveAuthorizationChecker) (*WindowsE2EELiveFrameChannel, error) {
	if crypto == nil || authorization == nil {
		return nil, ErrWindowsE2EELiveInvalidContext
	}
	return newWindowsE2EELiveFrameChannel(context, crypto, authorization)
}

func newWindowsE2EELiveFrameChannel(context WindowsE2EELiveSessionContext, crypto WindowsE2EELiveCryptographicSession, authorization WindowsE2EELiveAuthorizationChecker) (*WindowsE2EELiveFrameChannel, error) {
	channel := &WindowsE2EELiveFrameChannel{
		context: context, crypto: crypto, authorization: authorization,
		outgoingNonces: make(map[string]struct{}), incomingSequences: make(map[uint32]struct{}),
		incomingNonces: make(map[string]struct{}),
	}
	if err := validateWindowsE2EELiveAuthorization(authorization.CurrentAuthorization(), context); err != nil {
		crypto.Destroy()
		channel.cryptoDestroyed = true
		return nil, err
	}
	runtime.SetFinalizer(channel, func(value *WindowsE2EELiveFrameChannel) { value.Terminate() })
	return channel, nil
}

func (c *WindowsE2EELiveFrameChannel) Protect(frame protocol.LivePTTBinaryFrame) (result WindowsE2EEOpaqueLiveFrame, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.terminal {
		return result, ErrWindowsE2EELiveTerminal
	}
	defer func() {
		if err != nil && !errors.Is(err, ErrWindowsE2EELiveInvalidFrame) {
			c.terminateLocked()
		}
	}()
	if err = validateWindowsE2EELiveAuthorization(c.authorization.CurrentAuthorization(), c.context); err != nil {
		return result, err
	}
	if c.lastPlaintextFrame != nil && windowsLiveFramesEqual(*c.lastPlaintextFrame, frame) && c.lastOpaqueFrame != nil {
		return cloneWindowsE2EEOpaqueLiveFrame(*c.lastOpaqueFrame), nil
	}
	if frame.SessionID != c.context.SessionID || frame.Sequence != c.outgoingSequence+1 ||
		frame.Sequence > windowsE2EELiveMaximumSequence || len(frame.Payload) > c.context.MaximumPlaintextBytes {
		return result, ErrWindowsE2EELiveInvalidFrame
	}
	if _, encodeErr := protocol.EncodeLivePTTBinaryFrame(frame); encodeErr != nil {
		return result, ErrWindowsE2EELiveInvalidFrame
	}
	if frame.Sequence == 1 {
		c.outgoingCaptureBaseUS = frame.CaptureMonotonicUS
	}
	if c.outgoingCaptureBaseUS == 0 || frame.CaptureMonotonicUS != c.outgoingCaptureBaseUS+uint64(frame.Sequence-1)*uint64(c.context.FrameMS*1000) {
		return result, ErrWindowsE2EELiveInvalidFrame
	}
	flags := frame.Flags & (protocol.LivePTTFlagStart | protocol.LivePTTFlagEnd)
	aad, err := windowsE2EELiveAuthenticatedData(c.context, flags, frame.Sequence, frame.CaptureMonotonicUS)
	if err != nil {
		return result, err
	}
	plaintext := append([]byte(nil), frame.Payload...)
	sealed, sealErr := c.crypto.Seal(plaintext, frame.Sequence, append([]byte(nil), aad...))
	sealed.NonceToken = append([]byte(nil), sealed.NonceToken...)
	sealed.WireCiphertext = append([]byte(nil), sealed.WireCiphertext...)
	zeroBytes(plaintext)
	if sealErr != nil {
		return result, ErrWindowsE2EELiveProviderFailure
	}
	if len(sealed.NonceToken) == 0 || len(sealed.NonceToken) > windowsE2EELiveMaximumNonceTokenBytes ||
		len(sealed.WireCiphertext) == 0 || len(sealed.WireCiphertext) > windowsE2EELiveMaximumCiphertextBytes {
		return result, ErrWindowsE2EELiveMalformedProviderOutput
	}
	nonce := string(append([]byte(nil), sealed.NonceToken...))
	if _, exists := c.outgoingNonces[nonce]; exists {
		return result, ErrWindowsE2EELiveNonceReuse
	}
	opaque := WindowsE2EEOpaqueLiveFrame{
		Flags: flags, SessionID: frame.SessionID, Epoch: c.context.Epoch,
		Generation: c.context.Generation, Sequence: frame.Sequence,
		CaptureMonotonicUS:   frame.CaptureMonotonicUS,
		TargetSnapshotDigest: c.context.TargetSnapshotDigest,
		Ciphertext:           append([]byte(nil), sealed.WireCiphertext...),
	}
	if _, encodeErr := opaque.Encode(); encodeErr != nil {
		return result, ErrWindowsE2EELiveMalformedProviderOutput
	}
	c.outgoingNonces[nonce] = struct{}{}
	c.outgoingSequence = frame.Sequence
	plainCopy := cloneWindowsLiveFrame(frame)
	opaqueCopy := cloneWindowsE2EEOpaqueLiveFrame(opaque)
	c.lastPlaintextFrame, c.lastOpaqueFrame = &plainCopy, &opaqueCopy
	return cloneWindowsE2EEOpaqueLiveFrame(opaque), nil
}

func (c *WindowsE2EELiveFrameChannel) Open(opaque WindowsE2EEOpaqueLiveFrame) (result protocol.LivePTTBinaryFrame, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.terminal {
		return result, ErrWindowsE2EELiveTerminal
	}
	defer func() {
		if err != nil {
			c.terminateLocked()
		}
	}()
	if err = validateWindowsE2EELiveAuthorization(c.authorization.CurrentAuthorization(), c.context); err != nil {
		return result, err
	}
	if opaque.SessionID != c.context.SessionID || opaque.Generation != c.context.Generation ||
		opaque.TargetSnapshotDigest != c.context.TargetSnapshotDigest {
		return result, ErrWindowsE2EELiveInvalidFrame
	}
	if opaque.Epoch != c.context.Epoch {
		return result, ErrWindowsE2EELiveStaleEpoch
	}
	if _, encodeErr := opaque.Encode(); encodeErr != nil {
		return result, ErrWindowsE2EELiveInvalidFrame
	}
	if _, exists := c.incomingSequences[opaque.Sequence]; exists {
		return result, ErrWindowsE2EELiveReplay
	}
	if c.incomingHighest == 0 {
		if opaque.Sequence != 1 {
			return result, ErrWindowsE2EELiveInvalidFrame
		}
	} else if uint64(opaque.Sequence)+uint64(protocol.LivePTTMaxGapFrames) < uint64(c.incomingHighest) ||
		uint64(opaque.Sequence) > uint64(c.incomingHighest)+uint64(protocol.LivePTTMaxGapFrames) {
		return result, ErrWindowsE2EELiveReplay
	}
	if opaque.Sequence == 1 {
		c.incomingCaptureBaseUS = opaque.CaptureMonotonicUS
	}
	if c.incomingCaptureBaseUS == 0 || opaque.CaptureMonotonicUS != c.incomingCaptureBaseUS+uint64(opaque.Sequence-1)*uint64(c.context.FrameMS*1000) {
		return result, ErrWindowsE2EELiveInvalidFrame
	}
	aad, err := windowsE2EELiveAuthenticatedData(c.context, opaque.Flags, opaque.Sequence, opaque.CaptureMonotonicUS)
	if err != nil {
		return result, err
	}
	opened, openErr := c.crypto.Open(append([]byte(nil), opaque.Ciphertext...), opaque.Sequence, append([]byte(nil), aad...))
	if openErr != nil {
		return result, ErrWindowsE2EELiveAuthenticationFailed
	}
	if len(opened.NonceToken) == 0 || len(opened.NonceToken) > windowsE2EELiveMaximumNonceTokenBytes ||
		len(opened.Plaintext) == 0 || len(opened.Plaintext) > c.context.MaximumPlaintextBytes {
		zeroBytes(opened.Plaintext)
		return result, ErrWindowsE2EELiveMalformedProviderOutput
	}
	nonce := string(append([]byte(nil), opened.NonceToken...))
	if _, exists := c.incomingNonces[nonce]; exists {
		zeroBytes(opened.Plaintext)
		return result, ErrWindowsE2EELiveNonceReuse
	}
	flags := byte(protocol.LivePTTFlagFEC)
	if opaque.Flags&protocol.LivePTTFlagStart != 0 {
		flags |= protocol.LivePTTFlagStart
	}
	if opaque.Flags&protocol.LivePTTFlagEnd != 0 {
		flags |= protocol.LivePTTFlagEnd
	}
	result = protocol.LivePTTBinaryFrame{
		Flags: flags, SessionID: opaque.SessionID, Sequence: opaque.Sequence,
		CaptureMonotonicUS: opaque.CaptureMonotonicUS, Payload: append([]byte(nil), opened.Plaintext...),
	}
	zeroBytes(opened.Plaintext)
	if _, encodeErr := protocol.EncodeLivePTTBinaryFrame(result); encodeErr != nil {
		zeroBytes(result.Payload)
		return protocol.LivePTTBinaryFrame{}, ErrWindowsE2EELiveMalformedProviderOutput
	}
	c.incomingNonces[nonce] = struct{}{}
	c.incomingSequences[opaque.Sequence] = struct{}{}
	if opaque.Sequence > c.incomingHighest {
		c.incomingHighest = opaque.Sequence
	}
	return result, nil
}

func (c *WindowsE2EELiveFrameChannel) Terminate() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.terminateLocked()
	c.mu.Unlock()
	runtime.SetFinalizer(c, nil)
}

func (c *WindowsE2EELiveFrameChannel) Close() error {
	c.Terminate()
	return nil
}

func (c *WindowsE2EELiveFrameChannel) IsTerminal() bool {
	if c == nil {
		return true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.terminal
}

func (c *WindowsE2EELiveFrameChannel) terminateLocked() {
	if c.terminal {
		return
	}
	c.terminal = true
	if !c.cryptoDestroyed {
		c.cryptoDestroyed = true
		c.crypto.Destroy()
	}
	for nonce := range c.outgoingNonces {
		delete(c.outgoingNonces, nonce)
	}
	for nonce := range c.incomingNonces {
		delete(c.incomingNonces, nonce)
	}
	clear(c.incomingSequences)
	if c.lastPlaintextFrame != nil {
		zeroBytes(c.lastPlaintextFrame.Payload)
	}
	c.lastPlaintextFrame, c.lastOpaqueFrame = nil, nil
}

func validateWindowsE2EELiveAuthorization(current WindowsE2EELiveAuthorizationSnapshot, context WindowsE2EELiveSessionContext) error {
	if current.GroupID != context.GroupID {
		return ErrWindowsE2EELiveMembershipChanged
	}
	if current.Epoch != context.Epoch {
		return ErrWindowsE2EELiveStaleEpoch
	}
	if current.CommitDigest != context.CommitDigest || current.TargetSnapshotDigest != context.TargetSnapshotDigest {
		return ErrWindowsE2EELiveMembershipChanged
	}
	if _, ok := current.AuthorizedSenderDeviceIDs[context.AuthorDeviceID]; !ok {
		return ErrWindowsE2EELiveSenderRemoved
	}
	return nil
}

func windowsE2EELiveAuthenticatedData(context WindowsE2EELiveSessionContext, flags byte, sequence uint32, captureMonotonicUS uint64) ([]byte, error) {
	if context.SessionID == ([16]byte{}) || sequence == 0 || captureMonotonicUS == 0 {
		return nil, ErrWindowsE2EELiveInvalidContext
	}
	data := []byte("barycenter.e2ee.live.aad.v1")
	appendString := func(value string) {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(value)))
		data = append(data, length[:]...)
		data = append(data, value...)
	}
	appendUint64 := func(value uint64) {
		var encoded [8]byte
		binary.BigEndian.PutUint64(encoded[:], value)
		data = append(data, encoded[:]...)
	}
	appendString(context.GroupID)
	appendString(context.AuthorDeviceID)
	appendString(context.SenderNodeID)
	appendString(context.TargetSnapshotDigest)
	appendString(context.PlaybackDomain)
	appendString(context.CodecProfile)
	data = append(data, context.SessionID[:]...)
	appendString(context.CommitDigest)
	appendUint64(context.Epoch)
	appendUint64(context.Generation)
	appendUint64(uint64(context.SenderActorID))
	appendUint64(uint64(context.SenderOrbitID))
	appendUint64(uint64(context.PlaybackDomainID))
	appendUint64(uint64(context.FrameMS))
	appendUint64(uint64(context.MaximumPlaintextBytes))
	appendUint64(uint64(context.JitterBufferMS))
	appendUint64(uint64(context.MaximumDurationMS))
	appendUint64(uint64(flags))
	appendUint64(uint64(sequence))
	appendUint64(captureMonotonicUS)
	return data, nil
}

type WindowsE2EELiveSenderBridge struct {
	channel       *WindowsE2EELiveFrameChannel
	trySendOpaque func([]byte) bool
}

func NewWindowsE2EELiveSenderBridge(channel *WindowsE2EELiveFrameChannel, trySendOpaque func([]byte) bool) *WindowsE2EELiveSenderBridge {
	if channel == nil || trySendOpaque == nil {
		return nil
	}
	return &WindowsE2EELiveSenderBridge{channel: channel, trySendOpaque: trySendOpaque}
}

func (b *WindowsE2EELiveSenderBridge) TrySend(frame protocol.LivePTTBinaryFrame) bool {
	if b == nil {
		return false
	}
	opaque, err := b.channel.Protect(frame)
	if err != nil {
		return false
	}
	wire, err := opaque.Encode()
	return err == nil && b.trySendOpaque(wire)
}

func (b *WindowsE2EELiveSenderBridge) Terminate() {
	if b != nil {
		b.channel.Terminate()
	}
}

type WindowsE2EELiveReceiverBridge struct {
	channel  *WindowsE2EELiveFrameChannel
	receiver windowsLiveJitterReceiving
}

func NewWindowsE2EELiveReceiverBridge(channel *WindowsE2EELiveFrameChannel, receiver windowsLiveJitterReceiving) *WindowsE2EELiveReceiverBridge {
	if channel == nil || receiver == nil {
		return nil
	}
	return &WindowsE2EELiveReceiverBridge{channel: channel, receiver: receiver}
}

func (b *WindowsE2EELiveReceiverBridge) ReceiveOpaque(raw []byte) protocol.LivePTTFrameDecision {
	if b == nil {
		return protocol.LivePTTFrameInvalid
	}
	opaque, err := DecodeWindowsE2EEOpaqueLiveFrame(raw)
	if err == nil {
		var frame protocol.LivePTTBinaryFrame
		frame, err = b.channel.Open(opaque)
		if err == nil {
			return b.receiver.Receive(frame)
		}
	}
	b.receiver.Revoke()
	switch {
	case errors.Is(err, ErrWindowsE2EELiveReplay):
		return protocol.LivePTTFrameDuplicate
	case errors.Is(err, ErrWindowsE2EELiveStaleEpoch), errors.Is(err, ErrWindowsE2EELiveMembershipChanged), errors.Is(err, ErrWindowsE2EELiveSenderRemoved):
		return protocol.LivePTTFrameStale
	default:
		return protocol.LivePTTFrameInvalid
	}
}

func (b *WindowsE2EELiveReceiverBridge) Terminate() {
	if b == nil {
		return
	}
	b.channel.Terminate()
	b.receiver.Revoke()
}

func cloneWindowsLiveFrame(frame protocol.LivePTTBinaryFrame) protocol.LivePTTBinaryFrame {
	frame.Payload = append([]byte(nil), frame.Payload...)
	return frame
}

func cloneWindowsE2EEOpaqueLiveFrame(frame WindowsE2EEOpaqueLiveFrame) WindowsE2EEOpaqueLiveFrame {
	frame.Ciphertext = append([]byte(nil), frame.Ciphertext...)
	return frame
}

func windowsE2EELiveFramesEqual(left, right WindowsE2EEOpaqueLiveFrame) bool {
	return left.Flags == right.Flags && left.SessionID == right.SessionID && left.Epoch == right.Epoch &&
		left.Generation == right.Generation && left.Sequence == right.Sequence &&
		left.CaptureMonotonicUS == right.CaptureMonotonicUS && left.TargetSnapshotDigest == right.TargetSnapshotDigest &&
		bytes.Equal(left.Ciphertext, right.Ciphertext)
}

func (c WindowsE2EELiveSessionContext) String() string {
	return fmt.Sprintf("WindowsE2EELiveSessionContext{group:<redacted> sender:<redacted> epoch:%d generation:%d}", c.Epoch, c.Generation)
}

func (c WindowsE2EELiveSessionContext) GoString() string { return c.String() }
