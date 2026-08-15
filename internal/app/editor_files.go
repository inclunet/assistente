package app

import (
	"os"
	"path/filepath"
	"time"

	"assistente/internal/configdir"
)

const editorOrphanDraftGracePeriod = 24 * time.Hour

// draftDir retorna o diretório onde os drafts são armazenados como arquivos.
// Usado pelo cleanup de órfãos no startup; I/O de drafts migrado para wailsapi.Editor.
func draftDir() string {
	return filepath.Join(configdir.GetHomeDir(), "editor", "drafts")
}

// cleanupEditorOrphanDraftsOnStartup remove arquivos de draft antigos não referenciados.
func (a *App) cleanupEditorOrphanDraftsOnStartup() error {
	dir := draftDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // diretório ainda não existe
		}
		return err
	}

	cutoff := time.Now().Add(-editorOrphanDraftGracePeriod)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(cutoff) {
			continue // recente demais para remover
		}
		_ = os.Remove(filepath.Join(dir, entry.Name()))
	}
	return nil
}
