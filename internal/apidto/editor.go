package apidto

// EditorMergeSession descreve um merge de 3 vias em andamento para uma aba.
type EditorMergeSession struct {
	OriginalPath    string `json:"originalPath"`
	MineDraftId     string `json:"mineDraftId"`
	DiskDraftId     string `json:"diskDraftId"`
	ConflictDraftId string `json:"conflictDraftId"`
	CreatedAt       int64  `json:"createdAt"`
}

// EditorState é o estado do editor persistido por usuário autenticado em
// ~/.assistente/users/<userID>/editor/state.json.
// Não inclui lista de abas (fica no workspace YAML) nem conteúdo de documentos (fica em arquivos).
type EditorState struct {
	FileModeByPath       map[string]string             `json:"fileModeByPath,omitempty"`
	MergeSessionsByTabId map[string]EditorMergeSession `json:"mergeSessionsByTabId,omitempty"`
}

// EditorOpenResult é o retorno do diálogo nativo de abrir arquivo.
type EditorOpenResult struct {
	Path        string   `json:"path"`
	Content     string   `json:"content"`
	Projected   bool     `json:"projected"`
	Format      string   `json:"format,omitempty"`
	ReadOnly    bool     `json:"readOnly"`
	Pages       int      `json:"pages,omitempty"`
	Warnings    []string `json:"warnings,omitempty"`
	WarningCode string   `json:"warningCode,omitempty"`
}

// FileDialogLabels carrega os rótulos já traduzidos pelo frontend para os
// diálogos nativos do SO (o SO renderiza a string crua; não há i18n no backend).
type FileDialogLabels struct {
	Title           string `json:"title"`
	MarkdownFilter  string `json:"markdownFilter"`
	AllFilesFilter  string `json:"allFilesFilter"`
	DefaultFilename string `json:"defaultFilename"`
}

// EditorCommandPrepareRequest é a entrega efêmera feita após o flush da UI,
// antes do diálogo nativo. Ticket/handoff são referências opacas ao broker.
type EditorCommandPrepareRequest struct {
	Ticket            string           `json:"ticket"`
	HandoffID         string           `json:"handoffId"`
	Content           string           `json:"content,omitempty"`
	Labels            FileDialogLabels `json:"labels"`
	SuggestedFilename string           `json:"suggestedFilename,omitempty"`
}

type EditorCommandPreparation struct {
	Token             string `json:"token"`
	Path              string `json:"path"`
	Cancelled         bool   `json:"cancelled"`
	RequiresOverwrite bool   `json:"requiresOverwrite"`
}

type EditorCommandCommitRequest struct {
	Ticket           string `json:"ticket"`
	HandoffID        string `json:"handoffId"`
	Token            string `json:"token"`
	ConfirmOverwrite bool   `json:"confirmOverwrite"`
}

type EditorCommandResult struct {
	Status  string            `json:"status"`
	TabID   string            `json:"tabId"`
	Path    string            `json:"path"`
	Opened  *EditorOpenResult `json:"opened,omitempty"`
	Written bool              `json:"written"`
}

// EditorFileInfo retorna metadados simples do arquivo para detectar mudanças externas.
type EditorFileInfo struct {
	Path      string `json:"path"`
	Exists    bool   `json:"exists"`
	IsDir     bool   `json:"isDir"`
	Size      int64  `json:"size"`
	ModTimeMs int64  `json:"modTimeMs"`
}
