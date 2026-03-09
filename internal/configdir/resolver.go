package configdir

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Source indica a origem de um arquivo resolvido
type Source string

const (
	SourceExe     Source = "exe"
	SourceHome    Source = "home"
	SourceWorkDir Source = "workdir"
)

// ResolvedFile representa um arquivo encontrado pelo resolver com sua origem
type ResolvedFile struct {
	Name     string // nome sem extensão (slug), ex: "padrao"
	Filename string // nome completo, ex: "padrao.json", "config.json"
	Path     string // caminho absoluto do arquivo válido
	Source   Source // origem: "exe", "home", "workdir"
}

// Resolver resolve arquivos dentro de pastas .assistente/ nos 3 diretórios.
// Opera EXCLUSIVAMENTE dentro de .assistente/ — nunca acessa arquivos fora desse escopo.
type Resolver struct {
	subdir string // "" para raiz, "profiles" para subdir, etc.
}

// NewResolver cria um resolver para um subdiretório de .assistente/.
// subdir="" resolve na raiz de .assistente/ (config.json, conversations.db).
// subdir="profiles" resolve em .assistente/profiles/.
func NewResolver(subdir string) *Resolver {
	return &Resolver{
		subdir: subdir,
	}
}

func (r *Resolver) getPaths() []string {
	basePaths := GetBasePaths()
	paths := make([]string, 0, len(basePaths))

	for _, base := range basePaths {
		if r.subdir != "" {
			paths = append(paths, filepath.Join(base, r.subdir))
		} else {
			paths = append(paths, base)
		}
	}

	return paths
}

// SourceForPath determina a Source com base no diretório base.
func SourceForPath(dirPath string) Source {
	initPaths()
	clean := filepath.Clean(dirPath)

	// Verifica de trás para frente (maior prioridade primeiro) para ser preciso
	if cachedWorkDir != "" && strings.HasPrefix(clean, filepath.Clean(cachedWorkDir)) {
		return SourceWorkDir
	}
	if cachedHomeDir != "" && strings.HasPrefix(clean, filepath.Clean(cachedHomeDir)) {
		return SourceHome
	}
	if cachedExeDir != "" && strings.HasPrefix(clean, filepath.Clean(cachedExeDir)) {
		return SourceExe
	}
	return SourceHome // fallback
}

// ValidateFilename verifica que o nome de arquivo é seguro (sem path traversal).
// Útil para operações de filesystem que aceitam apenas basename (sem diretórios).
func ValidateFilename(filename string) error {
	if filename == "" {
		return fmt.Errorf("filename cannot be empty")
	}
	// Rejeita path traversal
	if strings.Contains(filename, "..") {
		return fmt.Errorf("filename cannot contain '..': %s", filename)
	}
	if strings.ContainsAny(filename, `/\`) {
		return fmt.Errorf("filename cannot contain path separators: %s", filename)
	}
	// Rejeita nomes perigosos
	cleaned := filepath.Clean(filename)
	if cleaned != filename {
		return fmt.Errorf("filename is not clean: %s", filename)
	}
	return nil
}

// slugFromFilename extrai o nome sem extensão
func slugFromFilename(filename string) string {
	ext := filepath.Ext(filename)
	return strings.TrimSuffix(filename, ext)
}

// Resolve encontra o arquivo válido (maior prioridade) entre os 3 diretórios.
// Retorna erro se o arquivo não existir em nenhum diretório.
func (r *Resolver) Resolve(filename string) (*ResolvedFile, error) {
	if err := ValidateFilename(filename); err != nil {
		return nil, err
	}

	var result *ResolvedFile
	paths := r.getPaths()

	// Itera na ordem de prioridade crescente — o último encontrado ganha
	for _, dir := range paths {
		fullPath := filepath.Join(dir, filename)
		if _, err := os.Stat(fullPath); err == nil {
			result = &ResolvedFile{
				Name:     slugFromFilename(filename),
				Filename: filename,
				Path:     fullPath,
				Source:   SourceForPath(fullPath),
			}
		}
	}

	if result == nil {
		return nil, fmt.Errorf("file not found in any directory: %s", filename)
	}

	return result, nil
}

// List lista todos os arquivos resolvidos no subdiretório, com resolução de prioridade.
// Arquivos com mesmo nome: o de maior prioridade prevalece.
func (r *Resolver) List() ([]ResolvedFile, error) {
	resolved := map[string]ResolvedFile{} // filename -> ResolvedFile
	paths := r.getPaths()

	// Itera na ordem de prioridade crescente — o último encontrado sobrescreve
	for _, dir := range paths {
		entries, err := os.ReadDir(dir)
		if err != nil {
			// Diretório não existe — tudo bem, pula
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}

			filename := entry.Name()
			fullPath := filepath.Join(dir, filename)

			resolved[filename] = ResolvedFile{
				Name:     slugFromFilename(filename),
				Filename: filename,
				Path:     fullPath,
				Source:   SourceForPath(fullPath),
			}
		}
	}

	// Converte map para slice
	result := make([]ResolvedFile, 0, len(resolved))
	for _, rf := range resolved {
		result = append(result, rf)
	}

	return result, nil
}

// Read lê o conteúdo do arquivo válido (maior prioridade).
// Retorna o conteúdo, informações do arquivo resolvido, e erro.
func (r *Resolver) Read(filename string) ([]byte, *ResolvedFile, error) {
	resolved, err := r.Resolve(filename)
	if err != nil {
		return nil, nil, err
	}

	data, err := os.ReadFile(resolved.Path)
	if err != nil {
		return nil, resolved, fmt.Errorf("failed to read file %s: %w", resolved.Path, err)
	}

	return data, resolved, nil
}

// Write escreve no arquivo válido (maior prioridade existente).
// Se o arquivo não existir em nenhum diretório, cria no home (~/.assistente/).
func (r *Resolver) Write(filename string, data []byte) error {
	if err := ValidateFilename(filename); err != nil {
		return err
	}

	resolved, err := r.Resolve(filename)
	if err != nil {
		// Arquivo não existe — criar no home
		return r.Create(filename, data)
	}

	// Escreve no arquivo válido
	return os.WriteFile(resolved.Path, data, 0644)
}

// Create cria um novo arquivo sempre no diretório home (~/.assistente/[subdir]/).
func (r *Resolver) Create(filename string, data []byte) error {
	if err := ValidateFilename(filename); err != nil {
		return err
	}

	if err := r.EnsureHomeDir(); err != nil {
		return err
	}

	homeDir := r.GetHomeDir()
	if homeDir == "" {
		return fmt.Errorf("home directory not available")
	}

	fullPath := filepath.Join(homeDir, filename)

	// Verifica se já existe
	if _, err := os.Stat(fullPath); err == nil {
		return fmt.Errorf("file already exists: %s", fullPath)
	}

	return os.WriteFile(fullPath, data, 0644)
}

// Delete remove o arquivo válido (maior prioridade).
// Se existir em camada inferior, essa passa a valer automaticamente.
func (r *Resolver) Delete(filename string) error {
	resolved, err := r.Resolve(filename)
	if err != nil {
		return err
	}

	return os.Remove(resolved.Path)
}

// GetSearchPaths retorna os caminhos de busca em ordem de prioridade crescente.
func (r *Resolver) GetSearchPaths() []string {
	paths := r.getPaths()
	result := make([]string, len(paths))
	copy(result, paths)
	return result
}

// EnsureHomeDir cria o diretório home (~/.assistente/[subdir]) se não existir.
func (r *Resolver) EnsureHomeDir() error {
	homeDir := r.GetHomeDir()
	if homeDir == "" {
		return fmt.Errorf("home directory not available")
	}
	return os.MkdirAll(homeDir, 0755)
}

// GetHomeDir retorna o caminho do diretório home para este resolver.
// O diretório home é sempre o segundo na lista de prioridade,
// mas se houver apenas 1 ou 2 paths (deduplicação), precisamos encontrá-lo.
func (r *Resolver) GetHomeDir() string {
	initPaths()

	baseHome := cachedHomeDir
	if baseHome == "" {
		return ""
	}

	if r.subdir != "" {
		return filepath.Join(baseHome, r.subdir)
	}
	return baseHome
}

// Exists verifica se o arquivo existe em pelo menos um dos diretórios.
func (r *Resolver) Exists(filename string) bool {
	_, err := r.Resolve(filename)
	return err == nil
}

// ResolveAll retorna todas as versões do arquivo em todos os diretórios onde existe,
// em ordem de prioridade crescente. Útil para debug/UI.
func (r *Resolver) ResolveAll(filename string) ([]ResolvedFile, error) {
	if err := ValidateFilename(filename); err != nil {
		return nil, err
	}

	var results []ResolvedFile
	paths := r.getPaths()

	for _, dir := range paths {
		fullPath := filepath.Join(dir, filename)
		if _, err := os.Stat(fullPath); err == nil {
			results = append(results, ResolvedFile{
				Name:     slugFromFilename(filename),
				Filename: filename,
				Path:     fullPath,
				Source:   SourceForPath(fullPath),
			})
		}
	}

	return results, nil
}
