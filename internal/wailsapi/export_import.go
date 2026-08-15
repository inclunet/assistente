package wailsapi

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"assistente/internal/core/ports"
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/portability"
)

// ExportImport é o bind Wails do domínio export_import (AEP-0088).
// Auth só via WithUser — sem chamar o helper de auth do App no call site.
// Tipos de request/response vêm de portability (sem apidto novo).
type ExportImport struct {
	mu         sync.RWMutex
	session    Session
	credMgr    *credentials.Manager
	dialog     func() ports.SystemDialogPort
	appVersion string
}

// NewExportImport cria o bind vazio; AttachExportImport preenche deps no startup.
func NewExportImport() *ExportImport {
	return &ExportImport{}
}

// AttachExportImport associa Session, credenciais, diálogo e versão após o startup.
// Função de pacote (não método) para não entrar no Bind do Wails.
func AttachExportImport(
	api *ExportImport,
	session Session,
	credMgr *credentials.Manager,
	dialog func() ports.SystemDialogPort,
	appVersion string,
) {
	if api == nil {
		return
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	api.session = session
	api.credMgr = credMgr
	api.dialog = dialog
	api.appVersion = appVersion
}

func (api *ExportImport) deps() (Session, *credentials.Manager, func() ports.SystemDialogPort, string, error) {
	api.mu.RLock()
	defer api.mu.RUnlock()
	if api.session == nil || api.dialog == nil {
		return nil, nil, nil, "", ErrExportImportNotWired
	}
	return api.session, api.credMgr, api.dialog, api.appVersion, nil
}

// ExportConversations exporta conversas selecionadas em JSON portátil.
func (api *ExportImport) ExportConversations(ids []string) (string, error) {
	session, credMgr, _, appVersion, err := api.deps()
	if err != nil {
		return "", err
	}
	return WithUser(session, func(ctx context.Context) (string, error) {
		return portability.ExportConversationsWithContext(ctx, ids, credMgr, portability.ExportRequest{
			OutputFormat: portability.FormatJSON,
		}, appVersion)
	})
}

// ExportData exporta dados portáteis conforme o request (JSON/HTML/MCP-JSON).
func (api *ExportImport) ExportData(req portability.ExportRequest) (string, error) {
	session, credMgr, _, appVersion, err := api.deps()
	if err != nil {
		return "", err
	}
	return WithUser(session, func(ctx context.Context) (string, error) {
		return exportData(ctx, credMgr, appVersion, req)
	})
}

// ExportConversationsToFile renderiza conversas e salva via diálogo nativo.
func (api *ExportImport) ExportConversationsToFile(ids []string, format string, options portability.ContentExportOptions) (string, error) {
	session, credMgr, dialogFn, appVersion, err := api.deps()
	if err != nil {
		return "", err
	}
	return WithUser(session, func(ctx context.Context) (string, error) {
		dialog := dialogFn()
		if dialog == nil {
			return "", fmt.Errorf("diálogo de sistema não inicializado")
		}

		switch format {
		case portability.FormatHTML, portability.FormatPDF, portability.FormatMarkdown:
		default:
			return "", fmt.Errorf("formato de exportação não suportado: %s", format)
		}

		includeTimestamps := options.IncludeTimestamps
		includeReasoning := options.IncludeReasoning
		includeMetadata := options.IncludeMetadata
		req := portability.ExportRequest{
			OutputFormat:      format,
			IncludeTimestamps: &includeTimestamps,
			IncludeReasoning:  &includeReasoning,
			IncludeMetadata:   &includeMetadata,
		}

		file, err := portability.BuildConversationExportFileWithContext(ctx, ids, credMgr, req, appVersion)
		if err != nil {
			return "", err
		}

		rendered, err := portability.RenderConversationExport(file, format)
		if err != nil {
			return "", err
		}

		path, err := dialog.SaveFileDialog(ports.SaveFileOptions{
			Title:           "Exportar conversas",
			DefaultFilename: defaultConversationExportFilename(format),
			Filters: []ports.FileFilter{
				{DisplayName: strings.ToUpper(format), Pattern: "*." + format},
				{DisplayName: "Todos os arquivos", Pattern: "*.*"},
			},
		})
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(path) == "" {
			return "", nil
		}
		if err := os.WriteFile(path, rendered, 0600); err != nil {
			return "", err
		}
		return path, nil
	})
}

// ExportDataToFile exporta dados portáteis para um caminho informado.
func (api *ExportImport) ExportDataToFile(req portability.ExportRequest, path string) (string, error) {
	session, credMgr, _, appVersion, err := api.deps()
	if err != nil {
		return "", err
	}
	return WithUser(session, func(ctx context.Context) (string, error) {
		if strings.TrimSpace(path) == "" {
			return "", fmt.Errorf("caminho de saída é obrigatório")
		}
		if req.OutputFormat == "" {
			req.OutputFormat = portability.FormatJSON
		}
		originalReq := req
		if err := validateDBOnlyExportRequest(req); err != nil {
			return "", err
		}

		switch req.OutputFormat {
		case portability.FormatJSON, portability.FormatMCPJSON:
			rendered, err := exportData(ctx, credMgr, appVersion, req)
			if err != nil {
				return "", err
			}
			if err := os.WriteFile(path, []byte(rendered), 0600); err != nil {
				return "", err
			}
			return path, nil
		case portability.FormatHTML, portability.FormatPDF, portability.FormatMarkdown:
			conversationIDs, err := resolveConversationIDs(ctx, req)
			if err != nil {
				return "", err
			}
			if hasUnsupportedRichConversationSelections(originalReq) {
				return "", fmt.Errorf("exportação HTML/PDF/Markdown atualmente suporta apenas conversas")
			}
			req, err = normalizeRichConversationExportRequest(req)
			if err != nil {
				return "", err
			}

			// Corrige bug: BuildConversationExportFile sem ctx perdia escopo de usuário.
			file, err := portability.BuildConversationExportFileWithContext(ctx, conversationIDs, credMgr, req, appVersion)
			if err != nil {
				return "", err
			}
			rendered, err := portability.RenderConversationExport(file, req.OutputFormat)
			if err != nil {
				return "", err
			}
			if err := os.WriteFile(path, rendered, 0600); err != nil {
				return "", err
			}
			return path, nil
		default:
			return "", fmt.Errorf("formato de exportação ainda não suportado: %s", req.OutputFormat)
		}
	})
}

// ImportConversations importa conversas a partir de JSON portátil.
func (api *ExportImport) ImportConversations(jsonData string) (*portability.ImportResult, error) {
	session, credMgr, _, _, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*portability.ImportResult, error) {
		return portability.ImportConversationsWithContext(ctx, jsonData, credMgr, "")
	})
}

// ImportData importa dados portáteis (com senha opcional de credenciais).
func (api *ExportImport) ImportData(jsonData string, credentialExportPassword string) (*portability.ImportResult, error) {
	session, credMgr, _, _, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*portability.ImportResult, error) {
		return portability.ImportConversationsWithContext(ctx, jsonData, credMgr, credentialExportPassword)
	})
}

// ImportDataWithResolutions importa dados aplicando resoluções de conflito.
func (api *ExportImport) ImportDataWithResolutions(req portability.ImportRequest) (*portability.ImportResult, error) {
	session, credMgr, _, _, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*portability.ImportResult, error) {
		return portability.ImportConversationsWithResolutions(
			ctx,
			req.JSONData,
			credMgr,
			req.CredentialExportPassword,
			req.Resolutions,
		)
	})
}

// AnalyzeImportData analisa um payload de importação sem aplicar mudanças.
func (api *ExportImport) AnalyzeImportData(jsonData string, credentialExportPassword string) (*portability.ImportAnalysis, error) {
	session, credMgr, _, _, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*portability.ImportAnalysis, error) {
		return portability.AnalyzeImportDataWithContext(ctx, jsonData, credMgr, credentialExportPassword)
	})
}

func exportData(ctx context.Context, credMgr *credentials.Manager, appVersion string, req portability.ExportRequest) (string, error) {
	if req.OutputFormat == "" {
		req.OutputFormat = portability.FormatJSON
	}
	originalReq := req
	if err := validateDBOnlyExportRequest(req); err != nil {
		return "", err
	}

	if req.OutputFormat == portability.FormatMCPJSON {
		if err := validateMCPJSONExportRequest(req); err != nil {
			return "", err
		}
		mcpServerSlugs, err := resolveMCPServerSlugs(ctx, req)
		if err != nil {
			return "", err
		}
		return portability.ExportMCPServersExternalJSONWithContext(ctx, mcpServerSlugs)
	}
	conversationIDs, err := resolveConversationIDs(ctx, req)
	if err != nil {
		return "", err
	}
	providerIDs, err := resolveProviderIDs(ctx, req)
	if err != nil {
		return "", err
	}
	mcpServerSlugs, err := resolveMCPServerSlugs(ctx, req)
	if err != nil {
		return "", err
	}
	req.MCPServerSlugs = mcpServerSlugs
	taskListIDs, err := resolveTaskListIDs(ctx, req)
	if err != nil {
		return "", err
	}
	memoryRecordIDs, err := resolveMemoryRecordIDs(ctx, req)
	if err != nil {
		return "", err
	}
	req.MemoryRecordIDs = memoryRecordIDs
	switch req.OutputFormat {
	case portability.FormatJSON:
		return portability.ExportPortableDataWithContext(ctx, conversationIDs, providerIDs, taskListIDs, credMgr, req, appVersion)
	case portability.FormatHTML:
		if hasUnsupportedRichConversationSelections(originalReq) {
			return "", fmt.Errorf("exportação HTML/PDF atualmente suporta apenas conversas")
		}
		req, err = normalizeRichConversationExportRequest(req)
		if err != nil {
			return "", err
		}
		file, err := portability.BuildConversationExportFileWithContext(ctx, conversationIDs, credMgr, req, appVersion)
		if err != nil {
			return "", err
		}
		rendered, err := portability.RenderConversationExport(file, req.OutputFormat)
		if err != nil {
			return "", err
		}
		return string(rendered), nil
	case portability.FormatPDF:
		return "", fmt.Errorf("use ExportConversationsToFile para exportação PDF")
	default:
		return "", fmt.Errorf("formato de exportação ainda não suportado: %s", req.OutputFormat)
	}
}

func validateMCPJSONExportRequest(req portability.ExportRequest) error {
	unsupported := make([]string, 0)
	if len(req.ConversationIDs) > 0 {
		unsupported = append(unsupported, "conversations")
	}
	if len(req.ProviderIDs) > 0 {
		unsupported = append(unsupported, "providers")
	}
	if len(req.ProfileSlugs) > 0 {
		unsupported = append(unsupported, "profiles")
	}
	if len(req.SkillSlugs) > 0 {
		unsupported = append(unsupported, "skills")
	}
	if len(req.AllowlistSlugs) > 0 {
		unsupported = append(unsupported, "allowlists")
	}
	if len(req.JobIDs) > 0 {
		unsupported = append(unsupported, "jobs")
	}
	if len(req.TaskListIDs) > 0 {
		unsupported = append(unsupported, "taskLists")
	}
	if len(req.MemoryRecordIDs) > 0 {
		unsupported = append(unsupported, "memoryRecords")
	}
	if len(req.ChannelNames) > 0 {
		unsupported = append(unsupported, "channels")
	}
	if req.IncludeContacts {
		unsupported = append(unsupported, "contacts")
	}
	if req.IncludeWorkspace {
		unsupported = append(unsupported, "workspace")
	}
	if req.IncludeAudio {
		unsupported = append(unsupported, "audio")
	}
	if req.IncludeCredentials || strings.TrimSpace(req.CredentialExportPassword) != "" {
		unsupported = append(unsupported, "credentials")
	}
	if len(unsupported) > 0 {
		return fmt.Errorf("formato mcp-json suporta apenas servidores MCP; remova seleções/opções incompatíveis: %s", strings.Join(unsupported, ", "))
	}
	return nil
}

func normalizeRichConversationExportRequest(req portability.ExportRequest) (portability.ExportRequest, error) {
	if req.IncludeCredentials {
		return req, fmt.Errorf("exportação HTML/PDF/Markdown não suporta credenciais")
	}
	if strings.TrimSpace(req.CredentialExportPassword) != "" {
		return req, fmt.Errorf("exportação HTML/PDF/Markdown não usa senha de credenciais")
	}
	req.IncludeCredentials = false
	req.CredentialExportPassword = ""
	return req, nil
}

func hasUnsupportedRichConversationSelections(req portability.ExportRequest) bool {
	return len(req.TaskListIDs) > 0 ||
		len(req.ProviderIDs) > 0 ||
		len(req.MCPServerSlugs) > 0 ||
		len(req.MemoryRecordIDs) > 0
}

func validateDBOnlyExportRequest(req portability.ExportRequest) error {
	unsupported := make([]string, 0)
	if len(req.ProfileSlugs) > 0 {
		unsupported = append(unsupported, "profiles")
	}
	if len(req.SkillSlugs) > 0 {
		unsupported = append(unsupported, "skills")
	}
	if len(req.AllowlistSlugs) > 0 {
		unsupported = append(unsupported, "allowlists")
	}
	if len(req.JobIDs) > 0 {
		unsupported = append(unsupported, "jobs")
	}
	if len(req.ChannelNames) > 0 {
		unsupported = append(unsupported, "channels")
	}
	if req.IncludeContacts {
		unsupported = append(unsupported, "contacts")
	}
	if req.IncludeWorkspace {
		unsupported = append(unsupported, "workspace")
	}
	if len(unsupported) == 0 {
		return nil
	}
	return fmt.Errorf(
		"recursos fora do escopo DB-only da AEP-0047 ainda não são suportados: %s",
		strings.Join(unsupported, ", "),
	)
}

func resolveConversationIDs(ctx context.Context, req portability.ExportRequest) ([]string, error) {
	if req.All {
		conversations, err := database.GetConversationsWithContext(ctx)
		if err != nil {
			return nil, err
		}
		ids := make([]string, 0, len(conversations))
		for _, conv := range conversations {
			ids = append(ids, conv.ID)
		}
		return ids, nil
	}

	if len(req.ConversationIDs) == 0 {
		if req.ExplicitSelection {
			return nil, nil
		}
		if len(req.ProviderIDs) > 0 || len(req.TaskListIDs) > 0 || len(req.MCPServerSlugs) > 0 || len(req.MemoryRecordIDs) > 0 {
			return nil, nil
		}
		conversations, err := database.GetConversationsWithContext(ctx)
		if err != nil {
			return nil, err
		}
		ids := make([]string, 0, len(conversations))
		for _, conv := range conversations {
			ids = append(ids, conv.ID)
		}
		return ids, nil
	}

	ids := make([]string, 0, len(req.ConversationIDs))
	for _, raw := range req.ConversationIDs {
		id := strings.TrimSpace(raw)
		if id == "" {
			return nil, fmt.Errorf("conversationId inválido: %q", raw)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func resolveProviderIDs(ctx context.Context, req portability.ExportRequest) ([]string, error) {
	if req.All {
		providers, err := database.GetLLMProvidersWithContext(ctx)
		if err != nil {
			return nil, err
		}
		ids := make([]string, 0, len(providers))
		for _, provider := range providers {
			ids = append(ids, provider.ID)
		}
		return ids, nil
	}

	if len(req.ProviderIDs) == 0 {
		return nil, nil
	}

	ids := make([]string, 0, len(req.ProviderIDs))
	for _, raw := range req.ProviderIDs {
		id := strings.TrimSpace(raw)
		if id == "" {
			return nil, fmt.Errorf("providerId inválido: %q", raw)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func resolveMCPServerSlugs(ctx context.Context, req portability.ExportRequest) ([]string, error) {
	if req.All {
		var rows []database.MCPServer
		if err := database.ScopeByUser(ctx, database.DB().WithContext(ctx), "user_id").Order("slug ASC").Find(&rows).Error; err != nil {
			return nil, err
		}
		slugs := make([]string, 0, len(rows))
		for _, row := range rows {
			slugs = append(slugs, row.Slug)
		}
		return slugs, nil
	}

	if len(req.MCPServerSlugs) == 0 {
		return nil, nil
	}

	slugs := make([]string, 0, len(req.MCPServerSlugs))
	for _, raw := range req.MCPServerSlugs {
		slug := strings.TrimSpace(raw)
		if slug == "" {
			return nil, fmt.Errorf("mcpServerSlug inválido: %q", raw)
		}
		slugs = append(slugs, slug)
	}
	return slugs, nil
}

func resolveTaskListIDs(ctx context.Context, req portability.ExportRequest) ([]string, error) {
	if req.All {
		taskLists, err := database.GetAllTaskListsWithContext(ctx)
		if err != nil {
			return nil, err
		}
		ids := make([]string, 0, len(taskLists))
		for _, taskList := range taskLists {
			ids = append(ids, taskList.ID)
		}
		return ids, nil
	}

	if len(req.TaskListIDs) == 0 {
		return nil, nil
	}

	ids := make([]string, 0, len(req.TaskListIDs))
	for _, raw := range req.TaskListIDs {
		id := strings.TrimSpace(raw)
		if id == "" {
			return nil, fmt.Errorf("taskListId inválido: %q", raw)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func resolveMemoryRecordIDs(ctx context.Context, req portability.ExportRequest) ([]string, error) {
	if req.All {
		var rows []database.MemoryRecord
		if err := database.ScopeByUser(ctx, database.DB().WithContext(ctx), "user_id").
			Select("id").
			Where("expires_at IS NULL OR expires_at > ?", time.Now()).
			Order("updated_at DESC").
			Find(&rows).Error; err != nil {
			return nil, err
		}
		ids := make([]string, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.ID)
		}
		return ids, nil
	}
	if len(req.MemoryRecordIDs) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(req.MemoryRecordIDs))
	for _, raw := range req.MemoryRecordIDs {
		id := strings.TrimSpace(raw)
		if id == "" {
			return nil, fmt.Errorf("memoryRecordId inválido: %q", raw)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func defaultConversationExportFilename(format string) string {
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	return "conversas_" + timestamp + "." + format
}
