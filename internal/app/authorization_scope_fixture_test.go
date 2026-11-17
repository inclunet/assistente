package app

import "assistente/internal/authorization"

func allAuthorizationScopesForTest() []authorization.Scope {
	return []authorization.Scope{authorization.ScopeOnce, authorization.ScopeSession, authorization.ScopeWorkspace, authorization.ScopeProfile, authorization.ScopeGlobal}
}
