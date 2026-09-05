package wailsapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"assistente/internal/apidto"
	"assistente/internal/configdir"
	"assistente/internal/core/ports"
	"assistente/internal/database"
	"assistente/internal/docextract"
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
	mu          sync.RWMutex
	migrationMu sync.Mutex
	session     Session
	hooks       EditorHooks
	cache       *docextract.ProjectionCache
}

var editorLegacyMigrationMu sync.Mutex

// NewEditor cria o bind vazio; AttachEditor preenche deps no startup.
func NewEditor() *Editor {
	return &Editor{cache: docextract.NewProjectionCache(docextract.DefaultCacheConfig())}
}

func (api *Editor) projectionCache() *docextract.ProjectionCache {
	api.mu.Lock()
	defer api.mu.Unlock()
	if api.cache == nil {
		api.cache = docextract.NewProjectionCache(docextract.DefaultCacheConfig())
	}
	return api.cache
}

func readEditorFilePrefix(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	buf := make([]byte, docextract.DetectPrefixBytes)
	n, err := io.ReadFull(f, buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return buf[:n], nil
}

func editorWarningCode(result *docextract.Result) string {
	if result == nil || len(result.Warnings) == 0 {
		return ""
	}
	if result.Kind == docextract.KindPDF && strings.TrimSpace(result.Markdown) == "" {
		return "no_extractable_text"
	}
	return "partial_extraction"
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

const editorMigrationVersion = 1

type editorMigrationClaim struct {
	Version   int    `json:"version"`
	UserID    string `json:"userId"`
	ClaimedAt int64  `json:"claimedAt"`
}

type editorUserPaths struct {
	root     string
	draftDir string
	state    string
}

func legacyEditorDir() string {
	return filepath.Join(configdir.GetHomeDir(), "editor")
}

func editorMigrationClaimPath() string {
	return filepath.Join(legacyEditorDir(), "user-scope-migration.json")
}

func editorPathsForUser(userID string) (editorUserPaths, error) {
	userID = strings.TrimSpace(userID)
	if err := configdir.ValidateFilename(userID); err != nil {
		return editorUserPaths{}, fmt.Errorf("userID inválido: %w", err)
	}
	root := filepath.Join(configdir.GetHomeDir(), "users", userID, "editor")
	return editorUserPaths{
		root:     root,
		draftDir: filepath.Join(root, "drafts"),
		state:    filepath.Join(root, "state.json"),
	}, nil
}

func editorDraftPath(paths editorUserPaths, draftId string) (string, error) {
	id := strings.TrimSpace(draftId)
	if err := configdir.ValidateFilename(id); err != nil {
		return "", fmt.Errorf("draftId inválido: %w", err)
	}
	return filepath.Join(paths.draftDir, id+".md"), nil
}

func writeJSONPrivate(path string, value any, exclusive bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if exclusive {
		flags = os.O_WRONLY | os.O_CREATE | os.O_EXCL
	}
	f, err := os.OpenFile(path, flags, 0600)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func readEditorMigrationClaim(path string) (editorMigrationClaim, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return editorMigrationClaim{}, err
	}
	var claim editorMigrationClaim
	if err := json.Unmarshal(data, &claim); err != nil {
		return editorMigrationClaim{}, fmt.Errorf("marcador de migração do editor inválido: %w", err)
	}
	if claim.Version != editorMigrationVersion || strings.TrimSpace(claim.UserID) == "" {
		return editorMigrationClaim{}, fmt.Errorf("marcador de migração do editor incompatível")
	}
	return claim, nil
}

func copyLegacyEditorFile(source, destination string, overwriteIncomplete bool) error {
	src, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		return err
	}
	flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	if overwriteIncomplete {
		flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}
	dst, err := os.OpenFile(destination, flags, 0600)
	if err != nil {
		if os.IsExist(err) && !overwriteIncomplete {
			return nil
		}
		return err
	}
	ok := false
	defer func() {
		_ = dst.Close()
		if !ok {
			_ = os.Remove(destination)
		}
	}()
	if _, err := io.Copy(dst, src); err != nil {
		return err
	}
	if err := dst.Sync(); err != nil {
		return err
	}
	if err := dst.Close(); err != nil {
		return err
	}
	ok = true
	return nil
}

func migrateLegacyEditorData(userID string, paths editorUserPaths) error {
	legacyDir := legacyEditorDir()
	claimPath := editorMigrationClaimPath()
	claim, err := readEditorMigrationClaim(claimPath)
	newClaim := false
	if os.IsNotExist(err) {
		claim = editorMigrationClaim{
			Version:   editorMigrationVersion,
			UserID:    userID,
			ClaimedAt: time.Now().UnixMilli(),
		}
		if err := writeJSONPrivate(claimPath, claim, true); err != nil {
			if !os.IsExist(err) {
				return fmt.Errorf("falha ao reservar migração legada do editor: %w", err)
			}
			claim, err = readEditorMigrationClaim(claimPath)
			if err != nil {
				return err
			}
		} else {
			newClaim = true
		}
	} else if err != nil {
		return err
	}

	// A reserva é instance-wide e imutável: somente o primeiro usuário que a
	// adquiriu pode adotar os dados legados. Outros usuários começam vazios.
	if claim.UserID != userID {
		return nil
	}
	completionPath := filepath.Join(paths.root, ".legacy-migration-v1-complete")
	if _, err := os.Stat(completionPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}

	overwriteIncomplete := !newClaim
	legacyState := filepath.Join(legacyDir, "state.json")
	if _, err := os.Stat(legacyState); err == nil {
		if err := copyLegacyEditorFile(legacyState, paths.state, overwriteIncomplete); err != nil {
			return fmt.Errorf("falha ao migrar estado legado do editor: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	legacyDraftDir := filepath.Join(legacyDir, "drafts")
	entries, err := os.ReadDir(legacyDraftDir)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("falha ao listar drafts legados: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if err := configdir.ValidateFilename(entry.Name()); err != nil {
			continue
		}
		source := filepath.Join(legacyDraftDir, entry.Name())
		destination := filepath.Join(paths.draftDir, entry.Name())
		if err := copyLegacyEditorFile(source, destination, overwriteIncomplete); err != nil {
			return fmt.Errorf("falha ao migrar draft legado %q: %w", entry.Name(), err)
		}
	}

	if err := os.MkdirAll(paths.root, 0700); err != nil {
		return err
	}
	completion, err := os.OpenFile(completionPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil && !os.IsExist(err) {
		return fmt.Errorf("falha ao concluir migração legada do editor: %w", err)
	}
	if err == nil {
		if closeErr := completion.Close(); closeErr != nil {
			return fmt.Errorf("falha ao concluir migração legada do editor: %w", closeErr)
		}
	}
	return nil
}

func (api *Editor) userPaths(ctx context.Context) (editorUserPaths, error) {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return editorUserPaths{}, err
	}
	paths, err := editorPathsForUser(userID)
	if err != nil {
		return editorUserPaths{}, err
	}
	api.migrationMu.Lock()
	defer api.migrationMu.Unlock()
	editorLegacyMigrationMu.Lock()
	defer editorLegacyMigrationMu.Unlock()
	if err := migrateLegacyEditorData(userID, paths); err != nil {
		return editorUserPaths{}, err
	}
	return paths, nil
}

// PrepareEditorUserStorage reserva/adota o storage legado durante o login.
// É função de pacote (não método) para não ampliar a superfície Wails.
func PrepareEditorUserStorage(api *Editor, ctx context.Context) error {
	if api == nil {
		return nil
	}
	_, err := api.userPaths(ctx)
	return err
}

func requireEditorUser(ctx context.Context) error {
	_, err := database.RequireUserID(ctx)
	return err
}

func pathInside(base, candidate string) bool {
	rel, err := filepath.Rel(filepath.Clean(base), filepath.Clean(candidate))
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// validateUserFilePath mantém a liberdade do editor para abrir arquivos
// escolhidos pelo usuário fora do storage interno, mas impede que um path
// preservado no workspace seja reinterpretado como arquivo comum e atravesse
// a fronteira de drafts/estado de outra conta.
func (api *Editor) validateUserFilePath(ctx context.Context, path string) error {
	paths, err := api.userPaths(ctx)
	if err != nil {
		return err
	}
	absolute, err := filepath.Abs(filepath.Clean(strings.TrimSpace(path)))
	if err != nil {
		return err
	}

	usersRoot := filepath.Join(configdir.GetHomeDir(), "users")
	if pathInside(usersRoot, absolute) && !pathInside(paths.root, absolute) {
		return database.ErrUserScopeRequired
	}
	if pathInside(legacyEditorDir(), absolute) {
		claim, err := readEditorMigrationClaim(editorMigrationClaimPath())
		if err != nil {
			return database.ErrUserScopeRequired
		}
		userID, _ := database.UserIDFromContext(ctx)
		if claim.UserID != userID {
			return database.ErrUserScopeRequired
		}
	}
	return nil
}

// ensurePrivatePath reforça 0700 no diretório e 0600 no arquivo.
// Necessário porque os.WriteFile/MkdirAll não corrigem modo de paths já
// existentes criados com permissões mais abertas em versões anteriores.
// Para drafts, também restringe o diretório editor do usuário.
func ensurePrivatePath(filePath string) error {
	dir := filepath.Dir(filePath)
	if err := os.Chmod(dir, 0700); err != nil {
		return fmt.Errorf("falha ao restringir diretório privado: %w", err)
	}
	if filepath.Base(dir) == "drafts" {
		parent := filepath.Dir(dir)
		if filepath.Base(parent) == "editor" {
			if err := os.Chmod(parent, 0700); err != nil {
				return fmt.Errorf("falha ao restringir diretório editor: %w", err)
			}
		}
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
		paths, err := api.userPaths(ctx)
		if err != nil {
			return "", err
		}
		return editorDraftPath(paths, draftId)
	})
}

// EditorWriteDraft persiste o conteúdo de um draft em disco.
func (api *Editor) EditorWriteDraft(draftId string, content string) error {
	session, hooks, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		paths, err := api.userPaths(ctx)
		if err != nil {
			return struct{}{}, err
		}
		p, err := editorDraftPath(paths, draftId)
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
		paths, err := api.userPaths(ctx)
		if err != nil {
			return "", err
		}
		p, err := editorDraftPath(paths, draftId)
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
		paths, err := api.userPaths(ctx)
		if err != nil {
			return struct{}{}, err
		}
		p, err := editorDraftPath(paths, draftId)
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
		paths, err := api.userPaths(ctx)
		if err != nil {
			return nil, err
		}
		p := paths.state
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
		paths, err := api.userPaths(ctx)
		if err != nil {
			return struct{}{}, err
		}
		p := paths.state
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

func dialogFilters(labels apidto.FileDialogLabels, includeDocuments bool) []ports.FileFilter {
	if includeDocuments {
		return []ports.FileFilter{
			{DisplayName: orDefault(labels.MarkdownFilter, "Documentos e texto"), Pattern: "*.md;*.markdown;*.txt;*.pdf;*.docx;*.xlsx;*.pptx;*.odt;*.ods;*.odp;*.epub"},
			{DisplayName: orDefault(labels.AllFilesFilter, "Todos os arquivos"), Pattern: "*.*"},
		}
	}
	return []ports.FileFilter{
		{DisplayName: orDefault(labels.MarkdownFilter, "Markdown"), Pattern: "*.md;*.markdown;*.txt"},
		{DisplayName: orDefault(labels.AllFilesFilter, "Todos os arquivos"), Pattern: "*.*"},
	}
}

func (api *Editor) readDocument(ctx context.Context, path string) (*apidto.EditorOpenResult, error) {
	p := strings.TrimSpace(path)
	if p == "" {
		return nil, fmt.Errorf("path vazio")
	}
	if err := api.validateUserFilePath(ctx, p); err != nil {
		return nil, err
	}
	info, err := os.Stat(p)
	if err != nil {
		return nil, fmt.Errorf("falha ao acessar arquivo: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("o path aponta para um diretório")
	}
	// O prefixo basta para classificar. Ler o arquivo inteiro aqui desperdiçaria
	// I/O justamente no caso que o cache existe para evitar: reabrir um documento
	// grande já projetado.
	prefix, err := readEditorFilePrefix(p)
	if err != nil {
		return nil, fmt.Errorf("falha ao ler arquivo: %w", err)
	}
	kind := docextract.Detect(prefix, p)
	if !docextract.IsOpaqueDocument(kind) {
		if !docextract.IsWritableText(kind) {
			return nil, docextract.ErrUnsupportedBinary()
		}
		data, err := filesystem.ReadFileBytes(p)
		if err != nil {
			return nil, fmt.Errorf("falha ao ler arquivo: %w", err)
		}
		// A confirmação é sobre o conteúdo todo: um NUL no meio do arquivo
		// desmente o prefixo textual.
		if !docextract.IsLikelyText(data) {
			return nil, docextract.ErrUnsupportedBinary()
		}
		return &apidto.EditorOpenResult{Path: p, Content: string(data)}, nil
	}

	identity := docextract.FileIdentityFromStat(info.Size(), info.ModTime().UnixNano())
	result, _, err := api.projectionCache().GetOrLoad(ctx, p+"\x00editor-view", identity, func(loadCtx context.Context) (*docextract.Result, error) {
		data, err := filesystem.ReadFileBytes(p)
		if err != nil {
			return nil, fmt.Errorf("falha ao ler arquivo: %w", err)
		}
		return docextract.ExtractModeContext(loadCtx, data, p, docextract.ModeAuto)
	})
	if err != nil {
		return nil, err
	}
	return &apidto.EditorOpenResult{
		Path:        p,
		Content:     result.Markdown,
		Projected:   true,
		Format:      string(result.Kind),
		ReadOnly:    true,
		Pages:       result.Pages,
		Warnings:    append([]string(nil), result.Warnings...),
		WarningCode: editorWarningCode(result),
	}, nil
}

// EditorOpenFile abre o diálogo nativo e retorna conteúdo + path.
// labels deve vir já traduzido do frontend (i18n); campos vazios usam fallback pt-BR.
func (api *Editor) EditorOpenFile(labels apidto.FileDialogLabels) (*apidto.EditorOpenResult, error) {
	session, hooks, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*apidto.EditorOpenResult, error) {
		if err := requireEditorUser(ctx); err != nil {
			return nil, err
		}
		if hooks.AppContext() == nil {
			return nil, fmt.Errorf("app não inicializado")
		}
		dialog := hooks.Dialog()
		if dialog == nil {
			return nil, fmt.Errorf("app não inicializado")
		}

		path, err := dialog.OpenFileDialog(ports.OpenFileOptions{
			Title:   orDefault(labels.Title, "Abrir arquivo"),
			Filters: dialogFilters(labels, true),
		})
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(path) == "" {
			return &apidto.EditorOpenResult{Path: "", Content: ""}, nil
		}
		return api.readDocument(ctx, path)
	})
}

// EditorReadFile lê um arquivo por path (usado na restauração da sessão).
func (api *Editor) EditorReadFile(path string) (*apidto.EditorOpenResult, error) {
	session, _, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*apidto.EditorOpenResult, error) {
		return api.readDocument(ctx, path)
	})
}

// EditorGetFileInfo retorna metadados simples do arquivo para detectar mudanças externas.
func (api *Editor) EditorGetFileInfo(path string) (*apidto.EditorFileInfo, error) {
	session, _, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*apidto.EditorFileInfo, error) {
		if err := requireEditorUser(ctx); err != nil {
			return nil, err
		}
		p := strings.TrimSpace(path)
		if p == "" {
			return nil, fmt.Errorf("path vazio")
		}
		if err := api.validateUserFilePath(ctx, p); err != nil {
			return nil, err
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
		if err := requireEditorUser(ctx); err != nil {
			return struct{}{}, err
		}
		p := strings.TrimSpace(path)
		if p == "" {
			return struct{}{}, fmt.Errorf("path vazio")
		}
		if err := api.validateUserFilePath(ctx, p); err != nil {
			return struct{}{}, err
		}
		if existing, readErr := readEditorFilePrefix(p); readErr == nil {
			if err := docextract.CheckWritable(existing, p); err != nil {
				return struct{}{}, err
			}
		} else if !os.IsNotExist(readErr) {
			return struct{}{}, fmt.Errorf("não foi possível classificar o arquivo antes de salvar: %w", readErr)
		}
		if err := docextract.CheckWritableString(content, p); err != nil {
			return struct{}{}, err
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
		if err := api.validateUserFilePath(ctx, oldPath); err != nil {
			return "", err
		}
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
		if err := requireEditorUser(ctx); err != nil {
			return "", err
		}
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
			Filters:         dialogFilters(labels, false),
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
		if err := requireEditorUser(ctx); err != nil {
			return struct{}{}, err
		}
		if hooks.AppContext() == nil {
			return struct{}{}, fmt.Errorf("app não inicializado")
		}
		p := strings.TrimSpace(path)
		if p == "" {
			return struct{}{}, fmt.Errorf("path vazio")
		}
		if err := api.validateUserFilePath(ctx, p); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, hooks.WatchFile(p)
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
		if err := requireEditorUser(ctx); err != nil {
			return struct{}{}, err
		}
		if hooks.AppContext() == nil {
			return struct{}{}, fmt.Errorf("app não inicializado")
		}
		p := strings.TrimSpace(path)
		if p == "" {
			return struct{}{}, fmt.Errorf("path vazio")
		}
		if err := api.validateUserFilePath(ctx, p); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, hooks.UnwatchFile(p)
	})
	return err
}
