package workspace

import (
	"context"
	"errors"
	"strings"
)

var ErrEditorTransferTarget = errors.New("workspace editor transfer target invalid")

// EditorTransferPlan reserves identity only. No content or tab is created here.
type EditorTransferPlan struct {
	TabID    string
	DraftID  string
	FilePath string
	Create   bool
}

func editorTransferPlan(tab *Tab) (EditorTransferPlan, error) {
	if tab == nil || tab.Type != TabTypeEditor {
		return EditorTransferPlan{}, ErrEditorTransferTarget
	}
	if readOnly, _ := tab.State["readOnly"].(bool); readOnly {
		return EditorTransferPlan{}, ErrEditorTransferTarget
	}
	if mode, _ := tab.State["displayMode"].(string); mode == "view" {
		return EditorTransferPlan{}, ErrEditorTransferTarget
	}
	plan := EditorTransferPlan{TabID: tab.ID}
	if value, exists := tab.State["draftId"]; exists && value != nil {
		var ok bool
		plan.DraftID, ok = value.(string)
		if !ok {
			return EditorTransferPlan{}, ErrEditorTransferTarget
		}
	}
	if value, exists := tab.State["filePath"]; exists && value != nil {
		var ok bool
		plan.FilePath, ok = value.(string)
		if !ok {
			return EditorTransferPlan{}, ErrEditorTransferTarget
		}
	}
	if plan.DraftID == "" && plan.FilePath == "" {
		plan.DraftID = tab.ID
	}
	return plan, nil
}

func (m *Manager) PrepareEditorTransferForCommand(ctx context.Context, expected CommandSnapshot, targetDocumentID, newTabID string) (EditorTransferPlan, error) {
	if m == nil || ctx == nil {
		return EditorTransferPlan{}, ErrEditorTransferTarget
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return EditorTransferPlan{}, err
	}
	current, err := m.commandSnapshotLocked()
	if err != nil || current != expected {
		return EditorTransferPlan{}, ErrCommandCreateTabStale
	}
	if targetDocumentID != "" {
		return editorTransferPlan(m.active.FindTab(targetDocumentID))
	}
	if !canonicalSnapshotIdentifier(newTabID) || m.active.FindTab(newTabID) != nil {
		return EditorTransferPlan{}, ErrEditorTransferTarget
	}
	return EditorTransferPlan{TabID: newTabID, DraftID: newTabID, Create: true}, nil
}

// OpenEditorTransferForCommand atomically compares the original workspace and
// applies exactly the prepared transition. The App owns idempotent replay.
func (m *Manager) OpenEditorTransferForCommand(ctx context.Context, expected CommandSnapshot, plan EditorTransferPlan, title string) (*Workspace, CommandSnapshot, error) {
	if m == nil || ctx == nil {
		return nil, CommandSnapshot{}, ErrEditorTransferTarget
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, CommandSnapshot{}, err
	}
	current, err := m.commandSnapshotLocked()
	if err != nil || current != expected {
		return nil, CommandSnapshot{}, ErrCommandCreateTabStale
	}
	if plan.Create {
		if !canonicalSnapshotIdentifier(plan.TabID) || plan.DraftID != plan.TabID || plan.FilePath != "" || m.active.FindTab(plan.TabID) != nil {
			return nil, CommandSnapshot{}, ErrEditorTransferTarget
		}
	} else {
		currentPlan, err := editorTransferPlan(m.active.FindTab(plan.TabID))
		if err != nil || currentPlan != plan {
			return nil, CommandSnapshot{}, ErrEditorTransferTarget
		}
	}
	previous, epoch := m.active, m.commandEpoch
	m.active = cloneWorkspace(previous)
	if plan.Create {
		title = strings.TrimSpace(title)
		m.active.Tabs.Items = append(m.active.Tabs.Items, Tab{ID: plan.TabID, Type: TabTypeEditor, Title: title, Position: len(m.active.Tabs.Items), State: map[string]any{"draftId": plan.DraftID}})
	} else if plan.FilePath == "" {
		tab := m.active.FindTab(plan.TabID)
		if tab.State == nil {
			tab.State = map[string]any{}
		}
		tab.State["draftId"] = plan.DraftID
	}
	m.active.Tabs.Active = plan.TabID
	m.commandMutationEpochLocked()
	destination, err := m.commandSnapshotLocked()
	if err == nil {
		err = ctx.Err()
	}
	if err == nil {
		err = m.saveWorkspaceForCommand(ctx, m.active, m.activePath)
	}
	if err != nil {
		m.active = previous
		m.commandEpoch = epoch
		return nil, CommandSnapshot{}, err
	}
	return m.cloneActiveSnapshotLocked(m.active), destination, nil
}
