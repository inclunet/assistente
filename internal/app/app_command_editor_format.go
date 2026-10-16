package app

import (
	"strings"

	"assistente/internal/commandcatalog"
)

const (
	commandEditorMermaidApplyID            = "editor.mermaid.apply"
	commandEditorMermaidRemoveID           = "editor.mermaid.remove"
	commandEditorFormatBoldID              = "editor.format.bold"
	commandEditorFormatItalicID            = "editor.format.italic"
	commandEditorFormatStrikeID            = "editor.format.strike"
	commandEditorFormatParagraphID         = "editor.format.paragraph"
	commandEditorFormatHeading1ID          = "editor.format.heading.h1"
	commandEditorFormatHeading2ID          = "editor.format.heading.h2"
	commandEditorFormatHeading3ID          = "editor.format.heading.h3"
	commandEditorFormatHeading4ID          = "editor.format.heading.h4"
	commandEditorFormatHeading5ID          = "editor.format.heading.h5"
	commandEditorFormatHeading6ID          = "editor.format.heading.h6"
	commandEditorFormatBlockquoteID        = "editor.format.blockquote"
	commandEditorFormatCodeBlockID         = "editor.format.code_block"
	commandEditorFormatListBulletID        = "editor.format.list.bullet"
	commandEditorFormatListOrderedID       = "editor.format.list.ordered"
	commandEditorFormatClearMarksID        = "editor.format.clear_marks"
	commandEditorFormatLinkRemoveID        = "editor.format.link.remove"
	commandEditorFormatTableRowBeforeID    = "editor.format.table.row.before"
	commandEditorFormatTableRowAfterID     = "editor.format.table.row.after"
	commandEditorFormatTableRowDeleteID    = "editor.format.table.row.delete"
	commandEditorFormatTableColumnBeforeID = "editor.format.table.column.before"
	commandEditorFormatTableColumnAfterID  = "editor.format.table.column.after"
	commandEditorFormatTableColumnDeleteID = "editor.format.table.column.delete"
	commandEditorFormatTableHeaderRowID    = "editor.format.table.header.row"
	commandEditorFormatTableHeaderColumnID = "editor.format.table.header.column"
	commandEditorFormatTableHeaderCellID   = "editor.format.table.header.cell"
	commandEditorFormatTableMergeID        = "editor.format.table.merge"
	commandEditorFormatTableSplitID        = "editor.format.table.split"
	commandEditorFormatTableDeleteID       = "editor.format.table.delete"
	commandEditorFormatLinkSetID           = "editor.format.link.set"
	commandEditorFormatTableInsertID       = "editor.format.table.insert"
	commandEditorFormatCodeBlockInsertID   = "editor.format.code_block.insert"
	commandEditorFormatMermaidInsertID     = "editor.format.mermaid.insert"
	commandEditorSlideInsertBasicID        = "editor.slide.insert.basic"
	commandEditorSlideInsertTitleID        = "editor.slide.insert.title"
	commandEditorSlideInsertTwoColumnsID   = "editor.slide.insert.two_columns"
	commandEditorSlideInsertImageRightID   = "editor.slide.insert.image_right"
	commandEditorSlideInsertImageLeftID    = "editor.slide.insert.image_left"
	commandEditorSlideInsertSectionID      = "editor.slide.insert.section"
	commandEditorSlideInsertAgendaID       = "editor.slide.insert.agenda"
	commandEditorSlideInsertQuoteID        = "editor.slide.insert.quote"
	commandEditorSlideInsertComparisonID   = "editor.slide.insert.comparison"
	commandEditorSlideInsertCodeID         = "editor.slide.insert.code"
	commandEditorSlideInsertDiagramID      = "editor.slide.insert.diagram"
)

var commandEditorFormatIDs = []string{
	commandEditorMermaidApplyID,
	commandEditorMermaidRemoveID,
	commandEditorFormatBoldID,
	commandEditorFormatItalicID,
	commandEditorFormatStrikeID,
	commandEditorFormatParagraphID,
	commandEditorFormatHeading1ID,
	commandEditorFormatHeading2ID,
	commandEditorFormatHeading3ID,
	commandEditorFormatHeading4ID,
	commandEditorFormatHeading5ID,
	commandEditorFormatHeading6ID,
	commandEditorFormatBlockquoteID,
	commandEditorFormatCodeBlockID,
	commandEditorFormatListBulletID,
	commandEditorFormatListOrderedID,
	commandEditorFormatClearMarksID,
	commandEditorFormatLinkRemoveID,
	commandEditorFormatTableRowBeforeID,
	commandEditorFormatTableRowAfterID,
	commandEditorFormatTableRowDeleteID,
	commandEditorFormatTableColumnBeforeID,
	commandEditorFormatTableColumnAfterID,
	commandEditorFormatTableColumnDeleteID,
	commandEditorFormatTableHeaderRowID,
	commandEditorFormatTableHeaderColumnID,
	commandEditorFormatTableHeaderCellID,
	commandEditorFormatTableMergeID,
	commandEditorFormatTableSplitID,
	commandEditorFormatTableDeleteID,
	commandEditorFormatLinkSetID,
	commandEditorFormatTableInsertID,
	commandEditorFormatCodeBlockInsertID,
	commandEditorFormatMermaidInsertID,
	commandEditorSlideInsertBasicID,
	commandEditorSlideInsertTitleID,
	commandEditorSlideInsertTwoColumnsID,
	commandEditorSlideInsertImageRightID,
	commandEditorSlideInsertImageLeftID,
	commandEditorSlideInsertSectionID,
	commandEditorSlideInsertAgendaID,
	commandEditorSlideInsertQuoteID,
	commandEditorSlideInsertComparisonID,
	commandEditorSlideInsertCodeID,
	commandEditorSlideInsertDiagramID,
}

func isEditorFormatCommand(commandID string) bool {
	for _, id := range commandEditorFormatIDs {
		if id == commandID {
			return true
		}
	}
	return false
}

func isEditorFormatDialogCommand(commandID string) bool {
	return commandID == commandEditorFormatLinkSetID || commandID == commandEditorFormatTableInsertID || commandID == commandEditorMermaidRemoveID
}

func editorFormatCommandRegistration(commandID string) (commandcatalog.Definition, commandcatalog.HandlerContract) {
	name := map[string][3]string{
		commandEditorMermaidApplyID:            {"Aplicar diagrama Mermaid", "Apply Mermaid diagram", "Aplicar diagrama Mermaid"},
		commandEditorMermaidRemoveID:           {"Remover diagrama Mermaid", "Remove Mermaid diagram", "Eliminar diagrama Mermaid"},
		commandEditorFormatBoldID:              {"Negrito", "Bold", "Negrita"},
		commandEditorFormatItalicID:            {"Itálico", "Italic", "Cursiva"},
		commandEditorFormatStrikeID:            {"Tachado", "Strikethrough", "Tachado"},
		commandEditorFormatParagraphID:         {"Parágrafo", "Paragraph", "Párrafo"},
		commandEditorFormatHeading1ID:          {"Título 1", "Heading 1", "Título 1"},
		commandEditorFormatHeading2ID:          {"Título 2", "Heading 2", "Título 2"},
		commandEditorFormatHeading3ID:          {"Título 3", "Heading 3", "Título 3"},
		commandEditorFormatHeading4ID:          {"Título 4", "Heading 4", "Título 4"},
		commandEditorFormatHeading5ID:          {"Título 5", "Heading 5", "Título 5"},
		commandEditorFormatHeading6ID:          {"Título 6", "Heading 6", "Título 6"},
		commandEditorFormatBlockquoteID:        {"Citação", "Blockquote", "Cita"},
		commandEditorFormatCodeBlockID:         {"Bloco de código", "Code block", "Bloque de código"},
		commandEditorFormatListBulletID:        {"Lista com marcadores", "Bullet list", "Lista con viñetas"},
		commandEditorFormatListOrderedID:       {"Lista numerada", "Ordered list", "Lista numerada"},
		commandEditorFormatClearMarksID:        {"Limpar formatação", "Clear formatting", "Limpiar formato"},
		commandEditorFormatLinkRemoveID:        {"Remover link", "Remove link", "Quitar enlace"},
		commandEditorFormatTableRowBeforeID:    {"Adicionar linha acima", "Add row above", "Añadir fila arriba"},
		commandEditorFormatTableRowAfterID:     {"Adicionar linha abaixo", "Add row below", "Añadir fila abajo"},
		commandEditorFormatTableRowDeleteID:    {"Excluir linha", "Delete row", "Eliminar fila"},
		commandEditorFormatTableColumnBeforeID: {"Adicionar coluna à esquerda", "Add column left", "Añadir columna a la izquierda"},
		commandEditorFormatTableColumnAfterID:  {"Adicionar coluna à direita", "Add column right", "Añadir columna a la derecha"},
		commandEditorFormatTableColumnDeleteID: {"Excluir coluna", "Delete column", "Eliminar columna"},
		commandEditorFormatTableHeaderRowID:    {"Alternar cabeçalho da linha", "Toggle header row", "Alternar encabezado de fila"},
		commandEditorFormatTableHeaderColumnID: {"Alternar cabeçalho da coluna", "Toggle header column", "Alternar encabezado de columna"},
		commandEditorFormatTableHeaderCellID:   {"Alternar cabeçalho da célula", "Toggle header cell", "Alternar encabezado de celda"},
		commandEditorFormatTableMergeID:        {"Mesclar células", "Merge cells", "Combinar celdas"},
		commandEditorFormatTableSplitID:        {"Dividir célula", "Split cell", "Dividir celda"},
		commandEditorFormatTableDeleteID:       {"Excluir tabela", "Delete table", "Eliminar tabla"},
		commandEditorFormatLinkSetID:           {"Definir link", "Set link", "Definir enlace"},
		commandEditorFormatTableInsertID:       {"Inserir tabela", "Insert table", "Insertar tabla"},
		commandEditorFormatCodeBlockInsertID:   {"Inserir bloco de código", "Insert code block", "Insertar bloque de código"},
		commandEditorFormatMermaidInsertID:     {"Inserir diagrama Mermaid", "Insert Mermaid diagram", "Insertar diagrama Mermaid"},
		commandEditorSlideInsertBasicID:        {"Inserir slide básico", "Insert basic slide", "Insertar diapositiva básica"},
		commandEditorSlideInsertTitleID:        {"Inserir slide de título", "Insert title slide", "Insertar diapositiva de título"},
		commandEditorSlideInsertTwoColumnsID:   {"Inserir slide de duas colunas", "Insert two-column slide", "Insertar diapositiva de dos columnas"},
		commandEditorSlideInsertImageRightID:   {"Inserir slide com imagem à direita", "Insert slide with image on right", "Insertar diapositiva con imagen a la derecha"},
		commandEditorSlideInsertImageLeftID:    {"Inserir slide com imagem à esquerda", "Insert slide with image on left", "Insertar diapositiva con imagen a la izquierda"},
		commandEditorSlideInsertSectionID:      {"Inserir slide de seção", "Insert section slide", "Insertar diapositiva de sección"},
		commandEditorSlideInsertAgendaID:       {"Inserir slide de agenda", "Insert agenda slide", "Insertar diapositiva de agenda"},
		commandEditorSlideInsertQuoteID:        {"Inserir slide de citação", "Insert quote slide", "Insertar diapositiva de cita"},
		commandEditorSlideInsertComparisonID:   {"Inserir slide de comparação", "Insert comparison slide", "Insertar diapositiva de comparación"},
		commandEditorSlideInsertCodeID:         {"Inserir slide de código", "Insert code slide", "Insertar diapositiva de código"},
		commandEditorSlideInsertDiagramID:      {"Inserir slide de diagrama", "Insert diagram slide", "Insertar diapositiva de diagrama"},
	}
	labels, ok := name[commandID]
	if !ok {
		return commandcatalog.Definition{}, commandcatalog.HandlerContract{}
	}
	const formatPrefix = "editor.format."
	const slidePrefix = "editor.slide.insert."
	routePrefix := "ui/editor/format/"
	presentationVersion := "editor-format-v2"
	category := "Editor"
	description := [3]string{
		"Aplica a formatação na seleção do editor ativo",
		"Applies formatting to the active editor selection",
		"Aplica formato a la selección del editor activo",
	}
	if strings.HasPrefix(commandID, slidePrefix) {
		routePrefix = "ui/editor/slide/insert/"
		presentationVersion = "editor-slide-v1"
		category = "Slides"
		description = [3]string{
			"Insere um slide no editor de apresentações ativo",
			"Inserts a slide in the active presentation editor",
			"Inserta una diapositiva en el editor de presentaciones activo",
		}
	} else if commandID == commandEditorMermaidApplyID || commandID == commandEditorMermaidRemoveID {
		routePrefix = "ui/editor/mermaid/"
		presentationVersion = "editor-mermaid-v1"
		description = [3]string{"Altera o diagrama Mermaid capturado no editor ativo", "Changes the captured Mermaid diagram in the active editor", "Modifica el diagrama Mermaid capturado en el editor activo"}
	} else if !strings.HasPrefix(commandID, formatPrefix) {
		return commandcatalog.Definition{}, commandcatalog.HandlerContract{}
	}
	if strings.HasPrefix(commandID, formatPrefix) {
		const markdownDescription = "Aplica formatação ao conteúdo do editor ativo, inclusive em Markdown"
		const markdownDescriptionEN = "Applies formatting to active editor content, including Markdown"
		const markdownDescriptionES = "Aplica formato al contenido del editor activo, incluido Markdown"
		if commandID == commandEditorFormatTableInsertID || commandID == commandEditorFormatCodeBlockInsertID ||
			commandID == commandEditorFormatMermaidInsertID || commandID == commandEditorFormatListBulletID ||
			commandID == commandEditorFormatListOrderedID || commandID == commandEditorFormatBlockquoteID {
			description = [3]string{markdownDescription, markdownDescriptionEN, markdownDescriptionES}
		}
	}
	routeSuffix := strings.TrimPrefix(commandID, formatPrefix)
	if strings.HasPrefix(commandID, "editor.mermaid.") {
		routeSuffix = strings.TrimPrefix(commandID, "editor.mermaid.")
	}
	if strings.HasPrefix(commandID, slidePrefix) {
		routeSuffix = strings.TrimPrefix(commandID, slidePrefix)
	}
	contract := commandcatalog.HandlerContract{
		Effect: commandcatalog.Write, HasMutableTarget: true,
		Route:          routePrefix + routeSuffix,
		Classification: commandcatalog.HandlerUI,
	}
	return commandcatalog.Definition{
		ID: commandID, Effect: commandcatalog.Write, Decision: commandcatalog.NoDecision, HasMutableTarget: true,
		AllowedSources: []commandcatalog.Source{commandcatalog.Palette, commandcatalog.KeyboardLocal, commandcatalog.StreamDeck},
		Context:        commandcatalog.ContextPolicy{Facts: []commandcatalog.ContextFact{{Provider: "workspace", Fact: "active_tab", Mode: commandcatalog.ExactVersion}}},
		Presentation: &commandcatalog.Presentation{Version: presentationVersion, Locales: map[string]commandcatalog.LocalizedMetadata{
			"pt-BR": {Name: labels[0], Description: description[0], Category: category},
			"en":    {Name: labels[1], Description: description[1], Category: category},
			"es":    {Name: labels[2], Description: description[2], Category: category},
		}},
		ArgumentsSchema: &commandcatalog.Schema{Type: commandcatalog.SchemaObject}, ResultSchema: &commandcatalog.Schema{Type: commandcatalog.SchemaObject},
		Risk: commandcatalog.RiskLow, Persistence: commandcatalog.PersistencePolicy{Arguments: commandcatalog.PersistenceNever, Result: commandcatalog.PersistenceNever, Audit: commandcatalog.PersistenceRedacted},
		Scopes: []commandcatalog.Scope{commandcatalog.ScopeWorkspace}, Availability: commandcatalog.Availability{Status: commandcatalog.Available}, HandlerRoute: contract.Route, HandlerClassification: contract.Classification,
	}, contract
}

func isAuditedUIContextualCommand(commandID string) bool {
	return isEditorFormatCommand(commandID) || isChatMessageUICommand(commandID)
}
