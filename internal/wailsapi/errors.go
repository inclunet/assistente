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
