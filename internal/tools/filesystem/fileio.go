package filesystem

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

var fileMutationLocks = struct {
	sync.Mutex
	entries map[string]*fileMutationLockEntry
}{entries: make(map[string]*fileMutationLockEntry)}

type fileMutationLockEntry struct {
	key  string
	info fs.FileInfo
	mu   sync.Mutex
	refs int
}

// lockFileMutation serializa mutações feitas pelo processo no mesmo path.
// O unlock devolvido também remove entradas ociosas para não acumular paths.
func lockFileMutation(path string) func() {
	if resolved, err := resolveForComparison(path); err == nil {
		path = resolved
	}
	key := normalizeForComparison(path)
	info, _ := os.Stat(path)
	fileMutationLocks.Lock()
	entry := fileMutationLocks.entries[key]
	if entry == nil && info != nil {
		for _, candidate := range fileMutationLocks.entries {
			if candidate.info != nil && os.SameFile(info, candidate.info) {
				entry = candidate
				break
			}
		}
	}
	if entry == nil {
		entry = &fileMutationLockEntry{key: key, info: info}
		fileMutationLocks.entries[key] = entry
	}
	entry.refs++
	fileMutationLocks.Unlock()

	entry.mu.Lock()
	return func() {
		entry.mu.Unlock()
		fileMutationLocks.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(fileMutationLocks.entries, entry.key)
		}
		fileMutationLocks.Unlock()
	}
}

// EnsureParentDir garante que o diretório pai de path exista.
func EnsureParentDir(path string) error {
	parent := filepath.Dir(path)
	if parent == "." || parent == "" {
		return nil
	}
	if err := os.MkdirAll(parent, 0755); err != nil {
		return fmt.Errorf("falha ao criar diretório pai: %w", err)
	}
	return nil
}

// ReadFileBytes lê um arquivo e retorna bytes.
func ReadFileBytes(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// WriteFileBytes escreve bytes em um arquivo. Cria diretórios intermediários automaticamente.
func WriteFileBytes(path string, content []byte, perm fs.FileMode) error {
	unlock := lockFileMutation(path)
	defer unlock()
	return writeFileBytesUnlocked(path, content, perm)
}

func writeFileBytesUnlocked(path string, content []byte, perm fs.FileMode) error {
	if err := EnsureParentDir(path); err != nil {
		return err
	}
	if err := os.WriteFile(path, content, perm); err != nil {
		return err
	}
	return nil
}
