package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"assistente/internal/configdir"
	"assistente/internal/tools/filesystem"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type EditorSession struct {
	Version         int                `json:"version"`
	AutoSaveEnabled bool               `json:"autoSaveEnabled"`
	ActiveTabId     string             `json:"activeTabId,omitempty"`
	ProfileSlug     string             `json:"profileSlug,omitempty"`
	Tabs            []EditorSessionTab `json:"tabs"`

	// Preferência de modo por arquivo (para reabrir mantendo o tipo de editor)
	FileModeByPath map[string]string `json:"fileModeByPath,omitempty"`

	// Recuperação pós-crash: se um arquivo estava em conflito externo, mantemos o lock.
	ExternalConflictLockedByTabId map[string]bool `json:"externalConflictLockedByTabId,omitempty"`
	// Recuperação pós-crash: se um merge estilo Git estava em andamento, mantemos os drafts e o link por tab.
	MergeSessionsByTabId map[string]EditorMergeSession `json:"mergeSessionsByTabId,omitempty"`
}

type EditorMergeSession struct {
	OriginalPath    string `json:"originalPath"`
	MineDraftId     string `json:"mineDraftId"`
	DiskDraftId     string `json:"diskDraftId"`
	ConflictDraftId string `json:"conflictDraftId"`
	CreatedAt       int64  `json:"createdAt"`
}

type EditorSessionTab struct {
	Id       string `json:"id"`
	Title    string `json:"title"`
	Mode     string `json:"mode"`
	FilePath string `json:"filePath,omitempty"`
	DraftId  string `json:"draftId,omitempty"`
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

func (a *App) editorBaseDir() (string, error) {
	base := configdir.GetHomeDir()
	if strings.TrimSpace(base) == "" {
		return "", fmt.Errorf("diretório home do app indisponível")
	}
	return filepath.Join(base, "editor"), nil
}

func (a *App) editorDraftsDir() (string, error) {
	base, err := a.editorBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "drafts"), nil
}

func (a *App) ensureEditorDirs() error {
	base, err := a.editorBaseDir()
	if err != nil {
		return err
	}
	drafts, err := a.editorDraftsDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(base, 0755); err != nil {
		return fmt.Errorf("falha ao criar dir do editor: %w", err)
	}
	if err := os.MkdirAll(drafts, 0755); err != nil {
		return fmt.Errorf("falha ao criar dir de drafts do editor: %w", err)
	}
	return nil
}

func editorSessionPath(baseDir string) string {
	return filepath.Join(baseDir, "session.json")
}

func draftFilename(draftId string) (string, error) {
	id := strings.TrimSpace(draftId)
	if err := configdir.ValidateFilename(id); err != nil {
		return "", fmt.Errorf("draftId inválido: %w", err)
	}
	return id + ".md", nil
}

// EditorLoadSession restaura a lista de guias abertas e preferências do editor.
// Arquivo: ~/.assistente/editor/session.json
func (a *App) EditorLoadSession() (*EditorSession, error) {
	if err := a.ensureEditorDirs(); err != nil {
		return nil, err
	}

	base, _ := a.editorBaseDir()
	p := editorSessionPath(base)

	data, err := filesystem.ReadFileBytes(p)
	if err != nil {
		if os.IsNotExist(err) {
			return &EditorSession{
				Version:                       2,
				AutoSaveEnabled:               true,
				ProfileSlug:                   "",
				Tabs:                          []EditorSessionTab{},
				FileModeByPath:                map[string]string{},
				ExternalConflictLockedByTabId: map[string]bool{},
				MergeSessionsByTabId:          map[string]EditorMergeSession{},
			}, nil
		}
		return nil, fmt.Errorf("falha ao ler sessão do editor: %w", err)
	}

	var sess EditorSession
	if err := json.Unmarshal(data, &sess); err != nil {
		// Se estiver corrompido, não quebra o app.
		return &EditorSession{
			Version:                       2,
			AutoSaveEnabled:               true,
			ProfileSlug:                   "",
			Tabs:                          []EditorSessionTab{},
			FileModeByPath:                map[string]string{},
			ExternalConflictLockedByTabId: map[string]bool{},
			MergeSessionsByTabId:          map[string]EditorMergeSession{},
		}, nil
	}

	if sess.Version == 0 {
		sess.Version = 2
	}
	// AutoSave default: ligado
	if !sess.AutoSaveEnabled {
		// mantém o valor salvo; apenas evita zero-value quebrar versões antigas
	}
	if sess.Tabs == nil {
		sess.Tabs = []EditorSessionTab{}
	}
	if sess.FileModeByPath == nil {
		sess.FileModeByPath = map[string]string{}
	}
	if sess.ExternalConflictLockedByTabId == nil {
		sess.ExternalConflictLockedByTabId = map[string]bool{}
	}
	if sess.MergeSessionsByTabId == nil {
		sess.MergeSessionsByTabId = map[string]EditorMergeSession{}
	}
	// ProfileSlug: vazio = usar perfil ativo global no chat; o frontend pode escolher um default.
	return &sess, nil
}

// EditorSaveSession persiste a sessão atual do editor.
func (a *App) EditorSaveSession(sess EditorSession) error {
	if err := a.ensureEditorDirs(); err != nil {
		return err
	}

	base, _ := a.editorBaseDir()
	p := editorSessionPath(base)

	if sess.Version == 0 {
		sess.Version = 2
	}
	if sess.Tabs == nil {
		sess.Tabs = []EditorSessionTab{}
	}
	if sess.FileModeByPath == nil {
		sess.FileModeByPath = map[string]string{}
	}
	if sess.ExternalConflictLockedByTabId == nil {
		sess.ExternalConflictLockedByTabId = map[string]bool{}
	}
	if sess.MergeSessionsByTabId == nil {
		sess.MergeSessionsByTabId = map[string]EditorMergeSession{}
	}

	b, err := json.MarshalIndent(&sess, "", "  ")
	if err != nil {
		return fmt.Errorf("falha ao serializar sessão do editor: %w", err)
	}
	if err := filesystem.WriteFileBytes(p, b, 0644); err != nil {
		return fmt.Errorf("falha ao salvar sessão do editor: %w", err)
	}
	return nil
}

// EditorOpenFile abre o diálogo nativo e retorna conteúdo + path.
func (a *App) EditorOpenFile() (*EditorOpenResult, error) {
	if a.ctx == nil {
		return nil, fmt.Errorf("app não inicializado")
	}

	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Abrir arquivo",
		Filters: []runtime.FileFilter{
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
// Não falha com erro quando o arquivo não existe.
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
// newBaseName deve ser apenas o nome do arquivo (sem diretórios).
// Retorna o novo path completo.
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
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Salvar arquivo",
		DefaultFilename: def,
		Filters: []runtime.FileFilter{
			{DisplayName: "Markdown", Pattern: "*.md;*.markdown;*.txt"},
			{DisplayName: "Todos os arquivos", Pattern: "*.*"},
		},
	})
	if err != nil {
		return "", err
	}
	return path, nil
}

// EditorWriteDraft persiste um arquivo temporário em ~/.assistente/editor/drafts/.
func (a *App) EditorWriteDraft(draftId string, content string) error {
	if err := a.ensureEditorDirs(); err != nil {
		return err
	}
	drafts, _ := a.editorDraftsDir()
	fn, err := draftFilename(draftId)
	if err != nil {
		return err
	}
	p := filepath.Join(drafts, fn)
	if err := filesystem.WriteFileBytes(p, []byte(content), 0644); err != nil {
		return fmt.Errorf("falha ao salvar draft: %w", err)
	}
	return nil
}

func (a *App) EditorReadDraft(draftId string) (string, error) {
	if err := a.ensureEditorDirs(); err != nil {
		return "", err
	}
	drafts, _ := a.editorDraftsDir()
	fn, err := draftFilename(draftId)
	if err != nil {
		return "", err
	}
	p := filepath.Join(drafts, fn)
	data, err := filesystem.ReadFileBytes(p)
	if err != nil {
		return "", fmt.Errorf("falha ao ler draft: %w", err)
	}
	return string(data), nil
}

func (a *App) EditorDeleteDraft(draftId string) error {
	if err := a.ensureEditorDirs(); err != nil {
		return err
	}
	drafts, _ := a.editorDraftsDir()
	fn, err := draftFilename(draftId)
	if err != nil {
		return err
	}
	p := filepath.Join(drafts, fn)
	if err := filesystem.RemoveFileWithPolicy(p, filesystem.EditorPolicy()); err != nil {
		return fmt.Errorf("falha ao remover draft: %w", err)
	}
	return nil
}
