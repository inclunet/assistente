// Package trustscope implementa o contrato comum de trust escopado usado por
// domínios como rede e filesystem.
package trustscope

// Scope define o alcance de uma decisão de trust, do mais efêmero ao mais amplo.
type Scope string

const (
	// ScopeOnce vale apenas para a tentativa atual e nunca é armazenado.
	ScopeOnce Scope = "once"
	// ScopeSession vale durante a conversa e fica somente em memória.
	ScopeSession Scope = "session"
	// ScopeWorkspace persiste no workspace ativo.
	ScopeWorkspace Scope = "workspace"
	// ScopeProfile persiste para o perfil ativo.
	ScopeProfile Scope = "profile"
	// ScopeGlobal persiste para todos os workspaces e perfis.
	ScopeGlobal Scope = "global"
)

// ValidScope reporta se s pertence ao contrato compartilhado.
func ValidScope(s Scope) bool {
	switch s {
	case ScopeOnce, ScopeSession, ScopeWorkspace, ScopeProfile, ScopeGlobal:
		return true
	default:
		return false
	}
}

// IsPersistent reporta se o escopo é armazenado em disco.
func (s Scope) IsPersistent() bool {
	return s == ScopeWorkspace || s == ScopeProfile || s == ScopeGlobal
}
