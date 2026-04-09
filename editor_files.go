package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"assistente/internal/configdir"
	"assistente/internal/core/ports"
	"assistente/internal/tools/filesystem"
)

const editorOrphanDraftGracePeriod = 24 * time.Hour

// EditorMergeSession descreve um merge de 3 vias em andamento para uma aba.
type EditorMergeSession struct {
	OriginalPath    string `json:"originalPath"`
	MineDraftId     string `json:"mineDraftId"`
	DiskDraftId     string `json:"diskDraftId"`
	ConflictDraftId string `json:"conflictDraftId"`
	CreatedAt       int64  `json:"createdAt"`
}

// EditorState é o estado global do editor persistido em ~/.assistente/editor/state.json.
// Não inclui lista de abas (fica no workspace YAML) nem conteúdo de documentos (fica em arquivos).
type EditorState struct {
	FileModeByPath       map[string]string             `json:"fileModeByPath,omitempty"`
	MergeSessionsByTabId map[string]EditorMergeSession `json:"mergeSessionsByTabId,omitempty"`
}

type EditorOpenResult struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type EditorFileInfo struct {
	Path      string `json:"path"`
	Exists    bool   `json:"exists"`
	IsDir     bool   `json:"isDir"`
	Size      int64  `json:"size"`
	ModTimeMs int64  `json:"modTimeMs"`
}

// draftDir retorna o diretório onde os drafts são armazenados como arquivos.
func draftDir() string {
	return filepath.Join(configdir.GetHomeDir(), "editor", "drafts")
}

// editorStatePath retorna o caminho do arquivo de estado global do editor.
func editorStatePath() string {
	return filepath.Join(configdir.GetHomeDir(), "editor", "state.json")
}

// draftPath retorna o caminho completo do arquivo de draft para um draftId.
func draftPath(draftId string) (string, error) {
	id := strings.TrimSpace(draftId)
	if err := configdir.ValidateFilename(id); err != nil {
		return "", fmt.Errorf("draftId inválido: %w", err)
	}
	return filepath.Join(draftDir(), id+".md"), nil
}

// EditorGetDraftPath retorna o caminho em disco de um draft (sem criá-lo).
// O frontend usa isso para saber qual filePath associar a um novo documento.
func (a *App) EditorGetDraftPath(draftId string) (string, error) {
	p, err := draftPath(draftId)
	if err != nil {
		return "", err
	}
	return p, nil
}

// EditorWriteDraft persiste o conteúdo de um draft em disco.
func (a *App) EditorWriteDraft(draftId string, content string) error {
	p, err := draftPath(draftId)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return fmt.Errorf("falha ao criar diretório de drafts: %w", err)
	}
	if err := filesystem.WriteFileBytes(p, []byte(content), 0644); err != nil {
		return fmt.Errorf("falha ao salvar draft: %w", err)
	}
	return nil
}

// EditorReadDraft lê o conteúdo de um draft do disco.
func (a *App) EditorReadDraft(draftId string) (string, error) {
	p, err := draftPath(draftId)
	if err != nil {
		return "", err
	}
	data, err := filesystem.ReadFileBytes(p)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("draft não encontrado")
		}
		return "", fmt.Errorf("falha ao ler draft: %w", err)
	}
	return string(data), nil
}

// EditorDeleteDraft remove o arquivo de draft do disco.
func (a *App) EditorDeleteDraft(draftId string) error {
	p, err := draftPath(draftId)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("falha ao remover draft: %w", err)
	}
	return nil
}

// EditorLoadState carrega o estado global do editor do arquivo state.json.
func (a *App) EditorLoadState() (*EditorState, error) {
	p := editorStatePath()
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return &EditorState{
				FileModeByPath:       map[string]string{},
				MergeSessionsByTabId: map[string]EditorMergeSession{},
			}, nil
		}
		return nil, fmt.Errorf("falha ao ler editor/state.json: %w", err)
	}

	var state EditorState
	if err := json.Unmarshal(data, &state); err != nil {
		// Se corrompido, retorna estado limpo
		return &EditorState{
			FileModeByPath:       map[string]string{},
			MergeSessionsByTabId: map[string]EditorMergeSession{},
		}, nil
	}
	if state.FileModeByPath == nil {
		state.FileModeByPath = map[string]string{}
	}
	if state.MergeSessionsByTabId == nil {
		state.MergeSessionsByTabId = map[string]EditorMergeSession{}
	}
	return &state, nil
}

// EditorSaveState persiste o estado global do editor no arquivo state.json.
func (a *App) EditorSaveState(state EditorState) error {
	p := editorStatePath()
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return fmt.Errorf("falha ao criar diretório do editor: %w", err)
	}
	b, err := json.MarshalIndent(&state, "", "  ")
	if err != nil {
		return fmt.Errorf("falha ao serializar editor state: %w", err)
	}
	if err := os.WriteFile(p, b, 0644); err != nil {
		return fmt.Errorf("falha ao salvar editor/state.json: %w", err)
	}
	return nil
}

// EditorOpenFile abre o diálogo nativo e retorna conteúdo + path.
func (a *App) EditorOpenFile() (*EditorOpenResult, error) {
	if a.ctx == nil {
		return nil, fmt.Errorf("app não inicializado")
	}

	path, err := a.dialogPort.OpenFileDialog(ports.OpenFileOptions{
		Title: "Abrir arquivo",
		Filters: []ports.FileFilter{
			{DisplayName: "Markdown", Pattern: "*.md;*.markdown;*.txt"},
			{DisplayName: "Todos os arquivos", Pattern: "*.*"},
		},
	})
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(path) == "" {
		return &EditorOpenResult{Path: "", Content: ""}, nil
	}

	data, err := filesystem.ReadFileBytes(path)
	if err != nil {
		return nil, fmt.Errorf("falha ao ler arquivo: %w", err)
	}

	return &EditorOpenResult{Path: path, Content: string(data)}, nil
}

// EditorReadFile lê um arquivo por path (usado na restauração da sessão).
func (a *App) EditorReadFile(path string) (string, error) {
	p := strings.TrimSpace(path)
	if p == "" {
		return "", fmt.Errorf("path vazio")
	}
	data, err := filesystem.ReadFileBytes(p)
	if err != nil {
		return "", fmt.Errorf("falha ao ler arquivo: %w", err)
	}
	return string(data), nil
}

// EditorGetFileInfo retorna metadados simples do arquivo para detectar mudanças externas.
func (a *App) EditorGetFileInfo(path string) (*EditorFileInfo, error) {
	p := strings.TrimSpace(path)
	if p == "" {
		return nil, fmt.Errorf("path vazio")
	}

	info, err := os.Stat(p)
	if err != nil {
		if os.IsNotExist(err) {
			return &EditorFileInfo{Path: p, Exists: false, IsDir: false, Size: 0, ModTimeMs: 0}, nil
		}
		return nil, fmt.Errorf("falha ao stat arquivo: %w", err)
	}

	return &EditorFileInfo{
		Path:      p,
		Exists:    true,
		IsDir:     info.IsDir(),
		Size:      info.Size(),
		ModTimeMs: info.ModTime().UnixMilli(),
	}, nil
}

// EditorWriteFile escreve conteúdo em um arquivo existente/destino escolhido.
func (a *App) EditorWriteFile(path string, content string) error {
	p := strings.TrimSpace(path)
	if p == "" {
		return fmt.Errorf("path vazio")
	}
	if err := filesystem.WriteFileBytes(p, []byte(content), 0644); err != nil {
		return fmt.Errorf("falha ao salvar arquivo: %w", err)
	}
	return nil
}

// EditorRenameFile renomeia um arquivo existente no disco.
func (a *App) EditorRenameFile(oldPath string, newBaseName string) (string, error) {
	return filesystem.RenameFileSameDirWithPolicy(oldPath, newBaseName, filesystem.EditorPolicy())
}

// EditorSaveFileDialog abre o diálogo nativo de salvar e retorna o path escolhido.
func (a *App) EditorSaveFileDialog(suggestedFilename string) (string, error) {
	if a.ctx == nil {
		return "", fmt.Errorf("app não inicializado")
	}
	def := strings.TrimSpace(suggestedFilename)
	if def == "" {
		def = "documento.md"
	}
	path, err := a.dialogPort.SaveFileDialog(ports.SaveFileOptions{
		Title:           "Salvar arquivo",
		DefaultFilename: def,
		Filters: []ports.FileFilter{
			{DisplayName: "Markdown", Pattern: "*.md;*.markdown;*.txt"},
			{DisplayName: "Todos os arquivos", Pattern: "*.*"},
		},
	})
	if err != nil {
		return "", err
	}
	return path, nil
}

// cleanupEditorOrphanDraftsOnStartup remove arquivos de draft antigos não referenciados.
func (a *App) cleanupEditorOrphanDraftsOnStartup() error {
	dir := draftDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // diretório ainda não existe
		}
		return err
	}

	cutoff := time.Now().Add(-editorOrphanDraftGracePeriod)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(cutoff) {
			continue // recente demais para remover
		}
		_ = os.Remove(filepath.Join(dir, entry.Name()))
	}
	return nil
}
