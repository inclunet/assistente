package apidto

// EditorMergeSession descreve um merge de 3 vias em andamento para uma aba.
type EditorMergeSession struct {
	OriginalPath    string `json:"originalPath"`
	MineDraftId     string `json:"mineDraftId"`
	DiskDraftId     string `json:"diskDraftId"`
	ConflictDraftId string `json:"conflictDraftId"`
	CreatedAt       int64  `json:"createdAt"`
}

// EditorState é o estado global do editor persistido em ~/.assistente/editor/state.json.
// Não inclui lista de abas (fica no workspace YAML) nem conteúdo de documentos (fica em arquivos).
type EditorState struct {
	FileModeByPath       map[string]string             `json:"fileModeByPath,omitempty"`
	MergeSessionsByTabId map[string]EditorMergeSession `json:"mergeSessionsByTabId,omitempty"`
}

// EditorOpenResult é o retorno do diálogo nativo de abrir arquivo.
type EditorOpenResult struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// FileDialogLabels carrega os rótulos já traduzidos pelo frontend para os
// diálogos nativos do SO (o SO renderiza a string crua; não há i18n no backend).
type FileDialogLabels struct {
	Title           string `json:"title"`
	MarkdownFilter  string `json:"markdownFilter"`
	AllFilesFilter  string `json:"allFilesFilter"`
	DefaultFilename string `json:"defaultFilename"`
}

// EditorFileInfo retorna metadados simples do arquivo para detectar mudanças externas.
type EditorFileInfo struct {
	Path      string `json:"path"`
	Exists    bool   `json:"exists"`
	IsDir     bool   `json:"isDir"`
	Size      int64  `json:"size"`
	ModTimeMs int64  `json:"modTimeMs"`
}
