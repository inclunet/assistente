package job

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"assistente/internal/jobs"
	"assistente/internal/tools"
)

type pipelineArgs struct {
	Slug        string         `json:"slug,omitempty"`
	Delete      bool           `json:"delete,omitempty"`
	Enabled     *bool          `json:"enabled,omitempty"`
	Name        string         `json:"name,omitempty"`
	Description string         `json:"description,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type PipelineTool struct {
	mgr ManagerProvider
}

func NewPipeline(mgr Manager) *PipelineTool {
	return NewPipelineWithProvider(func() Manager { return mgr })
}

func NewPipelineWithProvider(provider ManagerProvider) *PipelineTool {
	return &PipelineTool{mgr: provider}
}

func (t *PipelineTool) Name() string { return "job_pipeline" }

func (t *PipelineTool) Description() string {
	return "Composite DB-backed job pipeline manager. No params lists persisted pipelines. slug reads a pipeline. With slug plus fields updates. Without slug plus name creates. delete removes a pipeline."
}

func (t *PipelineTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "slug": {"type": "string", "description": "Pipeline slug. Required for read, update and delete. Omit to list all pipelines or create a new one."},
    "delete": {"type": "boolean", "description": "Delete the referenced pipeline. Requires slug."},
    "enabled": {"type": "boolean", "description": "Enabled state for create/update."},
    "name": {"type": "string", "description": "Pipeline display name. Required when creating."},
    "description": {"type": "string"},
    "metadata": {"type": "object", "additionalProperties": true}
  },
  "additionalProperties": false
}`)
}

func (t *PipelineTool) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	if strings.TrimSpace(string(args)) == "" {
		args = json.RawMessage(`{}`)
	}
	var params pipelineArgs
	if err := json.Unmarshal(args, &params); err != nil {
		return tools.ToolResult{Content: "Error parsing arguments: " + err.Error(), IsError: true}, nil
	}
	mgr := t.manager()
	if mgr == nil {
		return tools.ToolResult{Content: "job manager not configured", IsError: true}, nil
	}
	slug := slugFromName(params.Slug)
	if params.Delete {
		if slug == "" {
			return tools.ToolResult{Content: "slug is required to delete a pipeline", IsError: true}, nil
		}
		if err := deletePipeline(ctx, mgr, slug); err != nil {
			return tools.ToolResult{Content: fmt.Sprintf("Error deleting pipeline: %v", err), IsError: true}, nil
		}
		payload := map[string]any{"action": "deleted", "slug": slug}
		data, _ := json.Marshal(payload)
		return tools.ToolResult{Content: string(data), Metadata: payload}, nil
	}

	hasWrite := params.Enabled != nil ||
		strings.TrimSpace(params.Name) != "" ||
		strings.TrimSpace(params.Description) != "" ||
		params.Metadata != nil

	if slug == "" && !hasWrite {
		return t.listPipelines(ctx, mgr)
	}
	if slug != "" && !hasWrite {
		return t.getPipeline(ctx, mgr, slug)
	}
	if slug == "" {
		return t.createPipeline(ctx, mgr, params)
	}
	return t.updatePipeline(ctx, mgr, slug, params)
}

func (t *PipelineTool) manager() Manager {
	if t.mgr == nil {
		return nil
	}
	return t.mgr()
}

func (t *PipelineTool) listPipelines(ctx context.Context, mgr Manager) (tools.ToolResult, error) {
	pipelines, err := listPipelines(ctx, mgr)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error listing pipelines: %v", err), IsError: true}, nil
	}
	data, _ := json.Marshal(pipelines)
	return tools.ToolResult{
		Content:  fmt.Sprintf("Found %d pipeline(s):\n%s", len(pipelines), string(data)),
		Metadata: map[string]any{"count": len(pipelines)},
	}, nil
}

func (t *PipelineTool) getPipeline(ctx context.Context, mgr Manager, slug string) (tools.ToolResult, error) {
	pipeline, ok, err := findPipeline(func() ([]jobs.Pipeline, error) {
		return listPipelines(ctx, mgr)
	}, slug)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error reading pipelines: %v", err), IsError: true}, nil
	}
	if !ok {
		return tools.ToolResult{Content: fmt.Sprintf("pipeline not found: %s", slug), IsError: true}, nil
	}
	data, _ := json.Marshal(pipeline)
	return tools.ToolResult{Content: string(data), Metadata: map[string]any{"slug": slug}}, nil
}

func (t *PipelineTool) createPipeline(ctx context.Context, mgr Manager, params pipelineArgs) (tools.ToolResult, error) {
	if strings.TrimSpace(params.Name) == "" {
		return tools.ToolResult{Content: "name is required to create a pipeline", IsError: true}, nil
	}
	slug := slugFromName(params.Slug)
	if slug == "" {
		slug = slugFromName(params.Name)
	}
	pipeline := &jobs.Pipeline{
		Slug:        slug,
		Name:        params.Name,
		Description: params.Description,
		Enabled:     true,
		Metadata:    params.Metadata,
	}
	if params.Enabled != nil {
		pipeline.Enabled = *params.Enabled
	}
	if err := savePipeline(ctx, mgr, pipeline); err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error creating pipeline: %v", err), IsError: true}, nil
	}
	payload := map[string]any{"action": "created", "slug": slug}
	data, _ := json.Marshal(payload)
	return tools.ToolResult{Content: string(data), Metadata: payload}, nil
}

func (t *PipelineTool) updatePipeline(ctx context.Context, mgr Manager, slug string, params pipelineArgs) (tools.ToolResult, error) {
	pipeline, ok, err := findPipeline(func() ([]jobs.Pipeline, error) {
		return listPipelines(ctx, mgr)
	}, slug)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error reading pipelines: %v", err), IsError: true}, nil
	}
	if !ok {
		return tools.ToolResult{Content: fmt.Sprintf("pipeline not found: %s", slug), IsError: true}, nil
	}
	if strings.TrimSpace(params.Name) != "" {
		pipeline.Name = params.Name
	}
	if strings.TrimSpace(params.Description) != "" {
		pipeline.Description = params.Description
	}
	if params.Enabled != nil {
		pipeline.Enabled = *params.Enabled
	}
	if params.Metadata != nil {
		pipeline.Metadata = params.Metadata
	}
	if err := savePipeline(ctx, mgr, &pipeline); err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error updating pipeline: %v", err), IsError: true}, nil
	}
	payload := map[string]any{"action": "updated", "slug": slug}
	data, _ := json.Marshal(payload)
	return tools.ToolResult{Content: string(data), Metadata: payload}, nil
}

func findPipeline(list func() ([]jobs.Pipeline, error), slug string) (jobs.Pipeline, bool, error) {
	pipelines, err := list()
	if err != nil {
		return jobs.Pipeline{}, false, err
	}
	for _, pipeline := range pipelines {
		if pipeline.Slug == slug {
			return pipeline, true, nil
		}
	}
	return jobs.Pipeline{}, false, nil
}
