package wailsapi

import "errors"

// errNilSession é interno: Session nil na borda (wiring incompleto).
var errNilSession = errors.New("wailsapi: session is nil")

// ErrTokensNotWired indica que o bind Tokens ainda não recebeu controller/session.
var ErrTokensNotWired = errors.New("wailsapi: tokens bind not wired")

// ErrAllowlistsNotWired indica que o bind Allowlists ainda não recebeu controller/session.
var ErrAllowlistsNotWired = errors.New("wailsapi: allowlists bind not wired")

// ErrSkillsNotWired indica que o bind Skills ainda não recebeu controller/session.
var ErrSkillsNotWired = errors.New("wailsapi: skills bind not wired")

// ErrToolsNotWired indica que o bind Tools ainda não recebeu controller/session.
var ErrToolsNotWired = errors.New("wailsapi: tools bind not wired")

// ErrUpdaterNotWired indica que o bind Updater ainda não recebeu controller/session.
var ErrUpdaterNotWired = errors.New("wailsapi: updater bind not wired")

// ErrProfilesNotWired indica que o bind Profiles ainda não recebeu controller/session.
var ErrProfilesNotWired = errors.New("wailsapi: profiles bind not wired")

// ErrHotkeysNotWired indica que o bind Hotkeys ainda não recebeu controller/session.
var ErrHotkeysNotWired = errors.New("wailsapi: hotkeys bind not wired")

// ErrNetTrustNotWired indica que o bind NetTrust ainda não recebeu controller/session.
var ErrNetTrustNotWired = errors.New("wailsapi: nettrust bind not wired")

// ErrCredentialsNotWired indica que o bind Credentials ainda não recebeu controller/session.
var ErrCredentialsNotWired = errors.New("wailsapi: credentials bind not wired")

// ErrSettingsNotWired indica que o bind Settings ainda não recebeu controller/session.
var ErrSettingsNotWired = errors.New("wailsapi: settings bind not wired")

// ErrMCPNotWired indica que o bind MCP ainda não recebeu controller/session.
var ErrMCPNotWired = errors.New("wailsapi: mcp bind not wired")

// ErrSignalNotWired indica que o bind Signal ainda não recebeu controller/session.
var ErrSignalNotWired = errors.New("wailsapi: signal bind not wired")

// ErrTerminalNotWired indica que o bind Terminal ainda não recebeu controller/session.
var ErrTerminalNotWired = errors.New("wailsapi: terminal bind not wired")

// ErrMemoryNotWired indica que o bind Memory ainda não recebeu controller/session.
var ErrMemoryNotWired = errors.New("wailsapi: memory bind not wired")

// ErrWelcomeNotWired indica que o bind Welcome ainda não recebeu controller/runtime.
var ErrWelcomeNotWired = errors.New("wailsapi: welcome bind not wired")

// ErrLegacyCleanupNotWired indica que o bind LegacyCleanup ainda não recebeu session.
var ErrLegacyCleanupNotWired = errors.New("wailsapi: legacy cleanup bind not wired")

// ErrDatabaseNotWired indica que o bind Database ainda não recebeu controller/session.
var ErrDatabaseNotWired = errors.New("wailsapi: database bind not wired")

// ErrDatabaseResetFailed é o erro genérico devolvido ao caller quando
// ResetDatabase falha. O detalhe real (paths, syscalls) só vai para o logger
// local — defesa contra leak de filesystem em multi-user (AEP-0052).
var ErrDatabaseResetFailed = errors.New("database reset failed")

// ErrSubagentNotWired indica que o bind Subagent ainda não recebeu manager/session.
var ErrSubagentNotWired = errors.New("wailsapi: subagent bind not wired")

// ErrTasklistActionsNotWired indica que o bind TasklistActions ainda não recebeu controller/session.
var ErrTasklistActionsNotWired = errors.New("wailsapi: tasklist actions bind not wired")

// ErrJobsNotWired indica que o bind Jobs ainda não recebeu controller/session.
var ErrJobsNotWired = errors.New("wailsapi: jobs bind not wired")

// ErrLLMProvidersNotWired indica que o bind LLMProviders ainda não recebeu controller/session.
var ErrLLMProvidersNotWired = errors.New("wailsapi: llm providers bind not wired")

// ErrACPCommandsNotWired indica que o bind ACPCommands ainda não recebeu manager/session.
var ErrACPCommandsNotWired = errors.New("wailsapi: acp commands bind not wired")

// ErrACPProvidersNotWired indica que o bind ACPProviders ainda não recebeu manager/session.
var ErrACPProvidersNotWired = errors.New("wailsapi: acp providers bind not wired")

// ErrACPRegistryNotWired indica que o bind ACPRegistry ainda não recebeu session/catalogOf.
var ErrACPRegistryNotWired = errors.New("wailsapi: acp registry bind not wired")
