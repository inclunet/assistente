package commandconfig

import (
	"context"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandautomation"
	"assistente/internal/commanddecision"
	"assistente/internal/commandledger"
	"assistente/internal/commandsecurity"
)

// MutationServiceConfig só é construído no host. Nenhuma dessas portas é
// desserializável como autoridade do candidato, inclusive Source/Presenter.
// Automation é a mesma store SQL usada pelo fluxo de regrant. Ela é opcional
// para manter as mutações não relacionadas a eventos compatíveis; RegrantEventRule
// falha fechado quando não está montada.
type MutationServiceConfig struct {
	Store      *Store
	Automation *commandautomation.Store
	Sessions   interface {
		AuthenticateLocalAccess(context.Context, string) (auth.LocalSessionPrincipal, error)
	}
	Epochs      *commandsecurity.EpochService
	Receipts    *commanddecision.Store
	Keys        commandledger.FingerprintKeyProvider
	KeyVersion  string
	DecisionTTL time.Duration
	Authorize   func(context.Context, auth.LocalSessionPrincipal, Scope, Operation) error
	Validate    MutationValidator
	// Version cobre catálogo/defaults e política do host, além do CAS SQLite.
	Version      func(context.Context) (string, error)
	Render       func(MutationDiff) (string, error)
	OnMutationTx MutationTxHook
	// BeforeCommit invalida estado volátil do host no handoff exclusivo,
	// imediatamente antes do writer. A porta não pode adquirir o gate, abrir
	// UI ou fazer I/O bloqueante; erro aborta o commit.
	// ImportBatch chama uma vez com UserID e WorkspaceID=nil para invalidar
	// o mapa inteiro do usuário, não apenas o primeiro escopo do lote.
	BeforeCommit func(context.Context, Scope) error
}
type MutationService struct{ config MutationServiceConfig }

func NewMutationService(c MutationServiceConfig) (*MutationService, error) {
	if c.Store == nil || c.Sessions == nil || c.Epochs == nil || c.Receipts == nil || c.Keys == nil || c.KeyVersion == "" || c.DecisionTTL <= 0 || c.Authorize == nil || c.Validate == nil || c.Version == nil || c.Render == nil || c.OnMutationTx == nil {
		return nil, ErrInvalid
	}
	return &MutationService{config: c}, nil
}

// Apply não aceita user/session, geração ou receipt do cliente. O workspace é
// candidato: Authorize tem de reconsultar acesso real antes de qualquer leitura.
// Todas as mutações deste serviço exigem decisão, inclusive as vindas de agente.
func (s *MutationService) Apply(ctx context.Context, token string, workspace *string, intent MutationIntent) (MutationDiff, error) {
	return s.applyPrepared(ctx, token, workspace, intent.Operation, func(ctx context.Context, scope Scope) (*PreparedMutation, error) {
		return s.config.Store.PrepareMutation(ctx, scope, intent, s.config.Validate)
	})
}

// Preview autentica, autoriza e prepara o diff exato sob o epoch atual, mas
// não cria receipt, não chama presenter e não escreve no banco. A operação
// efetiva deve voltar a passar por Apply para obter a decisão e o CAS.
func (s *MutationService) Preview(ctx context.Context, token string, workspace *string, intent MutationIntent) (MutationDiff, error) {
	if s == nil || ctx == nil {
		return MutationDiff{}, ErrInvalid
	}
	workspace = cloneScope(Scope{WorkspaceID: workspace}).WorkspaceID
	var principal auth.LocalSessionPrincipal
	var scope Scope
	epoch, err := s.config.Epochs.CaptureAuthenticated(ctx, func(ctx context.Context) (string, string, error) {
		p, err := s.config.Sessions.AuthenticateLocalAccess(ctx, token)
		if err != nil {
			return "", "", err
		}
		principal = p
		scope = Scope{UserID: p.UserID, WorkspaceID: workspace}
		if !validScope(scope) {
			return "", "", ErrInvalid
		}
		if err := s.config.Authorize(ctx, p, cloneScope(scope), intent.Operation); err != nil {
			return "", "", err
		}
		return p.UserID, p.SessionID, nil
	})
	if err != nil {
		return MutationDiff{}, err
	}
	var prepared *PreparedMutation
	err = s.config.Epochs.Admit(ctx, epoch, func(ctx context.Context) error {
		current, err := s.config.Sessions.AuthenticateLocalAccess(ctx, token)
		if err != nil {
			return err
		}
		if current.UserID != principal.UserID || current.SessionID != principal.SessionID {
			return ErrStale
		}
		return s.config.Authorize(ctx, current, cloneScope(scope), intent.Operation)
	}, func() error {
		version, err := s.config.Version(ctx)
		if err != nil {
			return err
		}
		if version == "" {
			return ErrInvalid
		}
		prepared, err = s.config.Store.PrepareMutation(ctx, scope, intent, s.config.Validate)
		return err
	})
	if err != nil || prepared == nil {
		return MutationDiff{}, err
	}
	return prepared.Diff(), nil
}

// applyPrepared é o único percurso de autenticação/decisão/commit também para
// upgrades e rebase. A função de preparação é interna, nunca recebida da UI.
func (s *MutationService) applyPrepared(ctx context.Context, token string, workspace *string, operation Operation, prepare func(context.Context, Scope) (*PreparedMutation, error)) (MutationDiff, error) {
	return s.applyPreparedWithRevalidation(ctx, token, workspace, operation, prepare, nil)
}

func (s *MutationService) applyPreparedWithRevalidation(ctx context.Context, token string, workspace *string, operation Operation, prepare func(context.Context, Scope) (*PreparedMutation, error), revalidateImport func(context.Context) error) (MutationDiff, error) {
	if s == nil || ctx == nil {
		return MutationDiff{}, ErrInvalid
	}
	workspace = cloneScope(Scope{WorkspaceID: workspace}).WorkspaceID
	var principal auth.LocalSessionPrincipal
	var scope Scope
	epoch, err := s.config.Epochs.CaptureAuthenticated(ctx, func(ctx context.Context) (string, string, error) {
		p, e := s.config.Sessions.AuthenticateLocalAccess(ctx, token)
		if e != nil {
			return "", "", e
		}
		principal = p
		scope = Scope{UserID: p.UserID, WorkspaceID: workspace}
		if !validScope(scope) {
			return "", "", ErrInvalid
		}
		if e := s.config.Authorize(ctx, p, cloneScope(scope), operation); e != nil {
			return "", "", e
		}
		return p.UserID, p.SessionID, nil
	})
	if err != nil {
		return MutationDiff{}, err
	}
	var p *PreparedMutation
	var version string
	revalidate := func(ctx context.Context) error {
		current, e := s.config.Sessions.AuthenticateLocalAccess(ctx, token)
		if e != nil {
			return e
		}
		if current.UserID != principal.UserID || current.SessionID != principal.SessionID {
			return ErrStale
		}
		return s.config.Authorize(ctx, current, cloneScope(scope), operation)
	}
	err = s.config.Epochs.Admit(ctx, epoch, revalidate, func() error {
		v, e := s.config.Version(ctx)
		if e != nil {
			return e
		}
		if v == "" {
			return ErrInvalid
		}
		version = v
		p, e = prepare(ctx, scope)
		if e == nil && (p == nil || p.store != s.config.Store || p.diff.Operation != operation) {
			return ErrInvalid
		}
		return e
	})
	if err != nil {
		return MutationDiff{}, err
	}
	watch, release, err := s.config.Epochs.WatchEpoch(ctx, epoch)
	if err != nil {
		return MutationDiff{}, err
	}
	defer release()
	confirmed, err := s.config.Store.ConfirmMutation(watch, p, epoch, s.config.Receipts, s.config.KeyVersion, s.config.Keys, time.Now().Add(s.config.DecisionTTL), s.config.Render)
	if err != nil {
		return MutationDiff{}, err
	}
	err = s.config.Epochs.AdmitMutation(ctx, epoch, func(ctx context.Context) error {
		if e := revalidate(ctx); e != nil {
			return e
		}
		v, e := s.config.Version(ctx)
		if e != nil {
			return e
		}
		if v != version {
			return ErrStale
		}
		if err := s.config.Validate(ctx, cloneConfigSnapshot(p.after)); err != nil {
			return err
		}
		if revalidateImport != nil {
			return revalidateImport(ctx)
		}
		return nil
	}, func() error {
		if s.config.BeforeCommit != nil {
			if err := s.config.BeforeCommit(ctx, cloneScope(scope)); err != nil {
				return err
			}
		}
		return s.config.Store.CommitConfirmedMutation(ctx, confirmed, epoch, s.config.OnMutationTx)
	})
	if err != nil {
		return MutationDiff{}, err
	}
	return p.Diff(), nil
}
