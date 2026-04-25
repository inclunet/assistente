package app

import (
	"context"
	"fmt"
	"os"
	"strconv"
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

func (a *App) ExportConversations(ids []uint) (string, error) {
	return portability.ExportConversations(ids, a.credMgr, portability.ExportRequest{
		OutputFormat: portability.FormatJSON,
	}, AppVersion)
}

func (a *App) ExportData(req ExportRequest) (string, error) {
	if req.OutputFormat == "" {
		req.OutputFormat = portability.FormatJSON
	}

	conversationIDs, err := resolveConversationIDs(req)
	if err != nil {
		return "", err
	}
	providerIDs, err := resolveProviderIDs(req)
	if err != nil {
		return "", err
	}
	taskListIDs, err := resolveTaskListIDs(req)
	if err != nil {
		return "", err
	}
	switch req.OutputFormat {
	case portability.FormatJSON:
		return portability.ExportPortableData(conversationIDs, providerIDs, taskListIDs, a.credMgr, req, AppVersion)
	case portability.FormatHTML:
		if len(taskListIDs) > 0 || len(providerIDs) > 0 {
			return "", fmt.Errorf("exportação HTML/PDF atualmente suporta apenas conversas")
		}
		file, err := portability.BuildConversationExportFile(conversationIDs, a.credMgr, req, AppVersion)
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

// ==================== Import Functions ====================

func (a *App) ImportConversations(jsonData string) (*ImportResult, error) {
	return portability.ImportConversationsWithContext(a.importExportContext(), jsonData, a.credMgr, "")
}

func (a *App) ImportData(jsonData string, credentialExportPassword string) (*ImportResult, error) {
	return portability.ImportConversationsWithContext(a.importExportContext(), jsonData, a.credMgr, credentialExportPassword)
}

func (a *App) ImportDataWithResolutions(req ImportRequest) (*ImportResult, error) {
	return portability.ImportConversationsWithResolutions(
		a.importExportContext(),
		req.JSONData,
		a.credMgr,
		req.CredentialExportPassword,
		req.Resolutions,
	)
}

func (a *App) AnalyzeImportData(jsonData string, credentialExportPassword string) (*ImportAnalysis, error) {
	return portability.AnalyzeImportData(jsonData, a.credMgr, credentialExportPassword)
}

func (a *App) ExportConversationsToFile(ids []uint, format string) (string, error) {
	if a.dialogPort == nil {
		return "", fmt.Errorf("diálogo de sistema não inicializado")
	}

	req := portability.ExportRequest{OutputFormat: format}
	switch format {
	case portability.FormatHTML, portability.FormatPDF:
	default:
		return "", fmt.Errorf("formato de exportação não suportado: %s", format)
	}

	file, err := portability.BuildConversationExportFile(ids, a.credMgr, req, AppVersion)
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

	switch req.OutputFormat {
	case portability.FormatJSON:
		rendered, err := a.ExportData(req)
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(path, []byte(rendered), 0600); err != nil {
			return "", err
		}
		return path, nil
	case portability.FormatHTML, portability.FormatPDF:
		conversationIDs, err := resolveConversationIDs(req)
		if err != nil {
			return "", err
		}
		providerIDs, err := resolveProviderIDs(req)
		if err != nil {
			return "", err
		}
		taskListIDs, err := resolveTaskListIDs(req)
		if err != nil {
			return "", err
		}
		if len(taskListIDs) > 0 || len(providerIDs) > 0 {
			return "", fmt.Errorf("exportação HTML/PDF atualmente suporta apenas conversas")
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

func resolveConversationIDs(req ExportRequest) ([]uint, error) {
	if req.All {
		conversations, err := database.GetConversations()
		if err != nil {
			return nil, err
		}
		ids := make([]uint, 0, len(conversations))
		for _, conv := range conversations {
			ids = append(ids, conv.ID)
		}
		return ids, nil
	}

	if len(req.ConversationIDs) == 0 {
		if len(req.ProviderIDs) > 0 || len(req.TaskListIDs) > 0 {
			return nil, nil
		}
		conversations, err := database.GetConversations()
		if err != nil {
			return nil, err
		}
		ids := make([]uint, 0, len(conversations))
		for _, conv := range conversations {
			ids = append(ids, conv.ID)
		}
		return ids, nil
	}

	ids := make([]uint, 0, len(req.ConversationIDs))
	for _, raw := range req.ConversationIDs {
		id64, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("conversationId inválido: %s", raw)
		}
		ids = append(ids, uint(id64))
	}
	return ids, nil
}

func resolveProviderIDs(req ExportRequest) ([]string, error) {
	if req.All {
		providers, err := database.GetLLMProviders()
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

func resolveTaskListIDs(req ExportRequest) ([]uint, error) {
	if req.All {
		taskLists, err := database.GetAllTaskLists()
		if err != nil {
			return nil, err
		}
		ids := make([]uint, 0, len(taskLists))
		for _, taskList := range taskLists {
			ids = append(ids, taskList.ID)
		}
		return ids, nil
	}

	if len(req.TaskListIDs) == 0 {
		return nil, nil
	}

	ids := make([]uint, 0, len(req.TaskListIDs))
	for _, raw := range req.TaskListIDs {
		id64, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("taskListId inválido: %s", raw)
		}
		ids = append(ids, uint(id64))
	}
	return ids, nil
}

func defaultConversationExportFilename(format string) string {
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	return "conversas_" + timestamp + "." + format
}

func (a *App) importExportContext() context.Context {
	if a != nil && a.ctx != nil {
		return a.ctx
	}
	return context.Background()
}
