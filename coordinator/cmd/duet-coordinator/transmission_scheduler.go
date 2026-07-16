package main

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"relux.works/duet/coordinator/internal/hub"
	"relux.works/duet/coordinator/internal/protocol"
	"relux.works/duet/coordinator/internal/session"
	"relux.works/duet/coordinator/internal/store"
)

type transmissionSignal struct {
	transmissionID string
	cancellation   *store.CancelTransmissionResult
}

func (l *loop) signalTransmission(signal transmissionSignal) {
	if signal.cancellation == nil {
		// Runtime wakes are level-triggered: one pending scan observes every
		// durable receipt. Coalescing keeps the single-threaded loop from
		// deadlocking itself during a burst of node messages.
		select {
		case l.transmissionCh <- signal:
		default:
		}
		return
	}
	// Cancellation carries the exact generation snapshots needed for the first
	// disarm attempt, so external HTTP producers receive bounded backpressure.
	select {
	case l.transmissionCh <- signal:
	case <-l.stopped:
	}
}

func (l *loop) transmissionAccepted(transmissionID string) {
	l.signalTransmission(transmissionSignal{transmissionID: transmissionID})
}

func (l *loop) transmissionCancelled(result store.CancelTransmissionResult) {
	copy := result
	copy.DisarmTargets = append([]store.TransmissionTarget{}, result.DisarmTargets...)
	l.signalTransmission(transmissionSignal{
		transmissionID: result.Transmission.ID,
		cancellation:   &copy,
	})
}

func (l *loop) handleTransmissionSignal(signal transmissionSignal) {
	if signal.cancellation != nil {
		l.deliverTransmissionCancellation(*signal.cancellation)
	}
	l.runTransmissionScheduler(time.Now().UnixMilli())
}

func (l *loop) deliverTransmissionCancellation(result store.CancelTransmissionResult) {
	work, err := l.st.GetTransmissionSchedulerWork(result.Transmission.ID)
	if err != nil {
		l.log.Error("load transmission cancellation work", "err", err)
		return
	}
	legacy := work.Scheduler.LegacyElementID != ""
	if legacy {
		if state := l.transmissionDomainState(work.Transmission); state != nil {
			l.apply(state, state.sess.CancelElement(work.Scheduler.LegacyElementID))
		}
		// The legacy Session FSM owns play_voice/stop and has no media_cancelled
		// receipt. Its exact element removal is the local acknowledgement.
		for _, target := range result.DisarmTargets {
			l.acknowledgeLocalLegacyCancellation(target, time.Now().UnixMilli())
		}
		return
	}
	for _, target := range result.DisarmTargets {
		action := "disarm"
		fadeMS := int64(0)
		resumeMain := false
		if target.StartedAt > 0 {
			action = "fade_stop"
			fadeMS = 120
			resumeMain = work.Transmission.EffectiveDelivery ==
				store.TransmissionDeliveryInterrupt
		}
		payload := &protocol.CancelMediaPayload{
			TransmissionID: target.TransmissionID,
			Generation:     target.Generation,
			Reason:         string(target.ReasonCode),
			Action:         action,
			ResumeMain:     resumeMain,
			FadeMS:         fadeMS,
		}
		l.hub.Send(
			hub.NodeKey{Orbit: target.OrbitID, Slot: protocol.NodeID(target.Slot)},
			protocol.TypeCancelMedia,
			payload,
		)
	}
}

func (l *loop) deliverTransmissionCancellations(results []store.CancelTransmissionResult) {
	for _, result := range results {
		l.deliverTransmissionCancellation(result)
	}
}

func (l *loop) cancelTransmissionInstallation(
	identity *store.MediaTargetIdentity,
	reason store.TransmissionReason,
	now int64,
) {
	if identity == nil {
		return
	}
	results, err := l.st.CancelTransmissionNode(
		identity.OrbitID, identity.ActorID, identity.Slot, reason, now,
	)
	if err != nil {
		l.log.Error("cancel installation transmissions", "orbit", identity.OrbitID,
			"slot", identity.Slot, "reason", reason, "err", err)
		return
	}
	l.deliverTransmissionCancellations(results)
}

func (l *loop) acknowledgeLocalLegacyCancellation(
	target store.TransmissionTarget,
	now int64,
) {
	if target.Status != store.TransmissionTargetCancelling {
		return
	}
	_, err := l.st.TransitionTransmissionTarget(store.TransitionTransmissionTargetParams{
		TransmissionID: target.TransmissionID,
		OrbitID:        target.OrbitID, ActorID: target.ActorID, Slot: target.Slot,
		ExpectedRevision: target.Revision, Generation: target.Generation,
		Status: store.TransmissionTargetCancelled, ReasonCode: target.ReasonCode,
		OccurredAt: maxCoordinatorTime(now, target.UpdatedAt),
	})
	if err != nil && !errors.Is(err, store.ErrTransmissionStateConflict) {
		l.log.Error("acknowledge local legacy cancellation", "err", err)
	}
}

func maxCoordinatorTime(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func (l *loop) transmissionRuntimeTargets() []store.TransmissionRuntimeTarget {
	snapshots := l.hub.NodeSnapshots()
	result := make([]store.TransmissionRuntimeTarget, 0, len(snapshots))
	for key, snapshot := range snapshots {
		result = append(result, store.TransmissionRuntimeTarget{
			OrbitID: key.Orbit, Slot: string(key.Slot),
			Connected: snapshot.Connected, LastSeenAt: snapshot.LastSeenAt,
			CredentialTokenHash: snapshot.CredentialTokenHash,
			MediaClipCapable:    snapshot.Capabilities.Supports(protocol.CapabilityMediaClip),
			OverlayCapable:      snapshot.Capabilities.Supports(protocol.CapabilityOverlayMix),
			InterruptCapable:    snapshot.Capabilities.Supports(protocol.CapabilityInterruptResume),
			InterruptResumeReady: snapshot.Capabilities.Supports(
				protocol.CapabilityInterruptResume,
			),
			RTTMS: snapshot.RTTMS, RTTSampledAt: snapshot.RTTSampledAt,
		})
	}
	return result
}

// reconcileTransmissionNode resends only persisted same-generation work to a
// freshly authenticated binding. It never creates a new acceptance time or
// extends a barrier/T deadline.
func (l *loop) reconcileTransmissionNode(key hub.NodeKey, now int64) {
	snapshot, connected := l.hub.NodeSnapshots()[key]
	if !connected || !snapshot.Connected || snapshot.CredentialTokenHash == "" {
		return
	}
	identity, err := l.st.CurrentInstallationTargetForSocket(
		key.Orbit, string(key.Slot), snapshot.CredentialTokenHash,
	)
	if err != nil {
		l.log.Error("resolve reconnect transmission identity", "err", err)
		return
	}
	if identity == nil {
		return
	}
	work, err := l.st.ListTransmissionSchedulerWork(1000)
	if err != nil {
		l.log.Error("list reconnect transmission work", "err", err)
		return
	}
	for _, item := range work {
		for _, target := range item.Targets {
			if target.OrbitID != identity.OrbitID || target.ActorID != identity.ActorID ||
				target.Slot != identity.Slot {
				continue
			}
			if target.Status == store.TransmissionTargetCancelling {
				l.deliverTransmissionCancellation(store.CancelTransmissionResult{
					Transmission:  item.Transmission,
					DisarmTargets: []store.TransmissionTarget{target},
				})
				continue
			}
			clipCapable := target.MediaClipCapable &&
				snapshot.Capabilities.Supports(protocol.CapabilityMediaClip)
			deliveryCapable := false
			switch item.Transmission.EffectiveDelivery {
			case store.TransmissionDeliveryOverlay:
				deliveryCapable = target.OverlayCapable &&
					snapshot.Capabilities.Supports(protocol.CapabilityOverlayMix)
			case store.TransmissionDeliveryInterrupt:
				deliveryCapable = target.InterruptCapable && target.InterruptResumeReady &&
					snapshot.Capabilities.Supports(protocol.CapabilityInterruptResume)
			case store.TransmissionDeliveryAfterCurrent:
				// The legacy Session bridge owns its own reconnect behavior.
				continue
			}
			if !clipCapable || !deliveryCapable {
				continue
			}
			switch target.Status {
			case store.TransmissionTargetPreparing:
				if item.Scheduler.PrepareDeadlineAt >= now && item.Media.Status == store.MediaStatusReady &&
					item.Media.ExpiresAt > now {
					l.sendTransmissionPrepare(store.OpenTransmissionBarrierResult{
						Work: item, PrepareTargets: []store.TransmissionTarget{target},
					})
				}
			case store.TransmissionTargetScheduled:
				if item.Scheduler.StartDeadlineCoordMS >= now {
					l.sendTransmissionPlay(store.DecideTransmissionBarrierResult{
						Work: item, ScheduledTargets: []store.TransmissionTarget{target},
						Decided: true,
					})
				}
			}
		}
	}
}

func transmissionDomainKey(transmission store.Transmission) string {
	return fmt.Sprintf("%s/%d", transmission.PlaybackDomainKind, transmission.PlaybackDomainID)
}

func (l *loop) runTransmissionScheduler(now int64) {
	if now <= 0 {
		return
	}
	for pass := 0; pass < 128; pass++ {
		// Refresh immediately before each transactional pass. The exact RTT
		// lead is measured from the decision pass, not from an earlier channel
		// wake that may have waited behind other playback domains.
		if l.transmissionNow != nil {
			now = maxCoordinatorTime(now, l.transmissionNow())
		}
		work, err := l.st.ListTransmissionSchedulerWork(1000)
		if err != nil {
			l.log.Error("list transmission scheduler work", "err", err)
			l.armTransmissionTimer(now+1000, now)
			return
		}
		if len(work) == 0 {
			l.clearTransmissionTimer()
			return
		}
		runtime := l.transmissionRuntimeTargets()
		changed := false
		nextDue := int64(0)
		for _, item := range work {
			if item.Transmission.CancellationCause == "" &&
				now >= item.Transmission.ExpiresAt {
				expiry, err := l.st.ExpireTransmissionDelivery(
					item.Transmission.ID, now,
				)
				if err != nil {
					l.log.Error("expire transmission delivery", "err", err)
					nextDue = earlierCoordinatorDeadline(nextDue, now+250)
					continue
				}
				if expiry.Changed || len(expiry.DisarmTargets) > 0 {
					l.deliverTransmissionCancellation(expiry)
					changed = true
					continue
				}
			}
			expired, err := l.st.ExpireTransmissionRuntime(item.Transmission.ID, now)
			if err != nil {
				l.log.Error("expire transmission runtime", "err", err)
				nextDue = earlierCoordinatorDeadline(nextDue, now+250)
				continue
			}
			if expired.Changed {
				if len(expired.DisarmTargets) > 0 {
					l.deliverTransmissionCancellation(store.CancelTransmissionResult{
						Transmission:  expired.Work.Transmission,
						Changed:       true,
						DisarmTargets: expired.DisarmTargets,
					})
				}
				changed = true
				continue
			}
			nextDue = earlierCoordinatorDeadline(nextDue, expired.NextDue)
			if item.Transmission.CancellationCause == "" {
				nextDue = earlierCoordinatorDeadline(nextDue, item.Transmission.ExpiresAt)
			}
			if item.Transmission.EffectiveDelivery == store.TransmissionDeliveryAfterCurrent {
				continue
			}
			rechecked, err := l.st.RecheckTransmissionRuntime(
				item.Transmission.ID, now, runtime,
			)
			if err != nil {
				l.log.Error("recheck transmission runtime", "err", err)
				nextDue = earlierCoordinatorDeadline(nextDue, now+250)
				continue
			}
			if rechecked.Changed {
				if len(rechecked.DisarmTargets) > 0 {
					l.deliverTransmissionCancellation(store.CancelTransmissionResult{
						Transmission:  rechecked.Work.Transmission,
						Changed:       true,
						DisarmTargets: rechecked.DisarmTargets,
					})
				}
				changed = true
			}
			if item.Transmission.CancellationCause != "" {
				// Whole-transmission cancellation leaves only acknowledgement or
				// playback-end watchdogs. It must never re-enter prepare/decision.
				continue
			}
		}
		if changed {
			continue
		}

		// after_current remains on the legacy Session FSM and intentionally does
		// not serialize against mixed overlay/interrupt work.
		for _, item := range work {
			if item.Transmission.EffectiveDelivery != store.TransmissionDeliveryAfterCurrent ||
				item.Scheduler.LegacyElementID != "" {
				continue
			}
			bridged, err := l.bridgeLegacyTransmission(item, now)
			if err != nil {
				l.log.Error("bridge legacy transmission", "err", err)
				nextDue = earlierCoordinatorDeadline(nextDue, now+250)
				continue
			}
			changed = changed || bridged
		}
		if changed {
			continue
		}

		domainSeen := make(map[string]struct{})
		for _, item := range work {
			if item.Transmission.EffectiveDelivery == store.TransmissionDeliveryAfterCurrent {
				continue
			}
			if l.livePTTBlocksTransmission(item.Transmission) {
				nextDue = earlierCoordinatorDeadline(nextDue, now+100)
				continue
			}
			domain := transmissionDomainKey(item.Transmission)
			if _, seen := domainSeen[domain]; seen {
				continue
			}
			domainSeen[domain] = struct{}{}
			if item.Scheduler.BarrierOpenedAt == 0 {
				opened, err := l.st.OpenTransmissionBarrier(
					item.Transmission.ID, now, runtime,
				)
				if errors.Is(err, store.ErrTransmissionNotFIFOHead) {
					continue
				}
				if err != nil {
					l.log.Error("open transmission barrier", "err", err)
					nextDue = earlierCoordinatorDeadline(nextDue, now+250)
					continue
				}
				if opened.Opened {
					l.sendTransmissionPrepare(opened)
				}
				changed = changed || opened.Changed
				if opened.Work.Scheduler.PrepareDeadlineAt > 0 {
					nextDue = earlierCoordinatorDeadline(
						nextDue, opened.Work.Scheduler.PrepareDeadlineAt,
					)
				}
				continue
			}
			if item.Scheduler.DecisionAt == 0 {
				decision, err := l.st.DecideTransmissionBarrier(
					item.Transmission.ID, now, runtime,
				)
				if err != nil {
					l.log.Error("decide transmission barrier", "err", err)
					nextDue = earlierCoordinatorDeadline(nextDue, now+250)
					continue
				}
				if decision.Decided && decision.Changed {
					l.sendTransmissionPlay(decision)
					if len(decision.DisarmTargets) > 0 {
						l.deliverTransmissionCancellation(store.CancelTransmissionResult{
							Transmission:  decision.Work.Transmission,
							Changed:       true,
							DisarmTargets: decision.DisarmTargets,
						})
					}
				}
				changed = changed || decision.Changed
				if !decision.Decided {
					nextDue = earlierCoordinatorDeadline(
						nextDue, item.Scheduler.PrepareDeadlineAt,
					)
				}
			}
		}
		if changed {
			continue
		}
		if nextDue > 0 {
			l.armTransmissionTimer(nextDue, now)
		}
		return
	}
	l.log.Error("transmission scheduler convergence limit reached")
	l.armTransmissionTimer(now+100, now)
}

func earlierCoordinatorDeadline(current, candidate int64) int64 {
	if candidate <= 0 {
		return current
	}
	if current == 0 || candidate < current {
		return candidate
	}
	return current
}

func (l *loop) armTransmissionTimer(due, coordinatorNow int64) {
	if due <= 0 || coordinatorNow <= 0 {
		return
	}
	// Keep an already armed earlier persisted deadline. Resetting the same
	// deadline from wall time after an unrelated wake would make a backwards
	// clock adjustment extend the exact three-second barrier.
	if l.transmissionTimerC != nil && l.transmissionTimerDue > 0 &&
		l.transmissionTimerDue <= due {
		return
	}
	delay := time.Duration(due-coordinatorNow) * time.Millisecond
	if delay <= 0 {
		delay = time.Millisecond
	}
	if l.transmissionTimer == nil {
		l.transmissionTimer = time.NewTimer(delay)
	} else {
		l.transmissionTimer.Reset(delay)
	}
	l.transmissionTimerDue = due
	l.transmissionTimerC = l.transmissionTimer.C
}

func (l *loop) clearTransmissionTimer() {
	if l.transmissionTimer != nil {
		l.transmissionTimer.Stop()
	}
	l.transmissionTimerC = nil
	l.transmissionTimerDue = 0
}

func (l *loop) sendTransmissionPrepare(result store.OpenTransmissionBarrierResult) {
	work := result.Work
	payloadFor := func(target store.TransmissionTarget) *protocol.PrepareMediaPayload {
		return &protocol.PrepareMediaPayload{
			TransmissionID: work.Transmission.ID,
			Generation:     target.Generation, MediaID: work.Media.ID,
			Kind:     string(work.Media.Kind),
			Delivery: string(work.Transmission.EffectiveDelivery),
			FileURL:  l.genericMediaURL(work.Media.ID), SHA256: work.Media.SHA256,
			SizeBytes: work.Media.SizeBytes, DurationMS: work.Media.DurationMS,
			MediaExpiresAtCoordMS:  work.Media.ExpiresAt,
			PrepareDeadlineCoordMS: work.Scheduler.PrepareDeadlineAt,
		}
	}
	for _, target := range result.PrepareTargets {
		if !l.hub.Send(
			hub.NodeKey{Orbit: target.OrbitID, Slot: protocol.NodeID(target.Slot)},
			protocol.TypePrepareMedia,
			payloadFor(target),
		) {
			l.markTransmissionTargetOffline(target, store.TransmissionReasonOfflineBeforePrepare)
		}
	}
}

func (l *loop) sendTransmissionPlay(result store.DecideTransmissionBarrierResult) {
	work := result.Work
	for _, target := range result.ScheduledTargets {
		payload := &protocol.PlayMediaAtPayload{
			TransmissionID: work.Transmission.ID, Generation: target.Generation,
			TCoordMS:             work.Scheduler.TCoordMS,
			StartDeadlineCoordMS: work.Scheduler.StartDeadlineCoordMS,
			Delivery:             string(work.Transmission.EffectiveDelivery),
		}
		switch work.Transmission.EffectiveDelivery {
		case store.TransmissionDeliveryOverlay:
			duckDB, attackMS, releaseMS := -12.0, int64(250), int64(600)
			payload.DuckDB, payload.AttackMS, payload.ReleaseMS =
				&duckDB, &attackMS, &releaseMS
		case store.TransmissionDeliveryInterrupt:
			fadeOutMS, fadeInMS := int64(250), int64(120)
			payload.FadeOutMS, payload.FadeInMS = &fadeOutMS, &fadeInMS
		default:
			continue
		}
		if !l.hub.Send(
			hub.NodeKey{Orbit: target.OrbitID, Slot: protocol.NodeID(target.Slot)},
			protocol.TypePlayMediaAt,
			payload,
		) {
			l.markTransmissionTargetOffline(target, store.TransmissionReasonOfflineBeforeStart)
		}
	}
}

// sendTransmissionSafetyCancel is best-effort cleanup after an authenticated
// but impossible receipt was durably rejected. The target is already terminal,
// so no acknowledgement is trusted; this command only prevents a buggy local
// player from keeping prepared or early-started audio alive.
func (l *loop) sendTransmissionSafetyCancel(
	key hub.NodeKey,
	work store.TransmissionSchedulerWork,
	generation int64,
	reason store.TransmissionReason,
	active bool,
) {
	action, fadeMS, resumeMain := "disarm", int64(0), false
	if active {
		action, fadeMS = "fade_stop", 120
		resumeMain = work.Transmission.EffectiveDelivery ==
			store.TransmissionDeliveryInterrupt
	}
	l.hub.Send(key, protocol.TypeCancelMedia, &protocol.CancelMediaPayload{
		TransmissionID: work.Transmission.ID, Generation: generation,
		Reason: string(reason), Action: action, ResumeMain: resumeMain, FadeMS: fadeMS,
	})
}

func (l *loop) markTransmissionTargetOffline(
	target store.TransmissionTarget,
	reason store.TransmissionReason,
) {
	_, err := l.st.TransitionTransmissionTarget(store.TransitionTransmissionTargetParams{
		TransmissionID: target.TransmissionID,
		OrbitID:        target.OrbitID, ActorID: target.ActorID, Slot: target.Slot,
		ExpectedRevision: target.Revision, Generation: target.Generation,
		Status: store.TransmissionTargetMissedOffline, ReasonCode: reason,
		OccurredAt: maxCoordinatorTime(time.Now().UnixMilli(), target.UpdatedAt),
	})
	if err != nil && !errors.Is(err, store.ErrTransmissionStateConflict) {
		l.log.Error("mark transmission target offline", "err", err)
	}
}

func (l *loop) genericMediaURL(mediaID string) string {
	if l.cfg.PublicURL != "" {
		return fmt.Sprintf("%s/v1/media/%s", strings.TrimRight(l.cfg.PublicURL, "/"), mediaID)
	}
	return fmt.Sprintf("http://%s/v1/media/%s", l.cfg.Listen, mediaID)
}

func (l *loop) transmissionDomainState(
	transmission store.Transmission,
) *orbitState {
	if transmission.AirID != "" {
		if state := l.airs[transmission.AirID]; state != nil {
			return state
		}
		// Re-resolve after restart or a just-committed activation. The accepted
		// target snapshot is still authoritative; this only locates its runtime.
		state := l.stateFor(transmission.SourceOrbitID)
		if state != nil && state.airID == transmission.AirID {
			return state
		}
		return nil
	}
	switch transmission.PlaybackDomainKind {
	case store.PlaybackDomainOrbit:
		return l.orbit(transmission.PlaybackDomainID)
	case store.PlaybackDomainApproach:
		return l.groups[transmission.PlaybackDomainID]
	default:
		return nil
	}
}

func (l *loop) bridgeLegacyTransmission(
	work store.TransmissionSchedulerWork,
	now int64,
) (bool, error) {
	state := l.transmissionDomainState(work.Transmission)
	if state == nil {
		return false, nil
	}
	targets := make([]protocol.NodeID, 0, len(work.Targets))
	for _, target := range work.Targets {
		if target.Status != store.TransmissionTargetAccepted {
			continue
		}
		node := protocol.NodeID(target.Slot)
		if work.Transmission.AirID != "" ||
			work.Transmission.PlaybackDomainKind == store.PlaybackDomainApproach {
			node = compositeID(target.OrbitID, node)
		}
		targets = append(targets, node)
	}
	if len(targets) == 0 {
		return false, nil
	}
	elementID := work.Transmission.ID
	element := session.Element{
		ID: elementID, Kind: session.KindVoice, MediaID: work.Media.ID,
		DurationMS: work.Media.DurationMS, Target: "both", Targets: targets,
		CreatedAt: work.Transmission.AcceptedAt,
	}
	if err := l.st.InsertElement(element); err != nil {
		return false, err
	}
	l.apply(state, state.sess.EnqueueVoice(element))
	claimed, err := l.st.ClaimLegacyTransmission(work.Transmission.ID, elementID, now)
	if err != nil {
		return false, err
	}
	return claimed.Changed, nil
}

func (l *loop) applyTransmissionReceipt(
	key hub.NodeKey,
	credentialTokenHash string,
	transmissionID string,
	generation int64,
	status store.TransmissionTargetStatus,
	reason store.TransmissionReason,
	now int64,
) bool {
	target, err := l.st.TransmissionTargetForReceipt(
		transmissionID, key.Orbit, string(key.Slot), credentialTokenHash,
	)
	if err != nil {
		if !errors.Is(err, store.ErrTransmissionNotFound) {
			l.log.Error("resolve transmission receipt target", "err", err)
		}
		return false
	}
	if target == nil || generation != target.Generation {
		return false
	}
	transition, err := l.st.TransitionTransmissionTarget(
		store.TransitionTransmissionTargetParams{
			TransmissionID: transmissionID,
			OrbitID:        key.Orbit, ActorID: target.ActorID, Slot: string(key.Slot),
			ExpectedRevision: target.Revision, Generation: generation,
			Status: status, ReasonCode: reason,
			OccurredAt: maxCoordinatorTime(now, target.UpdatedAt),
		},
	)
	if err != nil {
		if !errors.Is(err, store.ErrTransmissionStateConflict) {
			l.log.Error("apply transmission receipt", "err", err)
		}
		return false
	}
	if transition.Changed {
		l.signalTransmission(transmissionSignal{transmissionID: transmissionID})
	}
	return transition.Changed
}

func mediaFailureReason(code string) (store.TransmissionReason, bool) {
	reason := store.TransmissionReason(code)
	switch reason {
	case store.TransmissionReasonMediaDownloadFailed,
		store.TransmissionReasonMediaAuthFailed,
		store.TransmissionReasonMediaExpired,
		store.TransmissionReasonHashMismatch,
		store.TransmissionReasonDecodeFailed,
		store.TransmissionReasonDurationMismatch,
		store.TransmissionReasonClockUnsynchronized,
		store.TransmissionReasonStalePlay,
		store.TransmissionReasonDeviceUnavailable,
		store.TransmissionReasonAudioGraphFailed,
		store.TransmissionReasonConnectionLost,
		store.TransmissionReasonCapabilityLost,
		store.TransmissionReasonInterruptCapabilityLost,
		store.TransmissionReasonCancelUnacknowledged,
		store.TransmissionReasonInternalError:
		return reason, true
	default:
		return "", false
	}
}

func (l *loop) handleMediaReady(
	key hub.NodeKey,
	credentialTokenHash string,
	payload *protocol.MediaReadyPayload,
) {
	work, err := l.st.GetTransmissionSchedulerWork(payload.TransmissionID)
	if err != nil {
		return
	}
	now := time.Now().UnixMilli()
	status, reason := store.TransmissionTargetReady, store.TransmissionReason("")
	if work.Scheduler.PrepareDeadlineAt > 0 && now > work.Scheduler.PrepareDeadlineAt {
		status, reason = store.TransmissionTargetMissedNotReady,
			store.TransmissionReasonPrepareDeadline
	} else if payload.DecodedDurationMS != work.Media.DurationMS {
		status, reason = store.TransmissionTargetFailed, store.TransmissionReasonDurationMismatch
	}
	changed := l.applyTransmissionReceipt(
		key, credentialTokenHash, payload.TransmissionID, payload.Generation,
		status, reason, now,
	)
	if changed && status != store.TransmissionTargetReady {
		l.sendTransmissionSafetyCancel(key, work, payload.Generation, reason, false)
	}
}

func (l *loop) handleMediaStarted(
	key hub.NodeKey,
	credentialTokenHash string,
	payload *protocol.MediaStartedPayload,
) {
	work, err := l.st.GetTransmissionSchedulerWork(payload.TransmissionID)
	if err != nil {
		return
	}
	status, reason := store.TransmissionTargetPlaying, store.TransmissionReason("")
	if payload.TFirstSampleCoordMS <= 0 || work.Scheduler.TCoordMS <= 0 {
		status, reason = store.TransmissionTargetFailed, store.TransmissionReasonInternalError
	} else if payload.TFirstSampleCoordMS < work.Scheduler.TCoordMS {
		status, reason = store.TransmissionTargetFailed, store.TransmissionReasonClockUnsynchronized
	} else if work.Scheduler.StartDeadlineCoordMS > 0 &&
		payload.TFirstSampleCoordMS > work.Scheduler.StartDeadlineCoordMS {
		status, reason = store.TransmissionTargetFailed, store.TransmissionReasonStalePlay
	}
	changed := l.applyTransmissionReceipt(
		key, credentialTokenHash, payload.TransmissionID, payload.Generation,
		status, reason, time.Now().UnixMilli(),
	)
	if changed && status != store.TransmissionTargetPlaying {
		l.sendTransmissionSafetyCancel(key, work, payload.Generation, reason, true)
	}
}

func (l *loop) handleMediaEnded(
	key hub.NodeKey,
	credentialTokenHash string,
	payload *protocol.MediaEndedPayload,
) {
	work, err := l.st.GetTransmissionSchedulerWork(payload.TransmissionID)
	if err != nil {
		return
	}
	if payload.Reason != string(store.TransmissionReasonCompleted) ||
		payload.TLastSampleCoordMS < work.Scheduler.TCoordMS+work.Media.DurationMS ||
		payload.TLastSampleCoordMS > work.Scheduler.StartDeadlineCoordMS+
			work.Media.DurationMS+store.TransmissionEndReceiptGraceMS {
		changed := l.applyTransmissionReceipt(
			key, credentialTokenHash, payload.TransmissionID, payload.Generation,
			store.TransmissionTargetFailed, store.TransmissionReasonInternalError,
			time.Now().UnixMilli(),
		)
		if changed {
			l.sendTransmissionSafetyCancel(
				key, work, payload.Generation, store.TransmissionReasonInternalError, true,
			)
		}
		return
	}
	l.applyTransmissionReceipt(
		key, credentialTokenHash, payload.TransmissionID, payload.Generation,
		store.TransmissionTargetPlayed, store.TransmissionReasonCompleted,
		time.Now().UnixMilli(),
	)
}

func (l *loop) handleMediaFailed(
	key hub.NodeKey,
	credentialTokenHash string,
	payload *protocol.MediaFailedPayload,
) {
	reason, ok := mediaFailureReason(payload.Code)
	if !ok {
		reason = store.TransmissionReasonInternalError
	}
	l.applyTransmissionReceipt(
		key, credentialTokenHash, payload.TransmissionID, payload.Generation,
		store.TransmissionTargetFailed, reason, time.Now().UnixMilli(),
	)
}

func (l *loop) handleMediaCancelled(
	key hub.NodeKey,
	credentialTokenHash string,
	payload *protocol.MediaCancelledPayload,
) {
	target, err := l.st.TransmissionTargetForReceipt(
		payload.TransmissionID, key.Orbit, string(key.Slot), credentialTokenHash,
	)
	if err != nil || target == nil || target.Status != store.TransmissionTargetCancelling ||
		payload.Action == "" || payload.Reason != string(target.ReasonCode) {
		return
	}
	work, err := l.st.GetTransmissionSchedulerWork(payload.TransmissionID)
	if err != nil {
		return
	}
	expectedAction := "disarm"
	expectedMainResumed := false
	if target.StartedAt > 0 {
		expectedAction = "fade_stop"
		expectedMainResumed = work.Transmission.EffectiveDelivery ==
			store.TransmissionDeliveryInterrupt
	}
	if payload.Action != expectedAction || payload.MainResumed != expectedMainResumed {
		l.applyTransmissionReceipt(
			key, credentialTokenHash, payload.TransmissionID, payload.Generation,
			store.TransmissionTargetFailed, store.TransmissionReasonInternalError,
			time.Now().UnixMilli(),
		)
		return
	}
	l.applyTransmissionReceipt(
		key, credentialTokenHash, payload.TransmissionID, payload.Generation,
		store.TransmissionTargetCancelled, target.ReasonCode,
		time.Now().UnixMilli(),
	)
}

func (l *loop) handleNodeDND(
	key hub.NodeKey,
	credentialTokenHash string,
	payload *protocol.SetDNDPayload,
) {
	identity, err := l.st.CurrentInstallationTargetForSocket(
		key.Orbit, string(key.Slot), credentialTokenHash,
	)
	if err != nil || identity == nil {
		if err != nil {
			l.log.Error("resolve node DND identity", "err", err)
		}
		return
	}
	mutedUntil := int64(0)
	if payload.MutedUntilCoordMS != nil {
		mutedUntil = *payload.MutedUntilCoordMS
	}
	_, err = l.st.SetNodeDND(store.SetNodeDNDParams{
		OrbitID: identity.OrbitID, ActorID: identity.ActorID, Slot: identity.Slot,
		Mode: store.DNDMode(payload.Mode), MutedUntil: mutedUntil,
		Revision: payload.Revision, UpdatedAt: time.Now().UnixMilli(),
	})
	if err != nil {
		if !errors.Is(err, store.ErrDNDRevisionConflict) {
			l.log.Error("persist node DND", "err", err)
		}
		return
	}
	l.signalTransmission(transmissionSignal{})
}

func (l *loop) handleLegacyTransmissionStarted(
	key hub.NodeKey,
	credentialTokenHash string,
	elementID string,
) {
	if !strings.HasPrefix(elementID, "tr_") {
		return
	}
	target, err := l.st.TransmissionTargetForReceipt(
		elementID, key.Orbit, string(key.Slot), credentialTokenHash,
	)
	if err != nil || target == nil {
		return
	}
	l.applyTransmissionReceipt(
		key, credentialTokenHash, elementID, target.Generation,
		store.TransmissionTargetPlaying, "", time.Now().UnixMilli(),
	)
}

func (l *loop) handleLegacyTransmissionEnded(
	key hub.NodeKey,
	credentialTokenHash string,
	elementID string,
) {
	if !strings.HasPrefix(elementID, "tr_") {
		return
	}
	target, err := l.st.TransmissionTargetForReceipt(
		elementID, key.Orbit, string(key.Slot), credentialTokenHash,
	)
	if err != nil || target == nil {
		return
	}
	l.applyTransmissionReceipt(
		key, credentialTokenHash, elementID, target.Generation,
		store.TransmissionTargetPlayed, store.TransmissionReasonCompleted,
		time.Now().UnixMilli(),
	)
}
