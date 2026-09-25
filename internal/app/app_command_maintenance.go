package app

import (
	"context"
	"errors"
	"math"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandactivation"
	"assistente/internal/commanddecision"
	"assistente/internal/commandjobactivation"
	"assistente/internal/commandjobevents"
	"assistente/internal/commandledger"
	"assistente/internal/commandmaintenance"
	"assistente/internal/commandsecurity"
	"assistente/internal/config"
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/jobs"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Montagem única por Manager/banco. Não instala timer: ConfigureCommandMaintenance
// transfere a cadência já pertencente ao Manager para os adapters reais.
type appCommandMaintenance struct {
	manager     *jobs.Manager
	db          *gorm.DB
	sessions    *auth.SessionService
	credentials *credentials.Manager
	version     string
	consumer    *commandjobactivation.Consumer
	ports       commandmaintenance.Ports
}

type appCommandJobAuthority struct {
	epoch   commandsecurity.EpochSnapshot
	watch   context.Context
	release func()
}

func (a *App) configureCommandMaintenance(ctx context.Context) error {
	if a == nil || ctx == nil || a.jobMgr == nil {
		return commandmaintenance.ErrInvalid
	}
	a.commandMaintenanceBuild.Lock()
	defer a.commandMaintenanceBuild.Unlock()
	a.authMu.RLock()
	manager, sessions, version, readyErr := a.credMgr, a.sessionSvc, a.commandStorageVersion, a.commandStorageErr
	a.authMu.RUnlock()
	db := database.DB()
	if db == nil || manager == nil || sessions == nil || readyErr != nil || version == "" {
		return commandjobactivation.ErrUnavailable
	}
	current := a.commandMaintenance.Load()
	if current != nil {
		if current.manager == a.jobMgr && current.db == db && current.sessions == sessions && current.credentials == manager && current.version == version {
			return ctx.Err()
		}
		if current.manager != a.jobMgr || current.db != db {
			return commandmaintenance.ErrInvalid
		}
	}
	core, err := a.commandSecurityService()
	if err != nil {
		return err
	}
	if err := core.BindInstance(ctx, db); err != nil {
		return err
	}
	proof, err := core.RestartProof(ctx, db)
	if err != nil {
		return err
	}
	drained := commandsecurity.FromRestartProof(proof)
	keys, err := commandledger.NewCredentialKeyProvider(manager)
	if err != nil {
		return err
	}
	// Verifica a chave fora do gate; o provider consulta somente cache carregado.
	key, err := keys(ctx, "command-request-hmac:"+version)
	clear(key)
	if err != nil {
		return err
	}
	settings, err := config.GetMaintenance()
	if err != nil {
		return err
	}
	if settings.CommandJobActivationLeaseSeconds <= 0 || int64(settings.CommandJobActivationLeaseSeconds) > math.MaxInt64/int64(time.Second) || settings.CommandActivationTerminalRetentionDays <= 0 || int64(settings.CommandActivationTerminalRetentionDays) > math.MaxInt64/int64(24*time.Hour) {
		return commandmaintenance.ErrInvalid
	}
	mounted := &appCommandMaintenance{manager: a.jobMgr, db: db, sessions: sessions, credentials: manager, version: version}
	consumer, err := commandjobactivation.New(db, a.commandGate, commandjobactivation.Ports{
		WithContext: a.withCommandJobContext,
		Authorize: func(ctx context.Context, tx *gorm.DB, fact commandjobevents.Fact, workspace *string) (commandactivation.Owner, error) {
			return a.authorizeCommandJob(ctx, tx, mounted, fact, workspace)
		},
		Layer:     a.commandJobLayer,
		Condition: a.commandJobCondition,
		Runtime: func(ctx context.Context, tx *gorm.DB, fact commandjobevents.Fact) (commandjobactivation.RuntimeIdentity, error) {
			identity, err := mounted.manager.CommandRuntimeIdentity(ctx, tx, fact)
			if errors.Is(err, jobs.ErrCommandMaintenanceUnavailable) {
				return commandjobactivation.RuntimeIdentity{}, commandjobactivation.ErrUnavailable
			}
			return identity, err
		},
		Keys: keys, KeyVersion: version,
	}, time.Duration(settings.CommandJobActivationLeaseSeconds)*time.Second, time.Duration(settings.CommandActivationTerminalRetentionDays)*24*time.Hour, time.Now)
	if err != nil {
		return err
	}
	mounted.consumer = consumer
	deliveryID, err := uuid.NewV7()
	if err != nil {
		return err
	}
	outbox, err := commandjobactivation.NewMaintenanceOutboxAdapter(consumer, deliveryID.String())
	if err != nil {
		return err
	}
	claims, err := commandjobactivation.NewMaintenanceRecoveryAdapter(consumer)
	if err != nil {
		return err
	}
	heartbeat, err := commandjobactivation.NewMaintenanceHeartbeatAdapter(consumer)
	if err != nil {
		return err
	}
	decisions, err := commanddecision.New(db, &commandDecisionPresenter{}, time.Now)
	if err != nil {
		return err
	}
	decisionRecovery, err := commanddecision.NewCoordinatorRecovery(decisions, drained)
	if err != nil {
		return err
	}
	ledger, err := commandledger.New(db, time.Now)
	if err != nil {
		return err
	}
	invocationRecovery, err := commandledger.NewCoordinatorRecovery(ledger, drained)
	if err != nil {
		return err
	}
	invocations, err := ledger.NewMaintenanceService()
	if err != nil {
		return err
	}
	invocationRetention, err := invocations.CoordinatorRetention()
	if err != nil {
		return err
	}
	activationStore, err := commandactivation.NewStore(db)
	if err != nil {
		return err
	}
	activations, err := activationStore.NewMaintenanceService()
	if err != nil {
		return err
	}
	activationRetention, err := activations.CoordinatorRetention()
	if err != nil {
		return err
	}
	mounted.ports = commandmaintenance.Ports{Heartbeat: heartbeat, Outbox: outbox, Decisions: decisionRecovery, Invocations: invocationRecovery, Claims: claims, InvocationDB: invocationRetention, Activations: activationRetention}
	// A migração instala o schema, não inventa uma política de replay. Publica
	// o primeiro epoch antes de Start; epochs/deadlines existentes são imutáveis.
	events := commandjobevents.NewStore(db)
	if !events.Ready(ctx) {
		if _, err := events.EnsureReplayPolicyEpoch(ctx, commandjobevents.ProducerType, time.Now().UTC(), commandjobevents.DefaultReplayHorizon); err != nil {
			return err
		}
	}
	// Registre o join antes de habilitar a cadência, para que nenhum caminho de
	// shutdown libere a prova enquanto a manutenção ainda usa o banco. O
	// registro, a configuração e a publicação são uma única transação do gate:
	// uma falha remove o drain e CloseAndDrain não pode observar um mounted
	// parcialmente publicado.
	commit := func() error {
		if current == nil {
			if err := mounted.manager.ConfigureCommandMaintenance(mounted.ports); err != nil {
				return err
			}
		} else if err := mounted.manager.ReconfigureCommandMaintenance(mounted.ports); err != nil {
			return err
		}
		if authority := a.commandJobAuthority.Swap(nil); authority != nil {
			authority.release()
		}
		a.commandMaintenance.Store(mounted)
		return nil
	}
	drain := func(ctx context.Context) error {
		if authority := a.commandJobAuthority.Swap(nil); authority != nil {
			authority.release()
		}
		return mounted.manager.CloseCommandMaintenance(ctx)
	}
	if current == nil {
		return core.RegisterExecutorDrainAction(ctx, drain, commit)
	}
	return core.WithExecutorLifecycle(ctx, commit)
}

// Esta captura ocorre no executor de jobs antes da persistência, fora do gate.
// O watch retornado protege a prova de vida, não cancela a tool já admitida.
func (a *App) captureCommandJobIdentity(ctx context.Context) (commandjobactivation.RuntimeIdentity, context.Context, func(), error) {
	var empty commandjobactivation.RuntimeIdentity
	if a == nil || ctx == nil {
		return empty, nil, nil, commandjobactivation.ErrUnavailable
	}
	mounted := a.commandMaintenance.Load()
	if mounted == nil {
		return empty, nil, nil, commandjobactivation.ErrUnavailable
	}
	principal, err := a.currentCommandPrincipal()
	if err != nil {
		return empty, nil, nil, err
	}
	if user, err := database.RequireUserID(ctx); err != nil || user != principal.UserID {
		return empty, nil, nil, commandjobactivation.ErrUnavailable
	}
	core, err := a.commandSecurityService()
	if err != nil {
		return empty, nil, nil, err
	}
	epoch, err := core.CaptureAuthenticated(ctx, func(ctx context.Context) (string, string, error) {
		if !a.commandPrincipalMatches(mounted.sessions, mounted.credentials, principal) {
			return "", "", commandjobactivation.ErrUnavailable
		}
		if _, err := mounted.sessions.RevalidateLocalSession(ctx, principal); err != nil {
			return "", "", err
		}
		a.authMu.RLock()
		host := a.commandHost
		a.authMu.RUnlock()
		if host == nil {
			return "", "", commandjobactivation.ErrUnavailable
		}
		ready, err := host.SourceSecurityReady(ctx)
		if err != nil {
			return "", "", err
		}
		if !ready {
			return "", "", commandjobactivation.ErrUnavailable
		}
		return principal.UserID, principal.SessionID, nil
	})
	if err != nil {
		return empty, nil, nil, err
	}
	// A publicação de bindings/camadas não encerra o run que fornece contexto.
	// Somente vida do run e autenticação/segurança compõem esta prova. A regra,
	// o grant e a condição continuam sendo revalidados pelo Consumer.
	watch, release, err := core.WatchSecurityEpoch(ctx, epoch)
	if err != nil {
		return empty, nil, nil, err
	}
	// Fence da sessão para eventos terminais: terminar o run remove sua prova
	// viva, mas não torna ilegítimo processar seu evento terminal autenticado.
	sessionWatch, releaseSession, err := core.WatchSecurityEpoch(context.Background(), epoch)
	if err != nil {
		release()
		return empty, nil, nil, err
	}
	// Publicação e substituição compartilham o gate do Consumer. Revalidar o
	// watch impede que uma captura atrasada substitua a fence após invalidação.
	err = a.commandGate.WithMutation(ctx, func() error {
		if sessionWatch.Err() != nil || watch.Err() != nil || a.commandMaintenance.Load() != mounted {
			return commandjobactivation.ErrUnavailable
		}
		if old := a.commandJobAuthority.Swap(&appCommandJobAuthority{epoch: epoch, watch: sessionWatch, release: releaseSession}); old != nil {
			old.release()
		}
		return nil
	})
	if err != nil {
		releaseSession()
		release()
		return empty, nil, nil, err
	}
	return commandjobactivation.RuntimeIdentity{UserID: principal.UserID, AuthContextType: "local_session", AuthContextID: principal.SessionID, AuthGeneration: epoch.AuthGeneration, SecurityGeneration: epoch.SecurityGeneration}, watch, release, nil
}

// Executada sob o gate compartilhado e dentro da transação do Consumer.
// Lê apenas a fence já inscrita: nunca chama Capture/Admit recursivamente.
func (a *App) authorizeCommandJob(ctx context.Context, tx *gorm.DB, mounted *appCommandMaintenance, fact commandjobevents.Fact, workspace *string) (commandactivation.Owner, error) {
	var empty commandactivation.Owner
	if a.commandMaintenance.Load() != mounted || database.DB() != mounted.db {
		return empty, commandjobactivation.ErrUnavailable
	}
	authority := a.commandJobAuthority.Load()
	if authority == nil || authority.watch.Err() != nil {
		return empty, commandjobactivation.ErrUnavailable
	}
	identity, err := jobs.CommandPersistedIdentityFromFact(ctx, tx, fact)
	if err != nil {
		if errors.Is(err, jobs.ErrCommandMaintenanceUnavailable) {
			return empty, commandjobactivation.ErrUnavailable
		}
		return empty, err
	}
	epoch := authority.epoch
	if identity.UserID != epoch.UserID || identity.AuthContextType != "local_session" || identity.AuthContextID != epoch.SessionID || identity.AuthGeneration != epoch.AuthGeneration || identity.SecurityGeneration != epoch.SecurityGeneration {
		return empty, commandjobactivation.ErrUnavailable
	}
	principal := auth.LocalSessionPrincipal{UserID: identity.UserID, SessionID: identity.AuthContextID}
	workspaceState, ok := ctx.Value(commandJobWorkspaceKey{}).(commandJobWorkspace)
	if !ok || !workspaceState.available || workspaceState.principal != principal || workspaceState.sessions != mounted.sessions || workspaceState.credentials != mounted.credentials {
		return empty, commandjobactivation.ErrUnavailable
	}
	if _, err := mounted.sessions.RevalidateLocalSessionTx(ctx, tx, principal); err != nil {
		if errors.Is(err, auth.ErrUnauthenticatedLocalSession) {
			return empty, commandjobactivation.ErrUnavailable
		}
		return empty, err
	}
	host, manager := workspaceState.host, workspaceState.manager
	if host == nil || manager == nil {
		return empty, commandjobactivation.ErrUnavailable
	}
	ready, err := host.SourceSecurityReady(ctx)
	if err != nil {
		return empty, err
	}
	if !ready {
		return empty, commandjobactivation.ErrUnavailable
	}
	if workspace != nil && workspaceState.snapshot.WorkspaceID != *workspace {
		return empty, commandjobactivation.ErrUnavailable
	}
	return commandactivation.Owner{Scope: commandactivation.Scope{UserID: identity.UserID, WorkspaceID: cloneCommandWorkspace(workspace)}, AuthContextType: identity.AuthContextType, AuthContextID: identity.AuthContextID, AuthGeneration: identity.AuthGeneration, SecurityGeneration: identity.SecurityGeneration}, nil
}
