package ports

// OpenFileOptions controla o comportamento do diálogo de abrir arquivo.
type OpenFileOptions struct {
	Title   string
	Filters []FileFilter
}

// SaveFileOptions controla o comportamento do diálogo de salvar arquivo.
type SaveFileOptions struct {
	Title           string
	DefaultFilename string
	Filters         []FileFilter
}

// FileFilter representa um filtro de extensão no diálogo de arquivo.
type FileFilter struct {
	DisplayName string
	Pattern     string
}

// SystemDialogPort abstrai diálogos nativos do sistema operacional.
// Implementações concretas: adapters/wails.DialogAdapter (desktop),
// adapters/noop.DialogAdapter (testes/CLI).
type SystemDialogPort interface {
	// OpenFileDialog exibe o diálogo nativo de seleção de arquivo.
	// Retorna o path selecionado ou "" se o usuário cancelou.
	OpenFileDialog(opts OpenFileOptions) (string, error)

	// SaveFileDialog exibe o diálogo nativo de salvar arquivo.
	// Retorna o path escolhido ou "" se o usuário cancelou.
	SaveFileDialog(opts SaveFileOptions) (string, error)
}
