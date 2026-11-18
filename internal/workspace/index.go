package workspace

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"assistente/internal/database"
	"assistente/internal/logging"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var ErrIndexChanged = errors.New("workspace index changed; retry operation")

func (m *Manager) indexScope() (string, error) {
	if m.indexClient != "" {
		return m.indexClient, nil
	}
	path, err := filepath.Abs(m.homeDir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(path, 0700); err != nil {
		return "", err
	}
	identityPath := filepath.Join(path, "desktop-client-id")
	data, err := os.ReadFile(identityPath)
	if errors.Is(err, os.ErrNotExist) {
		temporary, createErr := os.CreateTemp(path, ".desktop-client-id-*")
		if createErr != nil {
			return "", createErr
		}
		temporaryPath := temporary.Name()
		defer func() { _ = temporary.Close(); _ = os.Remove(temporaryPath) }()
		_, writeErr := temporary.WriteString(uuid.NewString())
		syncErr := temporary.Sync()
		closeErr := temporary.Close()
		if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
			return "", err
		}
		// Link publica bytes completos sem substituir a identidade vencedora caso
		// dois bancos sejam abertos simultaneamente pelo mesmo cliente desktop.
		if err := os.Link(temporaryPath, identityPath); err != nil && !errors.Is(err, os.ErrExist) {
			return "", err
		}
		data, err = os.ReadFile(identityPath)
	}

	if err != nil {
		return "", err
	}
	id, err := uuid.Parse(strings.TrimSpace(string(data)))
	if err != nil {
		return "", fmt.Errorf("invalid desktop client identity: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	m.indexClient = fmt.Sprintf("%x", sha256.Sum256([]byte(id.String()+"\n"+filepath.Clean(path))))
	return m.indexClient, nil
}

func (m *Manager) loadIndex() (*Index, error) {
	if m.indexDB == nil {
		return nil, errors.New("workspace index database unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var index Index
	scope, err := m.indexScope()
	if err != nil {
		return nil, err
	}
	err = m.indexDB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var state database.DesktopWorkspaceState
		err := tx.Where("client_scope = ?", scope).Take(&state).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		index.LastOpened, index.revision, index.legacyImported, index.legacyDigest = state.LastOpened, state.Revision, state.LegacyImported, state.LegacyDigest
		var rows []database.DesktopWorkspaceEntry
		if err := tx.Where("client_scope = ?", state.ClientScope).Order("last_used DESC, workspace_id ASC").Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			index.Workspaces = append(index.Workspaces, IndexEntry{ID: row.WorkspaceID, Name: row.Name, Path: row.Path, LastUsed: row.LastUsed})
		}
		return nil
	})
	return &index, err
}

func (m *Manager) saveIndex(idx *Index) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return m.saveIndexContext(ctx, idx)
}

func (m *Manager) saveIndexContext(ctx context.Context, idx *Index) error {
	if m.indexDB == nil {
		return errors.New("workspace index database unavailable")
	}
	scope, err := m.indexScope()
	if err != nil {
		return err
	}
	err = database.WithSQLiteBusyRetry(ctx, "workspace index", func() error {
		return m.indexDB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			state := database.DesktopWorkspaceState{ClientScope: scope, LastOpened: idx.LastOpened, Revision: idx.revision + 1, LegacyImported: idx.legacyImported, LegacyDigest: idx.legacyDigest}
			if idx.revision == 0 {
				if err := tx.Create(&state).Error; err != nil {
					return err
				}
			} else {
				result := tx.Model(&database.DesktopWorkspaceState{}).Where("client_scope = ? AND revision = ?", scope, idx.revision).Updates(map[string]any{"last_opened": state.LastOpened, "revision": state.Revision, "legacy_imported": state.LegacyImported, "legacy_digest": state.LegacyDigest})
				if result.Error != nil {
					return result.Error
				}
				if result.RowsAffected != 1 {
					return ErrIndexChanged
				}
			}
			if err := tx.Where("client_scope = ?", scope).Delete(&database.DesktopWorkspaceEntry{}).Error; err != nil {
				return err
			}
			for _, entry := range idx.Workspaces {
				row := database.DesktopWorkspaceEntry{ClientScope: scope, WorkspaceID: entry.ID, Name: entry.Name, Path: entry.Path, LastUsed: entry.LastUsed}
				if err := tx.Create(&row).Error; err != nil {
					return err
				}
			}
			return nil
		})
	})
	if err == nil {
		idx.revision++
	}
	return err
}

// A publicação confirmada no banco precede a limpeza. Não percorre nem apaga
// subpastas: os workspaces avulsos antigos continuam em seus caminhos originais.
func (m *Manager) importLegacyIndex() error {
	index, err := m.loadIndex()
	if err != nil {
		return err
	}
	path := filepath.Join(m.homeDir, workspacesDir, indexFile)
	if !index.legacyImported {
		legacy, data, err := readLegacyWorkspaceIndex(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err == nil {
			known := make(map[string]string)
			for _, entry := range index.Workspaces {
				known[entry.ID] = filepath.Clean(entry.Path)
			}
			for _, entry := range legacy.Workspaces {
				if strings.TrimSpace(entry.ID) == "" || !filepath.IsAbs(entry.Path) {
					return fmt.Errorf("invalid legacy workspace reference: %q", entry.ID)
				}
				if previous, exists := known[entry.ID]; exists && previous != filepath.Clean(entry.Path) {
					return fmt.Errorf("legacy workspace ID has conflicting paths: %s", entry.ID)
				} else if !exists {
					index.Workspaces = append(index.Workspaces, entry)
					known[entry.ID] = filepath.Clean(entry.Path)
				}
			}
			if index.LastOpened == "" {
				index.LastOpened = legacy.LastOpened
			}
		}
		if err == nil {
			index.legacyDigest = fmt.Sprintf("%x", sha256.Sum256(data))
		}
		index.legacyImported = true
		if err := m.saveIndex(index); err != nil {
			return err
		}
	}
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err == nil {
		if index.legacyDigest == "" || fmt.Sprintf("%x", sha256.Sum256(data)) != index.legacyDigest {
			logging.Warnf(context.Background(), "workspace.manager", "Índice YAML alterado após importação; arquivo preservado em %s", path)
			return nil
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove migrated workspace index: %w", err)
		}
	}
	// Remove apenas diretório vazio. Conteúdo legado ou desconhecido é preservado.
	_ = os.Remove(filepath.Dir(path))
	return nil
}
