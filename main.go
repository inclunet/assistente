package main

import (
	"assistente/internal/logging"
	"context"
	"embed"
	"errors"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"

	"assistente/adapters/wails"
	application "assistente/internal/app"
	"assistente/internal/configdir"
	"assistente/internal/database"
	"assistente/internal/desktopinstance"
	"assistente/internal/wailsapi"

	wailslib "github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:frontend/dist
var assets embed.FS

type desktopReservation interface {
	io.Closer
	DatabasePath() string
}

var (
	// Mantém os handles vivos até os.Exit: Shutdown pode preservar serviços
	// quando não consegue comprovar a drenagem. Não liberar a reserva nesse intervalo.
	desktopProcessReservation io.Closer
	resolveDesktopDatabase    = database.ResolvePath
	logUserDirectory          = configdir.GetHomeDir
	acquireDesktopInstance    = func(path string, activate func()) (desktopReservation, error) {
		return desktopinstance.Acquire(path, activate)
	}
	activateDesktop = func(ctx context.Context) {
		// GTK precisa de present/deiconify; no Windows, Show já restaura
		// e mantém todo o despacho nativo assíncrono.
		if runtime.GOOS == "linux" {
			wailsruntime.WindowUnminimise(ctx)
		}
		wailsruntime.WindowShow(ctx)
	}
	runDesktop   = wailslib.Run
	quitDesktop  = wailsruntime.Quit
	startDesktop = func(a *application.App, ctx context.Context) error {
		return a.StartupWithAdapters(
			ctx,
			wails.NewEmitterAdapter(ctx),
			wails.NewWindowAdapter(ctx),
			wails.NewDialogAdapter(ctx),
		)
	}
)

func main() {
	os.Exit(run(os.Args))
}

// Erros fatais anteriores à reserva ainda devem chegar ao destino explícito
// --log-file. O arquivo é aberto apenas depois da falha, sem disputar o lock
// de rotação enquanto outra instância tenta se tornar a principal.
func reportPreReservationFailure(path, message string) {
	output := io.Writer(os.Stderr)
	configPath := ""
	if dir := logUserDirectory(); dir != "" {
		configPath = filepath.Join(dir, "logging.json")
	}
	if path != "" && !logging.ConflictsWithSettings(configPath, path) {
		if file, err := logging.OpenExplicitLog(path); err == nil {
			defer func() { _ = file.Close() }()
			output = logging.DuplicateTo(os.Stderr, file)
		}
	}
	reportFatalError(output, message)
}

func run(args []string) (exitCode int) {
	logPath, remainingArgs, err := logging.ParseLogFileArgs(args[1:])
	if err != nil {
		reportFatalError(os.Stderr, startupLogConfigurationError(err))
		return 2
	}
	logLevel, remainingArgs, err := logging.ParseToolLedgerLogLevelArgs(remainingArgs)
	if err != nil {
		reportFatalError(os.Stderr, startupLogConfigurationError(err))
		return 2
	}

	workspaceDir, remainingArgs, err := parseWorkspaceArgs(remainingArgs)
	if err != nil {
		reportPreReservationFailure(logPath, startupApplicationError(err))
		return 2
	}

	originalArgs := os.Args
	os.Args = append([]string{args[0]}, remainingArgs...)
	defer func() {
		os.Args = originalArgs
	}()

	databasePath, err := resolveDesktopDatabase()
	if err != nil {
		reportPreReservationFailure(logPath, startupApplicationError(err))
		return 1
	}
	activationRequests := make(chan struct{}, 1)
	instance, err := acquireDesktopInstance(databasePath, func() {
		select {
		case activationRequests <- struct{}{}:
		default:
		}
	})
	if errors.Is(err, desktopinstance.ErrAlreadyRunning) {
		return 0
	}
	if err != nil {
		reportPreReservationFailure(logPath, startupInstanceError(err))
		return 1
	}
	var runtimeEntered bool
	defer func() {
		if runtimeEntered {
			desktopProcessReservation = instance
			return
		}
		if err := instance.Close(); err != nil {
			logging.Errorf(context.Background(), "main", "Falha ao liberar reserva desktop: %v", err)
			exitCode = 1
		}
	}()
	// A reserva precede a abertura do destino para que apenas a instância que
	// permanecerá ativa retenha o lock da rotação de logs.
	configPath := ""
	if dir := logUserDirectory(); dir != "" {
		configPath = filepath.Join(dir, "logging.json")
	}
	logManager, err := logging.NewManager(configPath, logPath)
	if err != nil {
		reportFatalError(os.Stderr, startupLogConfigurationError(err))
		return 2
	}
	previousLogWriter := log.Writer()
	log.SetOutput(logging.DuplicateTo(previousLogWriter, logManager))
	if _, _, problem := logManager.Settings(); problem != "" {
		log.Printf("Falha ao iniciar logs configurados: %s", problem)
	}
	database.SetLogOutput(logManager)
	errorOutput := io.Writer(logging.DuplicateTo(os.Stderr, logManager))
	defer func() {
		database.SetLogOutput(nil)
		log.SetOutput(previousLogWriter)
		if err := logManager.Close(); err != nil {
			reportFatalError(os.Stderr, startupLogCloseError(logPath, err))
			exitCode = 1
		}
	}()
	previousLogger := slog.Default()
	if err := logging.ConfigureToolLedgerLevel(logLevel); err != nil {
		reportFatalError(errorOutput, startupLogConfigurationError(err))
		return 2
	}
	defer slog.SetDefault(previousLogger)
	activationReady, stopActivation := startDesktopActivations(activationRequests, activateDesktop)
	defer stopActivation()
	a := application.NewApp()
	application.SetStartupWorkspaceDirectory(a, workspaceDir)
	application.SetDesktopDatabasePath(a, instance.DatabasePath())
	tokensAPI := wailsapi.NewTokens()
	application.SetTokensAPI(a, tokensAPI)
	allowlistsAPI := wailsapi.NewAllowlists()
	application.SetAllowlistsAPI(a, allowlistsAPI)
	pluginsAPI := wailsapi.NewPlugins()
	application.SetPluginsAPI(a, pluginsAPI)
	skillsAPI := wailsapi.NewSkills()
	application.SetSkillsAPI(a, skillsAPI)
	toolsAPI := wailsapi.NewTools()
	application.SetToolsAPI(a, toolsAPI)
	commandCatalogAPI := wailsapi.NewCommandCatalog()
	application.SetCommandCatalogAPI(a, commandCatalogAPI)
	updaterAPI := wailsapi.NewUpdater()
	application.SetUpdaterAPI(a, updaterAPI)
	profilesAPI := wailsapi.NewProfiles()
	application.SetProfilesAPI(a, profilesAPI)
	hotkeysAPI := wailsapi.NewHotkeys()
	application.SetHotkeysAPI(a, hotkeysAPI)
	permissionsAPI := wailsapi.NewPermissions()
	application.SetPermissionsAPI(a, permissionsAPI)
	fsTrustAPI := wailsapi.NewFSTrust()
	application.SetFSTrustAPI(a, fsTrustAPI)
	credentialsAPI := wailsapi.NewCredentials()
	application.SetCredentialsAPI(a, credentialsAPI)
	settingsAPI := wailsapi.NewSettings()
	wailsapi.AttachLogManager(settingsAPI, logManager)
	application.SetSettingsAPI(a, settingsAPI)
	mcpAPI := wailsapi.NewMCP()
	application.SetMCPAPI(a, mcpAPI)
	signalAPI := wailsapi.NewSignal()
	application.SetSignalAPI(a, signalAPI)
	terminalAPI := wailsapi.NewTerminal()
	application.SetTerminalAPI(a, terminalAPI)
	memoryAPI := wailsapi.NewMemory()
	application.SetMemoryAPI(a, memoryAPI)
	messagingAPI := wailsapi.NewMessaging()
	application.SetMessagingAPI(a, messagingAPI)
	welcomeAPI := wailsapi.NewWelcome()
	application.SetWelcomeAPI(a, welcomeAPI)
	workspaceAPI := wailsapi.NewWorkspace()
	application.SetWorkspaceAPI(a, workspaceAPI)
	legacyCleanupAPI := wailsapi.NewLegacyCleanup()
	application.SetLegacyCleanupAPI(a, legacyCleanupAPI)
	databaseAPI := wailsapi.NewDatabase()
	application.SetDatabaseAPI(a, databaseAPI)
	subagentAPI := wailsapi.NewSubagent()
	application.SetSubagentAPI(a, subagentAPI)
	tasklistAPI := wailsapi.NewTasklist()
	application.SetTasklistAPI(a, tasklistAPI)
	tasklistActionsAPI := wailsapi.NewTasklistActions()
	application.SetTasklistActionsAPI(a, tasklistActionsAPI)
	conversationsAPI := wailsapi.NewConversations()
	application.SetConversationsAPI(a, conversationsAPI)
	speechAPI := wailsapi.NewSpeech()
	application.SetSpeechAPI(a, speechAPI)
	jobsAPI := wailsapi.NewJobs()
	application.SetJobsAPI(a, jobsAPI)
	llmProvidersAPI := wailsapi.NewLLMProviders()
	application.SetLLMProvidersAPI(a, llmProvidersAPI)
	llmModelsAPI := wailsapi.NewLLMModels()
	application.SetLLMModelsAPI(a, llmModelsAPI)
	llmCatalogAPI := wailsapi.NewLLMCatalog()
	application.SetLLMCatalogAPI(a, llmCatalogAPI)
	chatAPI := wailsapi.NewChat()
	application.SetChatAPI(a, chatAPI)
	chatTimelineAPI := wailsapi.NewChatTimeline()
	application.SetChatTimelineAPI(a, chatTimelineAPI)
	acpCommandsAPI := wailsapi.NewACPCommands()
	application.SetACPCommandsAPI(a, acpCommandsAPI)
	acpProvidersAPI := wailsapi.NewACPProviders()
	application.SetACPProvidersAPI(a, acpProvidersAPI)
	acpOptionsAPI := wailsapi.NewACPOptions()
	application.SetACPOptionsAPI(a, acpOptionsAPI)
	acpRegistryAPI := wailsapi.NewACPRegistry()
	application.SetACPRegistryAPI(a, acpRegistryAPI)
	acpWorkDirAPI := wailsapi.NewACPWorkDir()
	application.SetACPWorkDirAPI(a, acpWorkDirAPI)
	acpInstallAPI := wailsapi.NewACPInstall()
	application.SetACPInstallAPI(a, acpInstallAPI)
	editorAPI := wailsapi.NewEditor()
	application.SetEditorAPI(a, editorAPI)
	exportImportAPI := wailsapi.NewExportImport()
	application.SetExportImportAPI(a, exportImportAPI)

	startupErrors := make(chan error, 1)
	runtimeEntered = true
	err = runDesktop(&options.App{
		Title:  "assistente",
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup: func(ctx context.Context) {
			if err := startDesktop(a, ctx); err != nil {
				logging.Errorf(ctx, "main", "Falha ao inicializar aplicação: %v", err)
				startupErrors <- err
				quitDesktop(ctx)
				return
			}
			activationReady(ctx)
		},
		OnShutdown: func(_ context.Context) {
			stopActivation()
			a.Shutdown()
		},
		// AEP-0088: multi-bind — App + binds de domínio migrados.
		Bind: []interface{}{
			a,
			wailsapi.NewProbe(),
			tokensAPI,
			allowlistsAPI,
			pluginsAPI,
			skillsAPI,
			toolsAPI,
			commandCatalogAPI,
			updaterAPI,
			profilesAPI,
			hotkeysAPI,
			permissionsAPI,
			fsTrustAPI,
			credentialsAPI,
			conversationsAPI,
			speechAPI,
			settingsAPI,
			mcpAPI,
			signalAPI,
			terminalAPI,
			memoryAPI,
			messagingAPI,
			welcomeAPI,
			workspaceAPI,
			legacyCleanupAPI,
			databaseAPI,
			subagentAPI,
			tasklistAPI,
			tasklistActionsAPI,
			jobsAPI,
			llmProvidersAPI,
			llmModelsAPI,
			llmCatalogAPI,
			chatAPI,
			chatTimelineAPI,
			acpCommandsAPI,
			acpProvidersAPI,
			acpOptionsAPI,
			acpRegistryAPI,
			acpWorkDirAPI,
			acpInstallAPI,
			editorAPI,
			exportImportAPI,
		},
		Debug: options.Debug{
			OpenInspectorOnStartup: false,
		},
	})

	select {
	case startupErr := <-startupErrors:
		reportFatalError(errorOutput, startupApplicationError(startupErr))
		return 1
	default:
	}
	if err != nil {
		reportFatalError(errorOutput, startupRunError(err))
		return 1
	}
	return 0
}
