// Package historyactions is the transport-neutral command layer behind the
// Phase 1 history projection. It performs only orchestration: every mutation
// is reauthorized by the service that owns its policy and durable state.
package historyactions

import (
	"errors"

	"relux.works/duet/coordinator/internal/media"
	"relux.works/duet/coordinator/internal/moderation"
	"relux.works/duet/coordinator/internal/store"
)

var ErrActionUnavailable = errors.New("history action is unavailable")
var ErrServiceUnavailable = errors.New("history action owner service is unavailable")

type Service struct {
	store      *store.Store
	lifecycle  *media.LifecycleService
	moderation *moderation.Service
}

func NewService(st *store.Store, lifecycle *media.LifecycleService, moderationService *moderation.Service) (*Service, error) {
	if st == nil {
		return nil, errors.New("invalid history action service dependencies")
	}
	return &Service{store: st, lifecycle: lifecycle, moderation: moderationService}, nil
}

type Actor struct {
	ExpectedActorID int64
	Identity        store.Identity
}

func (service *Service) item(actor Actor, historyItemID string, now int64) (store.HistoryQueryItem, error) {
	return service.store.GetAuthorizedHistoryItem(actor.ExpectedActorID, actor.Identity, historyItemID, now)
}

type ReplayParams struct {
	Actor              Actor
	HistoryItemID      string
	IdempotencyKeyHash string
	RequestHash        string
	AudienceKind       store.TransmissionAudienceKind
	Selectors          []store.TransmissionAudienceSelector
	OriginKind         store.TransmissionOriginKind
	IncludeOrigin      bool
	RequestedDelivery  store.TransmissionDelivery
	AcceptedAt         int64
	Availability       []store.TransmissionTargetAvailability
	Confirmation       *store.ConfirmTransmissionFallback
	ChallengeTokenHash string
}

// Replay creates a fresh transmission acceptance. The caller cannot select a
// media ID, acceptance timestamp from the old row, or an old target snapshot.
func (service *Service) Replay(params ReplayParams) (store.ResolvedTransmissionCreation, error) {
	item, err := service.item(params.Actor, params.HistoryItemID, params.AcceptedAt)
	if err != nil {
		return store.ResolvedTransmissionCreation{}, err
	}
	create := store.CreateResolvedTransmissionParams{
		ExpectedActorID:    params.Actor.ExpectedActorID,
		IdempotencyKeyHash: params.IdempotencyKeyHash,
		RequestHash:        params.RequestHash,
		MediaID:            item.Media.ID,
		AudienceKind:       params.AudienceKind,
		Selectors:          params.Selectors,
		OriginKind:         params.OriginKind,
		IncludeOrigin:      params.IncludeOrigin,
		RequestedDelivery:  params.RequestedDelivery,
		AcceptedAt:         params.AcceptedAt,
		Availability:       params.Availability,
		Confirmation:       params.Confirmation,
		ChallengeTokenHash: params.ChallengeTokenHash,
	}
	if params.Actor.Identity.Kind == store.IdentityBearer {
		create.Bearer = params.Actor.Identity.Token
	} else {
		create.Identity = params.Actor.Identity
	}
	return service.store.CreateResolvedTransmission(create)
}

func (service *Service) Delete(actor Actor, historyItemID string, now int64) (store.MediaItem, error) {
	if service.lifecycle == nil {
		return store.MediaItem{}, ErrServiceUnavailable
	}
	item, err := service.item(actor, historyItemID, now)
	if err != nil {
		return store.MediaItem{}, err
	}
	if !item.CanDelete && item.Media.Status != store.MediaStatusDeleted {
		return store.MediaItem{}, ErrActionUnavailable
	}
	return service.lifecycle.DeleteAuthorizedForIdentity(
		actor.ExpectedActorID, actor.Identity, item.Media.ID)
}

func (service *Service) Report(actor Actor, historyItemID string, params store.CreateModerationReportParams, now int64) (store.ModerationReportCreation, error) {
	if service.moderation == nil {
		return store.ModerationReportCreation{}, ErrServiceUnavailable
	}
	item, err := service.item(actor, historyItemID, now)
	if err != nil {
		return store.ModerationReportCreation{}, err
	}
	if !item.CanReport {
		return store.ModerationReportCreation{}, ErrActionUnavailable
	}
	params.MediaID = item.Media.ID
	return service.moderation.CreateReportForIdentity(actor.ExpectedActorID, actor.Identity, params)
}

type BlockKind string

const (
	BlockActor BlockKind = "block_actor"
	BlockOrbit BlockKind = "block_orbit"
)

type BlockParams struct {
	Actor              Actor
	HistoryItemID      string
	Kind               BlockKind
	IdempotencyKeyHash string
	RequestHash        string
	CreatedAt          int64
}

func (service *Service) Block(params BlockParams) (store.PublicTransmissionBlock, error) {
	item, err := service.item(params.Actor, params.HistoryItemID, params.CreatedAt)
	if err != nil {
		return store.PublicTransmissionBlock{}, err
	}
	var subjectKind store.BlockedSubjectKind
	var subjectID int64
	var ownerScope store.BlockOwnerScope
	allowed := false
	switch params.Kind {
	case BlockActor:
		allowed = item.CanBlockActor
		ownerScope = store.BlockOwnerActor
		if params.Actor.Identity.Kind == store.IdentityTelegram {
			ownerScope = store.BlockOwnerOrbit
		}
		subjectKind, subjectID = store.BlockedSubjectActor, item.SourceActorID
	case BlockOrbit:
		allowed = item.CanBlockOrbit
		subjectKind, subjectID, ownerScope = store.BlockedSubjectOrbit, item.SourceOrbitID, store.BlockOwnerOrbit
	default:
		return store.PublicTransmissionBlock{}, ErrActionUnavailable
	}
	subjectRef := ""
	if !allowed {
		// Once a block exists the read model intentionally replaces block_* with
		// unblock. Preserve retries by finding only the caller's exact active
		// owner-service result; this does not authorize a new block.
		blocks, listErr := service.store.AuthorizedListTransmissionBlocksForIdentity(
			params.Actor.ExpectedActorID, params.Actor.Identity)
		if listErr != nil {
			return store.PublicTransmissionBlock{}, listErr
		}
		for _, block := range blocks {
			matches := block.OwnerScope == ownerScope && block.SubjectKind == subjectKind
			if subjectKind == store.BlockedSubjectActor {
				matches = matches && block.Internal.BlockedActorID == subjectID
			} else {
				matches = matches && block.Internal.BlockedOrbitID == subjectID
			}
			if matches {
				subjectRef = block.SubjectRef
				break
			}
		}
		if subjectRef == "" {
			return store.PublicTransmissionBlock{}, ErrActionUnavailable
		}
	}
	if subjectRef == "" {
		ref, mintErr := service.store.MintTransmissionSubjectReferenceForIdentity(
			params.Actor.ExpectedActorID, params.Actor.Identity, subjectKind, subjectID, params.CreatedAt)
		if mintErr != nil {
			return store.PublicTransmissionBlock{}, mintErr
		}
		subjectRef = ref.PublicID
	}
	return service.store.AuthorizedCreateTransmissionBlock(store.AuthorizedCreateBlockParams{
		ExpectedActorID:    params.Actor.ExpectedActorID,
		Identity:           params.Actor.Identity,
		OwnerScope:         ownerScope,
		SubjectRef:         subjectRef,
		IdempotencyKeyHash: params.IdempotencyKeyHash,
		RequestHash:        params.RequestHash,
		CreatedAt:          params.CreatedAt,
	})
}
