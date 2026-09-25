package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"unicode/utf8"

	"assistente/internal/commandjson"
)

var (
	ErrCommandSnapshotNilManager           = errors.New("workspace command snapshot: manager nil")
	ErrCommandSnapshotNilContext           = errors.New("workspace command snapshot: context nil")
	ErrCommandSnapshotNilCallback          = errors.New("workspace command snapshot: callback nil")
	ErrCommandSnapshotUninitialized        = errors.New("workspace command snapshot: manager não inicializado")
	ErrCommandSnapshotActiveTabUnavailable = errors.New("workspace command snapshot: aba ativa indisponível")
	ErrCommandSnapshotInvalidData          = errors.New("workspace command snapshot: dados inválidos")
	ErrCommandSnapshotEpochExhausted       = errors.New("workspace command snapshot: epoch esgotado")
)

const maxCommandSnapshotIdentifierLength = 4096

// CommandSnapshot é a projeção autoritativa mínima do workspace usada por
// comandos. Ele não é SurfaceContext: não contém surfaceId nem
// surfaceSnapshotVersion, que só podem vir do adapter da surface real.
//
// O Manager não possui usuário. A camada App deve vincular este snapshot ao
// principal da sessão ativa ao montar o contexto do comando; não se deve
// inferir ou acrescentar essa identidade a partir do workspace.
type CommandSnapshot struct {
	WorkspaceID      string             `json:"workspace_id"`
	ActiveTabID      string             `json:"active_tab_id"`
	Tab              CommandTabSnapshot `json:"tab"`
	WorkspaceProfile string             `json:"workspace_profile,omitempty"`
	StateVersion     string             `json:"state_version"`
	Fingerprint      string             `json:"fingerprint"`
	Version          string             `json:"version"`
}

// CommandTabSnapshot contém somente a identidade e a configuração semântica
// da aba ativa. StateVersion resume o State completo sem expô-lo.
type CommandTabSnapshot struct {
	ID                  string  `json:"id"`
	Type                TabType `json:"type"`
	ConversationID      string  `json:"conversation_id,omitempty"`
	ProfileOverrideSlug string  `json:"profile_override_slug,omitempty"`
}

type commandSnapshotSemantic struct {
	WorkspaceID      string               `json:"workspace_id"`
	ActiveTabID      string               `json:"active_tab_id"`
	WorkspaceProfile string               `json:"workspace_profile,omitempty"`
	Tabs             []commandTabSemantic `json:"tabs"`
}

type commandTabSemantic struct {
	ID                  string  `json:"id"`
	Type                TabType `json:"type"`
	ConversationID      string  `json:"conversation_id,omitempty"`
	ProfileOverrideSlug string  `json:"profile_override_slug,omitempty"`
	StateVersion        string  `json:"state_version"`
}

// CommandSnapshot captura, sob o read lock do Manager, o workspace e a aba
// ativa em uma projeção independente. O fingerprint é SHA-256 do documento
// JCS dos campos semânticos da projeção; LastUsed e campos de apresentação não
// participam. State é canonicalizado e resumido, mas nunca retornado.
func (m *Manager) CommandSnapshot() (CommandSnapshot, error) {
	if m == nil {
		return CommandSnapshot{}, ErrCommandSnapshotNilManager
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.commandSnapshotLocked()
}

// WithCommandSnapshot mantém o read lock durante toda a execução de fn para
// que a projeção e o estado que ela autoriza permaneçam estáveis. fn não deve
// reler o Manager nem chamar mutadores enquanto o callback estiver ativo.
func (m *Manager) WithCommandSnapshot(ctx context.Context, fn func(CommandSnapshot) error) error {
	if m == nil {
		return ErrCommandSnapshotNilManager
	}
	if ctx == nil {
		return ErrCommandSnapshotNilContext
	}
	if fn == nil {
		return ErrCommandSnapshotNilCallback
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	snapshot, err := m.commandSnapshotLocked()
	if err != nil {
		return err
	}
	return fn(snapshot)
}

func (m *Manager) commandSnapshotLocked() (CommandSnapshot, error) {
	if m.commandEpoch == math.MaxUint64 {
		return CommandSnapshot{}, ErrCommandSnapshotEpochExhausted
	}

	if m.active == nil {
		return CommandSnapshot{}, ErrCommandSnapshotUninitialized
	}

	activeTabID := m.active.Tabs.Active
	if !canonicalSnapshotIdentifier(m.active.ID) {
		return CommandSnapshot{}, ErrCommandSnapshotInvalidData
	}
	if activeTabID == "" {
		return CommandSnapshot{}, ErrCommandSnapshotActiveTabUnavailable
	}
	if !canonicalSnapshotIdentifier(activeTabID) {
		return CommandSnapshot{}, ErrCommandSnapshotInvalidData
	}

	activeTabIndex := -1
	semanticTabs := make([]commandTabSemantic, 0, len(m.active.Tabs.Items))
	var activeTab CommandTabSnapshot
	var activeStateVersion string
	seenTabIDs := make(map[string]struct{}, len(m.active.Tabs.Items))
	for i := range m.active.Tabs.Items {
		current := &m.active.Tabs.Items[i]
		if !canonicalSnapshotIdentifier(current.ID) || !validCommandTabType(current.Type) {
			return CommandSnapshot{}, ErrCommandSnapshotInvalidData
		}
		if _, exists := seenTabIDs[current.ID]; exists {
			return CommandSnapshot{}, ErrCommandSnapshotInvalidData
		}
		seenTabIDs[current.ID] = struct{}{}
		if current.ConversationID != "" && !canonicalSnapshotIdentifier(current.ConversationID) {
			return CommandSnapshot{}, ErrCommandSnapshotInvalidData
		}
		profileOverrideSlug, err := commandProfileOverrideSlug(current)
		if err != nil {
			return CommandSnapshot{}, err
		}
		stateVersion, err := fingerprintState(current.State)
		if err != nil {
			return CommandSnapshot{}, err
		}
		semanticTabs = append(semanticTabs, commandTabSemantic{
			ID:                  current.ID,
			Type:                current.Type,
			ConversationID:      current.ConversationID,
			ProfileOverrideSlug: profileOverrideSlug,
			StateVersion:        stateVersion,
		})
		if current.ID == activeTabID {
			activeTabIndex = i
			activeTab = CommandTabSnapshot{
				ID:                  current.ID,
				Type:                current.Type,
				ConversationID:      current.ConversationID,
				ProfileOverrideSlug: profileOverrideSlug,
			}
			activeStateVersion = stateVersion
		}
	}
	if activeTabIndex < 0 {
		return CommandSnapshot{}, ErrCommandSnapshotActiveTabUnavailable
	}

	semantic := commandSnapshotSemantic{
		WorkspaceID:      m.active.ID,
		ActiveTabID:      activeTabID,
		WorkspaceProfile: m.active.Profile,
		Tabs:             semanticTabs,
	}

	canonical, err := commandjson.Marshal(semantic)
	if err != nil {
		return CommandSnapshot{}, fmt.Errorf("workspace command snapshot: semantic fields inválidos: %w", err)
	}
	fingerprint := sha256.Sum256(canonical)
	fingerprintText := hex.EncodeToString(fingerprint[:])

	return CommandSnapshot{
		WorkspaceID:      semantic.WorkspaceID,
		ActiveTabID:      semantic.ActiveTabID,
		Tab:              activeTab,
		WorkspaceProfile: semantic.WorkspaceProfile,
		StateVersion:     activeStateVersion,
		Fingerprint:      fingerprintText,
		Version:          fmt.Sprintf("command-snapshot:v1:%s:%d", fingerprintText, m.commandEpoch),
	}, nil
}

// commandMutationEpochLocked avança a época sem wrap. Deve ser chamado por
// mutadores sob m.mu; o reader CommandSnapshot apenas observa o valor.
func (m *Manager) commandMutationEpochLocked() {
	if m.commandEpoch < math.MaxUint64 {
		m.commandEpoch++
	}
}

func (m *Manager) commandMutationFingerprintLocked() (string, bool) {
	snapshot, err := m.commandSnapshotLocked()
	if err != nil {
		return "", false
	}
	return snapshot.Fingerprint, true
}

func commandTabSemanticallyChanged(before, after *Tab) bool {
	if before == nil || after == nil {
		return before != after
	}
	if before.ConversationID != after.ConversationID {
		return true
	}
	if _, err := commandProfileOverrideSlug(before); err != nil {
		return true
	}
	if _, err := commandProfileOverrideSlug(after); err != nil {
		return true
	}
	beforeSlug, beforeSlugOK := before.ProfileOverride["slug"]
	afterSlug, afterSlugOK := after.ProfileOverride["slug"]
	if !beforeSlugOK || !afterSlugOK {
		if beforeSlugOK != afterSlugOK {
			return true
		}
	} else if !reflect.DeepEqual(beforeSlug, afterSlug) {
		return true
	}
	beforeState, beforeErr := fingerprintState(before.State)
	afterState, afterErr := fingerprintState(after.State)
	if beforeErr != nil || afterErr != nil {
		// Estado inválido torna a projeção indisponível; falha fechada, nunca
		// um falso no-op e nunca uma comparação dinâmica potencialmente insegura.
		return true
	}
	return beforeState != afterState
}

func commandProfileOverrideSlug(tab *Tab) (string, error) {
	if tab == nil || tab.ProfileOverride == nil {
		return "", nil
	}
	raw, exists := tab.ProfileOverride["slug"]
	if !exists {
		return "", nil
	}
	slug, ok := raw.(string)
	if !ok || !canonicalSnapshotIdentifier(slug) {
		return "", fmt.Errorf("%w: profile override slug", ErrCommandSnapshotInvalidData)
	}
	return slug, nil
}

func canonicalSnapshotIdentifier(value string) bool {
	if value == "" || len(value) > maxCommandSnapshotIdentifierLength || !utf8.ValidString(value) || strings.TrimSpace(value) != value || strings.ContainsRune(value, '\x00') {
		return false
	}
	return true
}

func validCommandTabType(tabType TabType) bool {
	switch tabType {
	case TabTypeChat, TabTypeEditor, TabTypeTerminal, TabTypeTasklist:
		return true
	default:
		return false
	}
}

func fingerprintState(state map[string]any) (string, error) {
	if state == nil {
		state = map[string]any{}
	}

	canonical, err := commandjson.Marshal(state)
	if err != nil {
		return "", fmt.Errorf("workspace command snapshot: state inválido: %w", err)
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}
