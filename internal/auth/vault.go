package auth

import (
	"context"
	"errors"
	"sync"

	"assistente/internal/credentials"
)

type VaultService struct {
	store       credentials.Store
	onUnlocked  func(dek []byte)
	loadKeyring func() ([]byte, error)
	saveKeyring func([]byte) error

	// runtimeMu protege runtimeUnlocked. O flag reflete o estado de
	// runtime: foi setado por Setup/Unlock e segue verdadeiro até a
	// próxima chamada explícita de Lock (quando existir). Existe para
	// que Status() não dependa exclusivamente da disponibilidade do
	// keyring (M7 do review da Fatia 1): em ambientes onde o keyring
	// pode ficar momentaneamente indisponível (logout do SO, etc.) o
	// app continua tendo a DEK em memória e está, do ponto de vista
	// funcional, "unlocked".
	runtimeMu       sync.RWMutex
	runtimeUnlocked bool
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

	unlocked := s.isRuntimeUnlocked()
	if !unlocked && configured && s.loadKeyring != nil {
		if _, err := s.loadKeyring(); err == nil {
			unlocked = true
		}
	}
	return VaultStatus{Configured: configured, Unlocked: unlocked}, nil
}

func (s *VaultService) isRuntimeUnlocked() bool {
	s.runtimeMu.RLock()
	defer s.runtimeMu.RUnlock()
	return s.runtimeUnlocked
}

func (s *VaultService) markRuntimeUnlocked() {
	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()
	s.runtimeUnlocked = true
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
			result, err = credentials.SetupMasterKeyWrapsForDEK(s.store, masterPassword, existingDEK)
		} else if !credentials.IsKeychainNotFound(loadErr) {
			return "", loadErr
		}
	}
	if result == nil {
		hasCredentials, hasCredErr := credentials.HasAnyStoredCredentials(ctx, s.store)
		if hasCredErr != nil {
			return "", hasCredErr
		}
		if hasCredentials {
			return "", credentials.ErrExistingCredentialsWithoutDEK
		}
		result, err = credentials.SetupMasterKey(s.store, masterPassword)
	}
	if err != nil {
		return "", err
	}
	s.markRuntimeUnlocked()
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
	s.markRuntimeUnlocked()
	if s.onUnlocked != nil {
		s.onUnlocked(dek)
	}
	return nil
}
