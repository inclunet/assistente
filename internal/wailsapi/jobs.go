package wailsapi

import (
	"assistente/controllers"
	"assistente/internal/jobs"
	"assistente/internal/logging"
	"assistente/internal/tools"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Jobs é o bind Wails do domínio jobs (AEP-0088).
// Auth só via WithUser — sem chamar o helper de auth do App no call site.
type Jobs struct {
	mu                         sync.RWMutex
	session                    Session
	ctrl                       *controllers.JobsController
	mcp                        DryRunMCPCatalog
	listCustomActionEventNames func(ctx context.Context) []string
}

// NewJobs cria o bind vazio; AttachJobs preenche deps no startup.
func NewJobs() *Jobs {
	return &Jobs{}
}

// AttachJobs associa Session, controller e hooks após o startup montar as deps.
// Função de pacote (não método) para não entrar no Bind do Wails.
// mcp e listCustomActionEventNames podem ser nil.
func AttachJobs(
	api *Jobs,
	session Session,
	ctrl *controllers.JobsController,
	mcp DryRunMCPCatalog,
	listCustomActionEventNames func(ctx context.Context) []string,
) {
	if api == nil {
		return
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	api.session = session
	api.ctrl = ctrl
	api.mcp = mcp
	api.listCustomActionEventNames = listCustomActionEventNames
}

func (api *Jobs) deps() (Session, *controllers.JobsController, DryRunMCPCatalog, func(context.Context) []string, error) {
	api.mu.RLock()
	defer api.mu.RUnlock()
	if api.session == nil || api.ctrl == nil {
		return nil, nil, nil, nil, ErrJobsNotWired
	}
	return api.session, api.ctrl, api.mcp, api.listCustomActionEventNames, nil
}

// GetJobs retorna info resumida de todos os jobs do usuário.
func (api *Jobs) GetJobs() ([]jobs.JobInfo, error) {
	session, ctrl, _, _, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]jobs.JobInfo, error) {
		result, err := ctrl.GetJobsContext(ctx)
		if err != nil {
			logging.Errorf(ctx, "wailsapi.jobs", "[Jobs] erro ao listar jobs: %v", err)
			return nil, err
		}
		return result, nil
	})
}

// GetJob retorna um job pelo id/slug.
func (api *Jobs) GetJob(id string) (*jobs.Job, error) {
	session, ctrl, _, _, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*jobs.Job, error) {
		return ctrl.GetJobContext(ctx, id)
	})
}

// ToggleJob habilita ou desabilita um job.
func (api *Jobs) ToggleJob(id string, enabled bool) error {
	session, ctrl, _, _, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.ToggleJobContext(ctx, id, enabled)
	})
	return err
}

// RunJob dispara a execução manual de um job.
func (api *Jobs) RunJob(id string) (*jobs.RunLog, error) {
	session, ctrl, _, _, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*jobs.RunLog, error) {
		return ctrl.RunJobContext(ctx, id)
	})
}

// DryRunJob executa um job em modo dry-run.
func (api *Jobs) DryRunJob(id string) (*jobs.DryRunResult, error) {
	session, ctrl, _, _, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*jobs.DryRunResult, error) {
		return ctrl.DryRunJobContext(ctx, id)
	})
}

// GetJobRuns lista runs recentes de um job.
func (api *Jobs) GetJobRuns(id string, limit int) ([]jobs.RunLog, error) {
	session, ctrl, _, _, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]jobs.RunLog, error) {
		return ctrl.GetJobRunsContext(ctx, id, limit)
	})
}

// ReplayRun reexecuta (dry-run) uma run com os inputs gravados.
func (api *Jobs) ReplayRun(jobID, runID string) (*jobs.TestToolResult, error) {
	session, ctrl, _, _, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*jobs.TestToolResult, error) {
		return ctrl.ReplayRunContext(ctx, jobID, runID)
	})
}

// GetJobEvents lista eventos de domínio do dia informado.
func (api *Jobs) GetJobEvents(date string) ([]jobs.EventEntry, error) {
	session, ctrl, _, _, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]jobs.EventEntry, error) {
		return ctrl.GetJobEventsContext(ctx, date)
	})
}

// GetJobEventsPage lista eventos paginados do dia informado.
func (api *Jobs) GetJobEventsPage(date string, limit, offset int) ([]jobs.EventEntry, error) {
	session, ctrl, _, _, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]jobs.EventEntry, error) {
		return ctrl.GetJobEventsPageContext(ctx, date, limit, offset)
	})
}

// GetJobPipelines lista pipelines de jobs.
func (api *Jobs) GetJobPipelines() ([]jobs.PipelineInfo, error) {
	session, ctrl, _, _, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]jobs.PipelineInfo, error) {
		result, err := ctrl.GetJobPipelinesContext(ctx)
		if err != nil {
			logging.Errorf(ctx, "wailsapi.jobs", "[Jobs] erro ao listar pipelines: %v", err)
			return nil, err
		}
		return result, nil
	})
}

// GetToolCatalog retorna o catálogo de tools disponíveis para jobs.
func (api *Jobs) GetToolCatalog() ([]jobs.CatalogEntry, error) {
	session, ctrl, _, _, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]jobs.CatalogEntry, error) {
		return ctrl.GetToolCatalog()
	})
}

// RegenerateJobCatalog regenera o catálogo de tools de jobs.
func (api *Jobs) RegenerateJobCatalog() error {
	session, ctrl, _, _, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.RegenerateJobCatalog()
	})
	return err
}

// SaveJob cria ou atualiza um job a partir do JSON.
func (api *Jobs) SaveJob(jobJSON string) error {
	session, ctrl, _, _, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.SaveJobContext(ctx, jobJSON)
	})
	return err
}

// TestToolDryRun executa dry-run de uma tool (com resolução MCP quando aplicável).
func (api *Jobs) TestToolDryRun(requestJSON string) (*jobs.TestToolResult, error) {
	session, ctrl, mcp, _, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*jobs.TestToolResult, error) {
		return testToolDryRun(ctx, ctrl, mcp, requestJSON)
	})
}

func testToolDryRun(
	ctx context.Context,
	ctrl *controllers.JobsController,
	mcp DryRunMCPCatalog,
	requestJSON string,
) (*jobs.TestToolResult, error) {
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
		if err := resolveMCPToolDryRunTarget(ctx, mcp, &req); err != nil {
			result := &jobs.TestToolResult{
				Success:       false,
				Error:         err.Error(),
				Origin:        req.Origin,
				MCPServerID:   req.MCPServerID,
				ToolName:      req.ToolName,
				ToolCatalogID: req.ToolCatalogID,
			}
			if req.ToolCatalogID != "" {
				recordToolDryRunStatus(ctx, mcp, result, tools.ToolTestStatusError)
			}
			return result, nil
		}
	}
	result, err := ctrl.TestToolDryRunContext(ctx, req)
	if err != nil {
		if mcp != nil && strings.TrimSpace(req.ToolCatalogID) != "" {
			result = &jobs.TestToolResult{
				Success:       false,
				Error:         err.Error(),
				Origin:        req.Origin,
				MCPServerID:   req.MCPServerID,
				ToolName:      req.ToolName,
				ToolCatalogID: req.ToolCatalogID,
			}
			recordToolDryRunStatus(ctx, mcp, result, tools.ToolTestStatusError)
			return result, nil
		}
		return result, err
	}
	if result == nil || mcp == nil {
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
	recordToolDryRunStatus(ctx, mcp, result, status)
	return result, nil
}

// InferEventSchema infere o schema de payload de um evento conhecido.
func (api *Jobs) InferEventSchema(eventName string) (map[string]any, error) {
	session, ctrl, _, _, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (map[string]any, error) {
		return ctrl.InferEventSchema(eventName), nil
	})
}

// ListKnownEvents une eventos do runtime de jobs com os das custom actions.
func (api *Jobs) ListKnownEvents() ([]string, error) {
	session, ctrl, _, listCustom, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]string, error) {
		events := ctrl.ListKnownEvents()
		seen := make(map[string]bool, len(events))
		for _, e := range events {
			seen[e] = true
		}
		if listCustom != nil {
			for _, e := range listCustom(ctx) {
				if e != "" && !seen[e] {
					seen[e] = true
					events = append(events, e)
				}
			}
		}
		sort.Strings(events)
		return events, nil
	})
}

// DeleteJob remove um job pelo id/slug.
func (api *Jobs) DeleteJob(id string) error {
	session, ctrl, _, _, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.DeleteJobContext(ctx, id)
	})
	return err
}
