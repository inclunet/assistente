package auth

import (
	"context"
	"errors"
	"sync"

	"assistente/internal/credentials"
)

// persistDEKFunc é a assinatura usada para gravar a DEK no keychain
// de forma consistente com os wraps (ver `credentials.PersistDEKConsistent`).
// Para forçar sobrescrita deliberada, use `forceSaveKeyring` separado.
type persistDEKFunc func(ctx context.Context, store credentials.Store, dek []byte) error

type VaultService struct {
	store            credentials.Store
	onUnlocked       func(dek []byte)
	loadKeyring      func() ([]byte, error)
	persistDEK       persistDEKFunc // gravação consistente; recusa sobrescrita
	forceSaveKeyring func([]byte) error // sobrescrita explícita (recovery confirmado)

	// runtimeMu protege runtimeUnlocked. O flag é setado por
	// Setup/Unlock (cofre acabou de ser configurado/aberto) e zerado
	// por Lock() (logout, lock explícito). Existe para que Status()
	// não dependa exclusivamente da disponibilidade do keyring
	// (M7 do review da Fatia 1): em ambientes onde o keyring pode
	// ficar momentaneamente indisponível (logout do SO, etc.) o app
	// continua tendo a DEK em memória e está, do ponto de vista
	// funcional, "unlocked".
	//
	// IMPORTANTE (P0-2 do re-review da Fatia 1): a flag é sinal
	// POSITIVO ("vault foi destravado nesta sessão"), nunca um
	// curto-circuito que faz Status devolver `unlocked: true` sem
	// confirmar que a DEK ainda existe. Status sempre tenta o
	// keyring antes de aceitar a flag como prova.
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
		persistDEK: func(ctx context.Context, store credentials.Store, dek []byte) error {
			return credentials.PersistDEKConsistent(ctx, store, dek, credentials.LoadDEKFromKeychain, nil)
		},
		forceSaveKeyring: nil, // só populado em testes ou via SetForceSaveKeyring
	}
}

// Status reporta o estado atual do cofre.
//
// Algoritmo (P0-2 do re-review da Fatia 1):
//
//  1. Sempre consulta o keyring quando o cofre está configurado.
//     `keyringOK` é a fonte autoritativa: a DEK está acessível AGORA.
//  2. `runtimeUnlocked` é tratado como sinal POSITIVO de fallback —
//     destravamos nesta sessão e ainda confiamos na DEK em memória
//     mesmo que o keyring fique momentaneamente fora do ar (logout
//     do SO, troca de keyring no GNOME, etc.).
//
// Antes desta correção, a flag curto-circuitava o keyring: bastava
// um Setup/Unlock prévio para Status devolver `unlocked: true` para
// sempre — mesmo se a DEK fosse perdida. A regressão era invisível
// porque a flag nunca era zerada (M7 corrigia "keyring sumiu mas
// DEK em memória OK"; aqui restauramos a checagem positiva sem
// regressão).
func (s *VaultService) Status(ctx context.Context) (VaultStatus, error) {
	if s == nil || s.store == nil {
		return VaultStatus{}, credentials.ErrStoreNotReady
	}
	configured, err := s.store.HasKeyWrap(ctx, credentials.KeyWrapKindMaster)
	if err != nil {
		return VaultStatus{}, err
	}

	keyringOK := false
	if configured && s.loadKeyring != nil {
		if dek, loadErr := s.loadKeyring(); loadErr == nil && len(dek) > 0 {
			keyringOK = true
		}
	}
	unlocked := keyringOK || (configured && s.isRuntimeUnlocked())

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

// Lock zera a flag de runtime. É a contraparte de
// `markRuntimeUnlocked` e deve ser chamada em qualquer ponto onde a
// sessão termina (Logout) ou onde o cofre é trancado explicitamente.
//
// NÃO apaga a DEK do keyring — esse é trabalho do caller (em Logout
// usamos `clearAuthRefreshToken` para o token e o cofre só some do
// keyring se o usuário rodar `Reset` ou trocar a senha mestre).
// Apenas marca a sessão de runtime como "destravada por nada", para
// que Status() volte a depender exclusivamente do keyring.
func (s *VaultService) Lock() {
	if s == nil {
		return
	}
	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()
	s.runtimeUnlocked = false
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

// Unlock recupera a DEK pela senha mestre / recovery key e a deixa
// disponível para a sessão (em runtime e, se possível, no keychain).
//
// CONTRATO DE INVARIANTE (AEP-0061):
//
//   - Se o keychain está vazio, grava a DEK desembrulhada nele
//     (caminho típico de "primeira execução em nova máquina" ou após
//     limpeza do keychain).
//
//   - Se o keychain já tem uma DEK e ela bate com a desembrulhada,
//     é no-op (idempotente).
//
//   - Se o keychain tem uma DEK DIFERENTE da desembrulhada, retorna
//     `credentials.ErrDEKWouldOverwrite` SEM sobrescrever. Esse caso é
//     o cenário do incidente AEP-0061: o keychain está com uma DEK_Y
//     que provavelmente cifrou créditos novos; sobrescrever por DEK_X
//     do wrap tornaria essas creds novas ilegíveis. O caller (UI)
//     deve confirmar com o usuário e usar `UnlockOverwriteKeychain`
//     se a sobrescrita for desejada.
//
// Em ambos os casos de sucesso, a DEK fica disponível em runtime via
// `onUnlocked` para que `credentials.Manager` possa decifrar creds
// nesta sessão, mesmo se a gravação no keychain tiver sido recusada.
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
	if s.persistDEK != nil {
		if err := s.persistDEK(ctx, s.store, dek); err != nil {
			if !errors.Is(err, credentials.ErrDEKWouldOverwrite) {
				return err
			}
			// Divergência detectada: NÃO sobrescrevemos o keychain. A
			// sessão fica unlocked em runtime mesmo assim — o caller
			// pode operar com as creds que decifram com a DEK do
			// wrap, e a UI deve oferecer recovery deliberada via
			// UnlockOverwriteKeychain.
		}
	}
	s.markRuntimeUnlocked()
	if s.onUnlocked != nil {
		s.onUnlocked(dek)
	}
	return nil
}

// UnlockOverwriteKeychain desembrulha a DEK pela senha mestre / recovery
// e SOBRESCREVE incondicionalmente a DEK do keychain, aceitando que
// credenciais cifradas com a DEK anterior fiquem ilegíveis. Use SOMENTE
// após confirmação explícita do usuário (UI mostrando o impacto).
func (s *VaultService) UnlockOverwriteKeychain(ctx context.Context, kind, secret string) error {
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
	if err := credentials.OverwriteKeychainDEK(ctx, s.store, dek, s.forceSaveKeyring); err != nil {
		return err
	}
	s.markRuntimeUnlocked()
	if s.onUnlocked != nil {
		s.onUnlocked(dek)
	}
	return nil
}
