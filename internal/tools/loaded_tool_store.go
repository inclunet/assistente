package tools

import (
	"sort"
	"strings"
	"sync"
)

type LoadedToolState string

const (
	LoadedToolStatePreloaded      LoadedToolState = "preloaded"
	LoadedToolStateOnDemand       LoadedToolState = "loaded_on_demand"
	LoadedToolStateControlPlane   LoadedToolState = "control_plane"
	LoadedToolRejectDisabled      string          = "disabled_by_profile"
	LoadedToolRejectUnavailable   string          = "unavailable"
	LoadedToolRejectUnknown       string          = "unknown_tool"
	LoadedToolRejectAlreadyLoaded string          = "already_loaded"
	LoadedToolRejectPreloaded     string          = "preloaded"
	LoadedToolRejectControlPlane  string          = "control_plane"
)

type LoadedToolRecord struct {
	Name  string          `json:"name"`
	State LoadedToolState `json:"state"`
}

type LoadedToolChange struct {
	Name   string `json:"name"`
	Reason string `json:"reason,omitempty"`
}

type loadedToolConversationState struct {
	profileSlug string
	loaded      map[string]struct{}
	// recent mantém uma LRU pequena, mais recente primeiro. O estado compartilha
	// o mesmo ciclo de vida in-memory por conversa das tools carregadas.
	recent []string
	// autoSearchAttempted garante uma única tentativa no ciclo de vida runtime
	// da conversa, mesmo quando a janela de contexto omite turnos antigos.
	autoSearchAttempted bool
}

// LoadedToolStore mantém, em memória, as tools carregadas sob demanda por
// conversa/perfil. O estado é naturalmente descartado no restart do app.
type LoadedToolStore struct {
	mu             sync.Mutex
	byConversation map[string]*loadedToolConversationState
}

func NewLoadedToolStore() *LoadedToolStore {
	return &LoadedToolStore{byConversation: map[string]*loadedToolConversationState{}}
}

func (s *LoadedToolStore) Loaded(conversationID, profileSlug string, visible []string) []string {
	if s == nil || strings.TrimSpace(conversationID) == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.ensureLocked(conversationID, profileSlug)
	if len(state.loaded) == 0 {
		return nil
	}
	visibleSet, constrained := nameSet(visible)
	names := make([]string, 0, len(state.loaded))
	for name := range state.loaded {
		if constrained {
			if _, ok := visibleSet[name]; !ok {
				delete(state.loaded, name)
				continue
			}
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

const maxRecentToolsPerConversation = 64

func (s *LoadedToolStore) ClaimAutoSearch(conversationID, profileSlug string) bool {
	if s == nil || strings.TrimSpace(conversationID) == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.ensureLocked(conversationID, profileSlug)
	if state.autoSearchAttempted {
		return false
	}
	state.autoSearchAttempted = true
	return true
}

func (s *LoadedToolStore) RecordUsage(conversationID, profileSlug string, names ...string) {
	if s == nil || strings.TrimSpace(conversationID) == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.ensureLocked(conversationID, profileSlug)
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" || name == ToolCatalogName {
			continue
		}
		next := make([]string, 0, len(state.recent)+1)
		next = append(next, name)
		for _, existing := range state.recent {
			if existing != name {
				next = append(next, existing)
			}
		}
		if len(next) > maxRecentToolsPerConversation {
			next = next[:maxRecentToolsPerConversation]
		}
		state.recent = next
	}
}

func (s *LoadedToolStore) RecentNames(conversationID, profileSlug string) []string {
	if s == nil || strings.TrimSpace(conversationID) == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.ensureLocked(conversationID, profileSlug)
	return append([]string(nil), state.recent...)
}

func (s *LoadedToolStore) Load(conversationID, profileSlug string, names []string, visible []string, preloaded []string, controlPlane []string) (loaded, rejected []LoadedToolChange) {
	if s == nil || strings.TrimSpace(conversationID) == "" {
		return nil, rejectAll(names, LoadedToolRejectUnavailable)
	}
	visibleSet, constrained := nameSet(visible)
	preloadedSet, _ := nameSet(preloaded)
	controlPlaneSet, _ := nameSet(controlPlane)

	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.ensureLocked(conversationID, profileSlug)
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		if constrained {
			if _, ok := visibleSet[name]; !ok {
				rejected = append(rejected, LoadedToolChange{Name: name, Reason: LoadedToolRejectDisabled})
				continue
			}
		}
		if _, ok := controlPlaneSet[name]; ok {
			rejected = append(rejected, LoadedToolChange{Name: name, Reason: LoadedToolRejectControlPlane})
			continue
		}
		if _, ok := preloadedSet[name]; ok {
			rejected = append(rejected, LoadedToolChange{Name: name, Reason: LoadedToolRejectPreloaded})
			continue
		}
		if _, ok := state.loaded[name]; ok {
			rejected = append(rejected, LoadedToolChange{Name: name, Reason: LoadedToolRejectAlreadyLoaded})
			continue
		}
		state.loaded[name] = struct{}{}
		loaded = append(loaded, LoadedToolChange{Name: name})
	}
	sortLoadedToolChanges(loaded)
	sortLoadedToolChanges(rejected)
	return loaded, rejected
}

func (s *LoadedToolStore) Unload(conversationID, profileSlug string, names []string, preloaded []string, controlPlane []string) (unloaded, rejected []LoadedToolChange) {
	if s == nil || strings.TrimSpace(conversationID) == "" {
		return nil, rejectAll(names, LoadedToolRejectUnavailable)
	}
	preloadedSet, _ := nameSet(preloaded)
	controlPlaneSet, _ := nameSet(controlPlane)

	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.ensureLocked(conversationID, profileSlug)
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		if _, ok := controlPlaneSet[name]; ok {
			rejected = append(rejected, LoadedToolChange{Name: name, Reason: LoadedToolRejectControlPlane})
			continue
		}
		if _, ok := preloadedSet[name]; ok {
			rejected = append(rejected, LoadedToolChange{Name: name, Reason: LoadedToolRejectPreloaded})
			continue
		}
		if _, ok := state.loaded[name]; !ok {
			rejected = append(rejected, LoadedToolChange{Name: name, Reason: LoadedToolRejectUnknown})
			continue
		}
		delete(state.loaded, name)
		unloaded = append(unloaded, LoadedToolChange{Name: name})
	}
	sortLoadedToolChanges(unloaded)
	sortLoadedToolChanges(rejected)
	return unloaded, rejected
}

func (s *LoadedToolStore) List(conversationID, profileSlug string, preloaded []string, controlPlane []string, visible []string) []LoadedToolRecord {
	loaded := s.Loaded(conversationID, profileSlug, visible)
	controlPlaneSet, _ := nameSet(controlPlane)
	records := make([]LoadedToolRecord, 0, len(preloaded)+len(loaded))
	seen := map[string]struct{}{}
	for _, name := range controlPlane {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		records = append(records, LoadedToolRecord{Name: name, State: LoadedToolStateControlPlane})
		seen[name] = struct{}{}
	}
	for _, name := range preloaded {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := controlPlaneSet[name]; ok {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		records = append(records, LoadedToolRecord{Name: name, State: LoadedToolStatePreloaded})
		seen[name] = struct{}{}
	}
	for _, name := range loaded {
		if _, ok := seen[name]; ok {
			continue
		}
		records = append(records, LoadedToolRecord{Name: name, State: LoadedToolStateOnDemand})
	}
	sort.SliceStable(records, func(i, j int) bool {
		if records[i].State != records[j].State {
			return loadedToolStateRank(records[i].State) < loadedToolStateRank(records[j].State)
		}
		return records[i].Name < records[j].Name
	})
	return records
}

func (s *LoadedToolStore) ResetConversation(conversationID string) {
	if s == nil || strings.TrimSpace(conversationID) == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.byConversation, conversationID)
}

func (s *LoadedToolStore) ensureLocked(conversationID, profileSlug string) *loadedToolConversationState {
	profileSlug = strings.TrimSpace(profileSlug)
	state, ok := s.byConversation[conversationID]
	if !ok || state.profileSlug != profileSlug {
		state = &loadedToolConversationState{profileSlug: profileSlug, loaded: map[string]struct{}{}}
		s.byConversation[conversationID] = state
	}
	return state
}

func nameSet(names []string) (map[string]struct{}, bool) {
	if names == nil {
		return nil, false
	}
	set := make(map[string]struct{}, len(names))
	for _, raw := range names {
		if name := strings.TrimSpace(raw); name != "" {
			set[name] = struct{}{}
		}
	}
	return set, true
}

func rejectAll(names []string, reason string) []LoadedToolChange {
	rejected := make([]LoadedToolChange, 0, len(names))
	for _, raw := range names {
		if name := strings.TrimSpace(raw); name != "" {
			rejected = append(rejected, LoadedToolChange{Name: name, Reason: reason})
		}
	}
	sortLoadedToolChanges(rejected)
	return rejected
}

func sortLoadedToolChanges(changes []LoadedToolChange) {
	sort.SliceStable(changes, func(i, j int) bool { return changes[i].Name < changes[j].Name })
}

func loadedToolStateRank(state LoadedToolState) int {
	switch state {
	case LoadedToolStateControlPlane:
		return 0
	case LoadedToolStatePreloaded:
		return 1
	case LoadedToolStateOnDemand:
		return 2
	default:
		return 10
	}
}
