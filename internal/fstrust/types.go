// Package fstrust implementa a allowlist escopável e o fluxo de consentimento
// para paths fora do sandbox das tools de filesystem (AEP-0092).
//
// Espelha internal/nettrust (escopos, persistência, limpeza de sessão) com
// entradas por path + operação e match file/dir.
package fstrust

import (
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Scope define o alcance de uma autorização de path, do mais efêmero ao mais
// amplo. A ordem de match no Manager é sessão → perfil → workspace → global.
type Scope string

const (
	// ScopeOnce libera apenas a tentativa atual; nunca é persistida.
	ScopeOnce Scope = "once"
	// ScopeSession vale enquanto durar a conversa/sessão (em memória).
	ScopeSession Scope = "session"
	// ScopeWorkspace persiste no .assistente/ do diretório de trabalho (projeto).
	ScopeWorkspace Scope = "workspace"
	// ScopeProfile persiste por perfil ativo (arquivo por slug em ~/.assistente/).
	ScopeProfile Scope = "profile"
	// ScopeGlobal persiste globalmente em ~/.assistente/.
	ScopeGlobal Scope = "global"
)

// ValidScope reporta se s é um escopo conhecido.
func ValidScope(s Scope) bool {
	switch s {
	case ScopeOnce, ScopeSession, ScopeWorkspace, ScopeProfile, ScopeGlobal:
		return true
	default:
		return false
	}
}

// IsPersistent reporta se o escopo é gravado em disco (workspace/profile/global).
func (s Scope) IsPersistent() bool {
	return s == ScopeWorkspace || s == ScopeProfile || s == ScopeGlobal
}

// Kind distingue autorização de arquivo exato versus diretório (prefixo).
type Kind string

const (
	// KindFile autoriza um path de arquivo exato + operação.
	KindFile Kind = "file"
	// KindDir autoriza qualquer path dentro do diretório (isWithinRoot) + operação.
	KindDir Kind = "dir"
)

// ValidKind reporta se k é um kind conhecido.
func ValidKind(k Kind) bool {
	switch k {
	case KindFile, KindDir:
		return true
	default:
		return false
	}
}

// AllowlistEntry é uma autorização de filesystem para um path + operação.
type AllowlistEntry struct {
	// Path é o caminho absoluto normalizado. Para KindDir é a raiz do diretório
	// (sem sufixo /**); o match usa isWithinRoot.
	Path string `json:"path"`
	// Kind é "file" (igualdade exata) ou "dir" (dentro do diretório).
	Kind Kind `json:"kind"`
	// Operation é a operação exata (read, write, edit, list, ...).
	Operation string `json:"operation"`
	// Scope é o alcance desta entrada. Entradas persistidas nunca têm ScopeOnce.
	Scope Scope `json:"scope"`
	// CreatedBy identifica a origem da autorização (ex.: "user", ou o skill slug).
	CreatedBy string `json:"created_by,omitempty"`
	// CreatedAt é o timestamp de criação (UTC).
	CreatedAt time.Time `json:"created_at"`
	// Reason é uma observação/motivo livre.
	Reason string `json:"reason,omitempty"`
}

// Matches reporta se esta entrada autoriza absPath para a operação dada.
//
// - Operation deve ser exatamente igual.
// - KindFile: igualdade de path normalizado (case-insensitive no Windows).
// - KindDir: absPath está dentro do diretório Path (semântica isWithinRoot).
func (e AllowlistEntry) Matches(absPath, operation string) bool {
	if e.Operation != operation {
		return false
	}
	path := NormalizePath(absPath)
	entryPath := NormalizePath(e.Path)
	if path == "" || entryPath == "" {
		return false
	}
	switch e.Kind {
	case KindFile:
		return path == entryPath
	case KindDir:
		return isWithinRoot(path, entryPath)
	default:
		return false
	}
}

// Decision é o resultado de uma consulta de autorização de path.
type Decision struct {
	Allowed  bool
	Scope    Scope
	Entry    *AllowlistEntry
	Prompted bool // true quando exigiu consentimento novo do usuário
}

// NormalizePath normaliza um path para comparação/persistência: Abs + Clean, e
// no Windows converte para minúsculas (mesma semântica de
// filesystem.normalizeForComparison após Abs).
func NormalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		abs = filepath.Clean(path)
	}
	if runtime.GOOS == "windows" {
		return strings.ToLower(abs)
	}
	return abs
}

// isWithinRoot reporta se absPath está dentro de absRoot (ou é o próprio root).
// Mesma semântica de internal/tools/filesystem.isWithinRoot.
func isWithinRoot(absPath, absRoot string) bool {
	root := normalizeForComparison(absRoot)
	path := normalizeForComparison(absPath)

	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}

// normalizeForComparison espelha filesystem.normalizeForComparison: Clean +
// lower no Windows (assume paths já absolutos quando possível).
func normalizeForComparison(p string) string {
	clean := filepath.Clean(p)
	if runtime.GOOS == "windows" {
		return strings.ToLower(clean)
	}
	return clean
}
