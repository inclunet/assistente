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
