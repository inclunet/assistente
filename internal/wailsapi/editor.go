package wailsapi

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"assistente/internal/apidto"
	"assistente/internal/configdir"
	"assistente/internal/core/ports"
	"assistente/internal/tools/filesystem"
)

// EditorHooks agrupa side effects do App que o bind não deve conhecer
// diretamente (AEP-0088): contexto, diálogos nativos, self-write do watcher
// e watch/unwatch. Watcher, eventos editor:fileChanged e assisted writes
// permanecem no *App.
type EditorHooks struct {
	AppContext    func() context.Context
	Dialog        func() ports.SystemDialogPort
	MarkSelfWrite func(path string) func(bool)
	WatchFile     func(path string) error
	UnwatchFile   func(path string) error
}

// Editor é o bind Wails do domínio editor (AEP-0088).
// Auth só via WithUser — sem chamar o helper de auth do App no call site.
// Editor é sensível: nenhum método aqui roda sem sessão autenticada (fail-closed).
type Editor struct {
	mu      sync.RWMutex
	session Session
	hooks   EditorHooks
}

// NewEditor cria o bind vazio; AttachEditor preenche deps no startup.
func NewEditor() *Editor {
	return &Editor{}
}

// AttachEditor associa Session e hooks após o startup.
// Função de pacote (não método) para não entrar no Bind do Wails.
func AttachEditor(api *Editor, session Session, hooks EditorHooks) {
	if api == nil {
		return
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	api.session = session
	api.hooks = hooks
}

func (api *Editor) deps() (Session, EditorHooks, error) {
	api.mu.RLock()
	defer api.mu.RUnlock()
	if api.session == nil ||
		api.hooks.AppContext == nil ||
		api.hooks.Dialog == nil ||
		api.hooks.MarkSelfWrite == nil ||
		api.hooks.WatchFile == nil ||
		api.hooks.UnwatchFile == nil {
		return nil, EditorHooks{}, ErrEditorNotWired
	}
	return api.session, api.hooks, nil
}

func editorDraftDir() string {
	return filepath.Join(configdir.GetHomeDir(), "editor", "drafts")
}

func editorStatePath() string {
	return filepath.Join(configdir.GetHomeDir(), "editor", "state.json")
}

func editorDraftPath(draftId string) (string, error) {
	id := strings.TrimSpace(draftId)
	if err := configdir.ValidateFilename(id); err != nil {
		return "", fmt.Errorf("draftId inválido: %w", err)
	}
	return filepath.Join(editorDraftDir(), id+".md"), nil
}

// ensurePrivatePath reforça 0700 no diretório e 0600 no arquivo.
// Necessário porque os.WriteFile/MkdirAll não corrigem modo de paths já
// existentes criados com permissões mais abertas em versões anteriores.
func ensurePrivatePath(filePath string) error {
	if err := os.Chmod(filepath.Dir(filePath), 0700); err != nil {
		return fmt.Errorf("falha ao restringir diretório privado: %w", err)
	}
	if err := os.Chmod(filePath, 0600); err != nil {
		return fmt.Errorf("falha ao restringir arquivo privado: %w", err)
	}
	return nil
}

func emptyEditorState() *apidto.EditorState {
	return &apidto.EditorState{
		FileModeByPath:       map[string]string{},
		MergeSessionsByTabId: map[string]apidto.EditorMergeSession{},
	}
}

// EditorGetDraftPath retorna o caminho em disco de um draft (sem criá-lo).
func (api *Editor) EditorGetDraftPath(draftId string) (string, error) {
	session, _, err := api.deps()
	if err != nil {
		return "", err
	}
	return WithUser(session, func(ctx context.Context) (string, error) {
		return editorDraftPath(draftId)
	})
}

// EditorWriteDraft persiste o conteúdo de um draft em disco.
func (api *Editor) EditorWriteDraft(draftId string, content string) error {
	session, hooks, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		p, err := editorDraftPath(draftId)
		if err != nil {
			return struct{}{}, err
		}
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			return struct{}{}, fmt.Errorf("falha ao criar diretório de drafts: %w", err)
		}
		commit := hooks.MarkSelfWrite(p)
		if err := filesystem.WriteFileBytes(p, []byte(content), 0600); err != nil {
			if commit != nil {
				commit(false)
			}
			return struct{}{}, fmt.Errorf("falha ao salvar draft: %w", err)
		}
		// Self-write commit antes do Chmod: o conteúdo já está no disco.
		if commit != nil {
			commit(true)
		}
		if err := ensurePrivatePath(p); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, nil
	})
	return err
}

// EditorReadDraft lê o conteúdo de um draft do disco.
func (api *Editor) EditorReadDraft(draftId string) (string, error) {
	session, _, err := api.deps()
	if err != nil {
		return "", err
	}
	return WithUser(session, func(ctx context.Context) (string, error) {
		p, err := editorDraftPath(draftId)
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
	})
}

// EditorDeleteDraft remove o arquivo de draft do disco.
func (api *Editor) EditorDeleteDraft(draftId string) error {
	session, _, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		p, err := editorDraftPath(draftId)
		if err != nil {
			return struct{}{}, err
		}
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return struct{}{}, fmt.Errorf("falha ao remover draft: %w", err)
		}
		return struct{}{}, nil
	})
	return err
}

// EditorLoadState carrega o estado global do editor do arquivo state.json.
func (api *Editor) EditorLoadState() (*apidto.EditorState, error) {
	session, _, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*apidto.EditorState, error) {
		p := editorStatePath()
		data, err := os.ReadFile(p)
		if err != nil {
			if os.IsNotExist(err) {
				return emptyEditorState(), nil
			}
			return nil, fmt.Errorf("falha ao ler editor/state.json: %w", err)
		}

		var state apidto.EditorState
		if err := json.Unmarshal(data, &state); err != nil {
			return emptyEditorState(), nil
		}
		if state.FileModeByPath == nil {
			state.FileModeByPath = map[string]string{}
		}
		if state.MergeSessionsByTabId == nil {
			state.MergeSessionsByTabId = map[string]apidto.EditorMergeSession{}
		}
		return &state, nil
	})
}

// EditorSaveState persiste o estado global do editor no arquivo state.json.
func (api *Editor) EditorSaveState(state apidto.EditorState) error {
	session, _, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		p := editorStatePath()
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			return struct{}{}, fmt.Errorf("falha ao criar diretório do editor: %w", err)
		}
		b, err := json.MarshalIndent(&state, "", "  ")
		if err != nil {
			return struct{}{}, fmt.Errorf("falha ao serializar editor state: %w", err)
		}
		if err := os.WriteFile(p, b, 0600); err != nil {
			return struct{}{}, fmt.Errorf("falha ao salvar editor/state.json: %w", err)
		}
		if err := ensurePrivatePath(p); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, nil
	})
	return err
}

// orDefault devolve v se não for vazio; senão fallback.
// Não é i18n: o SO renderiza a string crua. O frontend envia rótulos já
// traduzidos; o fallback pt-BR evita diálogo com título/filtro em branco
// quando CLI, testes ou chamadas antigas passam FileDialogLabels{} zerado
// (degradação segura, não tradução).
func orDefault(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return strings.TrimSpace(v)
}

func dialogFilters(labels apidto.FileDialogLabels) []ports.FileFilter {
	return []ports.FileFilter{
		{DisplayName: orDefault(labels.MarkdownFilter, "Markdown"), Pattern: "*.md;*.markdown;*.txt"},
		{DisplayName: orDefault(labels.AllFilesFilter, "Todos os arquivos"), Pattern: "*.*"},
	}
}

// EditorOpenFile abre o diálogo nativo e retorna conteúdo + path.
// labels deve vir já traduzido do frontend (i18n); campos vazios usam fallback pt-BR.
func (api *Editor) EditorOpenFile(labels apidto.FileDialogLabels) (*apidto.EditorOpenResult, error) {
	session, hooks, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*apidto.EditorOpenResult, error) {
		if hooks.AppContext() == nil {
			return nil, fmt.Errorf("app não inicializado")
		}
		dialog := hooks.Dialog()
		if dialog == nil {
			return nil, fmt.Errorf("app não inicializado")
		}

		path, err := dialog.OpenFileDialog(ports.OpenFileOptions{
			Title:   orDefault(labels.Title, "Abrir arquivo"),
			Filters: dialogFilters(labels),
		})
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(path) == "" {
			return &apidto.EditorOpenResult{Path: "", Content: ""}, nil
		}

		data, err := filesystem.ReadFileBytes(path)
		if err != nil {
			return nil, fmt.Errorf("falha ao ler arquivo: %w", err)
		}

		return &apidto.EditorOpenResult{Path: path, Content: string(data)}, nil
	})
}

// EditorReadFile lê um arquivo por path (usado na restauração da sessão).
func (api *Editor) EditorReadFile(path string) (string, error) {
	session, _, err := api.deps()
	if err != nil {
		return "", err
	}
	return WithUser(session, func(ctx context.Context) (string, error) {
		p := strings.TrimSpace(path)
		if p == "" {
			return "", fmt.Errorf("path vazio")
		}
		data, err := filesystem.ReadFileBytes(p)
		if err != nil {
			return "", fmt.Errorf("falha ao ler arquivo: %w", err)
		}
		return string(data), nil
	})
}

// EditorGetFileInfo retorna metadados simples do arquivo para detectar mudanças externas.
func (api *Editor) EditorGetFileInfo(path string) (*apidto.EditorFileInfo, error) {
	session, _, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*apidto.EditorFileInfo, error) {
		p := strings.TrimSpace(path)
		if p == "" {
			return nil, fmt.Errorf("path vazio")
		}

		info, err := os.Stat(p)
		if err != nil {
			if os.IsNotExist(err) {
				return &apidto.EditorFileInfo{Path: p, Exists: false, IsDir: false, Size: 0, ModTimeMs: 0}, nil
			}
			return nil, fmt.Errorf("falha ao stat arquivo: %w", err)
		}

		return &apidto.EditorFileInfo{
			Path:      p,
			Exists:    true,
			IsDir:     info.IsDir(),
			Size:      info.Size(),
			ModTimeMs: info.ModTime().UnixMilli(),
		}, nil
	})
}

// EditorWriteFile escreve conteúdo em um arquivo existente/destino escolhido.
func (api *Editor) EditorWriteFile(path string, content string) error {
	session, hooks, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		p := strings.TrimSpace(path)
		if p == "" {
			return struct{}{}, fmt.Errorf("path vazio")
		}
		perm := os.FileMode(0644)
		if info, statErr := os.Stat(p); statErr == nil {
			perm = info.Mode().Perm()
		}
		commit := hooks.MarkSelfWrite(p)
		if err := filesystem.WriteFileBytes(p, []byte(content), perm); err != nil {
			if commit != nil {
				commit(false)
			}
			return struct{}{}, fmt.Errorf("falha ao salvar arquivo: %w", err)
		}
		if commit != nil {
			commit(true)
		}
		return struct{}{}, nil
	})
	return err
}

// EditorRenameFile renomeia um arquivo existente no disco.
func (api *Editor) EditorRenameFile(oldPath string, newBaseName string) (string, error) {
	session, _, err := api.deps()
	if err != nil {
		return "", err
	}
	return WithUser(session, func(ctx context.Context) (string, error) {
		return filesystem.RenameFileSameDirWithPolicy(oldPath, newBaseName, filesystem.EditorPolicy())
	})
}

// EditorSaveFileDialog abre o diálogo nativo de salvar e retorna o path escolhido.
// labels deve vir já traduzido do frontend (i18n); campos vazios usam fallback pt-BR.
// Precedência do nome sugerido: suggestedFilename > labels.DefaultFilename > "documento.md".
func (api *Editor) EditorSaveFileDialog(suggestedFilename string, labels apidto.FileDialogLabels) (string, error) {
	session, hooks, err := api.deps()
	if err != nil {
		return "", err
	}
	return WithUser(session, func(ctx context.Context) (string, error) {
		if hooks.AppContext() == nil {
			return "", fmt.Errorf("app não inicializado")
		}
		dialog := hooks.Dialog()
		if dialog == nil {
			return "", fmt.Errorf("app não inicializado")
		}
		def := strings.TrimSpace(suggestedFilename)
		if def == "" {
			def = orDefault(labels.DefaultFilename, "documento.md")
		}
		path, err := dialog.SaveFileDialog(ports.SaveFileOptions{
			Title:           orDefault(labels.Title, "Salvar arquivo"),
			DefaultFilename: def,
			Filters:         dialogFilters(labels),
		})
		if err != nil {
			return "", err
		}
		return path, nil
	})
}

// EditorWatchFile observa mudanças externas no arquivo.
func (api *Editor) EditorWatchFile(path string) error {
	session, hooks, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		if hooks.AppContext() == nil {
			return struct{}{}, fmt.Errorf("app não inicializado")
		}
		return struct{}{}, hooks.WatchFile(path)
	})
	return err
}

// EditorUnwatchFile deixa de observar o arquivo.
func (api *Editor) EditorUnwatchFile(path string) error {
	session, hooks, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		if hooks.AppContext() == nil {
			return struct{}{}, fmt.Errorf("app não inicializado")
		}
		return struct{}{}, hooks.UnwatchFile(path)
	})
	return err
}
