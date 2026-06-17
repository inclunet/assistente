package memory

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"assistente/internal/database"
)

type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) List(ctx context.Context, filter Filter) (ListResult, error) {
	return s.store.List(ctx, filter)
}

func (s *Service) Get(ctx context.Context, id string) (*database.MemoryRecord, error) {
	return s.store.Get(ctx, strings.TrimSpace(id))
}

func (s *Service) Create(ctx context.Context, input RecordInput) (*database.MemoryRecord, error) {
	normalized, err := normalizeInput(input, true)
	if err != nil {
		return nil, err
	}
	return s.store.Create(ctx, normalized)
}

func (s *Service) Import(ctx context.Context, record database.MemoryRecord) (*database.MemoryRecord, error) {
	if strings.TrimSpace(record.ID) == "" {
		return nil, errors.New("id obrigatório")
	}
	normalized, err := normalizeInput(RecordInput{
		Content:    record.Content,
		Summary:    record.Summary,
		LoadPolicy: record.LoadPolicy,
		Kind:       record.Kind,
		Scope:      record.Scope,
		ScopeRef:   record.ScopeRef,
		Tags:       unmarshalTags(record.Tags),
		Importance: record.Importance,
		Confidence: record.Confidence,
		SourceType: record.SourceType,
		SourceID:   record.SourceID,
		ExpiresAt:  record.ExpiresAt,
	}, false)
	if err != nil {
		return nil, err
	}
	normalized.ID = strings.TrimSpace(record.ID)
	normalized.CreatedAt = record.CreatedAt
	normalized.LastUsedAt = record.LastUsedAt
	normalized.ArchivedFromPolicy = normalizedArchivedFromPolicy(normalized.LoadPolicy, record.ArchivedFromPolicy)
	return s.store.Upsert(ctx, normalized)
}

func (s *Service) Update(ctx context.Context, id string, input RecordInput) (*database.MemoryRecord, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("id obrigatório")
	}
	normalized, err := normalizeInput(input, false)
	if err != nil {
		return nil, err
	}
	current, err := s.store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	normalized.ArchivedFromPolicy = resolveArchivedFromPolicy(current.LoadPolicy, current.ArchivedFromPolicy, normalized.LoadPolicy)
	return s.store.Update(ctx, id, updateMap(normalized))
}

func (s *Service) Patch(ctx context.Context, id string, patch RecordPatch) (*database.MemoryRecord, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("id obrigatório")
	}
	current, err := s.store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	normalized, err := normalizePatch(patch, current)
	if err != nil {
		return nil, err
	}
	normalized.ArchivedFromPolicy = resolveArchivedFromPolicy(current.LoadPolicy, current.ArchivedFromPolicy, normalized.LoadPolicy)
	return s.store.Update(ctx, id, updateMap(normalized))
}

func updateMap(record *database.MemoryRecord) map[string]any {
	return map[string]any{
		"content":              record.Content,
		"summary":              record.Summary,
		"load_policy":          record.LoadPolicy,
		"archived_from_policy": record.ArchivedFromPolicy,
		"kind":                 record.Kind,
		"scope":                record.Scope,
		"scope_ref":            record.ScopeRef,
		"tags":                 record.Tags,
		"importance":           record.Importance,
		"confidence":           record.Confidence,
		"source_type":          record.SourceType,
		"source_id":            record.SourceID,
		"expires_at":           record.ExpiresAt,
	}
}

func resolveArchivedFromPolicy(currentPolicy, currentArchivedFrom, nextPolicy string) string {
	if nextPolicy == LoadPolicyArchived {
		if currentPolicy != "" && currentPolicy != LoadPolicyArchived {
			return currentPolicy
		}
		if currentArchivedFrom != "" && currentArchivedFrom != LoadPolicyArchived {
			return currentArchivedFrom
		}
		return LoadPolicyRetrievable
	}
	return ""
}

func normalizedArchivedFromPolicy(loadPolicy, archivedFrom string) string {
	if loadPolicy != LoadPolicyArchived {
		return ""
	}
	policy := normalizeLoadPolicy(archivedFrom)
	if policy == "" || policy == LoadPolicyArchived {
		return LoadPolicyRetrievable
	}
	return policy
}

func (s *Service) Archive(ctx context.Context, id string) (*database.MemoryRecord, error) {
	id = strings.TrimSpace(id)
	current, err := s.store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	archivedFrom := current.LoadPolicy
	if archivedFrom == "" || archivedFrom == LoadPolicyArchived {
		archivedFrom = LoadPolicyRetrievable
	}
	return s.store.Update(ctx, id, map[string]any{
		"load_policy":          LoadPolicyArchived,
		"archived_from_policy": archivedFrom,
	})
}

func (s *Service) Unarchive(ctx context.Context, id string, loadPolicy string) (*database.MemoryRecord, error) {
	policy := normalizeLoadPolicy(loadPolicy)
	if policy == "" || policy == LoadPolicyArchived {
		current, err := s.store.Get(ctx, strings.TrimSpace(id))
		if err != nil {
			return nil, err
		}
		policy = normalizeLoadPolicy(current.ArchivedFromPolicy)
		if policy == "" || policy == LoadPolicyArchived {
			policy = LoadPolicyRetrievable
		}
	}
	return s.store.Update(ctx, strings.TrimSpace(id), map[string]any{
		"load_policy":          policy,
		"archived_from_policy": "",
	})
}

func (s *Service) Delete(ctx context.Context, id string) error {
	return s.store.Delete(ctx, strings.TrimSpace(id))
}

func (s *Service) PolicySummary(ctx context.Context) (PolicySummary, error) {
	return s.store.PolicySummary(ctx)
}

func normalizeInput(input RecordInput, creating bool) (*database.MemoryRecord, error) {
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return nil, errors.New("content obrigatório")
	}
	policy := normalizeLoadPolicy(input.LoadPolicy)
	if policy == "" {
		policy = LoadPolicyRetrievable
	}
	kind := normalizeKind(input.Kind)
	if kind == "" {
		kind = database.MemoryKindHistoricalNote
	}
	scope := normalizeScope(input.Scope)
	if scope == "" {
		scope = database.MemoryScopeUser
	}
	scopeRef := strings.TrimSpace(input.ScopeRef)
	if requiresScopeRef(scope) && scopeRef == "" {
		return nil, errors.New("scopeRef obrigatório para escopos conversation, workspace e project")
	}
	importance := input.Importance
	if importance == 0 && creating {
		importance = 3
	}
	if importance < 1 {
		importance = 1
	}
	if importance > 5 {
		importance = 5
	}
	confidence := input.Confidence
	if confidence == 0 && creating {
		confidence = 80
	}
	if confidence < 0 {
		confidence = 0
	}
	if confidence > 100 {
		confidence = 100
	}
	tags, err := marshalTags(input.Tags)
	if err != nil {
		return nil, err
	}
	return &database.MemoryRecord{
		Content:    content,
		Summary:    strings.TrimSpace(input.Summary),
		LoadPolicy: policy,
		Kind:       kind,
		Scope:      scope,
		ScopeRef:   scopeRef,
		Tags:       tags,
		Importance: importance,
		Confidence: confidence,
		SourceType: strings.TrimSpace(input.SourceType),
		SourceID:   strings.TrimSpace(input.SourceID),
		ExpiresAt:  input.ExpiresAt,
	}, nil
}

func normalizePatch(patch RecordPatch, current *database.MemoryRecord) (*database.MemoryRecord, error) {
	if current == nil {
		return nil, errors.New("registro de memória não encontrado")
	}
	merged := RecordInput{
		Content:    current.Content,
		Summary:    current.Summary,
		LoadPolicy: current.LoadPolicy,
		Kind:       current.Kind,
		Scope:      current.Scope,
		ScopeRef:   current.ScopeRef,
		Tags:       unmarshalTags(current.Tags),
		Importance: current.Importance,
		Confidence: current.Confidence,
		SourceType: current.SourceType,
		SourceID:   current.SourceID,
		ExpiresAt:  current.ExpiresAt,
	}
	applyStringPatch(patch.Content, &merged.Content)
	applyStringPatch(patch.Summary, &merged.Summary)
	applyStringPatch(patch.LoadPolicy, &merged.LoadPolicy)
	applyStringPatch(patch.Kind, &merged.Kind)
	applyStringPatch(patch.Scope, &merged.Scope)
	applyStringPatch(patch.ScopeRef, &merged.ScopeRef)
	applyStringPatch(patch.SourceType, &merged.SourceType)
	applyStringPatch(patch.SourceID, &merged.SourceID)
	if patch.Tags != nil {
		merged.Tags = *patch.Tags
	}
	if patch.Importance != nil {
		merged.Importance = *patch.Importance
	}
	if patch.Confidence != nil {
		merged.Confidence = *patch.Confidence
	}
	if patch.ClearExpiresAt {
		merged.ExpiresAt = nil
	} else if patch.ExpiresAt != nil {
		merged.ExpiresAt = patch.ExpiresAt
	}
	return normalizeInput(merged, false)
}

func applyStringPatch(value *string, target *string) {
	if value != nil {
		*target = *value
	}
}

func normalizeLoadPolicy(value string) string {
	switch strings.TrimSpace(value) {
	case LoadPolicyCore, LoadPolicyPinned, LoadPolicyAuto, LoadPolicyRetrievable, LoadPolicyArchived:
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

func normalizeKind(value string) string {
	switch strings.TrimSpace(value) {
	case database.MemoryKindUserPreference, database.MemoryKindIdentity, database.MemoryKindProjectFact,
		database.MemoryKindDecision, database.MemoryKindConvention, database.MemoryKindHistoricalNote,
		database.MemoryKindResolvedIssue:
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

func normalizeScope(value string) string {
	switch strings.TrimSpace(value) {
	case database.MemoryScopeGlobal, database.MemoryScopeUser, database.MemoryScopeWorkspace,
		database.MemoryScopeProject, database.MemoryScopeConversation:
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

func requiresScopeRef(scope string) bool {
	switch scope {
	case database.MemoryScopeConversation, database.MemoryScopeWorkspace, database.MemoryScopeProject:
		return true
	default:
		return false
	}
}

func marshalTags(tags []string) (string, error) {
	cleaned := cleanStrings(tags)
	sort.Strings(cleaned)
	if len(cleaned) == 0 {
		return "", nil
	}
	data, err := json.Marshal(cleaned)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func unmarshalTags(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var tags []string
	if err := json.Unmarshal([]byte(raw), &tags); err != nil {
		return nil
	}
	return tags
}

func ParseExpiresAt(raw string) (*time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(raw))
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}
