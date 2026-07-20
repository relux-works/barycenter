package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	protocol "relux.works/duet/pulsar-win/wire"
)

type windowsE2EELiveAuthorizationBox struct {
	mu      sync.Mutex
	current WindowsE2EELiveAuthorizationSnapshot
}

func (b *windowsE2EELiveAuthorizationBox) CurrentAuthorization() WindowsE2EELiveAuthorizationSnapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	result := b.current
	result.AuthorizedSenderDeviceIDs = make(map[string]struct{}, len(b.current.AuthorizedSenderDeviceIDs))
	for value := range b.current.AuthorizedSenderDeviceIDs {
		result.AuthorizedSenderDeviceIDs[value] = struct{}{}
	}
	return result
}

func (b *windowsE2EELiveAuthorizationBox) update(value WindowsE2EELiveAuthorizationSnapshot) {
	b.mu.Lock()
	b.current = value
	b.mu.Unlock()
}

type windowsE2EELiveFixtureCrypto struct {
	mu                  sync.Mutex
	sealCount           int
	openCount           int
	destroyCount        int
	reuseNonce          bool
	malformedCiphertext bool
}

type windowsE2EELiveAliasingCrypto struct {
	windowsE2EELiveFixtureCrypto
	expected []byte
}

func (c *windowsE2EELiveAliasingCrypto) Seal(plaintext []byte, sequence uint32, aad []byte) (WindowsE2EELiveSealedPayload, error) {
	c.mu.Lock()
	c.sealCount++
	c.mu.Unlock()
	c.expected = append([]byte(nil), plaintext...)
	return WindowsE2EELiveSealedPayload{
		NonceToken: plaintext[:4], WireCiphertext: plaintext,
	}, nil
}

func (*windowsE2EELiveFixtureCrypto) ProductionApproved() bool { return false }

func (c *windowsE2EELiveFixtureCrypto) Seal(plaintext []byte, sequence uint32, aad []byte) (WindowsE2EELiveSealedPayload, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sealCount++
	nonce := make([]byte, 4)
	if !c.reuseNonce {
		binary.BigEndian.PutUint32(nonce, sequence)
	}
	stream := windowsE2EELiveFixtureDigest(append(append(append([]byte("fixture-key"), aad...), nonce...), byte(sequence)))
	encrypted := windowsE2EELiveFixtureXOR(plaintext, stream[:])
	tagInput := append(append(append(append([]byte("fixture-key"), aad...), nonce...), encrypted...), byte(sequence))
	tag := windowsE2EELiveFixtureDigest(tagInput)
	wire := append(append(append([]byte(nil), nonce...), encrypted...), tag[:16]...)
	if c.malformedCiphertext {
		wire = nil
	}
	return WindowsE2EELiveSealedPayload{NonceToken: append([]byte(nil), nonce...), WireCiphertext: wire}, nil
}

func (c *windowsE2EELiveFixtureCrypto) Open(wire []byte, sequence uint32, aad []byte) (WindowsE2EELiveOpenedPayload, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.openCount++
	if len(wire) < 20 {
		return WindowsE2EELiveOpenedPayload{}, errors.New("invalid")
	}
	nonce := append([]byte(nil), wire[:4]...)
	encrypted := append([]byte(nil), wire[4:len(wire)-16]...)
	tagInput := append(append(append(append([]byte("fixture-key"), aad...), nonce...), encrypted...), byte(sequence))
	tag := windowsE2EELiveFixtureDigest(tagInput)
	if !bytes.Equal(wire[len(wire)-16:], tag[:16]) {
		return WindowsE2EELiveOpenedPayload{}, errors.New("invalid")
	}
	stream := windowsE2EELiveFixtureDigest(append(append(append([]byte("fixture-key"), aad...), nonce...), byte(sequence)))
	return WindowsE2EELiveOpenedPayload{NonceToken: nonce, Plaintext: windowsE2EELiveFixtureXOR(encrypted, stream[:])}, nil
}

func (c *windowsE2EELiveFixtureCrypto) Destroy() {
	c.mu.Lock()
	c.destroyCount++
	c.mu.Unlock()
}

func (c *windowsE2EELiveFixtureCrypto) counts() (seal, open, destroy int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sealCount, c.openCount, c.destroyCount
}

type windowsE2EELiveFixtureDeriver struct {
	mu       sync.Mutex
	contexts []WindowsE2EELiveSessionContext
}

func (*windowsE2EELiveFixtureDeriver) ProductionApproved() bool { return false }

func (d *windowsE2EELiveFixtureDeriver) Derive(context WindowsE2EELiveSessionContext, identity *WindowsE2EEDeviceIdentityLease, group *WindowsE2EEGroupStateLease) (WindowsE2EELiveCryptographicSession, error) {
	if identity == nil || group == nil || group.Metadata.Epoch != context.Epoch {
		return nil, errors.New("invalid witnessed derivation")
	}
	d.mu.Lock()
	d.contexts = append(d.contexts, context)
	d.mu.Unlock()
	return &windowsE2EELiveFixtureCrypto{}, nil
}

func (d *windowsE2EELiveFixtureDeriver) snapshot() []WindowsE2EELiveSessionContext {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]WindowsE2EELiveSessionContext(nil), d.contexts...)
}

type windowsE2EELiveReceiverSpy struct {
	mu      sync.Mutex
	frames  []protocol.LivePTTBinaryFrame
	revoked int
}

func (*windowsE2EELiveReceiverSpy) Start(protocol.LivePTTStartPayload, bool) bool { return true }
func (r *windowsE2EELiveReceiverSpy) Receive(frame protocol.LivePTTBinaryFrame) protocol.LivePTTFrameDecision {
	r.mu.Lock()
	r.frames = append(r.frames, cloneWindowsLiveFrame(frame))
	r.mu.Unlock()
	return protocol.LivePTTFrameApply
}
func (*windowsE2EELiveReceiverSpy) End(protocol.LivePTTEndPayload)       {}
func (*windowsE2EELiveReceiverSpy) Cancel(protocol.LivePTTCancelPayload) {}
func (r *windowsE2EELiveReceiverSpy) Revoke() {
	r.mu.Lock()
	r.revoked++
	r.mu.Unlock()
}
func (*windowsE2EELiveReceiverSpy) Snapshot() WindowsLiveJitterSnapshot {
	return WindowsLiveJitterSnapshot{Phase: WindowsLiveIdle}
}
func (r *windowsE2EELiveReceiverSpy) snapshot() ([]protocol.LivePTTBinaryFrame, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	frames := make([]protocol.LivePTTBinaryFrame, len(r.frames))
	for index := range r.frames {
		frames[index] = cloneWindowsLiveFrame(r.frames[index])
	}
	return frames, r.revoked
}

func windowsE2EELiveStart(generation int64, target string) protocol.LivePTTStartPayload {
	return protocol.LivePTTStartPayload{
		SessionID: "00112233445566778899aabbccddeeff", Generation: generation,
		SenderActorID: 10, SenderOrbitID: 20, SenderNodeID: "win-node-1",
		TargetSnapshot: "lts1.fixture", TargetSHA256: target, TargetCount: 2,
		PlaybackDomain: "air", PlaybackDomainID: 44,
		CodecProfile: protocol.LivePTTCodecProfile, FrameMS: protocol.LivePTTFrameMS,
		MaxPayloadBytes: protocol.LivePTTMaxPayloadBytes, JitterBufferMS: protocol.LivePTTJitterBufferMS,
		StartedAtCoordMS: 10_000, AcceptDeadlineCoordMS: 11_000,
		MaxDurationMS: protocol.LivePTTMaxDurationMS, MixedVersionPolicy: protocol.LivePTTMixedVersionRequireAll,
		LateJoinPolicy: protocol.LivePTTLateJoinPolicy, CaptureAuthority: protocol.LivePTTCaptureAuthority,
	}
}

func windowsE2EELiveContext(t testing.TB) WindowsE2EELiveSessionContext {
	t.Helper()
	context, err := NewWindowsE2EELiveSessionContext(
		"air-group-fixture-00000000000001", "windows-device-fixture-0001", 9,
		stringsOf('b', 64), windowsE2EELiveStart(7, stringsOf('a', 64)),
	)
	if err != nil {
		t.Fatal(err)
	}
	return context
}

func windowsE2EELiveAuthorization(context WindowsE2EELiveSessionContext) WindowsE2EELiveAuthorizationSnapshot {
	return WindowsE2EELiveAuthorizationSnapshot{
		GroupID: context.GroupID, Epoch: context.Epoch, CommitDigest: context.CommitDigest,
		TargetSnapshotDigest:      context.TargetSnapshotDigest,
		AuthorizedSenderDeviceIDs: map[string]struct{}{context.AuthorDeviceID: {}},
	}
}

func windowsE2EELiveFrame(sequence uint32, payload []byte) protocol.LivePTTBinaryFrame {
	flags := byte(protocol.LivePTTFlagFEC)
	if sequence == 1 {
		flags |= protocol.LivePTTFlagStart
	}
	return protocol.LivePTTBinaryFrame{
		Flags:     flags,
		SessionID: [16]byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff},
		Sequence:  sequence, CaptureMonotonicUS: 1_000_000 + uint64(sequence-1)*20_000,
		Payload: append([]byte(nil), payload...),
	}
}

func TestWindowsE2EELiveOpaqueFrameMatchesAcceptedBEWire(t *testing.T) {
	context := windowsE2EELiveContext(t)
	frame := WindowsE2EEOpaqueLiveFrame{
		Flags: protocol.LivePTTFlagStart, SessionID: context.SessionID, Epoch: 9, Generation: 7,
		Sequence: 1, CaptureMonotonicUS: 1_000_000, TargetSnapshotDigest: stringsOf('a', 64),
		Ciphertext: []byte{0xde, 0xad, 0xbe, 0xef},
	}
	wire, err := frame.Encode()
	if err != nil {
		t.Fatal(err)
	}
	want := "4245010100112233445566778899aabbccddeeff000000000000000900000000000000070000000100000000000f4240" + stringsOf('a', 64) + "00040000deadbeef"
	if fmt.Sprintf("%x", wire) != want || len(wire) != 88 {
		t.Fatalf("wire=%x", wire)
	}
	decoded, err := DecodeWindowsE2EEOpaqueLiveFrame(wire)
	if err != nil || !windowsE2EELiveFramesEqual(decoded, frame) {
		t.Fatalf("decoded=%+v err=%v", decoded, err)
	}
	legacy, _ := protocol.EncodeLivePTTBinaryFrame(windowsE2EELiveFrame(1, []byte("opus")))
	if _, err := DecodeWindowsE2EEOpaqueLiveFrame(legacy); !errors.Is(err, ErrWindowsE2EELiveInvalidFrame) {
		t.Fatalf("legacy downgrade err=%v", err)
	}
}

func TestWindowsE2EELiveRetryReusesCiphertextAndAuthenticatesBeforeJitter(t *testing.T) {
	context := windowsE2EELiveContext(t)
	authorization := &windowsE2EELiveAuthorizationBox{current: windowsE2EELiveAuthorization(context)}
	senderCrypto, receiverCrypto := &windowsE2EELiveFixtureCrypto{}, &windowsE2EELiveFixtureCrypto{}
	senderChannel, _ := newWindowsE2EELiveFrameChannelForAudit(context, senderCrypto, authorization)
	receiverChannel, _ := newWindowsE2EELiveFrameChannelForAudit(context, receiverCrypto, authorization)
	var attempts [][]byte
	sender := NewWindowsE2EELiveSenderBridge(senderChannel, func(wire []byte) bool {
		attempts = append(attempts, append([]byte(nil), wire...))
		return len(attempts) > 1
	})
	plaintext := []byte("recognizable-opus-plaintext")
	frame := windowsE2EELiveFrame(1, plaintext)
	if sender.TrySend(frame) || !sender.TrySend(frame) {
		t.Fatal("retry result mismatch")
	}
	seal, _, _ := senderCrypto.counts()
	if seal != 1 || len(attempts) != 2 || !bytes.Equal(attempts[0], attempts[1]) || bytes.Contains(attempts[0], plaintext) {
		t.Fatalf("seal=%d attempts=%d plaintextVisible=%v", seal, len(attempts), bytes.Contains(attempts[0], plaintext))
	}
	spy := &windowsE2EELiveReceiverSpy{}
	receiver := NewWindowsE2EELiveReceiverBridge(receiverChannel, spy)
	if decision := receiver.ReceiveOpaque(attempts[1]); decision != protocol.LivePTTFrameApply {
		t.Fatalf("decision=%s", decision)
	}
	frames, revoked := spy.snapshot()
	_, opens, _ := receiverCrypto.counts()
	if revoked != 0 || opens != 1 || len(frames) != 1 || !bytes.Equal(frames[0].Payload, plaintext) {
		t.Fatalf("frames=%d revoked=%d opens=%d", len(frames), revoked, opens)
	}
}

func TestWindowsE2EELiveTamperReplayAndNonceReuseFailClosed(t *testing.T) {
	context := windowsE2EELiveContext(t)
	authorization := &windowsE2EELiveAuthorizationBox{current: windowsE2EELiveAuthorization(context)}
	sender, _ := newWindowsE2EELiveFrameChannelForAudit(context, &windowsE2EELiveFixtureCrypto{}, authorization)
	opaque, err := sender.Protect(windowsE2EELiveFrame(1, []byte("opus")))
	if err != nil {
		t.Fatal(err)
	}
	wire, _ := opaque.Encode()
	tampered := append([]byte(nil), wire...)
	tampered[len(tampered)-1] ^= 1
	receiver, _ := newWindowsE2EELiveFrameChannelForAudit(context, &windowsE2EELiveFixtureCrypto{}, authorization)
	spy := &windowsE2EELiveReceiverSpy{}
	bridge := NewWindowsE2EELiveReceiverBridge(receiver, spy)
	if decision := bridge.ReceiveOpaque(tampered); decision != protocol.LivePTTFrameInvalid || !receiver.IsTerminal() {
		t.Fatalf("tamper decision=%s terminal=%v", decision, receiver.IsTerminal())
	}
	frames, revoked := spy.snapshot()
	if len(frames) != 0 || revoked != 1 {
		t.Fatalf("tamper reached jitter frames=%d revoked=%d", len(frames), revoked)
	}

	replayReceiver, _ := newWindowsE2EELiveFrameChannelForAudit(context, &windowsE2EELiveFixtureCrypto{}, authorization)
	replaySpy := &windowsE2EELiveReceiverSpy{}
	replayBridge := NewWindowsE2EELiveReceiverBridge(replayReceiver, replaySpy)
	if replayBridge.ReceiveOpaque(wire) != protocol.LivePTTFrameApply || replayBridge.ReceiveOpaque(wire) != protocol.LivePTTFrameDuplicate {
		t.Fatal("replay classification mismatch")
	}

	reuseCrypto := &windowsE2EELiveFixtureCrypto{reuseNonce: true}
	reuse, _ := newWindowsE2EELiveFrameChannelForAudit(context, reuseCrypto, authorization)
	if _, err := reuse.Protect(windowsE2EELiveFrame(1, []byte("one"))); err != nil {
		t.Fatal(err)
	}
	if _, err := reuse.Protect(windowsE2EELiveFrame(2, []byte("two"))); !errors.Is(err, ErrWindowsE2EELiveNonceReuse) || !reuse.IsTerminal() {
		t.Fatalf("nonce reuse err=%v terminal=%v", err, reuse.IsTerminal())
	}
}

func TestWindowsE2EELiveProviderOutputAndDurationBounds(t *testing.T) {
	context := windowsE2EELiveContext(t)
	authorization := &windowsE2EELiveAuthorizationBox{current: windowsE2EELiveAuthorization(context)}
	malformedCrypto := &windowsE2EELiveFixtureCrypto{malformedCiphertext: true}
	malformed, _ := newWindowsE2EELiveFrameChannelForAudit(context, malformedCrypto, authorization)
	if _, err := malformed.Protect(windowsE2EELiveFrame(1, []byte("one"))); !errors.Is(err, ErrWindowsE2EELiveMalformedProviderOutput) || !malformed.IsTerminal() {
		t.Fatalf("malformed err=%v terminal=%v", err, malformed.IsTerminal())
	}

	boundedCrypto := &windowsE2EELiveFixtureCrypto{}
	bounded, _ := newWindowsE2EELiveFrameChannelForAudit(context, boundedCrypto, authorization)
	bounded.outgoingSequence = windowsE2EELiveMaximumSequence
	bounded.outgoingCaptureBaseUS = 1_000_000
	if _, err := bounded.Protect(windowsE2EELiveFrame(windowsE2EELiveMaximumSequence+1, []byte{1})); !errors.Is(err, ErrWindowsE2EELiveInvalidFrame) || bounded.IsTerminal() {
		t.Fatalf("duration err=%v terminal=%v", err, bounded.IsTerminal())
	}
	seal, _, _ := boundedCrypto.counts()
	if seal != 0 {
		t.Fatalf("sealed after duration bound: %d", seal)
	}
}

func TestWindowsE2EELiveProviderAndCallerAliasingCannotMutateRetryState(t *testing.T) {
	context := windowsE2EELiveContext(t)
	authorization := &windowsE2EELiveAuthorizationBox{current: windowsE2EELiveAuthorization(context)}
	crypto := &windowsE2EELiveAliasingCrypto{}
	channel, err := newWindowsE2EELiveFrameChannelForAudit(context, crypto, authorization)
	if err != nil {
		t.Fatal(err)
	}
	frame := windowsE2EELiveFrame(1, bytes.Repeat([]byte{0x51}, 128))
	first, err := channel.Protect(frame)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Ciphertext, crypto.expected) || bytes.Equal(first.Ciphertext, make([]byte, len(first.Ciphertext))) {
		t.Fatal("provider alias was zeroed before service ownership was frozen")
	}
	first.Ciphertext[0] ^= 0xff
	frame.Payload[0] = 0x51
	retry, err := channel.Protect(frame)
	if err != nil || !bytes.Equal(retry.Ciphertext, crypto.expected) {
		t.Fatalf("caller mutation changed retry state err=%v", err)
	}
}

func TestWindowsE2EELiveMembershipAndCommitChangeTerminate(t *testing.T) {
	context := windowsE2EELiveContext(t)
	authorization := &windowsE2EELiveAuthorizationBox{current: windowsE2EELiveAuthorization(context)}
	crypto := &windowsE2EELiveFixtureCrypto{}
	channel, _ := newWindowsE2EELiveFrameChannelForAudit(context, crypto, authorization)
	if _, err := channel.Protect(windowsE2EELiveFrame(1, []byte("one"))); err != nil {
		t.Fatal(err)
	}
	changed := windowsE2EELiveAuthorization(context)
	changed.Epoch++
	changed.CommitDigest = stringsOf('c', 64)
	changed.AuthorizedSenderDeviceIDs = map[string]struct{}{}
	authorization.update(changed)
	if _, err := channel.Protect(windowsE2EELiveFrame(2, []byte("two"))); !errors.Is(err, ErrWindowsE2EELiveStaleEpoch) || !channel.IsTerminal() {
		t.Fatalf("change err=%v terminal=%v", err, channel.IsTerminal())
	}
	_, _, destroyed := crypto.counts()
	channel.Terminate()
	_, _, destroyedAgain := crypto.counts()
	if destroyed != 1 || destroyedAgain != 1 {
		t.Fatalf("destroy counts=%d/%d", destroyed, destroyedAgain)
	}
}

func TestWindowsE2EELiveAADBindsSharedContextNotLocalRevision(t *testing.T) {
	context := windowsE2EELiveContext(t)
	first, err := windowsE2EELiveAuthenticatedData(context, protocol.LivePTTFlagStart, 1, 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	changed := context
	changed.PlaybackDomainID++
	second, _ := windowsE2EELiveAuthenticatedData(changed, protocol.LivePTTFlagStart, 1, 1_000_000)
	sequence, _ := windowsE2EELiveAuthenticatedData(context, 0, 2, 1_020_000)
	if bytes.Equal(first, second) || bytes.Equal(first, sequence) || !bytes.Contains(first, []byte(context.CommitDigest)) ||
		!bytes.Contains(first, []byte(context.AuthorDeviceID)) || !bytes.Contains(first, []byte(context.CodecProfile)) {
		t.Fatal("AAD binding mismatch")
	}
	if bytes.Contains([]byte(context.String()), []byte(context.GroupID)) || bytes.Contains([]byte(context.String()), []byte(context.AuthorDeviceID)) {
		t.Fatalf("unsafe context string: %s", context)
	}
}

func TestWindowsE2EELiveUnreviewedProviderCannotCrossProductionFactory(t *testing.T) {
	context := windowsE2EELiveContext(t)
	authorization := &windowsE2EELiveAuthorizationBox{current: windowsE2EELiveAuthorization(context)}
	fixture := newWindowsE2EEFixture(t, 0x61)
	if _, err := NewWindowsE2EELiveSessionFactory(fixture.repository, &windowsE2EELiveFixtureDeriver{}, authorization); !errors.Is(err, ErrWindowsE2EELiveProviderNotApproved) {
		t.Fatalf("production gate err=%v", err)
	}
	crypto := &windowsE2EELiveFixtureCrypto{}
	if _, err := NewWindowsE2EELiveFrameChannel(context, crypto, authorization); !errors.Is(err, ErrWindowsE2EELiveProviderNotApproved) {
		t.Fatalf("channel gate err=%v", err)
	}
	_, _, destroyed := crypto.counts()
	if destroyed != 1 {
		t.Fatalf("unapproved session destroy=%d", destroyed)
	}
}

func TestWindowsE2EELiveFactoryReservesWitnessedGeneration(t *testing.T) {
	fixture := newWindowsE2EEFixture(t, 0x62)
	group := installWindowsE2EEGroup(t, fixture)
	context, err := NewWindowsE2EELiveSessionContext(group.GroupID, fixture.identity.DeviceID, group.Epoch, group.CommitDigest, windowsE2EELiveStart(1, group.TargetSnapshotDigest))
	if err != nil {
		t.Fatal(err)
	}
	authorization := &windowsE2EELiveAuthorizationBox{current: windowsE2EELiveAuthorization(context)}
	deriver := &windowsE2EELiveFixtureDeriver{}
	factory, _ := newWindowsE2EELiveSessionFactoryForAudit(fixture.repository, deriver, authorization)
	prepared, err := factory.PrepareOutgoing(WindowsE2EELiveOutgoingRequest{
		GroupID: group.GroupID, AuthorDeviceID: fixture.identity.DeviceID,
		ExpectedGroupRevision: group.Revision, ExpectedTargetSnapshotDigest: group.TargetSnapshotDigest, NowMS: 2000,
	}, func(reservation WindowsE2EESendReservation) (protocol.LivePTTStartPayload, error) {
		return windowsE2EELiveStart(int64(reservation.Generation), group.TargetSnapshotDigest), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	contexts := deriver.snapshot()
	if prepared.Reservation.Domain != "live_ptt" || prepared.Reservation.Generation != 1 || prepared.Reservation.Revision != group.Revision+1 ||
		len(contexts) != 1 || contexts[0].CommitDigest != group.CommitDigest || contexts[0].Epoch != group.Epoch {
		t.Fatalf("prepared=%+v contexts=%+v", prepared.Reservation, contexts)
	}
}

func TestWindowsE2EELiveCrossInstallationRoundTripWithSkewedLocalRevisions(t *testing.T) {
	groupID := "air-group-fixture-00000000000001"
	target, commit := stringsOf('a', 64), stringsOf('b', 64)
	senderRepository, senderIdentity := newWindowsE2EELiveRepository(t, 0x71, "windows-device-fixture-0001")
	receiverRepository, receiverIdentity := newWindowsE2EELiveRepository(t, 0x72, "windows-device-fixture-0002")
	senderGroup, err := senderRepository.PersistGroupState(senderIdentity.InstallationID, groupID, 9, "", commit, target, bytes.Repeat([]byte{0x33}, 64), 0, 2000)
	if err != nil {
		t.Fatal(err)
	}
	receiverGroup, err := receiverRepository.PersistGroupState(receiverIdentity.InstallationID, groupID, 9, "", commit, target, bytes.Repeat([]byte{0x66}, 64), 0, 2000)
	if err != nil {
		t.Fatal(err)
	}
	context, _ := NewWindowsE2EELiveSessionContext(groupID, senderIdentity.DeviceID, 9, commit, windowsE2EELiveStart(1, target))
	authorization := &windowsE2EELiveAuthorizationBox{current: windowsE2EELiveAuthorization(context)}
	senderFactory, _ := newWindowsE2EELiveSessionFactoryForAudit(senderRepository, &windowsE2EELiveFixtureDeriver{}, authorization)
	receiverFactory, _ := newWindowsE2EELiveSessionFactoryForAudit(receiverRepository, &windowsE2EELiveFixtureDeriver{}, authorization)
	outgoing, err := senderFactory.PrepareOutgoing(WindowsE2EELiveOutgoingRequest{
		GroupID: groupID, AuthorDeviceID: senderIdentity.DeviceID, ExpectedGroupRevision: senderGroup.Revision,
		ExpectedTargetSnapshotDigest: target, NowMS: 3000,
	}, func(reservation WindowsE2EESendReservation) (protocol.LivePTTStartPayload, error) {
		return windowsE2EELiveStart(int64(reservation.Generation), target), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if outgoing.Reservation.Revision == receiverGroup.Revision {
		t.Fatal("local revisions must deliberately differ")
	}
	incoming, err := receiverFactory.PrepareIncoming(WindowsE2EELiveIncomingRequest{
		GroupID: groupID, LocalDeviceID: receiverIdentity.DeviceID, AuthorDeviceID: senderIdentity.DeviceID,
		Epoch: receiverGroup.Epoch, ExpectedLocalGroupRevision: receiverGroup.Revision, Start: outgoing.Start,
	})
	if err != nil {
		t.Fatal(err)
	}
	plaintext := windowsE2EELiveFrame(1, []byte("cross-installation-opus"))
	opaque, err := outgoing.Channel.Protect(plaintext)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := incoming.Open(opaque)
	if err != nil || !windowsLiveFramesEqual(opened, plaintext) {
		t.Fatalf("opened=%+v err=%v", opened, err)
	}
}

func TestWindowsE2EELiveIncomingReorderRemainsWithinExistingWindow(t *testing.T) {
	context := windowsE2EELiveContext(t)
	authorization := &windowsE2EELiveAuthorizationBox{current: windowsE2EELiveAuthorization(context)}
	sender, _ := newWindowsE2EELiveFrameChannelForAudit(context, &windowsE2EELiveFixtureCrypto{}, authorization)
	receiver, _ := newWindowsE2EELiveFrameChannelForAudit(context, &windowsE2EELiveFixtureCrypto{}, authorization)
	frames := make([]WindowsE2EEOpaqueLiveFrame, 3)
	for sequence := uint32(1); sequence <= 3; sequence++ {
		frames[sequence-1], _ = sender.Protect(windowsE2EELiveFrame(sequence, []byte{byte(sequence)}))
	}
	for _, index := range []int{0, 2, 1} {
		opened, err := receiver.Open(frames[index])
		if err != nil || opened.Sequence != frames[index].Sequence {
			t.Fatalf("index=%d opened=%d err=%v", index, opened.Sequence, err)
		}
	}
}

func newWindowsE2EELiveRepository(t testing.TB, randomByte byte, deviceID string) (*WindowsE2EEKeyStateRepository, WindowsE2EEDeviceIdentityMetadata) {
	t.Helper()
	repository, err := NewWindowsE2EEKeyStateRepository(WindowsE2EEKeyStateOptions{
		Directory: filepath.Join("C:", "Users", "test", fmt.Sprintf("Pulsar-live-%02x", randomByte)),
		Protector: &testProtector{}, Files: newTestSecureFileOps(),
		Random: bytes.NewReader(bytes.Repeat([]byte{randomByte}, 1<<20)),
	})
	if err != nil {
		t.Fatal(err)
	}
	identity, err := repository.InstallDeviceIdentity(deviceID, "fixture-v1", bytes.Repeat([]byte{0x11}, 32), bytes.Repeat([]byte{0x22}, 32), 1000)
	if err != nil {
		t.Fatal(err)
	}
	return repository, identity
}

func windowsE2EELiveFixtureDigest(value []byte) [32]byte { return sha256.Sum256(value) }

func windowsE2EELiveFixtureXOR(value, stream []byte) []byte {
	result := make([]byte, len(value))
	for index := range value {
		result[index] = value[index] ^ stream[index%len(stream)]
	}
	return result
}
