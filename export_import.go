package main

import "assistente/internal/export"

// ==================== Export/Import Types ====================

// Re-exporta tipos de internal/export para manter compatibilidade com o frontend Wails.
type ExportMetadata = export.Metadata
type ConversationExport = export.ConversationExport
type ConversationsExportFile = export.ExportFile
type ImportResult = export.ImportResult

// ==================== Export Functions ====================

func (a *App) ExportConversations(ids []uint) (string, error) {
	return export.ExportConversations(ids)
}

// ==================== Import Functions ====================

func (a *App) ImportConversations(jsonData string) (*ImportResult, error) {
	return export.ImportConversations(jsonData)
}

