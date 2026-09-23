package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

var (
	ErrCommandCreateTabNilContext      = errors.New("workspace command create tab: context nil")
	ErrCommandCreateTabStale           = errors.New("workspace command create tab: snapshot stale")
	ErrCommandCreateTabUnsupportedType = errors.New("workspace command create tab: unsupported tab type")
	ErrCommandCreateTabDuplicateID     = errors.New("workspace command create tab: duplicate tab id")
)

// TerminalSessionValidator é a fronteira mínima para validar uma sessão que
// já foi criada pelo manager de terminal. A criação e a compensação pertencem
// ao orquestrador; o workspace nunca cria ou fecha uma sessão.
type TerminalSessionValidator interface {
	Has(sessionID string) bool
}

// AddTabForCommand cria uma aba de conteúdo não terminal somente no workspace e estado
// autorizados pelo snapshot capturado pelo comando. A comparação do snapshot
// e a preparação do clone acontecem sob o mesmo lock exclusivo que protege a
// troca de workspace e as demais mutações.
func (m *Manager) AddTabForCommand(
	ctx context.Context,
	expected CommandSnapshot,
	tab Tab,
) (*Workspace, error) {
	if m == nil {
		return nil, fmt.Errorf("no workspace manager")
	}
	if ctx == nil {
		return nil, ErrCommandCreateTabNilContext
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	switch tab.Type {
	case TabTypeChat:
	case TabTypeEditor, TabTypeTasklist:
		if tab.ConversationID != "" || len(tab.State) != 0 {
			return nil, fmt.Errorf("workspace command create tab: %s tab must have empty conversation and state", tab.Type)
		}
	default:
		return nil, ErrCommandCreateTabUnsupportedType
	}
	if err := tab.Validate(); err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if m.active == nil {
		return nil, fmt.Errorf("no active workspace")
	}

	current, err := m.commandSnapshotLocked()
	if err != nil {
		return nil, err
	}
	if current.WorkspaceID != expected.WorkspaceID || current.Version != expected.Version {
		return nil, ErrCommandCreateTabStale
	}
	for _, existing := range m.active.Tabs.Items {
		if existing.ID == tab.ID {
			return nil, ErrCommandCreateTabDuplicateID
		}
	}

	previousActive := m.active
	previousEpoch := m.commandEpoch
	m.active = cloneWorkspace(previousActive)
	if err := m.addTabLocked(cloneTab(tab)); err != nil {
		m.active = previousActive
		m.commandEpoch = previousEpoch
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		m.active = previousActive
		m.commandEpoch = previousEpoch
		return nil, err
	}
	if err := m.saveWorkspaceForCommand(ctx, m.active, m.activePath); err != nil {
		m.active = previousActive
		m.commandEpoch = previousEpoch
		return nil, err
	}

	return m.cloneActiveSnapshotLocked(m.active), nil
}

// AddTerminalTabForCommand registra uma aba terminal para uma sessão já
// criada. A validação de vida ocorre antes e imediatamente antes da gravação;
// a sessão pode sair naturalmente depois dessa validação, sem que o workspace
// tente reverter uma persistência já concluída. O dono da sessão compensa
// somente falhas desta criação. RemoveTab continua sem encerrar sessões.
func (m *Manager) AddTerminalTabForCommand(
	ctx context.Context,
	expected CommandSnapshot,
	tab Tab,
	sessions TerminalSessionValidator,
) (*Workspace, error) {
	if m == nil {
		return nil, fmt.Errorf("no workspace manager")
	}
	if ctx == nil {
		return nil, ErrCommandCreateTabNilContext
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if tab.Type != TabTypeTerminal {
		return nil, ErrCommandCreateTabUnsupportedType
	}
	if tab.ConversationID != "" || len(tab.State) != 1 {
		return nil, fmt.Errorf("workspace command create terminal: conversation and extra state must be empty")
	}
	sessionID, ok := tab.State["sessionId"].(string)
	if !ok || sessionID == "" || sessions == nil || !sessions.Has(sessionID) {
		return nil, fmt.Errorf("workspace command create terminal: live session required")
	}
	if err := tab.Validate(); err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if m.active == nil {
		return nil, fmt.Errorf("no active workspace")
	}
	current, err := m.commandSnapshotLocked()
	if err != nil {
		return nil, err
	}
	if current.WorkspaceID != expected.WorkspaceID || current.Version != expected.Version || current.ActiveTabID != expected.ActiveTabID {
		return nil, ErrCommandCreateTabStale
	}
	for _, existing := range m.active.Tabs.Items {
		if existing.ID == tab.ID {
			return nil, ErrCommandCreateTabDuplicateID
		}
	}
	if !sessions.Has(sessionID) {
		return nil, fmt.Errorf("workspace command create terminal: session is no longer live")
	}

	previousActive := m.active
	previousEpoch := m.commandEpoch
	m.active = cloneWorkspace(previousActive)
	if err := m.addTabLocked(cloneTab(tab)); err != nil {
		m.active = previousActive
		m.commandEpoch = previousEpoch
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		m.active = previousActive
		m.commandEpoch = previousEpoch
		return nil, err
	}
	if !sessions.Has(sessionID) {
		m.active = previousActive
		m.commandEpoch = previousEpoch
		return nil, fmt.Errorf("workspace command create terminal: session is no longer live")
	}
	if err := m.saveWorkspaceForCommand(ctx, m.active, m.activePath); err != nil {
		m.active = previousActive
		m.commandEpoch = previousEpoch
		return nil, err
	}
	return m.cloneActiveSnapshotLocked(m.active), nil
}

// saveWorkspaceForCommand grava o conteúdo em um temporário no mesmo diretório
// e tenta substituir o destino via os.Rename. O rename pode falhar
// transitoriamente, então há retries limitados e canceláveis. Este helper não
// promete atomicidade específica do Windows nem durabilidade/crash-safety:
// não há fsync do diretório nem protocolo de backup.
func (m *Manager) saveWorkspaceForCommand(ctx context.Context, ws *Workspace, basePath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dir := filepath.Join(basePath, assistenteDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create command workspace directory %s: %w", dir, err)
	}
	data, err := yaml.Marshal(ws)
	if err != nil {
		return fmt.Errorf("failed to marshal command workspace: %w", err)
	}
	temporary, err := os.CreateTemp(dir, ".workspace.yaml-command-*")
	if err != nil {
		return fmt.Errorf("failed to create command workspace temporary file: %w", err)
	}
	temporaryName := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryName)
	}()
	target := filepath.Join(dir, workspaceFile)
	mode := os.FileMode(0644)
	if existing, statErr := os.Stat(target); statErr == nil {
		mode = existing.Mode().Perm()
	}
	if err := temporary.Chmod(mode); err != nil {
		return fmt.Errorf("failed to set command workspace temporary file permissions: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		return fmt.Errorf("failed to write command workspace temporary file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("failed to close command workspace temporary file: %w", err)
	}
	var replaceErr error
	for attempt := 0; attempt < 5; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if replaceErr = os.Rename(temporaryName, target); replaceErr == nil {
			return nil
		}
		if attempt == 4 {
			break
		}
		timer := time.NewTimer(time.Duration(attempt+1) * 10 * time.Millisecond)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
	return fmt.Errorf("failed to replace command workspace file after retries: %w", replaceErr)
}
