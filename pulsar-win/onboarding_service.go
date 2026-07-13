package main

import (
	"context"
	"errors"
)

// WindowsOnboardingService exposes typed controller hooks without owning any
// Win32 view state. The UI task can call these methods from its dispatcher.
type WindowsOnboardingService struct {
	client     *OnboardingClient
	repository *ProtectedCredentialRepository
	recovery   *RecoveryService
}

func NewWindowsOnboardingService(client *OnboardingClient, repository *ProtectedCredentialRepository, recovery *RecoveryService) (*WindowsOnboardingService, error) {
	if client == nil || repository == nil || recovery == nil || recovery.repository != repository || recovery.client != client {
		return nil, errOnboardingServiceIncoherent
	}
	return &WindowsOnboardingService{client: client, repository: repository, recovery: recovery}, nil
}

var errOnboardingServiceIncoherent = errors.New("onboarding service dependencies are incoherent")

func (s *WindowsOnboardingService) Create(ctx context.Context, title, installationAttemptID string) (CreateOrbitResult, error) {
	result, err := s.client.CreateOrbit(ctx, title, installationAttemptID)
	if err != nil {
		return CreateOrbitResult{}, err
	}
	if err := s.repository.SaveBundle(result.Bundle); err != nil {
		// The caller still receives the one-time material and can explicitly
		// export it; storage failure is not misreported as onboarding success.
		return result, err
	}
	return result, nil
}

func (s *WindowsOnboardingService) Join(ctx context.Context, inviteCode string) (JoinOrbitResult, error) {
	result, err := s.client.JoinOrbit(ctx, inviteCode)
	if err != nil {
		return JoinOrbitResult{}, err
	}
	if err := s.repository.SaveBundle(result.Bundle); err != nil {
		return result, err
	}
	return result, nil
}

func (s *WindowsOnboardingService) Recover(ctx context.Context, input RecoveryInput) (RecoveryResult, error) {
	return s.recovery.Recover(ctx, input)
}

func (s *WindowsOnboardingService) ResumeRecovery(ctx context.Context, actorID int64) (RecoveryResult, error) {
	return s.recovery.Resume(ctx, actorID)
}

func (s *WindowsOnboardingService) RotateRecovery(ctx context.Context, capability ControlCapability) (*RecoveryMaterial, error) {
	material, err := s.client.RotateRecovery(ctx, capability)
	if err != nil {
		return nil, err
	}
	actorID, recoveryID, ok := material.metadata()
	if !ok {
		return nil, errOneTimeMaterialGone
	}
	if err := s.repository.UpdateRecoveryMetadata(s.client.Origin(), actorID, recoveryID); err != nil {
		return material, err
	}
	return material, nil
}

func (s *WindowsOnboardingService) AcknowledgeRecoveryBackup(actorID int64, recoveryID string) error {
	return s.repository.AcknowledgeRecoveryBackup(s.client.Origin(), actorID, recoveryID)
}

func (s *WindowsOnboardingService) TelegramLink(ctx context.Context, capability ControlCapability, desiredRole string) (TelegramLink, error) {
	return s.client.IssueTelegramLink(ctx, capability, desiredRole)
}

func (s *WindowsOnboardingService) DeviceInvite(ctx context.Context, capability ControlCapability, intendedRole string) (DeviceInvite, error) {
	return s.client.IssueDeviceInvite(ctx, capability, intendedRole)
}
