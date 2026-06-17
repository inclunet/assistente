package memory

import (
	"time"

	"assistente/internal/database"
)

const (
	LoadPolicyCore        = database.MemoryLoadPolicyCore
	LoadPolicyPinned      = database.MemoryLoadPolicyPinned
	LoadPolicyAuto        = database.MemoryLoadPolicyAuto
	LoadPolicyRetrievable = database.MemoryLoadPolicyRetrievable
	LoadPolicyArchived    = database.MemoryLoadPolicyArchived
)

type Record = database.MemoryRecord

// Filter descreve filtros de listagem/busca para UI e tools.
type Filter struct {
	Query           string   `json:"query,omitempty"`
	LoadPolicies    []string `json:"loadPolicies,omitempty"`
	Kinds           []string `json:"kinds,omitempty"`
	Scopes          []string `json:"scopes,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	IncludeArchived bool     `json:"includeArchived,omitempty"`
	Limit           int      `json:"limit,omitempty"`
	Offset          int      `json:"offset,omitempty"`
}

type ListResult struct {
	Records []database.MemoryRecord `json:"records"`
	Total   int64                   `json:"total"`
}

type PromptCandidateFilter struct {
	LoadPolicies   []string
	ConversationID string
	WorkspaceID    string
	ProjectID      string
	RelevanceText  string
	Limit          int
}

type RecordInput struct {
	Content    string     `json:"content"`
	Summary    string     `json:"summary,omitempty"`
	LoadPolicy string     `json:"loadPolicy,omitempty"`
	Kind       string     `json:"kind,omitempty"`
	Scope      string     `json:"scope,omitempty"`
	ScopeRef   string     `json:"scopeRef,omitempty"`
	Tags       []string   `json:"tags,omitempty"`
	Importance int        `json:"importance,omitempty"`
	Confidence int        `json:"confidence,omitempty"`
	SourceType string     `json:"sourceType,omitempty"`
	SourceID   string     `json:"sourceId,omitempty"`
	ExpiresAt  *time.Time `json:"expiresAt,omitempty"`
}

type RecordPatch struct {
	Content        *string    `json:"content,omitempty"`
	Summary        *string    `json:"summary,omitempty"`
	LoadPolicy     *string    `json:"loadPolicy,omitempty"`
	Kind           *string    `json:"kind,omitempty"`
	Scope          *string    `json:"scope,omitempty"`
	ScopeRef       *string    `json:"scopeRef,omitempty"`
	Tags           *[]string  `json:"tags,omitempty"`
	Importance     *int       `json:"importance,omitempty"`
	Confidence     *int       `json:"confidence,omitempty"`
	SourceType     *string    `json:"sourceType,omitempty"`
	SourceID       *string    `json:"sourceId,omitempty"`
	ExpiresAt      *time.Time `json:"expiresAt,omitempty"`
	ClearExpiresAt bool       `json:"clearExpiresAt,omitempty"`
}

type PolicySummary struct {
	Core        int64 `json:"core"`
	Pinned      int64 `json:"pinned"`
	Auto        int64 `json:"auto"`
	Retrievable int64 `json:"retrievable"`
	Archived    int64 `json:"archived"`
	Total       int64 `json:"total"`
}
