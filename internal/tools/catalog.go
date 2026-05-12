package tools

import (
	"encoding/json"
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
