package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"assistente/internal/configdir"
	"assistente/internal/database"
	"assistente/internal/jobs"
	"assistente/internal/mcp"
	"assistente/internal/tools"

	"gorm.io/gorm"
)

// credentialSecretStore adapta credentials.Manager para a interface jobs.SecretStore.
type credentialSecretStore struct {
	app *App
}

func (s *credentialSecretStore) GetSecret(ctx context.Context, key string) (string, error) {
	if s.app.credMgr == nil {
		return "", nil
	}
	if _, err := database.RequireUserID(ctx); err != nil {
		return "", err
	}
	auth, err := s.app.credMgr.GetByPatternWithContext(ctx, key)
	if err != nil {
		return "", err
	}
	if auth == nil {
		return "", nil
	}
	if auth.Token != "" {
		return auth.Token, nil
	}
	if auth.Password != "" {
		return auth.Password, nil
	}
	return "", nil
}

func (a *App) initJobs() {
	baseDir := filepath.Join(configdir.GetHomeDir(), "jobs")

	a.jobMgr = jobs.NewManager(jobs.ManagerConfig{
		BaseDir:         baseDir,
		Repository:      jobs.NewDBRepository(database.DB()),
		ContextProvider: a.jobsAuthenticatedContext,
		ToolRegistry:    a.toolRegistry,
		ToolInvocations: a.toolInvocationSvc,
		HotkeyManager:   a.hotkeyCtrl.Manager(),
		MsgGateway:      a.msgGateway,
		SecretStore:     &credentialSecretStore{app: a},
		EmitEvent: func(event string, data any) {
			a.emitter.Emit(event, data)
		},
	})
}

// --- Métodos Wails-bound para o frontend ---

func (a *App) GetJobs() []jobs.JobInfo {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil
	}
	result, err := a.jobsCtrl.GetJobsContext(ctx)
	if err != nil {
		log.Printf("[Jobs] erro ao listar jobs: %v", err)
		return nil
	}
	return result
}

func (a *App) jobsAuthenticatedContext() context.Context {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil
	}
	return ctx
}

func (a *App) GetJob(id string) (*jobs.Job, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.jobsCtrl.GetJobContext(ctx, id)
}
func (a *App) ToggleJob(id string, enabled bool) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	return a.jobsCtrl.ToggleJobContext(ctx, id, enabled)
}
func (a *App) RunJob(id string) (*jobs.RunLog, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.jobsCtrl.RunJobContext(ctx, id)
}
func (a *App) DryRunJob(id string) (*jobs.DryRunResult, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.jobsCtrl.DryRunJobContext(ctx, id)
}

func (a *App) GetJobRuns(id string, limit int) ([]jobs.RunLog, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.jobsCtrl.GetJobRunsContext(ctx, id, limit)
}
func (a *App) ReplayRun(jobID, runID string) (*jobs.TestToolResult, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.jobsCtrl.ReplayRunContext(ctx, jobID, runID)
}
func (a *App) GetJobEvents(date string) ([]jobs.EventEntry, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.jobsCtrl.GetJobEventsContext(ctx, date)
}

func (a *App) GetJobEventsPage(date string, limit, offset int) ([]jobs.EventEntry, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.jobsCtrl.GetJobEventsPageContext(ctx, date, limit, offset)
}

func (a *App) GetJobPipelines() []jobs.PipelineInfo {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil
	}
	result, err := a.jobsCtrl.GetJobPipelinesContext(ctx)
	if err != nil {
		log.Printf("[Jobs] erro ao listar pipelines: %v", err)
		return nil
	}
	return result
}
func (a *App) GetToolCatalog() ([]jobs.CatalogEntry, error) {
	if _, err := a.requireAuthenticatedContext(); err != nil {
		return nil, err
	}
	return a.jobsCtrl.GetToolCatalog()
}
func (a *App) RegenerateJobCatalog() error {
	if _, err := a.requireAuthenticatedContext(); err != nil {
		return err
	}
	return a.jobsCtrl.RegenerateJobCatalog()
}
func (a *App) SaveJob(jobJSON string) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	return a.jobsCtrl.SaveJobContext(ctx, jobJSON)
}

func (a *App) TestToolDryRun(requestJSON string) (*jobs.TestToolResult, error) {
	ctx, authErr := a.requireAuthenticatedContext()
	if authErr != nil {
		return nil, authErr
	}
	var req jobs.TestToolRequest
	if strings.TrimSpace(requestJSON) != "" {
		if err := json.Unmarshal([]byte(requestJSON), &req); err != nil {
			return nil, fmt.Errorf("invalid dry-run request: %w", err)
		}
	}
	if req.Inputs == nil {
		req.Inputs = make(map[string]any)
	}
	if strings.TrimSpace(req.MCPServerID) == "" && isMCPBridgeDryRunName(req.ToolName) && !req.AllowUnsafe {
		return &jobs.TestToolResult{
			Success:  false,
			Error:    "dry-run bloqueado por política para tool MCP sem mcp_server_id",
			Blocked:  true,
			Origin:   tools.ToolOriginMCPBridge,
			ToolName: strings.TrimSpace(req.ToolName),
		}, nil
	}
	if strings.TrimSpace(req.MCPServerID) != "" {
		if err := a.resolveMCPToolDryRunTarget(ctx, &req); err != nil {
			result := &jobs.TestToolResult{
				Success:       false,
				Error:         err.Error(),
				Origin:        req.Origin,
				MCPServerID:   req.MCPServerID,
				ToolName:      req.ToolName,
				ToolCatalogID: req.ToolCatalogID,
			}
			if req.ToolCatalogID != "" {
				a.recordToolDryRunStatus(ctx, result, tools.ToolTestStatusError)
			}
			return result, nil
		}
	}
	result, err := a.jobsCtrl.TestToolDryRunContext(ctx, req)
	if err != nil || result == nil || a.mcpMgr == nil {
		return result, err
	}
	if result.ToolName == "" {
		result.ToolName = strings.TrimSpace(req.ToolName)
	}
	status := tools.ToolTestStatusOK
	if result.Blocked {
		status = tools.ToolTestStatusBlocked
	} else if !result.Success {
		status = tools.ToolTestStatusError
	}
	a.recordToolDryRunStatus(ctx, result, status)
	return result, nil
}

func isMCPBridgeDryRunName(toolName string) bool {
	toolName = strings.TrimSpace(toolName)
	if strings.HasPrefix(toolName, "mcp_native__") {
		return false
	}
	_, _, ok := mcp.ParseToolName(toolName)
	return ok
}

func (a *App) recordToolDryRunStatus(ctx context.Context, result *jobs.TestToolResult, status string) {
	if a.mcpMgr == nil || result == nil || result.Origin == tools.ToolOriginMCPNative {
		return
	}
	toolName := strings.TrimSpace(result.ToolName)
	if toolName == "" {
		return
	}
	if err := a.mcpMgr.RecordToolTestStatus(ctx, toolName, status, result.Error); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Printf("[Tools] erro ao registrar resultado de dry-run para %s: %v", toolName, err)
	}
}

func (a *App) resolveMCPToolDryRunTarget(ctx context.Context, req *jobs.TestToolRequest) error {
	if a.mcpMgr == nil {
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
	entries, err := a.mcpMgr.ListToolCatalog(ctx, tools.ToolCatalogFilter{
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

func (a *App) InferEventSchema(eventName string) map[string]any {
	if _, err := a.requireAuthenticatedContext(); err != nil {
		return nil
	}
	return a.jobsCtrl.InferEventSchema(eventName)
}
func (a *App) ListKnownEvents() []string {
	if _, err := a.requireAuthenticatedContext(); err != nil {
		return nil
	}
	return a.jobsCtrl.ListKnownEvents()
}
func (a *App) DeleteJob(id string) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	return a.jobsCtrl.DeleteJobContext(ctx, id)
}
