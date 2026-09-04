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
	NameIn             []string `json:"name_in,omitempty"`
	Origin             string   `json:"origin,omitempty"`
	MCPServerID        string   `json:"mcp_server_id,omitempty"`
	Category           string   `json:"category,omitempty"`
	Class              string   `json:"class,omitempty"`
	Package            string   `json:"package,omitempty"`
	Risk               string   `json:"risk,omitempty"`
	AvailabilityStatus string   `json:"availability_status,omitempty"`
	IncludeUnavailable bool     `json:"include_unavailable,omitempty"`
	Limit              int      `json:"limit,omitempty"`
	Offset             int      `json:"offset,omitempty"`
}

func CatalogEntryFromTool(tool Tool) ToolCatalogEntry {
	if tool == nil {
		return ToolCatalogEntry{}
	}
	name := tool.Name()
	schema := tool.Parameters()
	metadata := CatalogMetadataForTool(tool)
	return ToolCatalogEntry{
		Name:               name,
		DisplayName:        name,
		Description:        tool.Description(),
		Origin:             ToolOriginBuiltin,
		Category:           metadata.Category,
		Class:              metadata.Class,
		Package:            metadata.Package,
		Risk:               metadata.Risk,
		Tags:               metadata.Tags,
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

// CatalogMetadata agrupa os metadados de catálogo declarados por uma builtin
// junto da sua própria definição (AEP-0077, Fase 1). Substitui o antigo mapa
// central name→metadata: agora cada tool é a fonte autoritativa dos seus
// próprios metadados, evitando uma fonte paralela fácil de esquecer.
type CatalogMetadata struct {
	Category string
	Class    string
	Package  string
	Risk     string
	Tags     []string
}

// CatalogMetadataProvider é implementada pelas builtins que declaram seus
// próprios metadados de catálogo. Tools que não a implementam (ex.: as pontes
// MCP, cujos metadados são montados em internal/mcp) recebem os metadados
// padrão de builtin via DefaultBuiltinCatalogMetadata.
type CatalogMetadataProvider interface {
	CatalogMetadata() CatalogMetadata
}

// DefaultBuiltinCatalogMetadata é o fallback aplicado a builtins que não
// declaram metadados próprios, preservando o comportamento histórico do mapa
// central para tools genéricas de aplicação.
func DefaultBuiltinCatalogMetadata() CatalogMetadata {
	return CatalogMetadata{Category: "app", Class: "app_tool", Package: "basic", Risk: "read"}
}

// CatalogMetadataForTool devolve os metadados declarados pela tool. É a API
// canônica para consumidores de política que precisam selecionar por pacote
// sem duplicar os defaults do catálogo.
func CatalogMetadataForTool(tool Tool) CatalogMetadata {
	if provider, ok := tool.(CatalogMetadataProvider); ok {
		return provider.CatalogMetadata()
	}
	return DefaultBuiltinCatalogMetadata()
}
