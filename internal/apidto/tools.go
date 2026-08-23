package apidto

import (
	"encoding/json"
	"time"
)

// ToolInfo é um resumo de ferramenta para listagem na borda Wails (AEP-0088).
type ToolInfo struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	SourceType  string `json:"source_type"`
	SourceLabel string `json:"source_label"`
	OptIn       bool   `json:"opt_in"`
}

// RuntimeToolCatalogFilter filtra o catálogo persistido de tools na borda Wails.
type RuntimeToolCatalogFilter struct {
	Origin             string `json:"origin,omitempty"`
	MCPServerID        string `json:"mcpServerId,omitempty"`
	Category           string `json:"category,omitempty"`
	Class              string `json:"class,omitempty"`
	Package            string `json:"package,omitempty"`
	Risk               string `json:"risk,omitempty"`
	AvailabilityStatus string `json:"availabilityStatus,omitempty"`
	IncludeUnavailable bool   `json:"includeUnavailable,omitempty"`
	Limit              int    `json:"limit,omitempty"`
	Offset             int    `json:"offset,omitempty"`
}

// RuntimeToolCatalogEntry é a entrada tipada do catálogo na borda Wails.
type RuntimeToolCatalogEntry struct {
	ID                 string          `json:"id"`
	UserID             string          `json:"userId,omitempty"`
	MCPServerID        string          `json:"mcpServerId,omitempty"`
	Name               string          `json:"name"`
	DisplayName        string          `json:"displayName"`
	Description        string          `json:"description,omitempty"`
	Origin             string          `json:"origin"`
	Category           string          `json:"category,omitempty"`
	Class              string          `json:"class,omitempty"`
	Package            string          `json:"package,omitempty"`
	Risk               string          `json:"risk,omitempty"`
	Schema             json.RawMessage `json:"schema,omitempty"`
	SchemaHash         string          `json:"schemaHash,omitempty"`
	SchemaBytes        int             `json:"schemaBytes,omitempty"`
	Tags               []string        `json:"tags,omitempty"`
	AvailabilityStatus string          `json:"availabilityStatus"`
	AvailabilityReason string          `json:"availabilityReason,omitempty"`
	LastSeenAt         *time.Time      `json:"lastSeenAt,omitempty"`
	LastAvailableAt    *time.Time      `json:"lastAvailableAt,omitempty"`
	LastUnavailableAt  *time.Time      `json:"lastUnavailableAt,omitempty"`
	LastTestedAt       *time.Time      `json:"lastTestedAt,omitempty"`
	LastTestStatus     string          `json:"lastTestStatus,omitempty"`
	LastTestError      string          `json:"lastTestError,omitempty"`
	CreatedAt          time.Time       `json:"createdAt"`
	UpdatedAt          time.Time       `json:"updatedAt"`
}
