package wailsapi

import (
	"assistente/internal/jobs"
	"assistente/internal/logging"
	"assistente/internal/mcp"
	"assistente/internal/tools"
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// DryRunMCPCatalog é a superfície mínima do mcp.Manager usada pelo dry-run
// de tools no bind Jobs (listagem + registro de status de teste).
type DryRunMCPCatalog interface {
	ListToolCatalog(ctx context.Context, filter tools.ToolCatalogFilter) ([]tools.ToolCatalogEntry, error)
	RecordToolTestStatus(ctx context.Context, toolName, status, errorMessage string) error
	RecordToolTestStatusByID(ctx context.Context, toolCatalogID, status, errorMessage string) error
}

func isMCPBridgeDryRunName(toolName string) bool {
	toolName = strings.TrimSpace(toolName)
	if strings.HasPrefix(toolName, "mcp_native__") {
		return false
	}
	_, _, ok := mcp.ParseToolName(toolName)
	return ok
}

func recordToolDryRunStatus(ctx context.Context, mcpCat DryRunMCPCatalog, result *jobs.TestToolResult, status string) {
	if mcpCat == nil || result == nil {
		return
	}
	if result.Origin == tools.ToolOriginMCPNative && strings.TrimSpace(result.ToolCatalogID) == "" {
		return
	}
	toolCatalogID := strings.TrimSpace(result.ToolCatalogID)
	if toolCatalogID != "" {
		if err := mcpCat.RecordToolTestStatusByID(ctx, toolCatalogID, status, result.Error); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			logging.Errorf(ctx, "wailsapi.jobs", "[Tools] erro ao registrar resultado de dry-run para catálogo %s: %v", toolCatalogID, err)
		}
		return
	}
	toolName := strings.TrimSpace(result.ToolName)
	if toolName == "" {
		return
	}
	if err := mcpCat.RecordToolTestStatus(ctx, toolName, status, result.Error); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		logging.Errorf(ctx, "wailsapi.jobs", "[Tools] erro ao registrar resultado de dry-run para %s: %v", toolName, err)
	}
}

func resolveMCPToolDryRunTarget(ctx context.Context, mcpCat DryRunMCPCatalog, req *jobs.TestToolRequest) error {
	if mcpCat == nil {
		return fmt.Errorf("mcp manager not configured")
	}
	serverID := strings.TrimSpace(req.MCPServerID)
	toolName := strings.TrimSpace(req.ToolName)
	if serverID == "" {
		return fmt.Errorf("mcp_server_id is required")
	}
	if toolName == "" {
		return fmt.Errorf("tool_name is required")
	}
	entries, err := mcpCat.ListToolCatalog(ctx, tools.ToolCatalogFilter{
		MCPServerID:        serverID,
		IncludeUnavailable: true,
	})
	if err != nil {
		return fmt.Errorf("list MCP tool catalog: %w", err)
	}
	for _, entry := range entries {
		if entry.DisplayName == toolName || entry.Name == toolName {
			req.ToolName = entry.Name
			req.ToolCatalogID = entry.ID
			req.MCPServerID = entry.MCPServerID
			req.Origin = entry.Origin
			req.Risk = entry.Risk
			if entry.AvailabilityStatus != "" && entry.AvailabilityStatus != tools.ToolAvailabilityAvailable {
				return fmt.Errorf("mcp tool %q is unavailable: %s", toolName, entry.AvailabilityReason)
			}
			return nil
		}
	}
	return fmt.Errorf("mcp tool %q not found for server %s", toolName, serverID)
}
