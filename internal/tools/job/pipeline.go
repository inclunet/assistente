package job

import (
	"context"
	"encoding/json"
	"errors"
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
	present     map[string]json.RawMessage
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
	return "Composite DB-backed job pipeline manager. No params lists persisted pipelines. slug reads a pipeline. With slug plus fields updates, or creates that explicit slug when not found and name is present. Without slug plus name creates with a generated slug. enabled toggles scheduling for jobs in the pipeline. delete removes a pipeline."
}

func (t *PipelineTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "slug": {"type": "string", "description": "Pipeline slug. Required for read, update and delete. When combined with name and the pipeline does not exist, creates a pipeline with this slug. Omit to list all pipelines or create with a generated slug."},
    "delete": {"type": "boolean", "description": "Delete the referenced pipeline. Requires slug."},
    "enabled": {"type": "boolean", "description": "Enable or disable scheduling for jobs in this pipeline."},
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
	_ = json.Unmarshal(args, &params.present)
	mgr := t.manager()
	if mgr == nil {
		return tools.ToolResult{Content: "job manager not configured", IsError: true}, nil
	}
	slug := slugFromName(params.Slug)
	hasWrite := params.has("name") ||
		params.Enabled != nil ||
		params.has("description") ||
		params.has("metadata")
	if params.Delete {
		if slug == "" {
			return tools.ToolResult{Content: "slug is required to delete a pipeline", IsError: true}, nil
		}
		if hasWrite {
			return tools.ToolResult{Content: "delete cannot be combined with pipeline write fields", IsError: true}, nil
		}
		if err := mgr.DeletePipelineContext(ctx, slug); err != nil {
			return tools.ToolResult{Content: fmt.Sprintf("Error deleting pipeline: %v", err), IsError: true}, nil
		}
		payload := map[string]any{"action": "deleted", "slug": slug}
		data, _ := json.Marshal(payload)
		return tools.ToolResult{Content: string(data), Metadata: payload}, nil
	}

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
	mgr := t.mgr()
	if managerIsNil(mgr) {
		return nil
	}
	return mgr
}

func (t *PipelineTool) listPipelines(ctx context.Context, mgr Manager) (tools.ToolResult, error) {
	pipelines, err := mgr.ListPipelinesContext(ctx)
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
		return mgr.ListPipelinesContext(ctx)
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
	if _, ok, err := findPipeline(func() ([]jobs.Pipeline, error) {
		return mgr.ListPipelinesContext(ctx)
	}, slug); err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error checking pipeline: %v", err), IsError: true}, nil
	} else if ok {
		return tools.ToolResult{Content: fmt.Sprintf("pipeline already exists: %s", slug), IsError: true}, nil
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
	if err := mgr.CreatePipelineContext(ctx, pipeline); err != nil {
		if errors.Is(err, jobs.ErrPipelineAlreadyExists) {
			return tools.ToolResult{Content: fmt.Sprintf("pipeline already exists: %s", slug), IsError: true}, nil
		}
		return tools.ToolResult{Content: fmt.Sprintf("Error creating pipeline: %v", err), IsError: true}, nil
	}
	payload := map[string]any{"action": "created", "slug": slug}
	data, _ := json.Marshal(payload)
	return tools.ToolResult{Content: string(data), Metadata: payload}, nil
}

func (t *PipelineTool) updatePipeline(ctx context.Context, mgr Manager, slug string, params pipelineArgs) (tools.ToolResult, error) {
	pipeline, ok, err := findPipeline(func() ([]jobs.Pipeline, error) {
		return mgr.ListPipelinesContext(ctx)
	}, slug)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error reading pipelines: %v", err), IsError: true}, nil
	}
	if !ok {
		if params.has("name") {
			params.Slug = slug
			return t.createPipeline(ctx, mgr, params)
		}
		return tools.ToolResult{Content: fmt.Sprintf("pipeline not found: %s", slug), IsError: true}, nil
	}
	if strings.TrimSpace(params.Name) != "" {
		pipeline.Name = params.Name
	}
	if params.has("description") {
		pipeline.Description = params.Description
	}
	if params.Enabled != nil {
		pipeline.Enabled = *params.Enabled
	}
	if params.Metadata != nil {
		pipeline.Metadata = params.Metadata
	}
	if err := mgr.SavePipelineContext(ctx, &pipeline); err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error updating pipeline: %v", err), IsError: true}, nil
	}
	payload := map[string]any{"action": "updated", "slug": slug}
	data, _ := json.Marshal(payload)
	return tools.ToolResult{Content: string(data), Metadata: payload}, nil
}

func (p pipelineArgs) has(field string) bool {
	_, ok := p.present[field]
	return ok
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
