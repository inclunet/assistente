package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	memorysvc "assistente/internal/memory"
	"assistente/internal/tools"
)

type Service interface {
	List(ctx context.Context, filter memorysvc.Filter) (memorysvc.ListResult, error)
	Get(ctx context.Context, id string) (*memorysvc.Record, error)
	Create(ctx context.Context, input memorysvc.RecordInput) (*memorysvc.Record, error)
	Update(ctx context.Context, id string, input memorysvc.RecordInput) (*memorysvc.Record, error)
	Patch(ctx context.Context, id string, patch memorysvc.RecordPatch) (*memorysvc.Record, error)
	Archive(ctx context.Context, id string) (*memorysvc.Record, error)
	Unarchive(ctx context.Context, id string, loadPolicy string) (*memorysvc.Record, error)
	Delete(ctx context.Context, id string) error
	PolicySummary(ctx context.Context) (memorysvc.PolicySummary, error)
}

type Tool struct {
	service Service
}

type args struct {
	Action          string   `json:"action,omitempty"`
	ID              string   `json:"id,omitempty"`
	Query           string   `json:"query,omitempty"`
	LoadPolicy      *string  `json:"load_policy,omitempty"`
	LoadPolicies    []string `json:"load_policies,omitempty"`
	Kind            *string  `json:"kind,omitempty"`
	Kinds           []string `json:"kinds,omitempty"`
	Scope           *string  `json:"scope,omitempty"`
	Scopes          []string `json:"scopes,omitempty"`
	ScopeRef        *string  `json:"scope_ref,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	Content         *string  `json:"content,omitempty"`
	Summary         *string  `json:"summary,omitempty"`
	Importance      *int     `json:"importance,omitempty"`
	Confidence      *int     `json:"confidence,omitempty"`
	SourceType      *string  `json:"source_type,omitempty"`
	SourceID        *string  `json:"source_id,omitempty"`
	ExpiresAt       *string  `json:"expires_at,omitempty"`
	ClearExpiresAt  bool     `json:"clear_expires_at,omitempty"`
	IncludeArchived bool     `json:"include_archived,omitempty"`
	Limit           int      `json:"limit,omitempty"`
	Offset          int      `json:"offset,omitempty"`
}

func New(service Service) *Tool {
	return &Tool{service: service}
}

func (t *Tool) Name() string { return "memory" }

func (t *Tool) Description() string {
	return `Search and govern structured long-term memory records that can influence future conversations: list/search/get, write or patch, archive/unarchive, delete, and summarize policy usage.
Use when: durable user preferences, identity, project facts, decisions, conventions, or resolved knowledge should be recalled across turns. Search first to reuse or update an existing record, then get details only when needed.
Do not use: do not store transient chat context, task progress (use update_plan/task/task_note), job state, large source documents, credentials, tokens, or uncertain guesses. A conversation message already remains in history without becoming memory.
Persistence, risk, and cost: records persist in the database. core/pinned/auto records may consume future prompt budget and bias answers; write narrowly scoped, concise facts with appropriate confidence and expiry. archive preserves a record outside normal retrieval; delete removes it. Broad searches and long content increase context cost.
Actions: explicit action is preferred. write with no id creates; write with id patches only supplied fields. search/list supports filters; summary returns policy counts.
Examples: search {"action":"search","query":"release convention","scopes":["project"]}; create {"action":"write","content":"Release tags use vMAJOR.MINOR.PATCH","kind":"convention","scope":"project","load_policy":"retrievable","confidence":90}; archive {"action":"archive","id":"<id>"}.`
}

func (t *Tool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {
				"type": "string",
				"enum": ["list", "search", "get", "write", "archive", "unarchive", "delete", "summary"],
				"description": "Operation to perform. Defaults to search/list when query or filters are present, get when id is present, and write when content is present"
			},
			"id": {"type": "string", "description": "Memory record id. Required for get/archive/unarchive/delete and update via write"},
			"query": {"type": "string", "description": "Text query for search/list"},
			"load_policy": {"type": "string", "enum": ["core", "pinned", "auto", "retrievable", "archived"], "description": "Policy for write/update or unarchive target policy"},
			"load_policies": {"type": "array", "items": {"type": "string", "enum": ["core", "pinned", "auto", "retrievable", "archived"]}, "description": "Filter by load policies"},
			"kind": {"type": "string", "enum": ["user_preference", "identity", "project_fact", "decision", "convention", "historical_note", "resolved_issue"], "description": "Kind for write/update"},
			"kinds": {"type": "array", "items": {"type": "string", "enum": ["user_preference", "identity", "project_fact", "decision", "convention", "historical_note", "resolved_issue"]}, "description": "Filter by kinds"},
			"scope": {"type": "string", "enum": ["global", "user", "workspace", "project", "conversation"], "description": "Scope for write/update"},
			"scopes": {"type": "array", "items": {"type": "string", "enum": ["global", "user", "workspace", "project", "conversation"]}, "description": "Filter by scopes"},
			"scope_ref": {"type": "string", "description": "Optional scope reference, such as workspace/project/conversation id"},
			"tags": {"type": "array", "items": {"type": "string"}, "description": "Tags for write/update or filtering"},
			"content": {"type": "string", "description": "Full memory content. Required for write/create/update"},
			"summary": {"type": "string", "description": "Short summary used when injecting memory into prompt"},
			"importance": {"type": "integer", "minimum": 1, "maximum": 5, "description": "Importance from 1 to 5"},
			"confidence": {"type": "integer", "minimum": 0, "maximum": 100, "description": "Confidence from 0 to 100"},
			"source_type": {"type": "string", "description": "Optional origin type, e.g. conversation, legacy_file, tool"},
			"source_id": {"type": "string", "description": "Optional origin identifier"},
			"expires_at": {"type": "string", "description": "Optional RFC3339 expiration timestamp"},
			"include_archived": {"type": "boolean", "description": "Include archived records in list/search"},
			"limit": {"type": "integer", "minimum": 1, "maximum": 500, "description": "Maximum records to return"},
			"offset": {"type": "integer", "minimum": 0, "description": "Pagination offset"}
		},
		"additionalProperties": false
	}`)
}

func (t *Tool) Execute(ctx context.Context, raw json.RawMessage) (tools.ToolResult, error) {
	if t.service == nil {
		return tools.ToolResult{Content: "memory service unavailable", IsError: true}, nil
	}
	var params args
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &params); err != nil {
			return tools.ToolResult{Content: "Error parsing arguments: " + err.Error(), IsError: true}, nil
		}
	}
	action := inferAction(params)
	switch action {
	case "summary":
		return t.summary(ctx)
	case "get":
		return t.get(ctx, params.ID)
	case "write":
		return t.write(ctx, params)
	case "archive":
		return t.archive(ctx, params.ID)
	case "unarchive":
		return t.unarchive(ctx, params.ID, stringValue(params.LoadPolicy))
	case "delete":
		return t.delete(ctx, params.ID)
	case "list", "search":
		return t.list(ctx, params)
	default:
		return tools.ToolResult{Content: fmt.Sprintf("unsupported memory action %q", action), IsError: true}, nil
	}
}

func inferAction(params args) string {
	if params.Action != "" {
		return strings.TrimSpace(params.Action)
	}
	if params.Content != nil && strings.TrimSpace(*params.Content) != "" {
		return "write"
	}
	if strings.TrimSpace(params.ID) != "" {
		return "get"
	}
	return "search"
}

func (t *Tool) list(ctx context.Context, params args) (tools.ToolResult, error) {
	result, err := t.service.List(ctx, memorysvc.Filter{
		Query:           params.Query,
		LoadPolicies:    mergeOptionalString(params.LoadPolicies, params.LoadPolicy),
		Kinds:           mergeOptionalString(params.Kinds, params.Kind),
		Scopes:          mergeOptionalString(params.Scopes, params.Scope),
		Tags:            params.Tags,
		IncludeArchived: params.IncludeArchived,
		Limit:           params.Limit,
		Offset:          params.Offset,
	})
	if err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}
	return structured(result)
}

func mergeOptionalString(values []string, value *string) []string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return values
	}
	return append(append([]string(nil), values...), strings.TrimSpace(*value))
}

func (t *Tool) get(ctx context.Context, id string) (tools.ToolResult, error) {
	if strings.TrimSpace(id) == "" {
		return tools.ToolResult{Content: "id is required", IsError: true}, nil
	}
	record, err := t.service.Get(ctx, id)
	if err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}
	return structured(record)
}

func (t *Tool) write(ctx context.Context, params args) (tools.ToolResult, error) {
	expiresAt, err := parseOptionalExpiresAt(params.ExpiresAt)
	if err != nil {
		return tools.ToolResult{Content: "invalid expires_at: " + err.Error(), IsError: true}, nil
	}
	var record *memorysvc.Record
	if strings.TrimSpace(params.ID) != "" {
		record, err = t.service.Patch(ctx, params.ID, memorysvc.RecordPatch{
			Content:        params.Content,
			Summary:        params.Summary,
			LoadPolicy:     params.LoadPolicy,
			Kind:           params.Kind,
			Scope:          params.Scope,
			ScopeRef:       params.ScopeRef,
			Tags:           optionalTags(params.Tags),
			Importance:     params.Importance,
			Confidence:     params.Confidence,
			SourceType:     params.SourceType,
			SourceID:       params.SourceID,
			ExpiresAt:      expiresAt,
			ClearExpiresAt: params.ClearExpiresAt,
		})
	} else {
		input := memorysvc.RecordInput{
			Content:    stringValue(params.Content),
			Summary:    stringValue(params.Summary),
			LoadPolicy: stringValue(params.LoadPolicy),
			Kind:       stringValue(params.Kind),
			Scope:      stringValue(params.Scope),
			ScopeRef:   stringValue(params.ScopeRef),
			Tags:       params.Tags,
			Importance: intValue(params.Importance),
			Confidence: intValue(params.Confidence),
			SourceType: stringValue(params.SourceType),
			SourceID:   stringValue(params.SourceID),
			ExpiresAt:  expiresAt,
		}
		record, err = t.service.Create(ctx, input)
	}
	if err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}
	return structured(record)
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func intValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func optionalTags(tags []string) *[]string {
	if tags == nil {
		return nil
	}
	return &tags
}

func parseOptionalExpiresAt(raw *string) (*time.Time, error) {
	if raw == nil {
		return nil, nil
	}
	return memorysvc.ParseExpiresAt(*raw)
}

func (t *Tool) archive(ctx context.Context, id string) (tools.ToolResult, error) {
	record, err := t.service.Archive(ctx, id)
	if err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}
	return structured(record)
}

func (t *Tool) unarchive(ctx context.Context, id, policy string) (tools.ToolResult, error) {
	record, err := t.service.Unarchive(ctx, id, policy)
	if err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}
	return structured(record)
}

func (t *Tool) delete(ctx context.Context, id string) (tools.ToolResult, error) {
	if err := t.service.Delete(ctx, id); err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}
	return structured(map[string]any{"deleted": true, "id": id})
}

func (t *Tool) summary(ctx context.Context) (tools.ToolResult, error) {
	summary, err := t.service.PolicySummary(ctx)
	if err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}
	return structured(summary)
}

func structured(value any) (tools.ToolResult, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}
	return tools.ToolResult{Content: string(data), Structured: true}, nil
}
