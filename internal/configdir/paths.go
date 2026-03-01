package configdir

import (
	"os"
	"path/filepath"
	"sync"
)

const assistenteDir = ".assistente"

// Cached paths (computados uma vez)
var (
	cachedExeDir  string
	cachedHomeDir string
	cachedWorkDir string
	pathsOnce     sync.Once
)

// ResetForTests limpa o cache de paths.
// Útil em testes que alteram env vars (ex: USERPROFILE) e/ou o diretório de trabalho.
// Não é seguro chamar concorrentemente com operações de I/O usando o resolver.
func ResetForTests() {
	cachedExeDir = ""
	cachedHomeDir = ""
	cachedWorkDir = ""
	pathsOnce = sync.Once{}
}

// initPaths computa os 3 diretórios base uma única vez
func initPaths() {
	pathsOnce.Do(func() {
		// 1. Diretório do executável
		if exePath, err := os.Executable(); err == nil {
			// Resolve symlinks para obter o diretório real do executável
			if resolved, err := filepath.EvalSymlinks(exePath); err == nil {
				exePath = resolved
			}
			cachedExeDir = filepath.Join(filepath.Dir(exePath), assistenteDir)
		}

		// 2. Diretório home do usuário
		if homeDir, err := os.UserHomeDir(); err == nil {
			cachedHomeDir = filepath.Join(homeDir, assistenteDir)
		}

		// 3. Diretório de trabalho atual
		if workDir, err := os.Getwd(); err == nil {
			cachedWorkDir = filepath.Join(workDir, assistenteDir)
		}
	})
}

// GetExecutableDir retorna o caminho .assistente/ na pasta do executável
func GetExecutableDir() string {
	initPaths()
	return cachedExeDir
}

// GetHomeDir retorna o caminho ~/.assistente/
func GetHomeDir() string {
	initPaths()
	return cachedHomeDir
}

// GetWorkDir retorna o caminho .assistente/ no diretório de trabalho atual
func GetWorkDir() string {
	initPaths()
	return cachedWorkDir
}

// GetBasePaths retorna os 3 diretórios base em ordem de prioridade CRESCENTE.
// [0] = exe (menor prioridade)
// [1] = home (média)
// [2] = workdir (maior prioridade)
// Diretórios vazios ou duplicados são filtrados.
func GetBasePaths() []string {
	initPaths()

	paths := []string{}
	seen := map[string]bool{}

	for _, p := range []string{cachedExeDir, cachedHomeDir, cachedWorkDir} {
		if p == "" {
			continue
		}
		// Normaliza para comparação
		normalized := filepath.Clean(p)
		if seen[normalized] {
			continue
		}
		seen[normalized] = true
		paths = append(paths, normalized)
	}

	return paths
}
