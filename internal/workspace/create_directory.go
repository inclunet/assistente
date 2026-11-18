package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// createInDirectoryLocked nunca substitui um documento existente nem remove
// a pasta escolhida. O índice só é publicado depois do documento completo.
func (m *Manager) createInDirectoryLocked(ctx context.Context, ws *Workspace, directory string) (*Workspace, error) {
	ws.Name = strings.TrimSpace(ws.Name)
	if ws.Name == "" || strings.TrimSpace(directory) == "" || !filepath.IsAbs(directory) {
		return nil, fmt.Errorf("workspace name and absolute directory are required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	directory = filepath.Clean(directory)
	if m.active != nil && sameWorkspaceDirectory(directory, m.activePath) {
		return nil, fmt.Errorf("cannot replace the active workspace")
	}
	info, err := os.Stat(directory)
	if err != nil {
		return nil, fmt.Errorf("workspace directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("workspace destination is not a directory")
	}
	index, err := m.loadIndex()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(directory, assistenteDir, workspaceFile)
	if err := m.createWorkspaceDocument(ws, directory); err != nil {
		return nil, err
	}
	entries := index.Workspaces[:0]
	for _, entry := range index.Workspaces {
		if sameWorkspaceDirectory(entry.Path, directory) {
			if index.LastOpened == entry.ID {
				index.LastOpened = ws.ID
			}
			continue
		}
		entries = append(entries, entry)
	}
	index.Workspaces = append(entries, IndexEntry{
		ID: ws.ID, Name: ws.Name, Path: directory, LastUsed: time.Now(),
	})
	if err := m.saveIndexContext(ctx, index); err != nil {
		// Um processo externo pode ter alterado/substituído o documento desde
		// a publicação. Sem transação entre banco e filesystem, não há exclusão
		// condicional atômica segura. Preservar o documento permite recuperá-lo
		// abrindo a pasta novamente, sem apagar alterações concorrentes.
		return nil, fmt.Errorf("workspace document preserved at %s; index registration failed: %w", path, err)
	}
	return ws, nil
}
