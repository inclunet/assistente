package wailsapi

import "errors"

// errNilSession é interno: Session nil na borda (wiring incompleto).
var errNilSession = errors.New("wailsapi: session is nil")

// ErrTokensNotWired indica que o bind Tokens ainda não recebeu controller/session.
var ErrTokensNotWired = errors.New("wailsapi: tokens bind not wired")

// ErrAllowlistsNotWired indica que o bind Allowlists ainda não recebeu controller/session.
var ErrAllowlistsNotWired = errors.New("wailsapi: allowlists bind not wired")
