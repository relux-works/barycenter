package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
)

const (
	RecoveryLossWarningEN    = "Loss of the sole installation plus an unsaved recovery secret is unrecoverable."
	RecoveryLossWarningRU    = "Потеря единственной установки вместе с несохранённым секретом восстановления необратима."
	RecoveryAbandonWarningEN = "If the server accepted this token from a prior attempt, deleting it means permanent loss of access."
	RecoveryAbandonWarningRU = "Если сервер принял этот токен в предыдущей попытке, его удаление означает безвозвратную потерю доступа."
)

type ControlTokenGenerator interface{ Generate() (string, error) }

type randomControlTokenGenerator struct{ reader io.Reader }

func (g randomControlTokenGenerator) Generate() (string, error) {
	reader := g.reader
	if reader == nil {
		reader = rand.Reader
	}
	value := make([]byte, 32)
	if _, err := io.ReadFull(reader, value); err != nil {
		zeroBytes(value)
		return "", storageError("random token")
	}
	token := hex.EncodeToString(value)
	zeroBytes(value)
	return token, nil
}

type RecoveryState string

const (
	RecoveryStateNone        RecoveryState = "none"
	RecoveryStateUnsent      RecoveryState = "unsent"
	RecoveryStateNeedsSecret RecoveryState = "needs_secret"
	RecoveryStatePending     RecoveryState = "pending"
	RecoveryStatePromoted    RecoveryState = "promoted"
	RecoveryStateLimited     RecoveryState = "limited"
	RecoveryStateCanAbandon  RecoveryState = "can_abandon"
)

type RecoveryResult struct {
	State             RecoveryState
	Bundle            *CredentialBundle
	RetryAfterSeconds int
}

func (r RecoveryResult) String() string {
	return "RecoveryResult{credentials:<redacted>}"
}
func (r RecoveryResult) GoString() string { return r.String() }

type RecoveryService struct {
	repository *ProtectedCredentialRepository
	client     *OnboardingClient
	generator  ControlTokenGenerator
}

func NewRecoveryService(repository *ProtectedCredentialRepository, client *OnboardingClient, generator ControlTokenGenerator) (*RecoveryService, error) {
	if repository == nil || client == nil {
		return nil, errCredentialStorageUnavailable
	}
	if generator == nil {
		generator = randomControlTokenGenerator{}
	}
	return &RecoveryService{repository: repository, client: client, generator: generator}, nil
}

func (s *RecoveryService) Recover(ctx context.Context, input RecoveryInput) (result RecoveryResult, resultErr error) {
	actorID, recoveryID, recoverySecret, err := input.take()
	if err != nil {
		return RecoveryResult{}, err
	}
	defer zeroBytes(recoverySecret)
	origin := s.client.Origin()
	release, err := s.repository.AcquireRecoveryScope(origin, actorID)
	if err != nil {
		return RecoveryResult{}, err
	}
	defer func() {
		if err := release(); err != nil && resultErr == nil {
			resultErr = err
		}
	}()

	existing, err := s.repository.LoadPending(origin, actorID)
	if err != nil {
		return RecoveryResult{}, err
	}
	if existing != nil && !existing.EverSent {
		bundle, err := s.repository.LoadBundle()
		if err != nil {
			return RecoveryResult{}, err
		}
		if bundle != nil && bundle.Control != nil && bundle.Control.ControlToken == existing.PendingControlToken {
			return RecoveryResult{State: RecoveryStateUnsent}, errCredentialStorageConflict
		}
		if existing.RecoveryID != recoveryID {
			return RecoveryResult{State: RecoveryStateUnsent}, nil
		}
		if ctx.Err() != nil {
			_ = s.repository.DeletePendingExact(*existing)
			return RecoveryResult{}, &OnboardingClientError{Kind: ClientErrorCancelled}
		}
		sent, err := s.repository.MarkPendingSent(*existing)
		if err != nil {
			return RecoveryResult{}, err
		}
		contextResult, err := s.client.consumeRecovery(ctx, sent.RecoveryID, recoverySecret, sent.PendingControlToken)
		if err != nil {
			return pendingRecoveryResult(err), err
		}
		return s.promoteLocked(sent, &contextResult, RecoveryStatePromoted)
	}
	if existing != nil && existing.EverSent {
		probe, probeErr := s.probeLocked(ctx, *existing)
		if probe.State == RecoveryStatePromoted || probe.State == RecoveryStateLimited {
			return probe, probeErr
		}
		if probe.State != RecoveryStateNeedsSecret {
			return probe, probeErr
		}
		if existing.RecoveryID != recoveryID {
			return RecoveryResult{State: RecoveryStateNeedsSecret}, nil
		}
		contextResult, err := s.client.consumeRecovery(ctx, existing.RecoveryID, recoverySecret, existing.PendingControlToken)
		if err != nil {
			var clientErr *OnboardingClientError
			if errors.As(err, &clientErr) && clientErr.Kind == ClientErrorCredential {
				return RecoveryResult{State: RecoveryStateCanAbandon}, err
			}
			return pendingRecoveryResult(err), err
		}
		return s.promoteLocked(*existing, &contextResult, RecoveryStatePromoted)
	}
	if existing == nil {
		bundle, err := s.repository.LoadBundle()
		if err != nil {
			return RecoveryResult{}, err
		}
		if bundle != nil && bundle.RecoveryConsumed && bundle.RecoveryID == recoveryID && bundle.CoordinatorOrigin == origin.String() && bundle.Control != nil && bundle.Control.ActorID == actorID {
			return RecoveryResult{State: stateForBundle(*bundle), Bundle: bundle}, nil
		}
	}

	token, err := s.generator.Generate()
	if err != nil || !lowerHexTokenPattern.MatchString(token) {
		return RecoveryResult{}, storageError("generate recovery candidate")
	}
	unsent := PendingRecoveryRecord{CanonicalCoordinatorOrigin: origin.String(), ActorID: actorID, RecoveryID: recoveryID, PendingControlToken: token, EverSent: false}
	if err := s.repository.ReplacePendingUnsent(unsent); err != nil {
		return RecoveryResult{}, err
	}
	if ctx.Err() != nil {
		_ = s.repository.DeletePendingExact(unsent)
		return RecoveryResult{}, &OnboardingClientError{Kind: ClientErrorCancelled}
	}
	sent, err := s.repository.MarkPendingSent(unsent)
	if err != nil {
		return RecoveryResult{}, err
	}
	contextResult, err := s.client.consumeRecovery(ctx, sent.RecoveryID, recoverySecret, sent.PendingControlToken)
	if err != nil {
		return pendingRecoveryResult(err), err
	}
	return s.promoteLocked(sent, &contextResult, RecoveryStatePromoted)
}

func (s *RecoveryService) Resume(ctx context.Context, actorID int64) (result RecoveryResult, resultErr error) {
	origin := s.client.Origin()
	release, err := s.repository.AcquireRecoveryScope(origin, actorID)
	if err != nil {
		return RecoveryResult{}, err
	}
	defer func() {
		if err := release(); err != nil && resultErr == nil {
			resultErr = err
		}
	}()
	record, err := s.repository.LoadPending(origin, actorID)
	if err != nil {
		return RecoveryResult{}, err
	}
	if record == nil {
		return RecoveryResult{State: RecoveryStateNone}, nil
	}
	if !record.EverSent {
		return RecoveryResult{State: RecoveryStateUnsent}, nil
	}
	return s.probeLocked(ctx, *record)
}

func (s *RecoveryService) probeLocked(ctx context.Context, record PendingRecoveryRecord) (RecoveryResult, error) {
	if bundle, err := s.repository.LoadBundle(); err != nil {
		return RecoveryResult{}, err
	} else if bundle != nil && bundle.Control != nil && bundle.Control.ControlToken == record.PendingControlToken {
		promoted, err := s.repository.PromotePending(record, nil)
		if err != nil {
			return RecoveryResult{State: RecoveryStatePending}, err
		}
		if err := s.repository.DeletePendingExact(record); err != nil {
			return RecoveryResult{State: stateForBundle(promoted), Bundle: &promoted}, err
		}
		return RecoveryResult{State: stateForBundle(promoted), Bundle: &promoted}, nil
	}
	capability := ControlCapability{origin: s.client.Origin(), value: ControlCredential{ActorID: record.ActorID, ControlToken: record.PendingControlToken, Context: ControlContextLimited}}
	contextResult, err := s.client.ActorContext(ctx, capability)
	if err == nil {
		return s.promoteLocked(record, &contextResult, RecoveryStatePromoted)
	}
	var clientErr *OnboardingClientError
	if !errors.As(err, &clientErr) {
		return RecoveryResult{State: RecoveryStatePending}, err
	}
	switch clientErr.Kind {
	case ClientErrorCapability:
		return s.promoteLocked(record, nil, RecoveryStateLimited)
	case ClientErrorUnauthorized:
		return RecoveryResult{State: RecoveryStateNeedsSecret}, nil
	case ClientErrorRateLimited:
		return RecoveryResult{State: RecoveryStatePending, RetryAfterSeconds: clientErr.RetryAfterSeconds}, err
	default:
		return RecoveryResult{State: RecoveryStatePending}, err
	}
}

func (s *RecoveryService) promoteLocked(record PendingRecoveryRecord, context *ActorContext, state RecoveryState) (RecoveryResult, error) {
	bundle, err := s.repository.PromotePending(record, context)
	if err != nil {
		return RecoveryResult{}, err
	}
	result := RecoveryResult{State: state, Bundle: &bundle}
	if err := s.repository.DeletePendingExact(record); err != nil {
		return result, err
	}
	return result, nil
}

func (s *RecoveryService) Abandon(ctx context.Context, actorID int64, explicitlyConfirmed bool) (resultErr error) {
	if !explicitlyConfirmed {
		return errRecoveryAbandonNotConfirmed
	}
	origin := s.client.Origin()
	release, err := s.repository.AcquireRecoveryScope(origin, actorID)
	if err != nil {
		return err
	}
	defer func() {
		if err := release(); err != nil && resultErr == nil {
			resultErr = err
		}
	}()
	if ctx.Err() != nil {
		return &OnboardingClientError{Kind: ClientErrorCancelled}
	}
	record, err := s.repository.LoadPending(origin, actorID)
	if err != nil || record == nil {
		return err
	}
	return s.repository.DeletePendingExact(*record)
}

func pendingRecoveryResult(err error) RecoveryResult {
	var clientErr *OnboardingClientError
	if errors.As(err, &clientErr) && clientErr.Kind == ClientErrorRateLimited {
		return RecoveryResult{State: RecoveryStatePending, RetryAfterSeconds: clientErr.RetryAfterSeconds}
	}
	return RecoveryResult{State: RecoveryStatePending}
}

func stateForBundle(bundle CredentialBundle) RecoveryState {
	if bundle.Control != nil && bundle.Control.Context == ControlContextLimited {
		return RecoveryStateLimited
	}
	return RecoveryStatePromoted
}

var errRecoveryAbandonNotConfirmed = errors.New("recovery abandon requires explicit confirmation")
