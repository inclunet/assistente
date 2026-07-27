package channels

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"assistente/internal/configdir"
	"assistente/internal/database"
)

const legacyBackupDirName = "channels.legacy-backup"

// LegacyCleanupOptions controla o cleanup opt-in de JSON legado (AEP-0083).
// Confirm=false (default) é dry-run: só lista paths elegíveis.
// Delete só ocorre com Confirm=true.
type LegacyCleanupOptions struct {
	Confirm         bool
	NoBackup        bool // quando Confirm=true, backup é o padrão recomendado
	ContactsUsingDB bool // caller informa contacts.UsingDatabase() (evita ciclo de import)
}

// LegacyCleanupItem descreve um arquivo legado candidato ou ignorado.
type LegacyCleanupItem struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"` // "channel" | "contacts"
	Slug   string `json:"slug,omitempty"`
	Reason string `json:"reason"`
}

// LegacyCleanupResult é o resultado do dry-run ou da remoção confirmada.
type LegacyCleanupResult struct {
	DryRun     bool                `json:"dryRun"`
	Eligible   []LegacyCleanupItem `json:"eligible"`
	Removed    []string            `json:"removed"`
	BackedUpTo string              `json:"backedUpTo,omitempty"`
	Skipped    []LegacyCleanupItem `json:"skipped"`
	Errors     []string            `json:"errors"`
	Warnings   []string            `json:"warnings"`
}

// CleanupLegacyJSONFiles remove (opt-in) channels/*.json e contacts.json após
// migração para DB. Nunca deve ser chamado pelo import pós-login automático.
func CleanupLegacyJSONFiles(ctx context.Context, opts LegacyCleanupOptions) (LegacyCleanupResult, error) {
	result := LegacyCleanupResult{DryRun: !opts.Confirm}
	if ctx == nil {
		ctx = context.Background()
	}
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return result, err
	}
	if !usingDB() {
		return result, fmt.Errorf("channels DB não habilitado; cleanup legado indisponível")
	}
	if !opts.ContactsUsingDB {
		return result, fmt.Errorf("contacts DB não habilitado; cleanup legado indisponível")
	}

	eligible, skipped, listErrs := listEligibleLegacyJSON(userID)
	result.Eligible = eligible
	result.Skipped = skipped
	result.Errors = append(result.Errors, listErrs...)

	if !opts.Confirm {
		return result, nil
	}
	if len(eligible) == 0 {
		result.Warnings = append(result.Warnings, "nenhum arquivo legado elegível para remoção")
		return result, nil
	}

	backupRoot := ""
	if !opts.NoBackup {
		homeDir := strings.TrimSpace(configdir.GetHomeDir())
		if homeDir == "" {
			return result, fmt.Errorf("diretório home do assistente indisponível; cleanup com backup abortado")
		}
		legacyBackupParent := filepath.Join(homeDir, legacyBackupDirName)
		if info, err := os.Lstat(legacyBackupParent); err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return result, fmt.Errorf("%s: symlink rejeitado como pasta de backup", legacyBackupParent)
			}
			if !info.IsDir() {
				return result, fmt.Errorf("%s: não é diretório; cleanup com backup abortado", legacyBackupParent)
			}
		} else if !os.IsNotExist(err) {
			return result, fmt.Errorf("verificar pasta de backup: %w", err)
		} else if err := os.MkdirAll(legacyBackupParent, 0700); err != nil {
			return result, fmt.Errorf("criar pasta de backup: %w", err)
		}
		backupRoot = filepath.Join(legacyBackupParent, time.Now().Format("20060102-150405.000000000"))
		if err := os.MkdirAll(backupRoot, 0700); err != nil {
			return result, fmt.Errorf("criar diretório de backup: %w", err)
		}
		result.BackedUpTo = backupRoot
	} else {
		result.Warnings = append(result.Warnings,
			"backup desabilitado: arquivos legados serão removidos sem cópia em channels.legacy-backup/")
	}

	for _, item := range eligible {
		if err := requireRegularFile(item.Path); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				result.Skipped = append(result.Skipped, LegacyCleanupItem{
					Path:   item.Path,
					Kind:   item.Kind,
					Slug:   item.Slug,
					Reason: "arquivo já ausente no momento da confirmação",
				})
				continue
			}
			result.Errors = append(result.Errors, err.Error())
			continue
		}
		if backupRoot != "" {
			rel, relErr := legacyBackupRelPath(item)
			if relErr != nil {
				result.Errors = append(result.Errors, relErr.Error())
				continue
			}
			dest := uniqueBackupDest(backupRoot, rel)
			if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("backup mkdir %s: %v", item.Path, err))
				continue
			}
			if err := copyFile(item.Path, dest); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("backup %s: %v", item.Path, err))
				continue
			}
		}
		if err := os.Remove(item.Path); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				result.Skipped = append(result.Skipped, LegacyCleanupItem{
					Path:   item.Path,
					Kind:   item.Kind,
					Slug:   item.Slug,
					Reason: "arquivo já ausente no momento da remoção",
				})
				continue
			}
			result.Errors = append(result.Errors, fmt.Sprintf("remover %s: %v", item.Path, err))
			continue
		}
		result.Removed = append(result.Removed, item.Path)
	}
	return result, nil
}

func listEligibleLegacyJSON(userID string) (eligible, skipped []LegacyCleanupItem, errs []string) {
	seenPaths := make(map[string]struct{})
	for _, base := range configdir.GetBasePaths() {
		dir := filepath.Join(base, channelsSubdir)
		entries, err := os.ReadDir(dir)
		if err != nil {
			if !os.IsNotExist(err) {
				errs = append(errs, fmt.Sprintf("listar %s: %v", dir, err))
			}
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if !strings.HasSuffix(strings.ToLower(name), ".json") {
				continue
			}
			slug := strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))
			if slug == "" {
				continue
			}
			path := filepath.Join(dir, name)
			if _, dup := seenPaths[path]; dup {
				continue
			}
			seenPaths[path] = struct{}{}
			if err := requireRegularFile(path); err != nil {
				skipped = append(skipped, LegacyCleanupItem{
					Path:   path,
					Kind:   "channel",
					Slug:   slug,
					Reason: err.Error(),
				})
				continue
			}
			exists, err := channelExistsForUser(userID, slug)
			if err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", path, err))
				continue
			}
			if !exists {
				skipped = append(skipped, LegacyCleanupItem{
					Path:   path,
					Kind:   "channel",
					Slug:   slug,
					Reason: "canal ausente no DB para o usuário autenticado",
				})
				continue
			}
			eligible = append(eligible, LegacyCleanupItem{
				Path:   path,
				Kind:   "channel",
				Slug:   slug,
				Reason: "canal presente no DB (import já aplicado ou skip por exists)",
			})
		}
	}

	for _, base := range configdir.GetBasePaths() {
		contactsPath := filepath.Join(base, "contacts.json")
		if err := requireRegularFile(contactsPath); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			skipped = append(skipped, LegacyCleanupItem{
				Path:   contactsPath,
				Kind:   "contacts",
				Reason: err.Error(),
			})
			continue
		}
		ok, reason, checkErr := contactsJSONEligible(contactsPath, userID)
		if checkErr != nil {
			errs = append(errs, checkErr.Error())
			continue
		}
		if !ok {
			skipped = append(skipped, LegacyCleanupItem{
				Path:   contactsPath,
				Kind:   "contacts",
				Reason: reason,
			})
			continue
		}
		eligible = append(eligible, LegacyCleanupItem{
			Path:   contactsPath,
			Kind:   "contacts",
			Reason: reason,
		})
	}
	return eligible, skipped, errs
}

func contactsJSONEligible(path, userID string) (ok bool, reason string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, "", fmt.Errorf("ler %s: %w", path, err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return true, "contacts.json vazio e contacts DB habilitado", nil
	}
	var file map[string]json.RawMessage
	if err := json.Unmarshal(data, &file); err != nil {
		return false, "JSON inválido — não remover", nil
	}
	if len(file) == 0 {
		return true, "contacts.json sem canais e contacts DB habilitado", nil
	}
	for rawSlug := range file {
		slug := strings.ToLower(strings.TrimSpace(rawSlug))
		if slug == "" {
			continue
		}
		exists, err := channelExistsForUser(userID, slug)
		if err != nil {
			return false, "", fmt.Errorf("contacts.json/%s: %w", slug, err)
		}
		if !exists {
			return false, fmt.Sprintf("canal %s ausente no DB para o usuário — contatos não migrados com segurança", slug), nil
		}
	}
	return true, "todos os canais do contacts.json estão no DB", nil
}

func legacyBackupRelPath(item LegacyCleanupItem) (string, error) {
	switch item.Kind {
	case "contacts":
		return "contacts.json", nil
	case "channel":
		base := filepath.Base(item.Path)
		if base == "" || base == "." || base == string(filepath.Separator) {
			return "", fmt.Errorf("nome de arquivo inválido: %s", item.Path)
		}
		return filepath.Join(channelsSubdir, base), nil
	default:
		return "", fmt.Errorf("kind desconhecido %q para %s", item.Kind, item.Path)
	}
}

func uniqueBackupDest(backupRoot, rel string) string {
	dest := filepath.Join(backupRoot, rel)
	if _, err := os.Stat(dest); err != nil {
		if os.IsNotExist(err) {
			return dest
		}
		// Em erro inesperado (permissão/I/O), usa sufixo único em vez de loop infinito.
		return filepath.Join(backupRoot, fmt.Sprintf("%s-%d%s",
			strings.TrimSuffix(rel, filepath.Ext(rel)),
			time.Now().UnixNano(),
			filepath.Ext(rel)))
	}
	ext := filepath.Ext(rel)
	stem := strings.TrimSuffix(rel, ext)
	for i := 2; i < 10000; i++ {
		candidate := filepath.Join(backupRoot, fmt.Sprintf("%s-%d%s", stem, i, ext))
		if _, err := os.Stat(candidate); err != nil {
			if os.IsNotExist(err) {
				return candidate
			}
			return filepath.Join(backupRoot, fmt.Sprintf("%s-%d%s", stem, time.Now().UnixNano(), ext))
		}
	}
	return filepath.Join(backupRoot, fmt.Sprintf("%s-%d%s", stem, time.Now().UnixNano(), ext))
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0600)
}

func requireRegularFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s: symlink rejeitado no cleanup legado", path)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s: não é arquivo regular", path)
	}
	return nil
}
