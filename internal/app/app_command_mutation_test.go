package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandactivation"
	"assistente/internal/commandautomation"
	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandconfig"
	"assistente/internal/commanddecision"
	"assistente/internal/commandexecution"
	"assistente/internal/commandsecurity"
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/questionnaire"
	"assistente/internal/workspace"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestCommandMutationFactoryUsesRealDBAndPresenterAndRevalidatesSession(t *testing.T) {
	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "command-mutation.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&database.User{}, &database.Session{}); err != nil {
		t.Fatal(err)
	}
	if err := commandconfig.Migrate(ctx, db); err != nil {
		t.Fatalf("commandconfig.Migrate: %v", err)
	}
	if err := commanddecision.Migrate(ctx, db); err != nil {
		t.Fatalf("commanddecision.Migrate: %v", err)
	}
	if err := commandautomation.Migrate(ctx, db); err != nil {
		t.Fatalf("commandautomation.Migrate: %v", err)
	}
	if err := commandactivation.Migrate(ctx, db); err != nil {
		t.Fatalf("commandactivation.Migrate: %v", err)
	}
	previousDB := database.DB()
	database.SetDB(db)
	t.Cleanup(func() { database.SetDB(previousDB) })

	user := database.User{Username: "mutation-fixture", PasswordHash: "unused", Role: database.UserRoleUser, IsActive: true}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	sessions, err := auth.NewSessionService(db, auth.SessionConfig{RefreshTokenPepper: bytes.Repeat([]byte{0x41}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	first, err := sessions.IssueSession(ctx, &user, "mutation-first")
	if err != nil {
		t.Fatal(err)
	}
	manager := credentials.NewManager(bytes.Repeat([]byte{0x42}, 32))
	if err := manager.RegisterInstanceSecret("internal-auth:command-request-hmac:v1", base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x43}, 32))); err != nil {
		t.Fatal(err)
	}

	epochs, err := commandsecurity.NewEpochService(&commandsecurity.DispatchGate{})
	if err != nil {
		t.Fatal(err)
	}
	state, err := commandexecution.NewHostState(epochs, "mutation-registry-v1")
	if err != nil {
		t.Fatal(err)
	}
	empty, err := commandbindings.NewConfiguration(nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.SetVaultUnlocked(ctx, true); err != nil {
		t.Fatal(err)
	}
	if err := state.SetOSSessionState(ctx, true, false); err != nil {
		t.Fatal(err)
	}
	if err := state.PublishUserConfiguration(ctx, user.ID, empty); err != nil {
		t.Fatal(err)
	}
	if err := state.RebuildUserConfiguration(ctx, func(context.Context) (auth.LocalSessionPrincipal, error) {
		return auth.LocalSessionPrincipal{UserID: user.ID, SessionID: first.SessionID}, nil
	}, func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
		return empty, nil, nil
	}); err != nil {
		t.Fatal(err)
	}

	store, err := commandconfig.New(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureScope(ctx, commandconfig.Scope{UserID: user.ID}); err != nil {
		t.Fatal(err)
	}
	events := make(chan map[string]any, 8)
	questionnaireManager := questionnaire.NewManager(func(event string, data any) {
		if event == questionnaire.EventQuestionnaire {
			events <- data.(map[string]any)
		}
	})
	app := &App{
		ctx:                   ctx,
		sessionSvc:            sessions,
		credMgr:               manager,
		questionnaireMgr:      questionnaireManager,
		commandEpochs:         epochs,
		commandHost:           state,
		commandStorageVersion: "v1",
	}
	app.setCurrentUserID(user.ID)
	app.setCurrentAuthUser(&AuthUser{UserID: user.ID, SessionID: first.SessionID, Role: user.Role})

	registry, err := completeFactoryRegistry()
	if err != nil {
		t.Fatal(err)
	}
	definition, _ := registry.Lookup("fixture.complete")
	definition.AllowedSources = []commandcatalog.Source{commandcatalog.KeyboardLocal}
	definition.Scopes = []commandcatalog.Scope{commandcatalog.ScopeGlobal}
	registry, err = commandcatalog.NewComplete([]commandcatalog.Registration{{Definition: definition, Handler: commandcatalog.HandlerContract{
		Effect: definition.Effect, Route: definition.HandlerRoute, Classification: definition.HandlerClassification,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	options := mutationProjectionOptions(registry)
	var mutationHookErr error
	factoryInputs := commandCompleteMutationInputs{
		Projection: func(context.Context, commandconfig.Scope) (commandconfig.CompleteProjection, error) {
			return options, nil
		},
		Authorize: func(_ context.Context, _ auth.LocalSessionPrincipal, _ commandconfig.Scope, operation commandconfig.Operation) error {
			if operation == "" {
				return errors.New("operação ausente")
			}
			return nil
		},
		Version:      func(context.Context) (string, error) { return "catalog-v1", nil },
		Render:       func(diff commandconfig.MutationDiff) (string, error) { return "diff:" + string(diff.Operation), nil },
		OnMutationTx: func(context.Context, *gorm.DB, commandconfig.MutationDiff) error { return mutationHookErr },
		DecisionTTL:  time.Minute,
	}
	service, err := app.newCommandCompleteMutationService(factoryInputs)
	if err != nil {
		t.Fatalf("fábrica de mutação: %v", err)
	}

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	database.SetDB(tx)
	if _, err := app.newCommandCompleteMutationService(factoryInputs); !errors.Is(err, commandexecution.ErrInvalidConfiguration) {
		t.Fatalf("fábrica aceitou DB transacional: %v", err)
	}
	database.SetDB(db)
	if err := tx.Rollback().Error; err != nil {
		t.Fatal(err)
	}
	otherDB, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "other-command-mutation.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	otherSQLDB, err := otherDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = otherSQLDB.Close() })
	database.SetDB(otherDB)
	if _, err := app.newCommandCompleteMutationService(factoryInputs); !errors.Is(err, commandexecution.ErrInvalidConfiguration) {
		t.Fatalf("fábrica aceitou DB divergente sem schema: %v", err)
	}
	database.SetDB(db)
	app.commandLifecycleMount.Lock()
	app.commandLifecycleClosing = true
	app.commandLifecycleMount.Unlock()
	if _, err := app.newCommandCompleteMutationService(factoryInputs); !errors.Is(err, commandexecution.ErrInvalidConfiguration) {
		t.Fatalf("fábrica aceitou lifecycle em closing: %v", err)
	}
	app.commandLifecycleMount.Lock()
	app.commandLifecycleClosing = false
	app.commandLifecycleMount.Unlock()
	app.commandHost = nil
	if _, err := app.newCommandCompleteMutationService(factoryInputs); !errors.Is(err, commandexecution.ErrInvalidConfiguration) {
		t.Fatalf("fábrica aceitou dependência de HostState ausente: %v", err)
	}
	app.commandHost = state

	apply := service.Apply
	applyDecision := func(token string, name string) (commandconfig.MutationDiff, error) {
		t.Helper()
		result := make(chan struct {
			diff commandconfig.MutationDiff
			err  error
		}, 1)
		go func() {
			diff, applyErr := apply(ctx, token, nil, commandconfig.MutationIntent{
				Operation: commandconfig.LayerCreate,
				Layer:     &commandconfig.Layer{Name: name, Description: "fixture", Enabled: true},
			})
			result <- struct {
				diff commandconfig.MutationDiff
				err  error
			}{diff, applyErr}
		}()
		var payload map[string]any
		select {
		case payload = <-events:
		case early := <-result:
			t.Fatalf("mutação terminou antes do presenter: %v", early.err)
		case <-time.After(2 * time.Second):
			t.Fatal("presenter real não publicou a decisão")
		}
		id, ok := payload["id"].(string)
		if !ok || id == "" {
			t.Fatalf("ID de questionário inválido: %#v", payload["id"])
		}
		if err := questionnaireManager.Respond(id, map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction}, false); err != nil {
			t.Fatal(err)
		}
		select {
		case result := <-result:
			return result.diff, result.err
		case <-time.After(2 * time.Second):
			t.Fatal("mutação não terminou após resposta")
			return commandconfig.MutationDiff{}, context.DeadlineExceeded
		}
	}
	app.commandStorageVersion = "catalog-v2"
	if _, err := service.Apply(ctx, first.AccessToken, nil, commandconfig.MutationIntent{
		Operation: commandconfig.LayerCreate,
		Layer:     &commandconfig.Layer{Name: "storage-trocado", Description: "fixture", Enabled: true},
	}); err == nil {
		t.Fatal("troca da dependência de storage foi aceita por serviço já composto")
	}
	app.commandStorageVersion = "v1"

	diff, err := applyDecision(first.AccessToken, "primeira")
	if err != nil || diff.Operation != commandconfig.LayerCreate {
		t.Fatalf("decisão confirmada: diff=%#v err=%v", diff, err)
	}
	var layers int64
	if err := db.Table("command_layers").Where("user_id = ?", user.ID).Count(&layers).Error; err != nil {
		t.Fatal(err)
	}
	if layers != 1 {
		t.Fatalf("camada confirmada não persistida: %d", layers)
	}
	if _, _, err := state.UserConfiguration(ctx, user.ID); !errors.Is(err, commandexecution.ErrHostUserNotPublished) {
		t.Fatalf("mapa antigo permaneceu após commit: %v", err)
	}
	if err := state.PublishUserConfiguration(ctx, user.ID, empty); err != nil {
		t.Fatal(err)
	}
	if err := state.RebuildUserConfiguration(ctx, func(context.Context) (auth.LocalSessionPrincipal, error) {
		return auth.LocalSessionPrincipal{UserID: user.ID, SessionID: first.SessionID}, nil
	}, func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
		return empty, nil, nil
	}); err != nil {
		t.Fatal(err)
	}

	if err := state.SetVaultUnlocked(ctx, false); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Apply(ctx, first.AccessToken, nil, commandconfig.MutationIntent{Operation: commandconfig.LayerCreate, Layer: &commandconfig.Layer{Name: "cofre-fechado", Description: "fixture", Enabled: true}}); !errors.Is(err, commandexecution.ErrDenied) {
		t.Fatalf("cofre fechado retornou %v", err)
	}
	if err := state.SetVaultUnlocked(ctx, true); err != nil {
		t.Fatal(err)
	}
	if err := state.SetOSSessionState(ctx, false, true); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Apply(ctx, first.AccessToken, nil, commandconfig.MutationIntent{Operation: commandconfig.LayerCreate, Layer: &commandconfig.Layer{Name: "so-desconhecido", Description: "fixture", Enabled: true}}); err == nil {
		t.Fatal("SO desconhecido foi aceito")
	}
	if err := state.SetOSSessionState(ctx, true, false); err != nil {
		t.Fatal(err)
	}
	if err := state.PublishUserConfiguration(ctx, user.ID, empty); err != nil {
		t.Fatal(err)
	}
	if err := state.RebuildUserConfiguration(ctx, func(context.Context) (auth.LocalSessionPrincipal, error) {
		return auth.LocalSessionPrincipal{UserID: user.ID, SessionID: first.SessionID}, nil
	}, func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
		return empty, nil, nil
	}); err != nil {
		t.Fatal(err)
	}
	applier, err := app.newCommandMutationApplier(factoryInputs)
	if err != nil {
		t.Fatal(err)
	}
	var rebuiltResult commandMutationResult
	options.ActiveUserLayerIDs = []string{diff.AfterLayers[0].ID}
	apply = func(ctx context.Context, token string, _ *string, _ commandconfig.MutationIntent) (commandconfig.MutationDiff, error) {
		var applyErr error
		commandID := "fixture.complete"
		intent := commandconfig.MutationIntent{Operation: commandconfig.BindingCreate, Binding: &commandconfig.Binding{
			LayerRefKind: "user", LayerRef: diff.AfterLayers[0].ID,
			TriggerType: "keyboard.local", TriggerSpec: `{"version":1,"code":"KeyA","modifiers":[]}`,
			CommandID: &commandID, Arguments: "{}", Condition: `{"version":1,"clauses":[]}`, Effect: "execute", Enabled: true,
			ReviewStatus: "active", Presentation: `{"version":1,"title_key":"commands.fixture"}`,
		}}
		rebuiltResult, applyErr = applier.Apply(ctx, token, intent)
		return rebuiltResult.Diff, applyErr
	}
	if _, err := applyDecision(first.AccessToken, "reconstruída"); err != nil {
		t.Fatal(err)
	}
	if !rebuiltResult.Committed || !rebuiltResult.Rebuilt {
		t.Fatalf("resultado: %+v", rebuiltResult)
	}
	if versions, err := state.Snapshot(ctx, auth.LocalSessionPrincipal{UserID: user.ID, SessionID: first.SessionID}); err != nil || !versions.Unlocked {
		t.Fatalf("rebuild não publicou sessão pronta: %+v %v", versions, err)
	}
	rebuiltMap, _, err := state.UserConfiguration(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := rebuiltMap.Resolve("keyboard.local:KeyA", nil, nil)
	if err != nil || resolved.CommandID != "fixture.complete" {
		t.Fatalf("binding recém-persistido ausente: %+v %v", resolved, err)
	}
	t.Run("publicacao-preserva-workspace-ativo-independente-do-alvo", func(t *testing.T) {
		wm := workspace.NewManager(filepath.Join(t.TempDir(), "home"))
		if err := wm.Initialize(filepath.Join(t.TempDir(), "workspace")); err != nil {
			t.Fatal(err)
		}
		if err := wm.AddTab(workspace.Tab{ID: "mutation-tab", Type: workspace.TabTypeEditor, State: map[string]any{"version": float64(1)}}); err != nil {
			t.Fatal(err)
		}
		app.workspaceMgr = wm
		t.Cleanup(func() { app.workspaceMgr = nil })
		principal := auth.LocalSessionPrincipal{UserID: user.ID, SessionID: first.SessionID}
		publicationScope, err := app.commandMutationCurrentScope(principal)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.EnsureScope(ctx, publicationScope); err != nil {
			t.Fatal(err)
		}
		localLayer := commandconfig.Layer{ID: uuid.Must(uuid.NewV7()).String(), UserID: user.ID, WorkspaceID: publicationScope.WorkspaceID, Name: "local ativa", Enabled: true, Source: "user"}
		if err := db.Create(&localLayer).Error; err != nil {
			t.Fatal(err)
		}
		localBinding := rebuiltResult.Diff.AfterBindings[0]
		localBinding.ID = uuid.Must(uuid.NewV7()).String()
		localBinding.WorkspaceID, localBinding.LayerRef = publicationScope.WorkspaceID, localLayer.ID
		if err := db.Create(&localBinding).Error; err != nil {
			t.Fatal(err)
		}
		options.ActiveUserLayerIDs = []string{diff.AfterLayers[0].ID, localLayer.ID}
		inactive := "workspace-inativo"
		if err := store.EnsureScope(ctx, commandconfig.Scope{UserID: user.ID, WorkspaceID: &inactive}); err != nil {
			t.Fatal(err)
		}
		for _, target := range []struct {
			name      string
			workspace *string
		}{{"global", nil}, {"ativo", publicationScope.WorkspaceID}, {"inativo", &inactive}} {
			apply = func(ctx context.Context, token string, _ *string, intent commandconfig.MutationIntent) (commandconfig.MutationDiff, error) {
				var err error
				rebuiltResult, err = applier.ApplyScoped(ctx, token, target.workspace, intent)
				return rebuiltResult.Diff, err
			}
			if _, err := applyDecision(first.AccessToken, "alvo-"+target.name); err != nil {
				t.Fatalf("%s: %v", target.name, err)
			}
			if !rebuiltResult.Committed || !rebuiltResult.Rebuilt {
				t.Fatalf("%s: %+v", target.name, rebuiltResult)
			}
			configuration, _, err := state.UserConfiguration(ctx, user.ID)
			if err != nil {
				t.Fatal(err)
			}
			selected, err := configuration.Resolve("keyboard.local:KeyA", nil, nil)
			if err != nil || !slices.Contains(selected.BindingIDs, localBinding.ID) {
				t.Fatalf("%s apagou binding do workspace ativo: %+v %v", target.name, selected, err)
			}
			var persisted commandconfig.Layer
			if err := db.Where("name = ?", "alvo-"+target.name).First(&persisted).Error; err != nil || !sameCommandWorkspace(persisted.WorkspaceID, target.workspace) {
				t.Fatalf("%s mudou alvo da escrita: %+v %v", target.name, persisted, err)
			}
		}
	})
	options.ActiveUserLayerIDs = nil
	for _, representation := range []string{"nil-vazio", "ordem"} {
		setInputs := factoryInputs
		projectionReads := 0
		setInputs.Projection = func(ctx context.Context, scope commandconfig.Scope) (commandconfig.CompleteProjection, error) {
			projected := options
			if _, _, err := state.UserConfiguration(ctx, scope.UserID); errors.Is(err, commandexecution.ErrHostUserNotPublished) {
				projectionReads++
				if representation == "nil-vazio" {
					if projectionReads > 1 {
						projected.ActiveUserLayerIDs = []string{}
					}
				} else {
					snapshot, err := store.Load(ctx, scope)
					if err != nil {
						return commandconfig.CompleteProjection{}, err
					}
					for _, layer := range snapshot.Layers {
						projected.ActiveUserLayerIDs = append(projected.ActiveUserLayerIDs, layer.ID)
					}
					if len(projected.ActiveUserLayerIDs) < 2 {
						return commandconfig.CompleteProjection{}, errors.New("fixture exige duas camadas")
					}
					if projectionReads > 1 {
						slices.Reverse(projected.ActiveUserLayerIDs)
					}
				}
			}
			return projected, nil
		}
		setApplier, err := app.newCommandMutationApplier(setInputs)
		if err != nil {
			t.Fatal(err)
		}
		apply = func(ctx context.Context, token string, _ *string, intent commandconfig.MutationIntent) (commandconfig.MutationDiff, error) {
			var applyErr error
			rebuiltResult, applyErr = setApplier.Apply(ctx, token, intent)
			return rebuiltResult.Diff, applyErr
		}
		if _, err := applyDecision(first.AccessToken, "conjunto-"+representation); err != nil {
			t.Fatalf("%s: %v", representation, err)
		}
		if !rebuiltResult.Committed || !rebuiltResult.Rebuilt || projectionReads != 2 {
			t.Fatalf("%s: resultado=%+v leituras=%d", representation, rebuiltResult, projectionReads)
		}
	}
	raceInputs := factoryInputs
	injected := false
	raceInputs.Version = func(ctx context.Context) (string, error) {
		if _, _, err := state.UserConfiguration(ctx, user.ID); errors.Is(err, commandexecution.ErrHostUserNotPublished) && !injected {
			// Injeção SQL controlada: representa uma geração concorrente antes
			// da captura do rebuild, que portanto não cancela seu watch futuro.
			injected = true
			if err := db.WithContext(ctx).Model(&commandconfig.Generation{}).Where("user_id = ? AND workspace_id IS NULL", user.ID).UpdateColumn("generation", gorm.Expr("generation + 1")).Error; err != nil {
				return "", err
			}
		}
		return "catalog-v1", nil
	}
	raceApplier, err := app.newCommandMutationApplier(raceInputs)
	if err != nil {
		t.Fatal(err)
	}
	apply = func(ctx context.Context, token string, _ *string, intent commandconfig.MutationIntent) (commandconfig.MutationDiff, error) {
		var applyErr error
		rebuiltResult, applyErr = raceApplier.Apply(ctx, token, intent)
		return rebuiltResult.Diff, applyErr
	}
	if _, err := applyDecision(first.AccessToken, "geração-concorrente"); !errors.Is(err, commandconfig.ErrStale) {
		t.Fatalf("geração posterior ao commit: %v", err)
	}
	if !rebuiltResult.Committed || rebuiltResult.Rebuilt {
		t.Fatalf("geração concorrente: %+v", rebuiltResult)
	}
	if _, _, err := state.UserConfiguration(ctx, user.ID); !errors.Is(err, commandexecution.ErrHostUserNotPublished) {
		t.Fatalf("mapa após geração concorrente: %v", err)
	}
	if err := state.RebuildUserConfiguration(ctx, func(ctx context.Context) (auth.LocalSessionPrincipal, error) {
		return sessions.AuthenticateLocalAccess(ctx, first.AccessToken)
	},
		func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
			return empty, nil, nil
		}); err != nil {
		t.Fatal(err)
	}
	apply = service.Apply
	mutationHookErr = errors.New("hook de ativação recusou")
	if _, err := applyDecision(first.AccessToken, "hook-falhou"); err == nil {
		t.Fatal("falha do hook não abortou o commit")
	}
	if err := db.Table("command_layers").Where("name = ?", "hook-falhou").Count(&layers).Error; err != nil {
		t.Fatal(err)
	}
	if layers != 0 {
		t.Fatal("rollback do hook deixou camada persistida")
	}
	if _, _, err := state.UserConfiguration(ctx, user.ID); !errors.Is(err, commandexecution.ErrHostUserNotPublished) {
		t.Fatalf("rollback do hook republicou mapa antigo: %v", err)
	}
	mutationHookErr = nil
	if err := state.PublishUserConfiguration(ctx, user.ID, empty); err != nil {
		t.Fatal(err)
	}
	if err := state.RebuildUserConfiguration(ctx, func(context.Context) (auth.LocalSessionPrincipal, error) {
		return auth.LocalSessionPrincipal{UserID: user.ID, SessionID: first.SessionID}, nil
	}, func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
		return empty, nil, nil
	}); err != nil {
		t.Fatal(err)
	}

	late := make(chan struct {
		diff commandconfig.MutationDiff
		err  error
	}, 1)
	go func() {
		diff, applyErr := service.Apply(ctx, first.AccessToken, nil, commandconfig.MutationIntent{
			Operation: commandconfig.LayerCreate,
			Layer:     &commandconfig.Layer{Name: "logout-tardio", Description: "fixture", Enabled: true},
		})
		late <- struct {
			diff commandconfig.MutationDiff
			err  error
		}{diff, applyErr}
	}()
	var latePayload map[string]any
	select {
	case latePayload = <-events:
	case <-time.After(2 * time.Second):
		t.Fatal("decisão tardia não foi apresentada")
	}
	if err := sessions.Logout(ctx, first.RefreshToken); err != nil {
		t.Fatal(err)
	}
	app.setCurrentUserID("")
	app.setCurrentAuthUser(nil)
	lateID, _ := latePayload["id"].(string)
	if err := questionnaireManager.Respond(lateID, map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction}, false); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-late:
		if result.err == nil {
			t.Fatal("logout tardio permitiu commit")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("logout tardio não terminou")
	}
	if err := db.Table("command_layers").Where("name = ?", "logout-tardio").Count(&layers).Error; err != nil {
		t.Fatal(err)
	}
	if layers != 0 {
		t.Fatal("mutação tardia deixou camada persistida")
	}
	if _, err := service.Apply(ctx, first.AccessToken, nil, commandconfig.MutationIntent{Operation: commandconfig.LayerCreate, Layer: &commandconfig.Layer{Name: "token-reutilizado", Description: "fixture", Enabled: true}}); err == nil {
		t.Fatal("token revogado foi reutilizado")
	}
	second, err := sessions.IssueSession(ctx, &user, "rebuild-logout")
	if err != nil {
		t.Fatal(err)
	}
	app.setCurrentUserID(user.ID)
	app.setCurrentAuthUser(&AuthUser{UserID: user.ID, SessionID: second.SessionID, Role: user.Role})
	if err := state.RebuildUserConfiguration(ctx, func(context.Context) (auth.LocalSessionPrincipal, error) {
		return sessions.AuthenticateLocalAccess(ctx, second.AccessToken)
	}, func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
		return empty, nil, nil
	}); err != nil {
		t.Fatal(err)
	}
	logoutInputs := factoryInputs
	logoutInputs.Projection = func(ctx context.Context, scope commandconfig.Scope) (commandconfig.CompleteProjection, error) {
		if _, _, err := state.UserConfiguration(ctx, scope.UserID); errors.Is(err, commandexecution.ErrHostUserNotPublished) {
			if err := sessions.Logout(ctx, second.RefreshToken); err != nil {
				return commandconfig.CompleteProjection{}, err
			}
		}
		return options, nil
	}
	logoutApplier, err := app.newCommandMutationApplier(logoutInputs)
	if err != nil {
		t.Fatal(err)
	}
	apply = func(ctx context.Context, token string, _ *string, intent commandconfig.MutationIntent) (commandconfig.MutationDiff, error) {
		var applyErr error
		rebuiltResult, applyErr = logoutApplier.Apply(ctx, token, intent)
		return rebuiltResult.Diff, applyErr
	}
	if _, err := applyDecision(second.AccessToken, "commit-antes-logout"); err == nil {
		t.Fatal("logout durante rebuild permitiu publicação")
	}
	if !rebuiltResult.Committed || rebuiltResult.Rebuilt {
		t.Fatalf("commit/rebuild: %+v", rebuiltResult)
	}
	if err := db.Table("command_layers").Where("name = ?", "commit-antes-logout").Count(&layers).Error; err != nil || layers != 1 {
		t.Fatalf("commit deve permanecer: %d %v", layers, err)
	}
	if _, _, err := state.UserConfiguration(ctx, user.ID); !errors.Is(err, commandexecution.ErrHostUserNotPublished) {
		t.Fatalf("mapa após logout: %v", err)
	}
}

func mutationProjectionOptions(registry *commandcatalog.Registry) commandconfig.CompleteProjection {
	return commandconfig.CompleteProjection{
		Registry: registry,
		TriggerPorts: map[commandcatalog.Source]commandconfig.TriggerPort{
			commandcatalog.KeyboardLocal: commandconfig.TriggerPortFunc(func(_ context.Context, raw []byte) (string, error) {
				if string(raw) != `{"version":1,"code":"KeyA","modifiers":[]}` {
					return "", errors.New("trigger de fixture desconhecido")
				}
				return "keyboard.local:KeyA", nil
			}),
		},
	}
}
