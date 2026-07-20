// Package moderation coordinates operator decisions across the canonical
// identity, media-lifecycle, transmission-scheduler and live-hub services.
package moderation

import (
	"context"
	"errors"
	"time"

	"relux.works/duet/coordinator/internal/media"
	"relux.works/duet/coordinator/internal/store"
)

type NodeDisconnector func(store.ModerationNodeIdentity)
type CancellationNotifier func(store.CancelTransmissionResult)

type Service struct {
	store      *store.Store
	lifecycle  *media.LifecycleService
	download   *media.DownloadService
	disconnect NodeDisconnector
	notify     CancellationNotifier
	now        func() time.Time
}

func NewService(
	st *store.Store,
	lifecycle *media.LifecycleService,
	download *media.DownloadService,
	disconnect NodeDisconnector,
	notify CancellationNotifier,
) (*Service, error) {
	if st == nil || lifecycle == nil || download == nil {
		return nil, errors.New("invalid moderation service dependencies")
	}
	if disconnect == nil {
		disconnect = func(store.ModerationNodeIdentity) {}
	}
	if notify == nil {
		notify = func(store.CancelTransmissionResult) {}
	}
	return &Service{
		store: st, lifecycle: lifecycle, download: download,
		disconnect: disconnect, notify: notify, now: time.Now,
	}, nil
}

func (service *Service) CreateReport(
	expectedActorID int64,
	bearer string,
	params store.CreateModerationReportParams,
) (store.ModerationReportCreation, error) {
	return service.CreateReportForIdentity(expectedActorID,
		store.Identity{Kind: store.IdentityBearer, Token: bearer}, params)
}

func (service *Service) CreateReportForIdentity(
	expectedActorID int64,
	identity store.Identity,
	params store.CreateModerationReportParams,
) (store.ModerationReportCreation, error) {
	now := service.now().UnixMilli()
	params.CreatedAt = now
	created, err := service.store.CreateModerationReportForIdentity(
		expectedActorID, identity, params,
	)
	if err != nil {
		return store.ModerationReportCreation{}, err
	}
	report := created.Report
	// A Telegram report can use a shared Barycenter receipt whose evidence
	// target belongs to another actor. Key cancellation to the reporting actor,
	// never to that evidence target, so the local protection cannot censor a
	// companion's playback.
	results, err := service.store.CancelTransmissionsForMediaToActor(
		report.MediaID, report.ReporterActorID, store.TransmissionReasonReported, now,
	)
	if err != nil {
		return store.ModerationReportCreation{}, err
	}
	service.notifyCancellations(results)
	return created, nil
}

func (service *Service) BlockReportedSender(
	expectedActorID int64,
	bearer, reportID string,
) (store.TransmissionBlockCreation, error) {
	result, err := service.store.CreateAuthorizedModerationReportBlock(
		expectedActorID, bearer, reportID, service.now().UnixMilli(),
	)
	if err != nil {
		return store.TransmissionBlockCreation{}, err
	}
	results, err := service.store.CancelTransmissionsFromSourceActorToNode(
		result.Report.ReportedActorID, result.Report.TargetOrbitID,
		result.Report.TargetActorID, result.Report.TargetSlot,
		store.TransmissionReasonSenderBlocked,
		service.now().UnixMilli(),
	)
	if err != nil {
		return store.TransmissionBlockCreation{}, err
	}
	service.notifyCancellations(results)
	return result.Block, nil
}

func (service *Service) OpenEvidence(
	ctx context.Context,
	operatorID, token, reportID string,
) (media.ModerationEvidenceDownload, error) {
	return service.download.OpenModerationEvidence(ctx, operatorID, token, reportID)
}

func (service *Service) ApplyDecision(
	ctx context.Context,
	operatorID, token, reportID string,
	action store.ModerationAction,
) (store.ModerationDecision, error) {
	if ctx == nil {
		return store.ModerationDecision{}, errors.New("nil moderation decision context")
	}
	request, err := service.store.BeginModerationDecision(
		operatorID, token, reportID, action, service.now().UnixMilli(),
	)
	if err != nil {
		return store.ModerationDecision{}, err
	}
	if request.Applied {
		return request.Decision, nil
	}
	switch action {
	case store.ModerationActionNoAction:
	case store.ModerationActionDeleteMedia:
		if _, err := service.lifecycle.DeleteForModeration(ctx, request.Report.MediaID); err != nil {
			return store.ModerationDecision{}, err
		}
	case store.ModerationActionDisableActor:
		result, err := service.store.DisableActorForModeration(
			request.Report.ReportedActorID, service.now().UnixMilli(),
		)
		if err != nil {
			return store.ModerationDecision{}, err
		}
		if err := service.cancelDisabledActor(ctx, request.Report.ReportedActorID, result.Nodes); err != nil {
			return store.ModerationDecision{}, err
		}
	case store.ModerationActionDisableOrbit:
		result, err := service.store.DisableOrbitForModeration(
			request.Report.ReportedOrbitID, service.now().UnixMilli(),
		)
		if err != nil {
			return store.ModerationDecision{}, err
		}
		if err := service.cancelDisabledOrbit(ctx, request.Report.ReportedOrbitID, result.Nodes); err != nil {
			return store.ModerationDecision{}, err
		}
	default:
		return store.ModerationDecision{}, store.ErrModerationInvalid
	}
	return service.store.CompleteModerationDecision(
		request.Decision.ID, service.now().UnixMilli(),
	)
}

// ApplyE2EEDecision is a production-dark application seam. Runtime routing is
// intentionally deferred; callers must opt into the E2EE report contract and
// the store still never receives client plaintext or decryption keys.
func (service *Service) ApplyE2EEDecision(
	ctx context.Context,
	operatorID, token, reportID string,
	action store.ModerationAction,
) (store.E2EEModerationDecision, error) {
	if ctx == nil {
		return store.E2EEModerationDecision{}, errors.New("nil E2EE moderation decision context")
	}
	request, err := service.store.BeginE2EEModerationDecision(
		operatorID, token, reportID, action, service.now().UnixMilli(),
	)
	if err != nil {
		return store.E2EEModerationDecision{}, err
	}
	if request.Applied {
		return request.Decision, nil
	}
	if err := ctx.Err(); err != nil {
		return store.E2EEModerationDecision{}, err
	}
	switch action {
	case store.ModerationActionNoAction:
	case store.ModerationActionDeleteMedia:
		object, err := service.store.GetE2EEProtectedObject(request.Report.ProtectedObjectID)
		if err != nil {
			return store.E2EEModerationDecision{}, err
		}
		if _, err := service.store.DeleteE2EEProtectedObjectForModeration(
			operatorID, token, reportID, object.Revision, service.now().UnixMilli(),
		); err != nil {
			return store.E2EEModerationDecision{}, err
		}
	case store.ModerationActionDisableActor:
		result, err := service.store.DisableActorForModeration(
			request.Report.ReportedActorID, service.now().UnixMilli(),
		)
		if err != nil {
			return store.E2EEModerationDecision{}, err
		}
		if err := service.cancelDisabledActor(ctx, request.Report.ReportedActorID, result.Nodes); err != nil {
			return store.E2EEModerationDecision{}, err
		}
	case store.ModerationActionDisableOrbit:
		result, err := service.store.DisableOrbitForModeration(
			request.Report.ReportedOrbitID, service.now().UnixMilli(),
		)
		if err != nil {
			return store.E2EEModerationDecision{}, err
		}
		if err := service.cancelDisabledOrbit(ctx, request.Report.ReportedOrbitID, result.Nodes); err != nil {
			return store.E2EEModerationDecision{}, err
		}
	default:
		return store.E2EEModerationDecision{}, store.ErrModerationInvalid
	}
	return service.store.CompleteE2EEModerationDecision(
		request.Decision.ID, service.now().UnixMilli(),
	)
}

func (service *Service) notifyCancellations(results []store.CancelTransmissionResult) {
	for _, result := range results {
		service.notify(result)
	}
}

func (service *Service) cancelDisabledActor(
	ctx context.Context,
	actorID int64,
	nodes []store.ModerationNodeIdentity,
) error {
	for _, node := range nodes {
		service.disconnect(node)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	now := service.now().UnixMilli()
	results, err := service.store.CancelTransmissionsForSourceActor(
		actorID, store.TransmissionReasonModerationDisabled, now,
	)
	if err != nil {
		return err
	}
	service.notifyCancellations(results)
	for _, node := range nodes {
		if err := ctx.Err(); err != nil {
			return err
		}
		results, err := service.store.CancelTransmissionNode(
			node.OrbitID, node.ActorID, node.Slot,
			store.TransmissionReasonModerationDisabled, service.now().UnixMilli(),
		)
		if err != nil {
			return err
		}
		service.notifyCancellations(results)
	}
	return nil
}

func (service *Service) cancelDisabledOrbit(
	ctx context.Context,
	orbitID int64,
	nodes []store.ModerationNodeIdentity,
) error {
	for _, node := range nodes {
		service.disconnect(node)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	results, err := service.store.CancelTransmissionsForSourceOrbit(
		orbitID, store.TransmissionReasonModerationDisabled, service.now().UnixMilli(),
	)
	if err != nil {
		return err
	}
	service.notifyCancellations(results)
	for _, node := range nodes {
		if err := ctx.Err(); err != nil {
			return err
		}
		results, err := service.store.CancelTransmissionNode(
			node.OrbitID, node.ActorID, node.Slot,
			store.TransmissionReasonModerationDisabled, service.now().UnixMilli(),
		)
		if err != nil {
			return err
		}
		service.notifyCancellations(results)
	}
	return nil
}
