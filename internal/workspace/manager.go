package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	assistenteDir  = ".assistente"
	workspaceFile  = "workspace.yaml"
	indexFile      = "index.yaml"
	workspacesDir  = "workspaces"
)

// Manager gerencia workspaces: CRUD, persistência YAML e índice global.
type Manager struct {
	mu        sync.RWMutex
	active    *Workspace
	activePath string // diretório base do workspace ativo (contém .assistente/)
	homeDir   string // ~/.assistente/
}

// NewManager cria um novo workspace manager.
// homeDir é o diretório ~/.assistente/ (onde ficam workspaces avulsos e o índice).
func NewManager(homeDir string) *Manager {
	return &Manager{
		homeDir: homeDir,
	}
}

// Initialize carrega ou cria o workspace conforme a lógica de resolução da AEP.
// workDir é o diretório de trabalho atual (pode ser vazio para abrir sem diretório).
func (m *Manager) Initialize(workDir string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Garante que os diretórios base existam
	if err := os.MkdirAll(filepath.Join(m.homeDir, workspacesDir), 0755); err != nil {
		return fmt.Errorf("failed to create workspaces directory: %w", err)
	}

	// 1. Se workDir fornecido, verifica se já tem workspace.yaml
	if workDir != "" {
		wsPath := filepath.Join(workDir, assistenteDir, workspaceFile)
		if _, err := os.Stat(wsPath); err == nil {
			ws, err := m.loadWorkspaceFile(wsPath)
			if err != nil {
				return fmt.Errorf("failed to load workspace at %s: %w", wsPath, err)
			}
			m.active = ws
			m.activePath = workDir
			m.touchIndex(ws, workDir)
			return nil
		}

		// workDir existe mas sem workspace — cria um novo ali
		ws := m.newWorkspace("Workspace")
		if err := m.saveWorkspace(ws, workDir); err != nil {
			return fmt.Errorf("failed to create workspace at %s: %w", workDir, err)
		}
		m.active = ws
		m.activePath = workDir
		m.touchIndex(ws, workDir)
		return nil
	}

	// 2. Sem workDir — carrega último usado via índice
	idx, _ := m.loadIndex()
	if idx != nil && idx.LastOpened != "" {
		for _, entry := range idx.Workspaces {
			if entry.ID == idx.LastOpened {
				wsPath := filepath.Join(entry.Path, assistenteDir, workspaceFile)
				if ws, err := m.loadWorkspaceFile(wsPath); err == nil {
					m.active = ws
					m.activePath = entry.Path
					return nil
				}
			}
		}
	}

	// 3. Fallback: cria workspace default
	defaultPath := filepath.Join(m.homeDir, workspacesDir, "default")
	defaultWsPath := filepath.Join(defaultPath, assistenteDir, workspaceFile)

	if _, err := os.Stat(defaultWsPath); err == nil {
		if ws, err := m.loadWorkspaceFile(defaultWsPath); err == nil {
			m.active = ws
			m.activePath = defaultPath
			m.touchIndex(ws, defaultPath)
			return nil
		}
	}

	ws := m.newWorkspace("Default")
	ws.ID = "ws-default"
	if err := m.saveWorkspace(ws, defaultPath); err != nil {
		return fmt.Errorf("failed to create default workspace: %w", err)
	}
	m.active = ws
	m.activePath = defaultPath
	m.touchIndex(ws, defaultPath)
	return nil
}

// Active retorna o workspace ativo (pode ser nil antes de Initialize).
func (m *Manager) Active() *Workspace {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.active
}

// ActivePath retorna o diretório base do workspace ativo.
func (m *Manager) ActivePath() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.activePath
}

// Save persiste o workspace ativo em disco.
func (m *Manager) Save() error {
	m.mu.RLock()
	ws := m.active
	path := m.activePath
	m.mu.RUnlock()

	if ws == nil {
		return fmt.Errorf("no active workspace")
	}

	ws.LastUsed = time.Now()
	return m.saveWorkspace(ws, path)
}

// List retorna todos os workspaces conhecidos no índice.
func (m *Manager) List() ([]WorkspaceInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	idx, err := m.loadIndex()
	if err != nil {
		return nil, err
	}

	var result []WorkspaceInfo
	for _, entry := range idx.Workspaces {
		info := WorkspaceInfo{
			ID:       entry.ID,
			Name:     entry.Name,
			Path:     entry.Path,
			IsActive: m.active != nil && m.active.ID == entry.ID,
		}

		// Tenta carregar mais detalhes
		wsPath := filepath.Join(entry.Path, assistenteDir, workspaceFile)
		if ws, err := m.loadWorkspaceFile(wsPath); err == nil {
			info.Profile = ws.Profile
			info.TabCount = len(ws.Tabs.Items)
		}

		result = append(result, info)
	}

	return result, nil
}

// Create cria um novo workspace avulso (em ~/.assistente/workspaces/<id>/).
func (m *Manager) Create(name string) (*Workspace, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	ws := m.newWorkspace(name)
	wsDir := filepath.Join(m.homeDir, workspacesDir, ws.ID)

	if err := m.saveWorkspace(ws, wsDir); err != nil {
		return nil, err
	}

	m.touchIndex(ws, wsDir)
	return ws, nil
}

// Switch alterna para outro workspace pelo ID.
func (m *Manager) Switch(workspaceID string) (*Workspace, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Salva o workspace atual antes de trocar
	if m.active != nil {
		m.active.LastUsed = time.Now()
		_ = m.saveWorkspace(m.active, m.activePath)
	}

	idx, err := m.loadIndex()
	if err != nil {
		return nil, fmt.Errorf("failed to load index: %w", err)
	}

	for _, entry := range idx.Workspaces {
		if entry.ID == workspaceID {
			wsPath := filepath.Join(entry.Path, assistenteDir, workspaceFile)
			ws, err := m.loadWorkspaceFile(wsPath)
			if err != nil {
				return nil, fmt.Errorf("failed to load workspace %s: %w", workspaceID, err)
			}

			m.active = ws
			m.activePath = entry.Path

			// Atualiza last_opened no índice
			idx.LastOpened = workspaceID
			_ = m.saveIndex(idx)

			return ws, nil
		}
	}

	return nil, fmt.Errorf("workspace not found: %s", workspaceID)
}

// Rename renomeia o workspace ativo.
func (m *Manager) Rename(newName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.active == nil {
		return fmt.Errorf("no active workspace")
	}
	if newName == "" {
		return fmt.Errorf("name cannot be empty")
	}

	m.active.Name = newName
	if err := m.saveWorkspace(m.active, m.activePath); err != nil {
		return err
	}

	m.touchIndex(m.active, m.activePath)
	return nil
}

// Delete remove um workspace pelo ID (não pode ser o ativo).
func (m *Manager) Delete(workspaceID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.active != nil && m.active.ID == workspaceID {
		return fmt.Errorf("cannot delete the active workspace")
	}

	idx, err := m.loadIndex()
	if err != nil {
		return err
	}

	var newEntries []IndexEntry
	var deletePath string
	for _, entry := range idx.Workspaces {
		if entry.ID == workspaceID {
			deletePath = entry.Path
			continue
		}
		newEntries = append(newEntries, entry)
	}

	if deletePath == "" {
		return fmt.Errorf("workspace not found: %s", workspaceID)
	}

	idx.Workspaces = newEntries
	if idx.LastOpened == workspaceID {
		idx.LastOpened = ""
	}
	_ = m.saveIndex(idx)

	// Remove workspace.yaml (mas não o diretório inteiro, pode ter outros arquivos)
	wsFile := filepath.Join(deletePath, assistenteDir, workspaceFile)
	_ = os.Remove(wsFile)

	return nil
}

// SetProfile define o perfil base do workspace ativo.
func (m *Manager) SetProfile(profileSlug string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.active == nil {
		return fmt.Errorf("no active workspace")
	}

	m.active.Profile = profileSlug
	return m.saveWorkspace(m.active, m.activePath)
}

// === Tab operations ===

// AddTab adiciona uma aba ao workspace ativo.
func (m *Manager) AddTab(tab Tab) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.active == nil {
		return fmt.Errorf("no active workspace")
	}

	if err := tab.Validate(); err != nil {
		return err
	}

	// Regra: no máximo 1 aba por conteúdo por workspace
	if tab.ContentID != "" {
		if existing := m.active.FindTabByContent(tab.Type, tab.ContentID); existing != nil {
			// Já existe — ativa essa aba em vez de criar duplicata
			m.active.Tabs.Active = existing.ID
			return m.saveWorkspace(m.active, m.activePath)
		}
	}

	// Posição no final se não especificada
	if tab.Position == 0 && len(m.active.Tabs.Items) > 0 {
		tab.Position = len(m.active.Tabs.Items)
	}

	m.active.Tabs.Items = append(m.active.Tabs.Items, tab)
	m.active.Tabs.Active = tab.ID

	return m.saveWorkspace(m.active, m.activePath)
}

// RemoveTab remove uma aba do workspace ativo.
func (m *Manager) RemoveTab(tabID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.active == nil {
		return fmt.Errorf("no active workspace")
	}

	idx := -1
	for i, t := range m.active.Tabs.Items {
		if t.ID == tabID {
			idx = i
			break
		}
	}

	if idx == -1 {
		return fmt.Errorf("tab not found: %s", tabID)
	}

	// Remove a aba
	m.active.Tabs.Items = append(m.active.Tabs.Items[:idx], m.active.Tabs.Items[idx+1:]...)

	// Se era a aba ativa, promove a próxima
	if m.active.Tabs.Active == tabID {
		if len(m.active.Tabs.Items) > 0 {
			nextIdx := idx
			if nextIdx >= len(m.active.Tabs.Items) {
				nextIdx = len(m.active.Tabs.Items) - 1
			}
			m.active.Tabs.Active = m.active.Tabs.Items[nextIdx].ID
		} else {
			m.active.Tabs.Active = ""
		}
	}

	// Recalcula posições
	for i := range m.active.Tabs.Items {
		m.active.Tabs.Items[i].Position = i
	}

	return m.saveWorkspace(m.active, m.activePath)
}

// SetActiveTab define a aba ativa.
func (m *Manager) SetActiveTab(tabID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.active == nil {
		return fmt.Errorf("no active workspace")
	}

	if m.active.FindTab(tabID) == nil {
		return fmt.Errorf("tab not found: %s", tabID)
	}

	m.active.Tabs.Active = tabID
	return m.saveWorkspace(m.active, m.activePath)
}

// UpdateTab atualiza campos de uma aba (título, estado, etc.).
func (m *Manager) UpdateTab(tabID string, updates map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.active == nil {
		return fmt.Errorf("no active workspace")
	}

	tab := m.active.FindTab(tabID)
	if tab == nil {
		return fmt.Errorf("tab not found: %s", tabID)
	}

	if title, ok := updates["title"].(string); ok {
		tab.Title = title
	}
	if contentID, ok := updates["content_id"].(string); ok {
		tab.ContentID = contentID
	}
	if state, ok := updates["state"].(map[string]any); ok {
		tab.State = state
	}
	if override, ok := updates["profile_override"].(map[string]any); ok {
		tab.ProfileOverride = override
	}

	return m.saveWorkspace(m.active, m.activePath)
}

// ReorderTabs reordena as abas conforme a lista de IDs fornecida.
func (m *Manager) ReorderTabs(orderedIDs []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.active == nil {
		return fmt.Errorf("no active workspace")
	}

	tabMap := make(map[string]*Tab, len(m.active.Tabs.Items))
	for i := range m.active.Tabs.Items {
		tabMap[m.active.Tabs.Items[i].ID] = &m.active.Tabs.Items[i]
	}

	reordered := make([]Tab, 0, len(orderedIDs))
	for i, id := range orderedIDs {
		if t, ok := tabMap[id]; ok {
			t.Position = i
			reordered = append(reordered, *t)
			delete(tabMap, id)
		}
	}

	// Adiciona tabs que não estavam na lista (edge case)
	for _, t := range tabMap {
		t.Position = len(reordered)
		reordered = append(reordered, *t)
	}

	m.active.Tabs.Items = reordered
	return m.saveWorkspace(m.active, m.activePath)
}

// === Persistência YAML ===

func (m *Manager) newWorkspace(name string) *Workspace {
	now := time.Now()
	return &Workspace{
		ID:        fmt.Sprintf("ws-%s", generateID()),
		Name:      name,
		CreatedAt: now,
		LastUsed:  now,
		Tabs: TabsState{
			Items: []Tab{},
		},
	}
}

func (m *Manager) loadWorkspaceFile(path string) (*Workspace, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var ws Workspace
	if err := yaml.Unmarshal(data, &ws); err != nil {
		return nil, fmt.Errorf("failed to parse workspace file: %w", err)
	}

	// Ordena tabs por posição
	sort.Slice(ws.Tabs.Items, func(i, j int) bool {
		return ws.Tabs.Items[i].Position < ws.Tabs.Items[j].Position
	})

	return &ws, nil
}

func (m *Manager) saveWorkspace(ws *Workspace, basePath string) error {
	dir := filepath.Join(basePath, assistenteDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	data, err := yaml.Marshal(ws)
	if err != nil {
		return fmt.Errorf("failed to marshal workspace: %w", err)
	}

	return os.WriteFile(filepath.Join(dir, workspaceFile), data, 0644)
}

func (m *Manager) loadIndex() (*Index, error) {
	path := filepath.Join(m.homeDir, workspacesDir, indexFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Index{}, nil
		}
		return nil, err
	}

	var idx Index
	if err := yaml.Unmarshal(data, &idx); err != nil {
		return &Index{}, nil
	}
	return &idx, nil
}

func (m *Manager) saveIndex(idx *Index) error {
	dir := filepath.Join(m.homeDir, workspacesDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := yaml.Marshal(idx)
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(dir, indexFile), data, 0644)
}

// touchIndex atualiza ou adiciona a entrada do workspace no índice global.
func (m *Manager) touchIndex(ws *Workspace, basePath string) {
	idx, _ := m.loadIndex()
	if idx == nil {
		idx = &Index{}
	}

	idx.LastOpened = ws.ID

	found := false
	for i, entry := range idx.Workspaces {
		if entry.ID == ws.ID {
			idx.Workspaces[i].Name = ws.Name
			idx.Workspaces[i].Path = basePath
			idx.Workspaces[i].LastUsed = time.Now()
			found = true
			break
		}
	}

	if !found {
		idx.Workspaces = append(idx.Workspaces, IndexEntry{
			ID:       ws.ID,
			Name:     ws.Name,
			Path:     basePath,
			LastUsed: time.Now(),
		})
	}

	_ = m.saveIndex(idx)
}

func generateID() string {
	// ID curto baseado em timestamp + random suffix
	return fmt.Sprintf("%x", time.Now().UnixNano()&0xFFFFFFFFFF)
}
