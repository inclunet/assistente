package app

import (
	"context"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandactivation"
	"assistente/internal/commandautomation"
	"assistente/internal/commandconfig"
	"assistente/internal/commanddecision"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"assistente/internal/commandsecurity"
	"assistente/internal/credentials"
	"assistente/internal/database"
	"gorm.io/gorm"
)

// commandCompleteMutationInputs são portas do bootstrap confiável. Nenhum
// campo é derivado de intent, diálogo ou outro payload do cliente.
//
// A fábrica não instala o serviço no lifecycle: a montagem dos catálogos,
// handlers e do hook de ativação continua sendo responsabilidade do host que
// conhece seus adapters. Assim, uma configuração incompleta não habilita
// comandos por acidente.
type commandCompleteMutationInputs struct {
	Projection   commandconfig.ProjectionProvider
	Authorize    func(context.Context, auth.LocalSessionPrincipal, commandconfig.Scope, commandconfig.Operation) error
	Version      func(context.Context) (string, error)
	Render       func(commandconfig.MutationDiff) (string, error)
	OnMutationTx commandconfig.MutationTxHook
	DecisionTTL  time.Duration
}

// newCommandCompleteMutationService compõe o serviço integral de mutações
// sobre as dependências já montadas no App. Sessions, epochs e HostState são
// capturados juntos e precisam continuar sendo a mesma instância durante toda
// a vida da composição; qualquer troca de sessão ou dependência é revalidada
// antes da decisão e do commit pelo MutationService.
//
// O banco de configuração, automação e presenter/receipts é deliberadamente
// construído a partir de database.DB(). Não há Store recebido do chamador que
// possa apontar para outra raiz, outra transação ou outro tenant.
func (a *App) newCommandCompleteMutationService(inputs commandCompleteMutationInputs) (*commandconfig.CompleteMutationService, error) {
	if a == nil || inputs.Projection == nil || inputs.Authorize == nil || inputs.Version == nil || inputs.Render == nil || inputs.OnMutationTx == nil || inputs.DecisionTTL <= 0 {
		return nil, commandexecution.ErrInvalidConfiguration
	}

	a.authMu.RLock()
	state, sessions, manager, epochs := a.commandHost, a.sessionSvc, a.credMgr, a.commandEpochs
	storageVersion, storageErr := a.commandStorageVersion, a.commandStorageErr
	questionnaireManager := a.questionnaireMgr
	a.authMu.RUnlock()
	if state == nil || sessions == nil || manager == nil || epochs == nil || state.Epochs() != epochs || storageErr != nil || storageVersion == "" || questionnaireManager == nil {
		return nil, commandexecution.ErrInvalidConfiguration
	}

	db := database.DB()
	if !commandMutationRootDatabase(db) || !commandMutationSchemaReady(db) {
		return nil, commandexecution.ErrInvalidConfiguration
	}
	if !a.commandMutationDependenciesStable(state, sessions, manager, epochs, storageVersion, db) {
		return nil, commandexecution.ErrInvalidConfiguration
	}
	store, err := commandconfig.New(db)
	if err != nil {
		return nil, commandexecution.ErrInvalidConfiguration
	}
	keys, err := commandledger.NewCredentialKeyProvider(manager)
	if err != nil {
		return nil, commandexecution.ErrInvalidConfiguration
	}
	receipts, err := commanddecision.New(db, &commandDecisionPresenter{manager: questionnaireManager}, time.Now)
	if err != nil {
		return nil, commandexecution.ErrInvalidConfiguration
	}
	automation, err := commandautomation.New(db, time.Now)
	if err != nil {
		return nil, commandexecution.ErrInvalidConfiguration
	}

	// O HostState é a fonte autoritativa das camadas ativas publicadas. A
	// projeção fornecida pelo bootstrap continua sendo a fonte dos adapters,
	// catálogo e referências; ActiveUserLayerIDs nunca vem do chamador.
	projection := func(ctx context.Context, scope commandconfig.Scope) (commandconfig.CompleteProjection, error) {
		if ctx == nil || scope.UserID == "" || !a.commandMutationDependenciesStable(state, sessions, manager, epochs, storageVersion, db) {
			return commandconfig.CompleteProjection{}, commandconfig.ErrInvalid
		}
		options, err := inputs.Projection(ctx, scope)
		if err != nil {
			return commandconfig.CompleteProjection{}, err
		}
		if options.Registry == nil || !options.Registry.Complete() || len(options.TriggerPorts) == 0 || !a.commandMutationDependenciesStable(state, sessions, manager, epochs, storageVersion, db) {
			return commandconfig.CompleteProjection{}, commandconfig.ErrInvalid
		}
		_, active, err := state.UserConfiguration(ctx, scope.UserID)
		if err != nil {
			return commandconfig.CompleteProjection{}, err
		}
		options.ActiveUserLayerIDs = append([]string(nil), active...)
		return options, nil
	}

	// O estado de locked/SO é um fato de segurança, não uma geração. Snapshot
	// pode retornar Unlocked=false sem erro; nesse caso a operação deve falhar
	// fechada antes de preparar ou apresentar qualquer decisão.
	authorize := func(ctx context.Context, principal auth.LocalSessionPrincipal, scope commandconfig.Scope, operation commandconfig.Operation) error {
		if ctx == nil || principal.UserID != scope.UserID || !a.commandMutationDependenciesStable(state, sessions, manager, epochs, storageVersion, db) || !a.commandPrincipalMatches(sessions, manager, principal) {
			return commandexecution.ErrDenied
		}
		versions, err := state.Snapshot(ctx, principal)
		if err != nil {
			return err
		}
		if !versions.Unlocked {
			return commandexecution.ErrDenied
		}
		if err := inputs.Authorize(ctx, principal, scope, operation); err != nil {
			return err
		}
		if !a.commandMutationDependenciesStable(state, sessions, manager, epochs, storageVersion, db) {
			return commandconfig.ErrStale
		}
		return nil
	}

	version := func(ctx context.Context) (string, error) {
		if ctx == nil || !a.commandMutationDependenciesStable(state, sessions, manager, epochs, storageVersion, db) {
			return "", commandconfig.ErrInvalid
		}
		value, err := inputs.Version(ctx)
		if err != nil {
			return "", err
		}
		if value == "" || !a.commandMutationDependenciesStable(state, sessions, manager, epochs, storageVersion, db) {
			return "", commandconfig.ErrInvalid
		}
		return value, nil
	}

	service, err := commandconfig.NewCompleteMutationService(commandconfig.MutationServiceConfig{
		Store:        store,
		Automation:   automation,
		Sessions:     sessions,
		Epochs:       epochs,
		Receipts:     receipts,
		Keys:         keys,
		KeyVersion:   storageVersion,
		DecisionTTL:  inputs.DecisionTTL,
		Authorize:    authorize,
		Version:      version,
		Render:       inputs.Render,
		OnMutationTx: inputs.OnMutationTx,
		BeforeCommit: func(ctx context.Context, scope commandconfig.Scope) error {
			if ctx == nil || scope.UserID == "" || !a.commandMutationDependenciesStable(state, sessions, manager, epochs, storageVersion, db) {
				return commandconfig.ErrStale
			}
			return state.SuspendUserConfiguration(ctx, scope.UserID)
		},
	}, projection)
	if err != nil {
		return nil, commandexecution.ErrInvalidConfiguration
	}
	return service, nil
}

func commandMutationRootDatabase(db *gorm.DB) bool {
	if db == nil || db.Config == nil || db.Statement == nil {
		return false
	}
	_, transactional := db.Statement.ConnPool.(gorm.TxCommitter)
	return !transactional
}

func (a *App) commandMutationDependenciesStable(state *commandexecution.HostState, sessions *auth.SessionService, manager *credentials.Manager, epochs *commandsecurity.EpochService, storageVersion string, db *gorm.DB) bool {
	if a == nil || state == nil || sessions == nil || manager == nil || db == nil || database.DB() != db {
		return false
	}
	a.commandLifecycleMount.Lock()
	closing := a.commandLifecycleClosing
	a.commandLifecycleMount.Unlock()
	a.authMu.RLock()
	defer a.authMu.RUnlock()
	return !closing && a.commandHost == state && a.sessionSvc == sessions && a.credMgr == manager && a.commandEpochs == epochs && a.commandStorageErr == nil && a.commandStorageVersion == storageVersion && storageVersion != ""
}

func commandMutationSchemaReady(db *gorm.DB) bool {
	if !commandMutationRootDatabase(db) {
		return false
	}
	// A migração central é responsável por validar o DDL/versionamento exato;
	// esta fábrica só faz a guarda de presença e nunca cria schema em runtime.
	for _, table := range []string{
		(commandconfig.Layer{}).TableName(),
		(commandconfig.Binding{}).TableName(),
		(commandconfig.Generation{}).TableName(),
		"command_config_mutations",
		"command_decision_receipts",
		"command_decision_receipt_events",
		(commandactivation.Rule{}).TableName(),
		"command_layer_activation_state",
		"command_activation_idempotency_keys",
		"command_layer_activation_generations",
		"command_layer_automation_grants",
	} {
		if !db.Migrator().HasTable(table) {
			return false
		}
	}
	return true
}
