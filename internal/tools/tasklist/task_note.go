package tasklist

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"assistente/internal/database"
	"assistente/internal/tools"
)

type taskNoteArgs struct {
	List              bool    `json:"list,omitempty"`
	TaskListID        string  `json:"task_list_id,omitempty"`
	TaskListSlug      string  `json:"task_list_slug,omitempty"`
	TaskID            *string `json:"task_id,omitempty"`
	TaskCode          string  `json:"task_code,omitempty"`
	Code              string  `json:"code,omitempty"`
	NoteID            *string `json:"note_id,omitempty"`
	Type              *int    `json:"type,omitempty"`
	Content           string  `json:"content,omitempty"`
	AuthorName        string  `json:"author_name,omitempty"`
	AuthorID          string  `json:"author_id,omitempty"`
	Source            *string `json:"source,omitempty"`
	ExternalID        *string `json:"external_id,omitempty"`
	ExternalParentID  *string `json:"external_parent_id,omitempty"`
	ExternalUpdatedAt string  `json:"external_updated_at,omitempty"`
	Limit             *int    `json:"limit,omitempty"`
	Cursor            *string `json:"cursor,omitempty"`
	Sort              *string `json:"sort,omitempty"`
}

type TaskNoteTool struct {
	mgr TaskListManager
}

func NewTaskNote(mgr TaskListManager) *TaskNoteTool {
	return &TaskNoteTool{mgr: mgr}
}

func (t *TaskNoteTool) Name() string { return "task_note" }

// CatalogMetadata declara os metadados de catálogo da tool (AEP-0077, Fase 1).
func (t *TaskNoteTool) CatalogMetadata() tools.CatalogMetadata {
	return tools.CatalogMetadata{Category: "tasklist", Class: "task_management", Package: "tasks", Risk: "write"}
}

func (t *TaskNoteTool) Description() string {
	return `List/filter/page task notes, or create/update one persistent note/comment attached to a task; this tool does not change the task's core fields or delete notes.
Use when: draining a bounded queue of notes, finding synchronized comments/threads, recording internal context, a customer response, an agent action, a system event, or synchronizing an external comment. Set list=true for reads. For sync writes, source + external_id performs an idempotent upsert.
Do not use: use task for title, status, assignee, hierarchy, or deletion; task_list for the container/workflow; memory only for durable knowledge that should outlive task tracking.
Persistence, risk, and cost: reads are database-backed and capped at 100; writes persist and note events may trigger jobs. Avoid secrets and redundant transcripts. Updating by note_id replaces its content.
Resolution: in list mode, task filters are optional; task_id wins, while task_code can be scoped by task_list_id/slug. source, type, external_id and external_parent_id are exact filters. null on a string filter selects notes without that external value. Cursors are opaque and bound to every filter and sort; created_at plus note id is the stable key. In write mode, existing resolution and upsert behavior are unchanged.
Examples: oldest customer notes {"list":true,"type":2,"limit":20,"sort":"created_at:asc"}; Jira thread {"list":true,"task_code":"FSD-123","source":"jira","external_parent_id":"comment-1","limit":20}; continue {"list":true,"type":2,"limit":20,"sort":"created_at:asc","cursor":"<next_cursor>"}; add {"task_id":"<id>","type":1,"content":"Waiting for approval"}; sync {"task_code":"FSD-123","source":"jira","external_id":"comment-9","type":2,"content":"Approved"}.`
}

func (t *TaskNoteTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"list": {
				"type": "boolean",
				"description": "Set true for a read-only filtered/paged listing. Omit or false to preserve the existing create/update/upsert behavior"
			},
			"task_list_id": {
				"type": "string",
				"description": "Task list UUID; use with task_list_slug and task_code/code to scope task resolution"
			},
			"task_list_slug": {
				"type": "string",
				"description": "Task list slug; with code identifies the task when task_id is omitted"
			},
			"task_id": {
				"type": ["string", "null"],
				"description": "Task UUID. In list mode, filters notes to that task. Optional if task_code identifies it; takes precedence over task_code"
			},
			"task_code": {
				"type": "string",
				"description": "Task.Code (e.g. FSD-12345) to find the task without task_id. Optional task_list_id or task_list_slug disambiguates if the same code exists in multiple lists"
			},
			"code": {
				"type": "string",
				"description": "Task code within a specific list when resolving with task_list_id/slug (not the same field as task_code)"
			},
			"note_id": {
				"type": "string",
				"description": "ID of an existing note to update. Omit to create or upsert"
			},
			"type": {
				"type": "integer",
				"description": "Note type: 1=internal, 2=customer, 3=agent, 4=system. Required for manual create; required on first external upsert when the note does not exist yet",
				"enum": [1, 2, 3, 4]
			},
			"content": {
				"type": "string",
				"description": "Note content (supports markdown). Required for create, update, or upsert; not used with list=true"
			},
			"author_name": {
				"type": "string",
				"description": "Display name of the note author (optional)"
			},
			"author_id": {
				"type": "string",
				"description": "Stable identifier for the author (optional)"
			},
			"source": {
				"type": ["string", "null"],
				"description": "External system key (e.g. jira). In list mode, exact filter; null selects notes without an external source. In write mode, use with external_id for idempotent sync"
			},
			"external_id": {
				"type": ["string", "null"],
				"description": "Stable remote identifier. In list mode, exact filter; null selects notes without one. In write mode, use with source"
			},
			"external_parent_id": {
				"type": ["string", "null"],
				"description": "Remote parent/thread reference. In list mode, exact filter; null selects top-level notes without a parent"
			},
			"external_updated_at": {
				"type": "string",
				"description": "Optional RFC3339 timestamp of last remote update"
			},
			"limit": {
				"type": "integer",
				"minimum": 1,
				"maximum": 100,
				"description": "Maximum notes returned when list=true (1-100); defaults to 100 server-side"
			},
			"cursor": {
				"type": ["string", "null"],
				"description": "Opaque next_cursor from a previous list response; it must be reused with identical filters and sort"
			},
			"sort": {
				"type": ["string", "null"],
				"enum": ["created_at:asc", "created_at:desc", null],
				"description": "Explicit stable order for list=true; note id is the deterministic tie-breaker"
			}
		},
		"additionalProperties": false
	}`)
}

// resolveTaskID resolves which task the note targets. Priority: task_id (with optional consistency checks) > task_code > task_list + code.
func (t *TaskNoteTool) resolveTaskID(ctx context.Context, params taskNoteArgs) (string, error) {
	listIP := uintPtrIfPositive(params.TaskListID)
	tidPtr := taskIDPtrForResolve(params.TaskID)
	taskCodeTrim := strings.TrimSpace(params.TaskCode)
	codeTrim := strings.TrimSpace(params.Code)

	if tidPtr != nil {
		task, err := t.mgr.GetTask(ctx, *tidPtr)
		if err != nil {
			return "", fmt.Errorf("task_id %s não encontrado: %w", *tidPtr, err)
		}
		if taskCodeTrim != "" && strings.TrimSpace(task.Code) != taskCodeTrim {
			return "", fmt.Errorf("task_id %s tem task_code %q, que não coincide com %q", task.ID, task.Code, taskCodeTrim)
		}
		if codeTrim != "" && task.Code != codeTrim {
			return "", fmt.Errorf("task_id %s e code %q não correspondem à mesma task", *tidPtr, codeTrim)
		}
		hasListRef := listIP != nil || strings.TrimSpace(params.TaskListSlug) != ""
		if hasListRef {
			listID, err := t.mgr.ResolveTaskListRef(ctx, listIP, params.TaskListSlug)
			if err != nil {
				return "", err
			}
			if task.TaskListID != listID {
				return "", fmt.Errorf("task_id %s e lista referenciada não correspondem à mesma task", *tidPtr)
			}
		}
		return task.ID, nil
	}

	if taskCodeTrim != "" {
		var scope *string
		if listIP != nil || strings.TrimSpace(params.TaskListSlug) != "" {
			lid, err := t.mgr.ResolveTaskListRef(ctx, listIP, params.TaskListSlug)
			if err != nil {
				return "", err
			}
			scope = &lid
		}
		return t.mgr.ResolveTaskIDByTaskCode(ctx, scope, taskCodeTrim)
	}

	return t.mgr.ResolveTaskRef(ctx, listIP, params.TaskListSlug, nil, codeTrim)
}

func (t *TaskNoteTool) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var params taskNoteArgs
	if err := json.Unmarshal(args, &params); err != nil {
		return tools.ToolResult{Content: "Error parsing arguments: " + err.Error(), IsError: true}, nil
	}
	var rawFields map[string]json.RawMessage
	if err := json.Unmarshal(args, &rawFields); err != nil {
		return tools.ToolResult{Content: "Error parsing arguments: " + err.Error(), IsError: true}, nil
	}
	params.TaskListID = strings.TrimSpace(params.TaskListID)
	if params.TaskID != nil {
		taskID := strings.TrimSpace(*params.TaskID)
		if taskID == "" {
			return tools.ToolResult{Content: "task_id must be a non-empty UUID or null", IsError: true}, nil
		}
		params.TaskID = &taskID
	}
	if params.NoteID != nil {
		noteID := strings.TrimSpace(*params.NoteID)
		if noteID == "" {
			return tools.ToolResult{Content: "note_id must be a non-empty UUID", IsError: true}, nil
		}
		params.NoteID = &noteID
	}

	if params.List {
		return t.listNotes(ctx, params, rawFields)
	}
	if params.Limit != nil || params.Cursor != nil || params.Sort != nil {
		return tools.ToolResult{Content: "limit, cursor, and sort require list=true", IsError: true}, nil
	}

	content := strings.TrimSpace(params.Content)
	if content == "" {
		return tools.ToolResult{Content: "content cannot be empty", IsError: true}, nil
	}

	listIP := uintPtrIfPositive(params.TaskListID)
	tidPtr := taskIDPtrForResolve(params.TaskID)
	taskCodeTrim := strings.TrimSpace(params.TaskCode)
	codeTrim := strings.TrimSpace(params.Code)

	if params.NoteID != nil {
		note, err := t.mgr.GetTaskNote(ctx, *params.NoteID)
		if err != nil {
			return tools.ToolResult{Content: fmt.Sprintf("Note not found (id=%s): %v", *params.NoteID, err), IsError: true}, nil
		}
		var taskID string
		if tidPtr != nil || taskCodeTrim != "" || codeTrim != "" || listIP != nil || strings.TrimSpace(params.TaskListSlug) != "" {
			resolved, err := t.resolveTaskID(ctx, params)
			if err != nil {
				return tools.ToolResult{Content: err.Error(), IsError: true}, nil
			}
			if resolved != note.TaskID {
				return tools.ToolResult{Content: fmt.Sprintf("note_id %s belongs to task %s, which does not match the resolved task %s", *params.NoteID, note.TaskID, resolved), IsError: true}, nil
			}
			taskID = resolved
		} else {
			taskID = note.TaskID
		}
		return t.updateNote(ctx, *params.NoteID, taskID, content)
	}

	resolvedTaskID, err := t.resolveTaskID(ctx, params)
	if err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}

	src := trimmedOptionalString(params.Source)
	extID := trimmedOptionalString(params.ExternalID)
	if src != "" || extID != "" {
		if src == "" || extID == "" {
			return tools.ToolResult{Content: "for external idempotent upsert, both source and external_id are required", IsError: true}, nil
		}
		return t.upsertExternalNote(ctx, resolvedTaskID, content, params.AuthorName, params.AuthorID, src, extID, trimmedOptionalString(params.ExternalParentID), params.ExternalUpdatedAt, params.Type)
	}

	if params.Type == nil {
		return tools.ToolResult{Content: "type is required when creating a new note without external source + external_id", IsError: true}, nil
	}
	return t.createNote(ctx, resolvedTaskID, *params.Type, content, params.AuthorName, params.AuthorID)
}

func trimmedOptionalString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func (t *TaskNoteTool) listNotes(ctx context.Context, params taskNoteArgs, rawFields map[string]json.RawMessage) (tools.ToolResult, error) {
	for _, field := range []string{"note_id", "content", "author_name", "author_id", "external_updated_at"} {
		if _, present := rawFields[field]; present {
			return tools.ToolResult{Content: fmt.Sprintf("%s cannot be combined with list=true", field), IsError: true}, nil
		}
	}
	if params.Limit != nil && (*params.Limit < 1 || *params.Limit > database.MaxTaskPageLimit) {
		return tools.ToolResult{Content: fmt.Sprintf("limit must be between 1 and %d", database.MaxTaskPageLimit), IsError: true}, nil
	}
	if params.Cursor != nil && strings.TrimSpace(*params.Cursor) == "" {
		return tools.ToolResult{Content: "cursor must be a non-empty next_cursor or null", IsError: true}, nil
	}
	if params.Sort != nil && strings.TrimSpace(*params.Sort) == "" {
		return tools.ToolResult{Content: "sort must be created_at:asc, created_at:desc, or null", IsError: true}, nil
	}

	hasTaskRef := taskIDPtrForResolve(params.TaskID) != nil ||
		strings.TrimSpace(params.TaskCode) != "" ||
		strings.TrimSpace(params.Code) != ""
	hasListRef := strings.TrimSpace(params.TaskListID) != "" || strings.TrimSpace(params.TaskListSlug) != ""
	if hasListRef && !hasTaskRef {
		return tools.ToolResult{Content: "task_list_id/task_list_slug require task_id, task_code, or code when list=true", IsError: true}, nil
	}

	var taskID *string
	if hasTaskRef {
		resolved, err := t.resolveTaskID(ctx, params)
		if err != nil {
			return tools.ToolResult{Content: err.Error(), IsError: true}, nil
		}
		taskID = &resolved
	}

	limit := database.DefaultTaskPageLimit
	if params.Limit != nil {
		limit = *params.Limit
	}
	cursor := trimmedOptionalString(params.Cursor)
	sort := trimmedOptionalString(params.Sort)
	if sort == "" {
		sort = database.TaskSortCreatedAtAsc
	}
	var noteType *database.TaskNoteType
	if params.Type != nil {
		value := database.TaskNoteType(*params.Type)
		noteType = &value
	}
	query := database.TaskNotePageQuery{
		TaskID:           taskID,
		Source:           nullableStringFilter(rawFields, "source", params.Source),
		Type:             noteType,
		ExternalID:       nullableStringFilter(rawFields, "external_id", params.ExternalID),
		ExternalParentID: nullableStringFilter(rawFields, "external_parent_id", params.ExternalParentID),
		Limit:            limit,
		Cursor:           cursor,
		Sort:             sort,
	}
	page, err := t.mgr.ListTaskNotesPage(ctx, query)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error listing task notes: %v", err), IsError: true}, nil
	}

	type pagedNote struct {
		ID                string                `json:"id"`
		TaskID            string                `json:"task_id"`
		Type              database.TaskNoteType `json:"type"`
		Content           string                `json:"content"`
		AuthorName        string                `json:"author_name,omitempty"`
		AuthorID          string                `json:"author_id,omitempty"`
		Source            string                `json:"source,omitempty"`
		ExternalID        string                `json:"external_id,omitempty"`
		ExternalParentID  string                `json:"external_parent_id,omitempty"`
		ExternalUpdatedAt *string               `json:"external_updated_at,omitempty"`
		CreatedAt         string                `json:"created_at"`
		UpdatedAt         string                `json:"updated_at"`
	}
	notes := make([]pagedNote, len(page.Notes))
	for i, note := range page.Notes {
		var externalUpdatedAt *string
		if note.ExternalUpdatedAt != nil {
			formatted := note.ExternalUpdatedAt.UTC().Format(time.RFC3339Nano)
			externalUpdatedAt = &formatted
		}
		notes[i] = pagedNote{
			ID:                note.ID,
			TaskID:            note.TaskID,
			Type:              note.Type,
			Content:           note.Content,
			AuthorName:        note.AuthorName,
			AuthorID:          note.AuthorID,
			Source:            note.ExternalSource,
			ExternalID:        note.ExternalID,
			ExternalParentID:  note.ExternalParentID,
			ExternalUpdatedAt: externalUpdatedAt,
			CreatedAt:         note.CreatedAt.UTC().Format(time.RFC3339Nano),
			UpdatedAt:         note.UpdatedAt.UTC().Format(time.RFC3339Nano),
		}
	}
	response := map[string]any{
		"notes":       notes,
		"limit":       limit,
		"sort":        sort,
		"has_more":    page.HasMore,
		"next_cursor": nil,
	}
	if taskID != nil {
		response["task_id"] = *taskID
	}
	addFilterToResponse(response, "source", query.Source)
	if noteType != nil {
		response["type"] = *noteType
	}
	addFilterToResponse(response, "external_id", query.ExternalID)
	addFilterToResponse(response, "external_parent_id", query.ExternalParentID)
	if page.NextCursor != "" {
		response["next_cursor"] = page.NextCursor
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error encoding task notes page: %v", err), IsError: true}, nil
	}
	metadata := map[string]any{
		"count":    len(notes),
		"limit":    limit,
		"sort":     sort,
		"has_more": page.HasMore,
	}
	if taskID != nil {
		metadata["task_id"] = *taskID
	}
	if page.NextCursor != "" {
		metadata["next_cursor"] = page.NextCursor
	}
	return tools.ToolResult{Content: string(encoded), Metadata: metadata, Structured: true}, nil
}

func nullableStringFilter(rawFields map[string]json.RawMessage, name string, value *string) database.NullableStringFilter {
	_, present := rawFields[name]
	return database.NullableStringFilter{Set: present, Value: value}
}

func addFilterToResponse(response map[string]any, name string, filter database.NullableStringFilter) {
	if !filter.Set {
		return
	}
	if filter.Value == nil {
		response[name] = nil
		return
	}
	response[name] = strings.TrimSpace(*filter.Value)
}

func (t *TaskNoteTool) upsertExternalNote(ctx context.Context, taskID string, content, authorName, authorID, source, externalID, externalParentID, externalUpdatedAtRaw string, typeArg *int) (tools.ToolResult, error) {
	var extTime *time.Time
	if strings.TrimSpace(externalUpdatedAtRaw) != "" {
		ts, err := parseExternalUpdatedAt(externalUpdatedAtRaw)
		if err != nil {
			return tools.ToolResult{Content: "invalid external_updated_at: " + err.Error(), IsError: true}, nil
		}
		extTime = ts
	}

	var typePtr *database.TaskNoteType
	if typeArg != nil {
		nt := int(*typeArg)
		if nt < 1 || nt > 4 {
			return tools.ToolResult{Content: "type must be 1 (internal), 2 (customer), 3 (agent), or 4 (system)", IsError: true}, nil
		}
		tv := database.TaskNoteType(nt)
		typePtr = &tv
	}

	note, created, err := t.mgr.UpsertTaskNoteByExternal(ctx, database.UpsertTaskNoteByExternalParams{
		TaskID:            taskID,
		Type:              typePtr,
		Content:           content,
		AuthorName:        authorName,
		AuthorID:          authorID,
		ExternalSource:    source,
		ExternalID:        externalID,
		ExternalParentID:  externalParentID,
		ExternalUpdatedAt: extTime,
	})
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error upserting external note on task %s: %v", taskID, err), IsError: true}, nil
	}

	action := "updated"
	if created {
		action = "created"
	}
	typeLabels := map[int]string{1: "internal note", 2: "customer response", 3: "agent action", 4: "system event"}
	typeLabel := typeLabels[int(note.Type)]
	if typeLabel == "" {
		typeLabel = fmt.Sprintf("type %d", note.Type)
	}

	resultMap := map[string]any{
		"id":          note.ID,
		"task_id":     note.TaskID,
		"type":        typeLabel,
		"action":      action,
		"source":      source,
		"external_id": externalID,
	}
	if note.AuthorName != "" {
		resultMap["author_name"] = note.AuthorName
	}
	if note.AuthorID != "" {
		resultMap["author_id"] = note.AuthorID
	}
	if note.ExternalParentID != "" {
		resultMap["external_parent_id"] = note.ExternalParentID
	}
	if note.ExternalUpdatedAt != nil {
		resultMap["external_updated_at"] = note.ExternalUpdatedAt.Format(time.RFC3339)
	}

	resultJSON, _ := json.Marshal(resultMap)
	md := map[string]any{"note_id": note.ID, "task_id": taskID, "action": action, "source": source, "external_id": externalID}
	return tools.ToolResult{
		Content:  fmt.Sprintf("Note %s on task %s (%s):\n%s", action, taskID, typeLabel, string(resultJSON)),
		Metadata: md,
	}, nil
}

func parseExternalUpdatedAt(s string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.999999999-0700", // Jira (ISO-8601, offset ±HHMM sem dois-pontos), fração opcional
		"2006-01-02T15:04:05-0700",           // Jira sem fração
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}
	var lastErr error
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return &t, nil
		} else {
			lastErr = err
		}
	}
	return nil, fmt.Errorf("could not parse timestamp: %v", lastErr)
}

func (t *TaskNoteTool) createNote(ctx context.Context, taskID string, noteType int, content, authorName, authorID string) (tools.ToolResult, error) {
	if noteType < 1 || noteType > 4 {
		return tools.ToolResult{Content: "type must be 1 (internal), 2 (customer), 3 (agent), or 4 (system)", IsError: true}, nil
	}

	authorName = strings.TrimSpace(authorName)
	authorID = strings.TrimSpace(authorID)

	note, err := t.mgr.CreateTaskNote(ctx, taskID, database.TaskNoteType(noteType), content, authorName, authorID)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error creating note on task %s: %v", taskID, err), IsError: true}, nil
	}

	typeLabels := map[int]string{1: "internal note", 2: "customer response", 3: "agent action", 4: "system event"}

	resultMap := map[string]any{
		"id":      note.ID,
		"task_id": note.TaskID,
		"type":    typeLabels[noteType],
		"action":  "created",
	}
	if note.AuthorName != "" {
		resultMap["author_name"] = note.AuthorName
	}
	if note.AuthorID != "" {
		resultMap["author_id"] = note.AuthorID
	}
	resultJSON, _ := json.Marshal(resultMap)
	return tools.ToolResult{
		Content:  fmt.Sprintf("Note added to task %s (%s):\n%s", taskID, typeLabels[noteType], string(resultJSON)),
		Metadata: map[string]any{"note_id": note.ID, "task_id": taskID, "action": "created"},
	}, nil
}

func (t *TaskNoteTool) updateNote(ctx context.Context, noteID string, taskID string, content string) (tools.ToolResult, error) {
	existing, err := t.mgr.GetTaskNote(ctx, noteID)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Note not found (id=%s): %v", noteID, err), IsError: true}, nil
	}

	if existing.TaskID != taskID {
		return tools.ToolResult{Content: fmt.Sprintf("Note %s does not belong to task %s", noteID, taskID), IsError: true}, nil
	}

	if err := t.mgr.UpdateTaskNote(ctx, noteID, content); err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Error updating note %s: %v", noteID, err), IsError: true}, nil
	}

	resultMap := map[string]any{
		"id":      noteID,
		"task_id": taskID,
		"action":  "updated",
	}
	resultJSON, _ := json.Marshal(resultMap)
	return tools.ToolResult{
		Content:  fmt.Sprintf("Note updated:\n%s", string(resultJSON)),
		Metadata: map[string]any{"note_id": noteID, "task_id": taskID, "action": "updated"},
	}, nil
}
