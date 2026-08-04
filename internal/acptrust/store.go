// Package acptrust guarda as autorizações permanentes que uma pessoa concede a
// um agente de código (AEP-0084 D9).
//
// Quando o agente pede permissão, ele oferece "permitir sempre" junto com
// "permitir uma vez". Escolher "sempre" só faz sentido se alguém lembrar disso
// depois — e o lugar de lembrar é o perfil, que é onde o agente está
// configurado. Cada perfil tem seu arquivo, como as allowlists de rede.
//
// A autorização vale por classe de ação (`execute`, `edit`, `read`…), e não
// pelo texto do pedido: o título que o agente manda é a linha de comando
// literal e muda a cada chamada, então guardar por texto nunca casaria de novo
// — e casar por pedaço de comando seria pior ainda, porque um prefixo idêntico
// não diz nada sobre o que vem depois dele.
package acptrust

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"assistente/internal/configdir"
)

// ErrEntryNotFound diz que não havia o que revogar. A interface não deve
// relatar revogação bem sucedida nesse caso.
var ErrEntryNotFound = errors.New("autorização permanente não encontrada")

// subdir é onde os arquivos ficam dentro de .assistente/. Separado das
// allowlists de rede e de comando, que têm formato e propósito próprios.
const subdir = "acp-permissions"

// storeFile é o formato em disco.
type storeFile struct {
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
}

// Entry é uma classe de ação que o perfil autoriza sem perguntar de novo.
type Entry struct {
	// Kind é a classe da ação, já normalizada pelo conjunto do protocolo.
	Kind string `json:"kind"`
	// GrantedAt é quando a pessoa autorizou. Serve para ela reconhecer a
	// autorização na hora de revogar.
	GrantedAt time.Time `json:"granted_at"`
}

// Store lê e grava as autorizações por perfil.
type Store struct {
	// mu serializa o acesso ao disco: gravar é ler-modificar-escrever, e duas
	// autorizações concorrentes perderiam uma delas sem exclusão mútua.
	mu      sync.RWMutex
	homeDir func() string
	now     func() time.Time
}

// NewStore usa o diretório canônico do configdir.
func NewStore() *Store {
	return &Store{homeDir: configdir.GetHomeDir, now: time.Now}
}

// NewStoreWithDir fixa o diretório base (testes).
func NewStoreWithDir(homeDir string) *Store {
	return &Store{homeDir: func() string { return homeDir }, now: time.Now}
}

// Allows diz se este perfil já autorizou esta classe de ação para sempre.
//
// Erro de leitura responde "não autoriza": diante de um arquivo ilegível, a
// única resposta segura é perguntar de novo, e não liberar uma ação na máquina
// de alguém por causa de um disco com problema.
func (s *Store) Allows(profileSlug, kind string) bool {
	kind = normalizeKind(kind)
	if s == nil || kind == "" {
		return false
	}
	path := s.profilePath(profileSlug)
	if path == "" {
		return false
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	entries, err := loadFile(path)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if normalizeKind(entry.Kind) == kind {
			return true
		}
	}
	return false
}

// Allow registra a autorização permanente. Repetir a mesma classe atualiza a
// data em vez de duplicar a entrada.
func (s *Store) Allow(profileSlug, kind string) error {
	kind = normalizeKind(kind)
	if s == nil {
		return errors.New("sem armazenamento de autorizações do agente")
	}
	if kind == "" {
		return errors.New("classe de ação vazia")
	}
	path := s.profilePath(profileSlug)
	if path == "" {
		return fmt.Errorf("perfil inválido para autorização permanente: %q", profileSlug)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := loadFile(path)
	if err != nil {
		// Não gravar por cima de um arquivo ilegível: apagaria autorizações
		// que ainda estão lá por causa de uma falha passageira.
		return fmt.Errorf("não foi possível ler as autorizações antes de gravar: %w", err)
	}
	return saveFile(path, upsert(entries, Entry{Kind: kind, GrantedAt: s.now()}))
}

// List devolve o que este perfil autoriza sem perguntar, para quem quiser ver
// ou revogar.
func (s *Store) List(profileSlug string) []Entry {
	if s == nil {
		return nil
	}
	path := s.profilePath(profileSlug)
	if path == "" {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	entries, err := loadFile(path)
	if err != nil {
		return nil
	}
	return entries
}

// Revoke tira a autorização permanente daquela classe.
func (s *Store) Revoke(profileSlug, kind string) error {
	kind = normalizeKind(kind)
	if s == nil {
		return errors.New("sem armazenamento de autorizações do agente")
	}
	path := s.profilePath(profileSlug)
	if path == "" || kind == "" {
		return ErrEntryNotFound
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := loadFile(path)
	if err != nil {
		return fmt.Errorf("não foi possível ler as autorizações antes de revogar: %w", err)
	}
	remaining, removed := without(entries, kind)
	if !removed {
		return ErrEntryNotFound
	}
	return saveFile(path, remaining)
}

func (s *Store) profilePath(profileSlug string) string {
	slug := sanitizeSlug(profileSlug)
	if slug == "" || s.homeDir == nil {
		return ""
	}
	home := s.homeDir()
	if home == "" {
		return ""
	}
	return filepath.Join(home, subdir, "profile-"+slug+".json")
}

// loadFile distingue arquivo ausente (nada autorizado, sem erro) de arquivo
// ilegível (erro), porque gravar em cima do segundo perderia dados.
func loadFile(path string) ([]Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("erro ao ler autorizações do agente em %s: %w", path, err)
	}
	var sf storeFile
	if err := json.Unmarshal(data, &sf); err != nil {
		return nil, fmt.Errorf("autorizações do agente inválidas em %s: %w", path, err)
	}
	return sf.Entries, nil
}

func saveFile(path string, entries []Entry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("erro ao criar diretório de autorizações do agente: %w", err)
	}
	data, err := json.MarshalIndent(storeFile{Version: 1, Entries: entries}, "", "  ")
	if err != nil {
		return fmt.Errorf("erro ao serializar autorizações do agente: %w", err)
	}
	// Escrita atômica: quem estiver lendo nunca vê um arquivo pela metade.
	tmp, err := os.CreateTemp(filepath.Dir(path), ".acp-permissions-*.tmp")
	if err != nil {
		return fmt.Errorf("erro ao criar arquivo temporário de autorizações: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("erro ao gravar autorizações do agente: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("erro ao finalizar autorizações do agente: %w", err)
	}
	if err := replaceFile(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("erro ao substituir autorizações do agente: %w", err)
	}
	return nil
}

// replaceFile renomeia por cima do destino, repetindo em caso de falha
// passageira. No Windows, antivírus e indexador seguram o arquivo por um
// instante e o rename falha por acesso negado; em POSIX isso não acontece.
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

func upsert(entries []Entry, entry Entry) []Entry {
	for i := range entries {
		if normalizeKind(entries[i].Kind) == entry.Kind {
			entries[i] = entry
			return entries
		}
	}
	return append(entries, entry)
}

func without(entries []Entry, kind string) ([]Entry, bool) {
	out := make([]Entry, 0, len(entries))
	removed := false
	for _, entry := range entries {
		if !removed && normalizeKind(entry.Kind) == kind {
			removed = true
			continue
		}
		out = append(out, entry)
	}
	return out, removed
}

func normalizeKind(kind string) string {
	return strings.ToLower(strings.TrimSpace(kind))
}

// sanitizeSlug tira separadores de caminho do slug: ele vira nome de arquivo, e
// um slug com ".." apontaria para fora do diretório.
func sanitizeSlug(slug string) string {
	slug = strings.TrimSpace(strings.ToLower(slug))
	slug = strings.ReplaceAll(slug, "..", "")
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return -1
		}
	}, slug)
}
