package nettrust

import (
	"context"
	"encoding/json"
	"errors"
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

// ErrEntryNotFound é devolvido por Remove quando nenhuma entrada casa com o
// (host, port) informado no escopo — a UI não deve reportar revogação bem
// sucedida nesse caso.
var ErrEntryNotFound = errors.New("entrada de allowlist de rede não encontrada")

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
	// activeProfileSlug resolve o slug do perfil ATIVO quando o invocationctx não
	// o traz (o que ocorre em várias origens/estados). Sem esse fallback, o escopo
	// de perfil falharia em persistir/casar de forma inconsistente com a API de
	// gestão (que usa GetActiveSlug). Opcional; nil = só usa o slug do contexto.
	activeProfileSlug func() string
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

// SetActiveProfileSlugFunc injeta o resolvedor do slug do perfil ativo, usado
// como fallback quando o invocationctx não traz ProfileSlug. Mantém a persistência
// e o match do escopo de perfil consistentes com a API de gestão (GetActiveSlug).
func (m *Manager) SetActiveProfileSlugFunc(f func() string) {
	m.mu.Lock()
	m.activeProfileSlug = f
	m.mu.Unlock()
}

// SetWorkspaceDirFunc injeta um resolvedor dinâmico do diretório .assistente do
// workspace ATIVO. Necessário porque configdir.GetWorkDir() congela o os.Getwd()
// na primeira chamada, enquanto a troca de workspace em runtime só muda o
// activePath do workspace.Manager (o cwd do processo não muda). Sem isto, o
// escopo "workspace" ficaria amarrado ao diretório de lançamento. f é avaliado a
// cada operação; se devolver "", cai no comportamento anterior (configdir).
func (m *Manager) SetWorkspaceDirFunc(f func() string) {
	if f == nil {
		return
	}
	m.mu.Lock()
	// Embrulha f garantindo a semântica documentada: se o resolvedor devolver ""
	// (ex.: nenhum workspace ativo), cai no comportamento anterior (configdir),
	// para o escopo workspace não deixar de enxergar o arquivo.
	m.workDir = func() string {
		if dir := f(); dir != "" {
			return dir
		}
		return configdir.GetWorkDir()
	}
	m.mu.Unlock()
}

// Match procura uma autorização para host(:port) em todos os escopos, na ordem
// sessão → perfil → workspace → global. Devolve a decisão com a entrada/escopo
// que casou (Prompted=false: match veio de allowlist existente).
func (m *Manager) Match(ctx context.Context, host, port string) NetworkTrustDecision {
	convID, profileSlug := m.identity(ctx)

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
		if e := firstMatch(m.loadFileOrEmpty(ctx, m.profilePath(profileSlug)), host, port); e != nil {
			return NetworkTrustDecision{Allowed: true, Scope: ScopeProfile, Entry: e}
		}
	}

	// Workspace
	if e := firstMatch(m.loadFileOrEmpty(ctx, m.workspacePath()), host, port); e != nil {
		return NetworkTrustDecision{Allowed: true, Scope: ScopeWorkspace, Entry: e}
	}

	// Global
	if e := firstMatch(m.loadFileOrEmpty(ctx, m.globalPath()), host, port); e != nil {
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

	// Resolve a identidade ANTES do lock (identity pode consultar o resolvedor de
	// perfil, que usa outro mutex) — evita reentrância no m.mu.
	convID, profileSlug := m.identity(ctx)

	m.mu.Lock()
	defer m.mu.Unlock()

	switch entry.Scope {
	case ScopeOnce:
		return nil
	case ScopeSession:
		if convID == "" {
			return fmt.Errorf("escopo de sessão requer ConversationID no contexto")
		}
		m.session[convID] = upsert(m.session[convID], entry)
		return nil
	case ScopeWorkspace:
		return m.addToFile(m.workspacePath(), entry)
	case ScopeProfile:
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
	convID, profileSlug := m.identity(ctx)

	m.mu.RLock()
	defer m.mu.RUnlock()

	if convID != "" {
		out = append(out, m.session[convID]...)
	}
	if profileSlug != "" {
		out = append(out, m.loadFileOrEmpty(ctx, m.profilePath(profileSlug))...)
	}
	out = append(out, m.loadFileOrEmpty(ctx, m.workspacePath())...)
	out = append(out, m.loadFileOrEmpty(ctx, m.globalPath())...)
	return out
}

// Remove apaga a primeira entrada que casar host(:port) no escopo indicado.
func (m *Manager) Remove(ctx context.Context, scope Scope, host, port string) error {
	host = normalizeHost(host)

	// Resolve a identidade ANTES do lock (ver Add).
	convID, profileSlug := m.identity(ctx)

	m.mu.Lock()
	defer m.mu.Unlock()

	switch scope {
	case ScopeSession:
		if convID == "" {
			return fmt.Errorf("escopo de sessão requer ConversationID no contexto")
		}
		entries, removed := removeMatch(m.session[convID], host, port)
		if !removed {
			return ErrEntryNotFound
		}
		m.session[convID] = entries
		return nil
	case ScopeWorkspace:
		return m.removeFromFile(m.workspacePath(), host, port)
	case ScopeProfile:
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

// loadFile lê as entradas persistidas. Distingue "arquivo ausente" (allowlist
// vazia, sem erro) de "erro de leitura/JSON inválido" (erro) — crucial para que
// uma escrita subsequente NÃO sobrescreva dados que ainda estão no disco por
// causa de uma falha transitória de leitura.
func (m *Manager) loadFile(path string) ([]AllowlistEntry, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil // arquivo ausente = allowlist vazia
		}
		return nil, fmt.Errorf("erro ao ler allowlist de rede %s: %w", path, err)
	}
	var sf storeFile
	if err := json.Unmarshal(data, &sf); err != nil {
		return nil, fmt.Errorf("allowlist de rede inválida %s: %w", path, err)
	}
	return sf.Entries, nil
}

// loadFileOrEmpty é a versão tolerante usada em leituras (Match/List): trata
// ausência OU erro como allowlist vazia (logando o erro). Nunca é usada antes de
// uma escrita — para isso usa-se loadFile, que propaga o erro. Recebe o ctx da
// operação para o log preservar correlação (conversation/profile/trace).
func (m *Manager) loadFileOrEmpty(ctx context.Context, path string) []AllowlistEntry {
	entries, err := m.loadFile(path)
	if err != nil {
		logging.Errorf(ctx, "nettrust.manager", "[NetTrust] %v", err)
		return nil
	}
	return entries
}

func (m *Manager) addToFile(path string, entry AllowlistEntry) error {
	if path == "" {
		return fmt.Errorf("caminho de allowlist indisponível para o escopo %q", entry.Scope)
	}
	existing, err := m.loadFile(path)
	if err != nil {
		// Não gravar por cima de um arquivo ilegível: sobrescreveria entradas
		// ainda presentes no disco por uma falha transitória.
		return fmt.Errorf("não foi possível ler a allowlist antes de gravar (evitando perda de dados): %w", err)
	}
	return m.saveFile(path, upsert(existing, entry))
}

func (m *Manager) removeFromFile(path string, host, port string) error {
	if path == "" {
		return fmt.Errorf("caminho de allowlist indisponível")
	}
	existing, err := m.loadFile(path)
	if err != nil {
		return fmt.Errorf("não foi possível ler a allowlist antes de remover: %w", err)
	}
	entries, removed := removeMatch(existing, host, port)
	if !removed {
		return ErrEntryNotFound
	}
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
	if err := replaceFile(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("erro ao substituir allowlist de rede: %w", err)
	}
	return nil
}

// replaceFile substitui path pelo conteúdo de tmpName via rename atômico.
//
// os.Rename já sobrescreve o destino existente no Windows (usa MoveFileEx com
// MOVEFILE_REPLACE_EXISTING), então "o destino já existe" não é um problema. O
// que PODE ocorrer no Windows é uma falha transitória (ERROR_ACCESS_DENIED /
// sharing violation) quando antivírus, indexador ou um leitor concorrente estão
// segurando o destino por um instante — nesse caso re-tentamos com um backoff
// curto antes de desistir. Em POSIX o rename é atômico e não sofre disso.
func replaceFile(tmpName, path string) error {
	const attempts = 5
	var err error
	for i := 0; i < attempts; i++ {
		if err = os.Rename(tmpName, path); err == nil {
			return nil
		}
		time.Sleep(time.Duration(i+1) * 10 * time.Millisecond)
	}
	return err
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

// removeMatch devolve as entradas sem a primeira que casar (host, port) e se
// alguma foi de fato removida. Não muta o slice de entrada.
func removeMatch(entries []AllowlistEntry, host, port string) ([]AllowlistEntry, bool) {
	out := make([]AllowlistEntry, 0, len(entries))
	removed := false
	for _, e := range entries {
		if !removed && strings.EqualFold(normalizeHost(e.Host), normalizeHost(host)) && strings.EqualFold(e.Port, port) {
			removed = true
			continue
		}
		out = append(out, e)
	}
	return out, removed
}

func identityFromContext(ctx context.Context) (conversationID, profileSlug string) {
	if inv, ok := invocationctx.Get(ctx); ok {
		return inv.ConversationID, inv.ProfileSlug
	}
	return "", ""
}

// identity resolve conversa/perfil do ctx, com fallback do slug de perfil ativo
// (activeProfileSlug) quando o contexto não o traz — mantendo persistência e
// match do escopo de perfil consistentes com a API de gestão. Deve ser chamado
// FORA de m.mu (adquire RLock brevemente para ler o resolvedor).
func (m *Manager) identity(ctx context.Context) (conversationID, profileSlug string) {
	conversationID, profileSlug = identityFromContext(ctx)
	if profileSlug != "" {
		return conversationID, profileSlug
	}
	m.mu.RLock()
	f := m.activeProfileSlug
	m.mu.RUnlock()
	if f != nil {
		profileSlug = f()
	}
	return conversationID, profileSlug
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
