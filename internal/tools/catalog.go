package tools

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
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
}

func CatalogEntryFromTool(tool Tool) ToolCatalogEntry {
	if tool == nil {
		return ToolCatalogEntry{}
	}
	name := tool.Name()
	schema := tool.Parameters()
	return ToolCatalogEntry{
		Name:               name,
		DisplayName:        name,
		Description:        tool.Description(),
		Origin:             ToolOriginBuiltin,
		Category:           inferBuiltinCategory(name),
		Class:              inferBuiltinClass(name),
		Package:            inferBuiltinPackage(name),
		Risk:               inferBuiltinRisk(name),
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

func inferBuiltinCategory(name string) string {
	switch {
	case strings.Contains(name, "file"), strings.Contains(name, "directory"), strings.Contains(name, "grep"), strings.Contains(name, "search"):
		return "filesystem"
	case strings.Contains(name, "web"):
		return "web"
	case strings.Contains(name, "http"):
		return "http"
	case strings.Contains(name, "command"):
		return "shell"
	case strings.Contains(name, "task"):
		return "tasklist"
	default:
		return "app"
	}
}

func inferBuiltinClass(name string) string {
	switch name {
	case "read_file", "list_directory", "search_files", "grep_search", "search_conversations":
		return "read_context"
	case "write_file", "edit_file", "move_file", "copy_file", "delete_file", "make_directory":
		return "edit_files"
	case "run_command":
		return "run_commands"
	case "web_search", "web_fetch":
		return "web_lookup"
	case "http_request":
		return "http_api"
	case "task", "task_list", "task_note":
		return "task_management"
	default:
		return "app_tool"
	}
}

func inferBuiltinPackage(name string) string {
	switch inferBuiltinClass(name) {
	case "read_context":
		return "coding_readonly"
	case "edit_files", "run_commands":
		return "coding_edit"
	case "web_lookup", "http_api":
		return "web"
	case "task_management":
		return "tasks"
	default:
		return "basic"
	}
}

func inferBuiltinRisk(name string) string {
	switch name {
	case "write_file", "edit_file", "move_file", "copy_file", "make_directory", "task", "task_list", "task_note":
		return "write"
	case "delete_file":
		return "destructive"
	case "run_command":
		return "shell"
	case "http_request", "web_fetch", "web_search":
		return "network"
	default:
		return "read"
	}
}
