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
	ID          string
	UserID      string
	MCPServerID string

	Name        string
	DisplayName string
	Description string
	Origin      string
	Category    string
	Class       string
	Package     string
	Risk        string
	Schema      json.RawMessage
	SchemaHash  string
	SchemaBytes int
	Tags        []string

	AvailabilityStatus string
	AvailabilityReason string
	LastSeenAt         *time.Time
	LastAvailableAt    *time.Time
	LastUnavailableAt  *time.Time
	LastTestedAt       *time.Time
	LastTestStatus     string
	LastTestError      string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type ToolCatalogFilter struct {
	Origin             string
	MCPServerID        string
	Category           string
	Class              string
	Package            string
	Risk               string
	AvailabilityStatus string
	IncludeUnavailable bool
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
