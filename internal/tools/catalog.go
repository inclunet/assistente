package tools

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"
)

const (
	ToolOriginBuiltin   = "builtin"
	ToolOriginMCPBridge = "mcp_bridge"
	ToolOriginMCPNative = "mcp_native"

	ToolAvailabilityAvailable   = "available"
	ToolAvailabilityUnavailable = "unavailable"

	ToolTestStatusOK      = "ok"
	ToolTestStatusError   = "error"
	ToolTestStatusBlocked = "blocked"
)

// ToolCatalogEntry descreve uma capability persistida no catálogo de tools.
// O registry runtime continua sendo a fonte executável; o catálogo é índice/metadata.
type ToolCatalogEntry struct {
	ID          string `json:"id"`
	UserID      string `json:"user_id,omitempty"`
	MCPServerID string `json:"mcp_server_id,omitempty"`

	Name        string          `json:"name"`
	DisplayName string          `json:"display_name"`
	Description string          `json:"description,omitempty"`
	Origin      string          `json:"origin"`
	Category    string          `json:"category,omitempty"`
	Class       string          `json:"class,omitempty"`
	Package     string          `json:"package,omitempty"`
	Risk        string          `json:"risk,omitempty"`
	Schema      json.RawMessage `json:"schema,omitempty"`
	SchemaHash  string          `json:"schema_hash,omitempty"`
	SchemaBytes int             `json:"schema_bytes,omitempty"`
	Tags        []string        `json:"tags,omitempty"`

	AvailabilityStatus string     `json:"availability_status"`
	AvailabilityReason string     `json:"availability_reason,omitempty"`
	LastSeenAt         *time.Time `json:"last_seen_at,omitempty"`
	LastAvailableAt    *time.Time `json:"last_available_at,omitempty"`
	LastUnavailableAt  *time.Time `json:"last_unavailable_at,omitempty"`
	LastTestedAt       *time.Time `json:"last_tested_at,omitempty"`
	LastTestStatus     string     `json:"last_test_status,omitempty"`
	LastTestError      string     `json:"last_test_error,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type ToolCatalogFilter struct {
	Origin             string `json:"origin,omitempty"`
	MCPServerID        string `json:"mcp_server_id,omitempty"`
	Category           string `json:"category,omitempty"`
	Class              string `json:"class,omitempty"`
	Package            string `json:"package,omitempty"`
	Risk               string `json:"risk,omitempty"`
	AvailabilityStatus string `json:"availability_status,omitempty"`
	IncludeUnavailable bool   `json:"include_unavailable,omitempty"`
	Limit              int    `json:"limit,omitempty"`
}

func CatalogEntryFromTool(tool Tool) ToolCatalogEntry {
	if tool == nil {
		return ToolCatalogEntry{}
	}
	name := tool.Name()
	schema := tool.Parameters()
	metadata := builtinToolCatalogMetadata(name)
	return ToolCatalogEntry{
		Name:               name,
		DisplayName:        name,
		Description:        tool.Description(),
		Origin:             ToolOriginBuiltin,
		Category:           metadata.Category,
		Class:              metadata.Class,
		Package:            metadata.Package,
		Risk:               metadata.Risk,
		Schema:             schema,
		SchemaHash:         SchemaHash(schema),
		SchemaBytes:        len(schema),
		AvailabilityStatus: ToolAvailabilityAvailable,
	}
}

func SchemaHash(schema json.RawMessage) string {
	if len(schema) == 0 {
		return ""
	}
	sum := sha256.Sum256(schema)
	return fmt.Sprintf("%x", sum[:])
}

type builtinToolMetadata struct {
	Category string
	Class    string
	Package  string
	Risk     string
}

var builtinToolMetadataByName = map[string]builtinToolMetadata{
	"read_file":            {Category: "filesystem", Class: "read_context", Package: "coding_readonly", Risk: "read"},
	"list_directory":       {Category: "filesystem", Class: "read_context", Package: "coding_readonly", Risk: "read"},
	"search_files":         {Category: "filesystem", Class: "read_context", Package: "coding_readonly", Risk: "read"},
	"grep_search":          {Category: "filesystem", Class: "read_context", Package: "coding_readonly", Risk: "read"},
	"write_file":           {Category: "filesystem", Class: "edit_files", Package: "coding_edit", Risk: "write"},
	"edit_file":            {Category: "filesystem", Class: "edit_files", Package: "coding_edit", Risk: "write"},
	"move_file":            {Category: "filesystem", Class: "edit_files", Package: "coding_edit", Risk: "write"},
	"copy_file":            {Category: "filesystem", Class: "edit_files", Package: "coding_edit", Risk: "write"},
	"delete_file":          {Category: "filesystem", Class: "edit_files", Package: "coding_edit", Risk: "destructive"},
	"make_directory":       {Category: "filesystem", Class: "edit_files", Package: "coding_edit", Risk: "write"},
	"run_command":          {Category: "shell", Class: "run_commands", Package: "coding_edit", Risk: "shell"},
	"web_search":           {Category: "web", Class: "web_lookup", Package: "web", Risk: "network"},
	"web_fetch":            {Category: "web", Class: "web_lookup", Package: "web", Risk: "network"},
	"http_request":         {Category: "http", Class: "http_api", Package: "web", Risk: "network"},
	"search_conversations": {Category: "history", Class: "read_context", Package: "history", Risk: "read"},
	"collect_responses":    {Category: "questionnaire", Class: "app_tool", Package: "basic", Risk: "read"},
	"task_list":            {Category: "tasklist", Class: "task_management", Package: "tasks", Risk: "write"},
	"task":                 {Category: "tasklist", Class: "task_management", Package: "tasks", Risk: "write"},
	"task_note":            {Category: "tasklist", Class: "task_management", Package: "tasks", Risk: "write"},
	"job":                  {Category: "jobs", Class: "automation_management", Package: "jobs", Risk: "write"},
	"job_pipeline":         {Category: "jobs", Class: "automation_management", Package: "jobs", Risk: "write"},
	"open_deep_link":       {Category: "app", Class: "app_tool", Package: "basic", Risk: "read"},
	"subagent":             {Category: "agents", Class: "agent_delegation", Package: "agents", Risk: "write"},
}

func builtinToolCatalogMetadata(name string) builtinToolMetadata {
	if metadata, ok := builtinToolMetadataByName[name]; ok {
		return metadata
	}
	return builtinToolMetadata{Category: "app", Class: "app_tool", Package: "basic", Risk: "read"}
}
