package main

import (
	"assistente/internal/logging"
	"context"
	"embed"
	"time"

	"assistente/adapters/wails"
	application "assistente/internal/app"
	"assistente/internal/wailsapi"

	wailslib "github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	a := application.NewApp()
	tokensAPI := wailsapi.NewTokens()
	application.SetTokensAPI(a, tokensAPI)
	allowlistsAPI := wailsapi.NewAllowlists()
	application.SetAllowlistsAPI(a, allowlistsAPI)
	skillsAPI := wailsapi.NewSkills()
	application.SetSkillsAPI(a, skillsAPI)
	toolsAPI := wailsapi.NewTools()
	application.SetToolsAPI(a, toolsAPI)
	updaterAPI := wailsapi.NewUpdater()
	application.SetUpdaterAPI(a, updaterAPI)
	profilesAPI := wailsapi.NewProfiles()
	application.SetProfilesAPI(a, profilesAPI)
	hotkeysAPI := wailsapi.NewHotkeys()
	application.SetHotkeysAPI(a, hotkeysAPI)
	netTrustAPI := wailsapi.NewNetTrust()
	application.SetNetTrustAPI(a, netTrustAPI)
	credentialsAPI := wailsapi.NewCredentials()
	application.SetCredentialsAPI(a, credentialsAPI)
	settingsAPI := wailsapi.NewSettings()
	application.SetSettingsAPI(a, settingsAPI)
	mcpAPI := wailsapi.NewMCP()
	application.SetMCPAPI(a, mcpAPI)
	signalAPI := wailsapi.NewSignal()
	application.SetSignalAPI(a, signalAPI)
	terminalAPI := wailsapi.NewTerminal()
	application.SetTerminalAPI(a, terminalAPI)
	memoryAPI := wailsapi.NewMemory()
	application.SetMemoryAPI(a, memoryAPI)
	welcomeAPI := wailsapi.NewWelcome()
	application.SetWelcomeAPI(a, welcomeAPI)
	legacyCleanupAPI := wailsapi.NewLegacyCleanup()
	application.SetLegacyCleanupAPI(a, legacyCleanupAPI)
	databaseAPI := wailsapi.NewDatabase()
	application.SetDatabaseAPI(a, databaseAPI)
	subagentAPI := wailsapi.NewSubagent()
	application.SetSubagentAPI(a, subagentAPI)
	tasklistActionsAPI := wailsapi.NewTasklistActions()
	application.SetTasklistActionsAPI(a, tasklistActionsAPI)
	jobsAPI := wailsapi.NewJobs()
	application.SetJobsAPI(a, jobsAPI)
	llmProvidersAPI := wailsapi.NewLLMProviders()
	application.SetLLMProvidersAPI(a, llmProvidersAPI)
	acpCommandsAPI := wailsapi.NewACPCommands()
	application.SetACPCommandsAPI(a, acpCommandsAPI)
	acpProvidersAPI := wailsapi.NewACPProviders()
	application.SetACPProvidersAPI(a, acpProvidersAPI)
	acpRegistryAPI := wailsapi.NewACPRegistry()
	application.SetACPRegistryAPI(a, acpRegistryAPI)
	acpWorkDirAPI := wailsapi.NewACPWorkDir()
	application.SetACPWorkDirAPI(a, acpWorkDirAPI)
	acpInstallAPI := wailsapi.NewACPInstall()
	application.SetACPInstallAPI(a, acpInstallAPI)
	acpTrustAPI := wailsapi.NewACPTrust()
	application.SetACPTrustAPI(a, acpTrustAPI)

	err := wailslib.Run(&options.App{
		Title:  "assistente",
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup: func(ctx context.Context) {
			if err := a.StartupWithAdapters(ctx,
				wails.NewEmitterAdapter(ctx),
				wails.NewWindowAdapter(ctx),
				wails.NewDialogAdapter(ctx),
			); err != nil {
				logging.Fatalf(ctx, "main", "Falha ao inicializar aplicação: %v", err)
			}
			// Restaura foco da janela (resolve bug do Wails no Windows)
			go func() {
				timer := time.NewTimer(400 * time.Millisecond)
				defer timer.Stop()
				select {
				case <-timer.C:
					a.ShowWindow()
				case <-ctx.Done():
					return
				}
			}()
		},
		OnShutdown: func(_ context.Context) {
			a.Shutdown()
		},
		// AEP-0088: multi-bind — App + binds de domínio migrados.
		Bind: []interface{}{
			a,
			wailsapi.NewProbe(),
			tokensAPI,
			allowlistsAPI,
			skillsAPI,
			toolsAPI,
			updaterAPI,
			profilesAPI,
			hotkeysAPI,
			netTrustAPI,
			credentialsAPI,
			settingsAPI,
			mcpAPI,
			signalAPI,
			terminalAPI,
			memoryAPI,
			welcomeAPI,
			legacyCleanupAPI,
			databaseAPI,
			subagentAPI,
			tasklistActionsAPI,
			jobsAPI,
			llmProvidersAPI,
			acpCommandsAPI,
			acpProvidersAPI,
			acpRegistryAPI,
			acpWorkDirAPI,
			acpInstallAPI,
			acpTrustAPI,
		},
		Debug: options.Debug{
			OpenInspectorOnStartup: false,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
