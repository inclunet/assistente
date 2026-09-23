package configdir

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ErrProfileTransactionRecovery indica que um journal de mutação encontrou
// um estado que não corresponde nem ao estado anterior nem ao posterior.
// O chamador deve parar de ler/escrever até que a recuperação seja resolvida
// explicitamente; nunca sobrescrevemos uma alteração externa desconhecida.
var ErrProfileTransactionRecovery = errors.New("profile transaction requires recovery")

// ProfileFileChange descreve o estado esperado e o estado desejado de um
// arquivo de perfil. BeforeExists/AfterExists distinguem arquivo vazio de
// arquivo ausente.
type ProfileFileChange struct {
	Path         string `json:"path"`
	Before       []byte `json:"before,omitempty"`
	BeforeExists bool   `json:"before_exists"`
	After        []byte `json:"after,omitempty"`
	AfterExists  bool   `json:"after_exists"`
}

type profileTransactionJournal struct {
	Version int                 `json:"version"`
	Created string              `json:"created"`
	Changes []ProfileFileChange `json:"changes"`
}

// RecoverProfileTransaction trata o journal como uma intenção durável. Cada
// arquivo precisa estar exatamente no estado anterior ou posterior; estados
// terceiros são conflito externo e falham fechados. Em qualquer combinação
// conhecida (inclusive um prefixo já aplicado), a recuperação faz roll-forward
// dos arquivos restantes e só remove o journal depois de verificar que todos
// estão no estado posterior.
func RecoverProfileTransaction(journalPath string, allowedRoots []string) error {
	data, err := os.ReadFile(journalPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read profile transaction journal: %w", err)
	}

	var journal profileTransactionJournal
	if err := json.Unmarshal(data, &journal); err != nil {
		return fmt.Errorf("decode profile transaction journal: %w", err)
	}
	if err := validateProfileTransactionJournal(journal, journalPath, allowedRoots); err != nil {
		return fmt.Errorf("invalid profile transaction journal: %w", err)
	}

	for _, change := range journal.Changes {
		current, exists, err := readProfileTransactionFile(change.Path)
		if err != nil {
			return fmt.Errorf("inspect profile transaction file %s: %w", change.Path, err)
		}
		if !sameProfileTransactionState(current, exists, change.Before, change.BeforeExists) &&
			!sameProfileTransactionState(current, exists, change.After, change.AfterExists) {
			return fmt.Errorf("%w: unknown file state for %s", ErrProfileTransactionRecovery, change.Path)
		}
	}
	for _, change := range journal.Changes {
		current, exists, err := readProfileTransactionFile(change.Path)
		if err != nil {
			return fmt.Errorf("reinspect profile transaction file %s: %w", change.Path, err)
		}
		if sameProfileTransactionState(current, exists, change.After, change.AfterExists) {
			continue
		}
		if !sameProfileTransactionState(current, exists, change.Before, change.BeforeExists) {
			return fmt.Errorf("%w: file changed during recovery for %s", ErrProfileTransactionRecovery, change.Path)
		}
		if err := applyProfileTransactionChange(change); err != nil {
			return fmt.Errorf("roll forward profile transaction file %s: %w", change.Path, err)
		}
	}
	for _, change := range journal.Changes {
		current, exists, err := readProfileTransactionFile(change.Path)
		if err != nil {
			return fmt.Errorf("verify recovered profile transaction file %s: %w", change.Path, err)
		}
		if !sameProfileTransactionState(current, exists, change.After, change.AfterExists) {
			return fmt.Errorf("%w: roll-forward did not reach after state for %s", ErrProfileTransactionRecovery, change.Path)
		}
	}
	if err := removeProfileTransactionJournal(journalPath); err != nil {
		return fmt.Errorf("remove recovered profile transaction journal: %w", err)
	}
	return nil
}

// CommitProfileTransaction grava um journal durável antes de tocar em mais de
// um arquivo. Cada arquivo é substituído por rename de um temporário no mesmo
// diretório; em caso de erro o journal permanece para que a próxima operação
// faça roll-forward de estados conhecidos ou falhe fechada em conflito.
func CommitProfileTransaction(journalPath string, allowedRoots []string, changes []ProfileFileChange) error {
	changes = compactProfileTransactionChanges(changes)
	if len(changes) == 0 {
		return nil
	}
	journal := profileTransactionJournal{Version: 1, Created: time.Now().UTC().Format(time.RFC3339Nano), Changes: changes}
	if err := validateProfileTransactionJournal(journal, journalPath, allowedRoots); err != nil {
		return fmt.Errorf("invalid profile transaction: %w", err)
	}
	for _, change := range changes {
		current, exists, err := readProfileTransactionFile(change.Path)
		if err != nil {
			return fmt.Errorf("inspect profile transaction file %s: %w", change.Path, err)
		}
		if !sameProfileTransactionState(current, exists, change.Before, change.BeforeExists) {
			return fmt.Errorf("profile transaction compare-and-swap failed for %s", change.Path)
		}
	}
	if err := writeProfileTransactionJournal(journalPath, journal); err != nil {
		return err
	}
	for _, change := range changes {
		if err := applyProfileTransactionChange(change); err != nil {
			return fmt.Errorf("apply profile transaction file %s: %w", change.Path, err)
		}
	}
	if err := removeProfileTransactionJournal(journalPath); err != nil {
		return fmt.Errorf("remove committed profile transaction journal: %w", err)
	}
	return nil
}

func compactProfileTransactionChanges(changes []ProfileFileChange) []ProfileFileChange {
	result := make([]ProfileFileChange, 0, len(changes))
	for _, change := range changes {
		change.Path = filepath.Clean(change.Path)
		if !change.BeforeExists && !change.AfterExists && len(change.Before) == 0 && len(change.After) == 0 {
			continue
		}
		result = append(result, change)
	}
	return result
}

func validateProfileTransactionJournal(journal profileTransactionJournal, journalPath string, allowedRoots []string) error {
	if journal.Version != 1 || len(journal.Changes) == 0 {
		return fmt.Errorf("unsupported journal version or empty changes")
	}
	if !filepath.IsAbs(journalPath) {
		return fmt.Errorf("journal path must be absolute")
	}
	seen := make(map[string]struct{}, len(journal.Changes))
	for _, change := range journal.Changes {
		path := filepath.Clean(change.Path)
		if !filepath.IsAbs(path) || path != change.Path {
			return fmt.Errorf("change path is not clean and absolute: %q", change.Path)
		}
		if _, ok := seen[path]; ok {
			return fmt.Errorf("duplicate change path: %s", path)
		}
		seen[path] = struct{}{}
		if filepath.Ext(path) != ".json" {
			return fmt.Errorf("change path is not a profile JSON file: %s", path)
		}
		if !profileTransactionProfilePathAllowed(path, allowedRoots) {
			return fmt.Errorf("change path outside direct resolver profile roots: %s", path)
		}
	}
	cleanJournalPath := filepath.Clean(journalPath)
	if filepath.Base(cleanJournalPath) != ".profile-mutation.journal" {
		return fmt.Errorf("journal path has unexpected filename: %s", journalPath)
	}
	if !profileTransactionPathAllowed(cleanJournalPath, allowedRoots, false) {
		return fmt.Errorf("journal path outside resolver roots: %s", journalPath)
	}
	return nil
}

func profileTransactionProfilePathAllowed(path string, roots []string) bool {
	cleanPath := filepath.Clean(path)
	for _, root := range roots {
		if root == "" {
			continue
		}
		if profileTransactionPathAllowed(cleanPath, []string{root}, true) {
			return true
		}
	}
	return false
}

func profileTransactionPathAllowed(path string, roots []string, directChild bool) bool {
	canonicalPath, err := canonicalProfileTransactionPath(path)
	if err != nil {
		return false
	}
	for _, root := range roots {
		if root == "" {
			continue
		}
		canonicalRoot, err := canonicalProfileTransactionPath(filepath.Clean(root))
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(canonicalRoot, canonicalPath)
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			continue
		}
		if directChild && filepath.Dir(rel) != "." {
			continue
		}
		return true
	}
	return false
}

// canonicalProfileTransactionPath resolve symlinks in the existing portion of
// a path and then appends the not-yet-created suffix. This prevents a journal
// from escaping a resolver root through a symlink without requiring the target
// profile to exist already.
func canonicalProfileTransactionPath(path string) (string, error) {
	cleanPath, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	missing := []string{}
	current := cleanPath
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return filepath.Clean(resolved), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", err
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

func readProfileTransactionFile(path string) ([]byte, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

func sameProfileTransactionState(current []byte, exists bool, expected []byte, expectedExists bool) bool {
	return exists == expectedExists && (!exists || bytes.Equal(current, expected))
}

func writeProfileTransactionJournal(path string, journal profileTransactionJournal) error {
	data, err := json.Marshal(journal)
	if err != nil {
		return fmt.Errorf("encode profile transaction journal: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create profile transaction journal directory: %w", err)
	}
	return writeProfileTransactionAtomic(path, data, 0600)
}

func applyProfileTransactionChange(change ProfileFileChange) error {
	if !change.AfterExists {
		if err := os.Remove(change.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(change.Path), 0755); err != nil {
		return err
	}
	return writeProfileTransactionAtomic(change.Path, change.After, 0644)
}

func writeProfileTransactionAtomic(path string, data []byte, mode os.FileMode) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".profile-mutation-*")
	if err != nil {
		return err
	}
	tempPath := file.Name()
	defer os.Remove(tempPath)
	if err := file.Chmod(mode); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	return syncProfileTransactionDirectory(filepath.Dir(path))
}

func syncProfileTransactionDirectory(path string) error {
	if runtime.GOOS == "windows" {
		// Windows não oferece fsync de diretório via os.File; os arquivos e o
		// journal individual já foram sincronizados antes dos renames.
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := file.Sync(); err != nil && !errors.Is(err, os.ErrInvalid) {
		return err
	}
	return nil
}

func removeProfileTransactionJournal(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return syncProfileTransactionDirectory(filepath.Dir(path))
}
