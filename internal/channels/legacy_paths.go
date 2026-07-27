package channels

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"assistente/internal/configdir"
)

// legacyChannelJSONFile é um channels/*.json encontrado sob um base path.
type legacyChannelJSONFile struct {
	Dir  string // diretório channels/ absoluto
	Name string // nome no FS (preserva case)
	Slug string // basename lowercase sem extensão
	Path string // filepath.Join(Dir, Name)
}

// listLegacyChannelJSONOptions controla validação de diretório no enumerador compartilhado.
// Zero value = comportamento do import: ignora erros de ReadDir, sem política de symlink.
type listLegacyChannelJSONOptions struct {
	// RequireRealDir, quando true, aplica requireRealDir antes do ReadDir (cleanup).
	// NotExist é silencioso; demais erros vão para dirErrs. Erros de ReadDir também.
	RequireRealDir bool
}

// listLegacyChannelJSONFiles enumera *.json em channels/ em todos GetBasePaths().
// Sem dedup de slug/path, sem elegibilidade, sem política por arquivo.
func listLegacyChannelJSONFiles(opts listLegacyChannelJSONOptions) (files []legacyChannelJSONFile, dirErrs []string) {
	return listLegacyChannelJSONInBases(configdir.GetBasePaths(), opts)
}

// listLegacyChannelJSONInBases é a implementação testável do enumerador.
func listLegacyChannelJSONInBases(basePaths []string, opts listLegacyChannelJSONOptions) (files []legacyChannelJSONFile, dirErrs []string) {
	for _, base := range basePaths {
		dir := filepath.Join(base, channelsSubdir)
		if opts.RequireRealDir {
			if err := requireRealDir(dir); err != nil {
				if os.IsNotExist(err) {
					continue
				}
				dirErrs = append(dirErrs, err.Error())
				continue
			}
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			if opts.RequireRealDir {
				dirErrs = append(dirErrs, fmt.Sprintf("listar %s: %v", dir, err))
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
			files = append(files, legacyChannelJSONFile{
				Dir:  dir,
				Name: name,
				Slug: slug,
				Path: filepath.Join(dir, name),
			})
		}
	}
	return files, dirErrs
}
