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

	ids, err := resolveConversationIDs(req)
	if err != nil {
		return "", err
	}
	switch req.OutputFormat {
	case portability.FormatJSON:
		return portability.ExportConversations(ids, a.credMgr, req, AppVersion)
	case portability.FormatHTML:
		file, err := portability.BuildConversationExportFile(ids, a.credMgr, req, AppVersion)
		if err != nil {
			return "", err
		}
		rendered, err := portability.RenderConversationExport(file, req.OutputFormat)
		if err != nil {
			return "", err
		}
		return string(rendered), nil
	case portability.FormatPDF:
		return "", fmt.Errorf("use ExportConversationsToFile para exportacao PDF")
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

func resolveConversationIDs(req ExportRequest) ([]uint, error) {
	if req.All || len(req.ConversationIDs) == 0 {
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
