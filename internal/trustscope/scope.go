package trustscope

import "assistente/internal/authorization"

// Os adaptadores e o store compartilham o mesmo tipo de escopo do serviço.
type Scope = authorization.Scope

const (
	ScopeOnce      = authorization.ScopeOnce
	ScopeSession   = authorization.ScopeSession
	ScopeWorkspace = authorization.ScopeWorkspace
	ScopeProfile   = authorization.ScopeProfile
	ScopeGlobal    = authorization.ScopeGlobal
)

func ValidScope(scope Scope) bool { return authorization.ValidScope(scope) }
