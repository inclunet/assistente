package nettrust

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"assistente/internal/configdir"
	"assistente/internal/logging"
	"assistente/internal/tools/invocationctx"
)

// subdir é o subdiretório dentro de .assistente/ onde as allowlists de rede
// persistentes ficam. Separado das allowlists de comando (allowlists/), que têm
// schema e propósito diferentes.
const subdir = "network-allowlist"

const (
	globalFile    = "global.json"
	workspaceFile = "workspace.json"
)

// storeFile é o formato em disco de uma allowlist de rede persistida.
type storeFile struct {
	Version int              `json:"version"`
	Entries []AllowlistEntry `json:"entries"`
}

// Manager guarda as allowlists de rede nos 5 escopos. Sessão fica em memória
// (chaveada por ConversationID); workspace/perfil/global são arquivos JSON sob
// .assistente/network-allowlist/. Reutiliza configdir para localizar os
// diretórios canônicos (home e workdir) — sem armazenamento paralelo.
type Manager struct {
	// mu serializa TODO acesso ao estado (mapa de sessão e arquivos persistidos).
	// Escritas em disco fazem read-modify-write, então precisam de exclusão mútua
	// para não perder entradas em autorizações concorrentes; leituras usam RLock
	// e ficam bloqueadas durante uma escrita, evitando leitura parcial.
	mu      sync.RWMutex
	session map[string][]AllowlistEntry // conversationID -> entradas de sessão

	// homeDir/workDir são injetáveis para testes; por padrão vêm do configdir.
	homeDir func() string
	workDir func() string
}

// NewManager cria um Manager usando os diretórios canônicos do configdir.
func NewManager() *Manager {
	return &Manager{
		session: make(map[string][]AllowlistEntry),
		homeDir: configdir.GetHomeDir,
		workDir: configdir.GetWorkDir,
	}
}

// NewManagerWithDirs cria um Manager com diretórios fixos (para testes). As
// funções recebem o .assistente base de cada camada.
func NewManagerWithDirs(homeDir, workDir string) *Manager {
	return &Manager{
		session: make(map[string][]AllowlistEntry),
		homeDir: func() string { return homeDir },
		workDir: func() string { return workDir },
	}
}

// Match procura uma autorização para host(:port) em todos os escopos, na ordem
// sessão → perfil → workspace → global. Devolve a decisão com a entrada/escopo
// que casou (Prompted=false: match veio de allowlist existente).
func (m *Manager) Match(ctx context.Context, host, port string) NetworkTrustDecision {
	convID, profileSlug := identityFromContext(ctx)

	// RLock cobre a leitura do mapa de sessão E dos arquivos: fica bloqueado
	// durante uma escrita (Add/Remove usam Lock), evitando leitura parcial.
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Sessão
	if convID != "" {
		if e := firstMatch(m.session[convID], host, port); e != nil {
			return NetworkTrustDecision{Allowed: true, Scope: ScopeSession, Entry: e}
		}
	}

	// Perfil
	if profileSlug != "" {
		if e := firstMatch(m.loadFile(m.profilePath(profileSlug)), host, port); e != nil {
			return NetworkTrustDecision{Allowed: true, Scope: ScopeProfile, Entry: e}
		}
	}

	// Workspace
	if e := firstMatch(m.loadFile(m.workspacePath()), host, port); e != nil {
		return NetworkTrustDecision{Allowed: true, Scope: ScopeWorkspace, Entry: e}
	}

	// Global
	if e := firstMatch(m.loadFile(m.globalPath()), host, port); e != nil {
		return NetworkTrustDecision{Allowed: true, Scope: ScopeGlobal, Entry: e}
	}

	return NetworkTrustDecision{Allowed: false}
}

// Add registra uma entrada no escopo indicado. ScopeOnce é no-op (a liberação
// da request corrente não é persistida). Preenche CreatedAt se ausente.
func (m *Manager) Add(ctx context.Context, entry AllowlistEntry) error {
	entry.Host = normalizeHost(entry.Host)
	if entry.Host == "" {
		return fmt.Errorf("host vazio não pode ser autorizado")
	}
	if !ValidScope(entry.Scope) {
		return fmt.Errorf("escopo inválido: %q", entry.Scope)
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	switch entry.Scope {
	case ScopeOnce:
		return nil
	case ScopeSession:
		convID, _ := identityFromContext(ctx)
		if convID == "" {
			return fmt.Errorf("escopo de sessão requer ConversationID no contexto")
		}
		m.session[convID] = upsert(m.session[convID], entry)
		return nil
	case ScopeWorkspace:
		return m.addToFile(m.workspacePath(), entry)
	case ScopeProfile:
		_, profileSlug := identityFromContext(ctx)
		if profileSlug == "" {
			return fmt.Errorf("escopo de perfil requer ProfileSlug no contexto")
		}
		return m.addToFile(m.profilePath(profileSlug), entry)
	case ScopeGlobal:
		return m.addToFile(m.globalPath(), entry)
	default:
		return fmt.Errorf("escopo não suportado: %q", entry.Scope)
	}
}

// List devolve todas as entradas persistidas (workspace + global + perfil, se
// houver slug no ctx) mais as de sessão da conversa corrente. Cada entrada tem
// seu Scope preenchido. Para UI/depuração.
func (m *Manager) List(ctx context.Context) []AllowlistEntry {
	var out []AllowlistEntry
	convID, profileSlug := identityFromContext(ctx)

	m.mu.RLock()
	defer m.mu.RUnlock()

	if convID != "" {
		out = append(out, m.session[convID]...)
	}
	if profileSlug != "" {
		out = append(out, m.loadFile(m.profilePath(profileSlug))...)
	}
	out = append(out, m.loadFile(m.workspacePath())...)
	out = append(out, m.loadFile(m.globalPath())...)
	return out
}

// Remove apaga a primeira entrada que casar host(:port) no escopo indicado.
func (m *Manager) Remove(ctx context.Context, scope Scope, host, port string) error {
	host = normalizeHost(host)

	m.mu.Lock()
	defer m.mu.Unlock()

	switch scope {
	case ScopeSession:
		convID, _ := identityFromContext(ctx)
		if convID == "" {
			return fmt.Errorf("escopo de sessão requer ConversationID no contexto")
		}
		m.session[convID] = removeMatch(m.session[convID], host, port)
		return nil
	case ScopeWorkspace:
		return m.removeFromFile(m.workspacePath(), host, port)
	case ScopeProfile:
		_, profileSlug := identityFromContext(ctx)
		if profileSlug == "" {
			return fmt.Errorf("escopo de perfil requer ProfileSlug no contexto")
		}
		return m.removeFromFile(m.profilePath(profileSlug), host, port)
	case ScopeGlobal:
		return m.removeFromFile(m.globalPath(), host, port)
	default:
		return fmt.Errorf("escopo não removível: %q", scope)
	}
}

// ---- paths ----

func (m *Manager) globalPath() string {
	return filepath.Join(m.homeDir(), subdir, globalFile)
}

func (m *Manager) workspacePath() string {
	base := m.workDir()
	if base == "" {
		return ""
	}
	return filepath.Join(base, subdir, workspaceFile)
}

func (m *Manager) profilePath(slug string) string {
	slug = sanitizeSlug(slug)
	if slug == "" {
		return ""
	}
	return filepath.Join(m.homeDir(), subdir, "profile-"+slug+".json")
}

// ---- file I/O ----

func (m *Manager) loadFile(path string) []AllowlistEntry {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil // arquivo ausente = allowlist vazia
	}
	var sf storeFile
	if err := json.Unmarshal(data, &sf); err != nil {
		logging.Errorf(context.Background(), "nettrust.manager", "[NetTrust] arquivo inválido %s: %v", path, err)
		return nil
	}
	return sf.Entries
}

func (m *Manager) addToFile(path string, entry AllowlistEntry) error {
	if path == "" {
		return fmt.Errorf("caminho de allowlist indisponível para o escopo %q", entry.Scope)
	}
	entries := upsert(m.loadFile(path), entry)
	return m.saveFile(path, entries)
}

func (m *Manager) removeFromFile(path string, host, port string) error {
	if path == "" {
		return fmt.Errorf("caminho de allowlist indisponível")
	}
	entries := removeMatch(m.loadFile(path), host, port)
	return m.saveFile(path, entries)
}

func (m *Manager) saveFile(path string, entries []AllowlistEntry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("erro ao criar diretório de allowlist de rede: %w", err)
	}
	data, err := json.MarshalIndent(storeFile{Version: 1, Entries: entries}, "", "  ")
	if err != nil {
		return fmt.Errorf("erro ao serializar allowlist de rede: %w", err)
	}
	// Escrita atômica: grava num arquivo temporário e renomeia por cima. Evita
	// que um leitor (mesmo em outro processo) veja um arquivo truncado no meio da
	// escrita — o rename é atômico no mesmo diretório.
	tmp, err := os.CreateTemp(filepath.Dir(path), ".allowlist-*.tmp")
	if err != nil {
		return fmt.Errorf("erro ao criar arquivo temporário de allowlist de rede: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("erro ao gravar allowlist de rede: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("erro ao finalizar allowlist de rede: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("erro ao substituir allowlist de rede: %w", err)
	}
	return nil
}

// ---- helpers ----

func firstMatch(entries []AllowlistEntry, host, port string) *AllowlistEntry {
	for i := range entries {
		if entries[i].Matches(host, port) {
			e := entries[i]
			return &e
		}
	}
	return nil
}

// upsert substitui uma entrada com mesmo host+port (idempotente) ou anexa.
func upsert(entries []AllowlistEntry, entry AllowlistEntry) []AllowlistEntry {
	for i := range entries {
		if strings.EqualFold(normalizeHost(entries[i].Host), normalizeHost(entry.Host)) &&
			strings.EqualFold(entries[i].Port, entry.Port) {
			entries[i] = entry
			return entries
		}
	}
	return append(entries, entry)
}

func removeMatch(entries []AllowlistEntry, host, port string) []AllowlistEntry {
	out := entries[:0]
	removed := false
	for _, e := range entries {
		if !removed && strings.EqualFold(normalizeHost(e.Host), normalizeHost(host)) && strings.EqualFold(e.Port, port) {
			removed = true
			continue
		}
		out = append(out, e)
	}
	return out
}

func identityFromContext(ctx context.Context) (conversationID, profileSlug string) {
	if inv, ok := invocationctx.Get(ctx); ok {
		return inv.ConversationID, inv.ProfileSlug
	}
	return "", ""
}

// sanitizeSlug remove separadores de caminho de um slug de perfil para evitar
// path traversal ao montar o nome do arquivo.
func sanitizeSlug(slug string) string {
	slug = strings.TrimSpace(strings.ToLower(slug))
	slug = strings.ReplaceAll(slug, "..", "")
	slug = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return -1
		}
	}, slug)
	return slug
}
