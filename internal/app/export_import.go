package app

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"assistente/internal/core/ports"
	"assistente/internal/database"
	"assistente/internal/portability"
)

// ==================== Export/Import Types ====================

type ExportRequest = portability.ExportRequest
type ConversationExport = portability.ConversationExport
type ConversationsExportFile = portability.ExportFile
type ImportResult = portability.ImportResult
type ImportAnalysis = portability.ImportAnalysis
type ImportRequest = portability.ImportRequest
type ImportResolution = portability.ImportResolution

// ==================== Export Functions ====================

func (a *App) ExportConversations(ids []string) (string, error) {
	ctx, err := a.importExportContext()
	if err != nil {
		return "", err
	}
	return portability.ExportConversationsWithContext(ctx, ids, a.credMgr, portability.ExportRequest{
		OutputFormat: portability.FormatJSON,
	}, AppVersion)
}

func (a *App) ExportData(req ExportRequest) (string, error) {
	if req.OutputFormat == "" {
		req.OutputFormat = portability.FormatJSON
	}
	if err := validateDBOnlyExportRequest(req); err != nil {
		return "", err
	}

	ctx, err := a.importExportContext()
	if err != nil {
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
	switch req.OutputFormat {
	case portability.FormatJSON:
		return portability.ExportPortableDataWithContext(ctx, conversationIDs, providerIDs, taskListIDs, a.credMgr, req, AppVersion)
	case portability.FormatHTML:
		if len(taskListIDs) > 0 || len(providerIDs) > 0 || len(mcpServerSlugs) > 0 {
			return "", fmt.Errorf("exportação HTML/PDF atualmente suporta apenas conversas")
		}
		req, err = normalizeRichConversationExportRequest(req)
		if err != nil {
			return "", err
		}
		file, err := portability.BuildConversationExportFileWithContext(ctx, conversationIDs, a.credMgr, req, AppVersion)
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

func validateMCPJSONExportRequest(req ExportRequest) error {
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

// ==================== Import Functions ====================

func (a *App) ImportConversations(jsonData string) (*ImportResult, error) {
	ctx, err := a.importExportContext()
	if err != nil {
		return nil, err
	}
	return portability.ImportConversationsWithContext(ctx, jsonData, a.credMgr, "")
}

func (a *App) ImportData(jsonData string, credentialExportPassword string) (*ImportResult, error) {
	ctx, err := a.importExportContext()
	if err != nil {
		return nil, err
	}
	return portability.ImportConversationsWithContext(ctx, jsonData, a.credMgr, credentialExportPassword)
}

func (a *App) ImportDataWithResolutions(req ImportRequest) (*ImportResult, error) {
	ctx, err := a.importExportContext()
	if err != nil {
		return nil, err
	}
	return portability.ImportConversationsWithResolutions(
		ctx,
		req.JSONData,
		a.credMgr,
		req.CredentialExportPassword,
		req.Resolutions,
	)
}

func (a *App) AnalyzeImportData(jsonData string, credentialExportPassword string) (*ImportAnalysis, error) {
	ctx, err := a.importExportContext()
	if err != nil {
		return nil, err
	}
	return portability.AnalyzeImportDataWithContext(ctx, jsonData, a.credMgr, credentialExportPassword)
}

func (a *App) ExportConversationsToFile(ids []string, format string, options portability.ContentExportOptions) (string, error) {
	if a.dialogPort == nil {
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

	ctx, err := a.importExportContext()
	if err != nil {
		return "", err
	}
	file, err := portability.BuildConversationExportFileWithContext(ctx, ids, a.credMgr, req, AppVersion)
	if err != nil {
		return "", err
	}

	rendered, err := portability.RenderConversationExport(file, format)
	if err != nil {
		return "", err
	}

	path, err := a.dialogPort.SaveFileDialog(ports.SaveFileOptions{
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
}

func (a *App) ExportDataToFile(req ExportRequest, path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("caminho de saída é obrigatório")
	}
	if req.OutputFormat == "" {
		req.OutputFormat = portability.FormatJSON
	}
	if err := validateDBOnlyExportRequest(req); err != nil {
		return "", err
	}

	switch req.OutputFormat {
	case portability.FormatJSON, portability.FormatMCPJSON:
		rendered, err := a.ExportData(req)
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(path, []byte(rendered), 0600); err != nil {
			return "", err
		}
		return path, nil
	case portability.FormatHTML, portability.FormatPDF, portability.FormatMarkdown:
		ctx, err := a.importExportContext()
		if err != nil {
			return "", err
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
		taskListIDs, err := resolveTaskListIDs(ctx, req)
		if err != nil {
			return "", err
		}
		if len(taskListIDs) > 0 || len(providerIDs) > 0 || len(mcpServerSlugs) > 0 {
			return "", fmt.Errorf("exportação HTML/PDF/Markdown atualmente suporta apenas conversas")
		}
		req, err = normalizeRichConversationExportRequest(req)
		if err != nil {
			return "", err
		}

		file, err := portability.BuildConversationExportFile(conversationIDs, a.credMgr, req, AppVersion)
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
}

func normalizeRichConversationExportRequest(req ExportRequest) (ExportRequest, error) {
	if req.IncludeCredentials {
		return req, fmt.Errorf("exportação HTML/PDF não suporta credenciais")
	}
	if strings.TrimSpace(req.CredentialExportPassword) != "" {
		return req, fmt.Errorf("exportação HTML/PDF não usa senha de credenciais")
	}
	req.IncludeCredentials = false
	req.CredentialExportPassword = ""
	return req, nil
}

func validateDBOnlyExportRequest(req ExportRequest) error {
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

func resolveConversationIDs(ctx context.Context, req ExportRequest) ([]string, error) {
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
		if len(req.ProviderIDs) > 0 || len(req.TaskListIDs) > 0 || len(req.MCPServerSlugs) > 0 {
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

func resolveProviderIDs(ctx context.Context, req ExportRequest) ([]string, error) {
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

func resolveMCPServerSlugs(ctx context.Context, req ExportRequest) ([]string, error) {
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

func resolveTaskListIDs(ctx context.Context, req ExportRequest) ([]string, error) {
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

func defaultConversationExportFilename(format string) string {
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	return "conversas_" + timestamp + "." + format
}

func (a *App) importExportContext() (context.Context, error) {
	if a == nil {
		return context.Background(), nil
	}
	return a.requireAuthenticatedContext()
}
