package auth

import (
	"context"
	"errors"

	"assistente/internal/credentials"
)

type VaultService struct {
	store       credentials.Store
	onUnlocked  func(dek []byte)
	loadKeyring func() ([]byte, error)
	saveKeyring func([]byte) error
}

type VaultStatus struct {
	Configured bool `json:"configured"`
	Unlocked   bool `json:"unlocked"`
}

func NewVaultService(store credentials.Store, onUnlocked func(dek []byte)) *VaultService {
	return &VaultService{
		store:       store,
		onUnlocked:  onUnlocked,
		loadKeyring: credentials.LoadDEKFromKeychain,
		saveKeyring: credentials.SaveDEKToKeychain,
	}
}

func (s *VaultService) Status(ctx context.Context) (VaultStatus, error) {
	if s == nil || s.store == nil {
		return VaultStatus{}, credentials.ErrStoreNotReady
	}
	configured, err := s.store.HasKeyWrap(ctx, credentials.KeyWrapKindMaster)
	if err != nil {
		return VaultStatus{}, err
	}

	unlocked := false
	if configured && s.loadKeyring != nil {
		if _, err := s.loadKeyring(); err == nil {
			unlocked = true
		}
	}
	return VaultStatus{Configured: configured, Unlocked: unlocked}, nil
}

func (s *VaultService) Setup(ctx context.Context, masterPassword string) (string, error) {
	if s == nil || s.store == nil {
		return "", credentials.ErrStoreNotReady
	}
	configured, hasWrapErr := s.store.HasKeyWrap(ctx, credentials.KeyWrapKindMaster)
	if hasWrapErr != nil {
		return "", hasWrapErr
	}
	if configured {
		return "", errors.New("cofre já configurado")
	}

	var result *credentials.MasterKeySetupResult
	var err error
	if s.loadKeyring != nil {
		existingDEK, loadErr := s.loadKeyring()
		if loadErr == nil {
			if len(existingDEK) == 0 {
				return "", errors.New("DEK existente no keyring está vazia")
			}
			result, err = credentials.SetupMasterKeyForDEK(s.store, masterPassword, existingDEK)
		} else if !credentials.IsKeychainNotFound(loadErr) {
			return "", loadErr
		}
	}
	if result == nil {
		result, err = credentials.SetupMasterKey(s.store, masterPassword)
	}
	if err != nil {
		return "", err
	}
	if s.onUnlocked != nil {
		s.onUnlocked(result.DEK)
	}
	return result.RecoveryKey, nil
}

func (s *VaultService) Unlock(ctx context.Context, kind, secret string) error {
	if s == nil || s.store == nil {
		return credentials.ErrStoreNotReady
	}
	if kind == "" {
		kind = credentials.KeyWrapKindMaster
	}
	if kind != credentials.KeyWrapKindMaster && kind != credentials.KeyWrapKindRecovery {
		return errors.New("tipo de desbloqueio inválido")
	}

	dek, err := credentials.UnlockDEKWithSecret(s.store, kind, secret)
	if err != nil {
		return err
	}
	if s.saveKeyring != nil {
		if err := s.saveKeyring(dek); err != nil {
			return err
		}
	}
	if s.onUnlocked != nil {
		s.onUnlocked(dek)
	}
	return nil
}
