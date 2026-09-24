package app

import "assistente/internal/commandcatalog"

const commandWorkspacePanelFocusID = "workspace.panel.focus"

type commandUINavigation struct {
	uiAction  bool
	id        string
	route     string
	ptName    string
	ptDesc    string
	ptAliases []string
	enName    string
	enDesc    string
	enAliases []string
	esName    string
	esDesc    string
	esAliases []string
}

var commandProductUINavigation = []commandUINavigation{
	{id: "navigation.landmark.next", route: "ui/navigation/landmark/next", ptName: "Próxima região", ptDesc: "Move o foco para a próxima região disponível", ptAliases: []string{"região", "foco", "F6"}, enName: "Next region", enDesc: "Moves focus to the next available region", enAliases: []string{"landmark", "focus", "F6"}, esName: "Siguiente región", esDesc: "Mueve el foco a la siguiente región disponible", esAliases: []string{"región", "foco", "F6"}},
	{id: "navigation.landmark.previous", route: "ui/navigation/landmark/previous", ptName: "Região anterior", ptDesc: "Move o foco para a região anterior disponível", ptAliases: []string{"região", "foco", "Shift+F6"}, enName: "Previous region", enDesc: "Moves focus to the previous available region", enAliases: []string{"landmark", "focus", "Shift+F6"}, esName: "Región anterior", esDesc: "Mueve el foco a la región anterior disponible", esAliases: []string{"región", "foco", "Shift+F6"}},
	{id: "navigation.landmark.default", route: "ui/navigation/landmark/default", ptName: "Região padrão", ptDesc: "Move o foco para a região padrão da superfície ativa", ptAliases: []string{"região", "foco"}, enName: "Default region", enDesc: "Moves focus to the default region of the active surface", enAliases: []string{"landmark", "focus"}, esName: "Región predeterminada", esDesc: "Mueve el foco a la región predeterminada de la superficie activa", esAliases: []string{"región", "foco"}},
	{id: "navigation.workspace.open", route: "ui/navigation/workspace/open", ptName: "Abrir workspace", ptDesc: "Abre a página inicial do workspace", ptAliases: []string{"início", "workspace"}, enName: "Open workspace", enDesc: "Opens the workspace home page", enAliases: []string{"home", "workspace"}, esName: "Abrir workspace", esDesc: "Abre la página de inicio del workspace", esAliases: []string{"inicio", "espacio de trabajo"}},
	{id: "navigation.history.open", route: "ui/navigation/history/open", ptName: "Abrir histórico", ptDesc: "Abre o histórico", ptAliases: []string{"histórico", "history"}, enName: "Open history", enDesc: "Opens history", enAliases: []string{"history", "recent"}, esName: "Abrir historial", esDesc: "Abre el historial", esAliases: []string{"historial", "history"}},
	{id: "navigation.memories.open", route: "ui/navigation/memories/open", ptName: "Abrir memórias", ptDesc: "Abre as memórias", ptAliases: []string{"memórias", "memory"}, enName: "Open memories", enDesc: "Opens memories", enAliases: []string{"memories", "memory"}, esName: "Abrir memorias", esDesc: "Abre las memorias", esAliases: []string{"memorias", "memory"}},
	{id: "navigation.tasklists.open", route: "ui/navigation/tasklists/open", ptName: "Abrir listas de tarefas", ptDesc: "Abre as listas de tarefas", ptAliases: []string{"tarefas", "tasklists"}, enName: "Open task lists", enDesc: "Opens task lists", enAliases: []string{"tasks", "tasklists"}, esName: "Abrir listas de tareas", esDesc: "Abre las listas de tareas", esAliases: []string{"tareas", "tasklists"}},
	{id: "navigation.jobs.open", route: "ui/navigation/jobs/open", ptName: "Abrir jobs", ptDesc: "Abre os jobs", ptAliases: []string{"jobs", "tarefas agendadas"}, enName: "Open jobs", enDesc: "Opens jobs", enAliases: []string{"jobs", "scheduled tasks"}, esName: "Abrir jobs", esDesc: "Abre los jobs", esAliases: []string{"jobs", "tareas programadas"}},
	{id: "navigation.profiles.open", route: "ui/navigation/profiles/open", ptName: "Abrir perfis", ptDesc: "Abre os perfis", ptAliases: []string{"perfis", "profiles"}, enName: "Open profiles", enDesc: "Opens profiles", enAliases: []string{"profiles", "personas"}, esName: "Abrir perfiles", esDesc: "Abre los perfiles", esAliases: []string{"perfiles", "profiles"}},
	{id: "navigation.settings.open", route: "ui/navigation/settings/open", ptName: "Abrir configurações", ptDesc: "Abre as configurações", ptAliases: []string{"configurações", "settings"}, enName: "Open settings", enDesc: "Opens settings", enAliases: []string{"settings", "preferences"}, esName: "Abrir configuración", esDesc: "Abre la configuración", esAliases: []string{"configuración", "settings"}},
	{id: "navigation.help.open", route: "ui/navigation/help/open", ptName: "Abrir ajuda", ptDesc: "Abre a ajuda", ptAliases: []string{"ajuda", "help"}, enName: "Open help", enDesc: "Opens help", enAliases: []string{"help", "support"}, esName: "Abrir ayuda", esDesc: "Abre la ayuda", esAliases: []string{"ayuda", "help"}},
	{id: "navigation.about.open", route: "ui/navigation/about/open", ptName: "Abrir sobre", ptDesc: "Abre informações sobre o Assistente", ptAliases: []string{"sobre", "about"}, enName: "Open about", enDesc: "Opens information about Assistente", enAliases: []string{"about", "version"}, esName: "Abrir acerca de", esDesc: "Abre información sobre Assistente", esAliases: []string{"acerca de", "about"}},
	{id: "navigation.menu.open", route: "ui/navigation/menu/open", ptName: "Abrir menu", ptDesc: "Abre o menu principal", ptAliases: []string{"menu", "navegação"}, enName: "Open menu", enDesc: "Opens the main menu", enAliases: []string{"menu", "navigation"}, esName: "Abrir menú", esDesc: "Abre el menú principal", esAliases: []string{"menú", "navegación"}},
	{id: "navigation.palette.open", route: "ui/navigation/palette/open", ptName: "Abrir paleta de comandos", ptDesc: "Abre a paleta de comandos", ptAliases: []string{"paleta", "comandos"}, enName: "Open command palette", enDesc: "Opens the command palette", enAliases: []string{"palette", "commands"}, esName: "Abrir paleta de comandos", esDesc: "Abre la paleta de comandos", esAliases: []string{"paleta", "comandos"}},
	{id: "navigation.data.export.open", route: "ui/navigation/data/export/open", ptName: "Abrir exportação de dados", ptDesc: "Abre a exportação de dados", ptAliases: []string{"exportar", "exportação", "dados"}, enName: "Open data export", enDesc: "Opens data export", enAliases: []string{"export", "data"}, esName: "Abrir exportación de datos", esDesc: "Abre la exportación de datos", esAliases: []string{"exportar", "exportación", "datos"}},
	{id: "navigation.data.import.open", route: "ui/navigation/data/import/open", ptName: "Abrir importação de dados", ptDesc: "Abre a importação de dados", ptAliases: []string{"importar", "importação", "dados"}, enName: "Open data import", enDesc: "Opens data import", enAliases: []string{"import", "data"}, esName: "Abrir importación de datos", esDesc: "Abre la importación de datos", esAliases: []string{"importar", "importación", "datos"}},
}

func commandNavigationRegistrations() []commandcatalog.Registration {
	return commandLocalUIRegistrations(commandProductUINavigation, "Navegação", "Navigation", "Navegación", "navigation-v1")
}

var commandProductChatPickers = []commandUINavigation{
	{id: "chat.focus.input", route: "ui/chat/focus/input", uiAction: true, ptName: "Focar campo de mensagem", ptDesc: "Move o foco para o campo de mensagem do chat", ptAliases: []string{"chat", "mensagem", "navegação"}, enName: "Focus message input", enDesc: "Moves focus to the chat message input", enAliases: []string{"chat", "message", "navigation"}, esName: "Enfocar campo de mensaje", esDesc: "Mueve el foco al campo de mensaje del chat", esAliases: []string{"chat", "mensaje", "navegación"}},
	{id: "chat.focus.messages", route: "ui/chat/focus/messages", uiAction: true, ptName: "Focar mensagens", ptDesc: "Move o foco para as mensagens do chat", ptAliases: []string{"chat", "mensagem", "navegação"}, enName: "Focus messages", enDesc: "Moves focus to the chat messages", enAliases: []string{"chat", "message", "navigation"}, esName: "Enfocar mensajes", esDesc: "Mueve el foco a los mensajes del chat", esAliases: []string{"chat", "mensaje", "navegación"}},
	{id: "chat.message.read.open", route: "ui/chat/message/read/open", uiAction: true, ptName: "Abrir leitura da mensagem", ptDesc: "Abre a visualização de leitura da mensagem, sem iniciar fala", ptAliases: []string{"chat", "mensagem", "navegação"}, enName: "Open message reading", enDesc: "Opens the message reading view without starting speech", enAliases: []string{"chat", "message", "navigation"}, esName: "Abrir lectura del mensaje", esDesc: "Abre la vista de lectura del mensaje sin iniciar voz", esAliases: []string{"chat", "mensaje", "navegación"}},
	{id: "chat.message.menu.open", route: "ui/chat/message/menu/open", uiAction: true, ptName: "Abrir menu da mensagem", ptDesc: "Abre o menu de ações da mensagem selecionada", ptAliases: []string{"chat", "mensagem", "navegação"}, enName: "Open message menu", enDesc: "Opens the selected message action menu", enAliases: []string{"chat", "message", "navigation"}, esName: "Abrir menú del mensaje", esDesc: "Abre el menú de acciones del mensaje seleccionado", esAliases: []string{"chat", "mensaje", "navegación"}},
	{id: "chat.message.reasoning.toggle", route: "ui/chat/message/reasoning/toggle", uiAction: true, ptName: "Alternar exibição do raciocínio", ptDesc: "Mostra ou oculta o raciocínio da mensagem selecionada", ptAliases: []string{"chat", "mensagem", "navegação"}, enName: "Toggle reasoning visibility", enDesc: "Shows or hides the selected message reasoning", enAliases: []string{"chat", "message", "navigation"}, esName: "Alternar visibilidad del razonamiento", esDesc: "Muestra u oculta el razonamiento del mensaje seleccionado", esAliases: []string{"chat", "mensaje", "navegación"}},
	{id: "chat.message.thread.expand", route: "ui/chat/message/thread/expand", uiAction: true, ptName: "Expandir respostas da mensagem", ptDesc: "Expande a exibição das respostas da mensagem selecionada", ptAliases: []string{"chat", "mensagem", "navegação"}, enName: "Expand message thread", enDesc: "Expands the selected message thread", enAliases: []string{"chat", "message", "navigation"}, esName: "Expandir respuestas del mensaje", esDesc: "Expande las respuestas del mensaje seleccionado", esAliases: []string{"chat", "mensaje", "navegación"}},
	{id: "chat.message.thread.collapse", route: "ui/chat/message/thread/collapse", uiAction: true, ptName: "Recolher respostas da mensagem", ptDesc: "Recolhe a exibição das respostas da mensagem selecionada", ptAliases: []string{"chat", "mensagem", "navegação"}, enName: "Collapse message thread", enDesc: "Collapses the selected message thread", enAliases: []string{"chat", "message", "navigation"}, esName: "Contraer respuestas del mensaje", esDesc: "Contrae las respuestas del mensaje seleccionado", esAliases: []string{"chat", "mensaje", "navegación"}},
	{id: commandMessageEditID, route: "ui/chat/message/edit/open", ptName: "Abrir edição da mensagem", ptDesc: "Abre o editor da mensagem selecionada, sem salvar alterações", ptAliases: []string{"editar", "mensagem"}, enName: "Open message editor", enDesc: "Opens the selected message editor without saving changes", enAliases: []string{"edit", "message"}, esName: "Abrir edición del mensaje", esDesc: "Abre el editor del mensaje seleccionado sin guardar cambios", esAliases: []string{"editar", "mensaje"}},
	{id: commandChatPinnedOpenID, route: "ui/chat/pinned/open", ptName: "Mensagens fixadas", ptDesc: "Abre as mensagens fixadas da conversa", ptAliases: []string{"fixadas", "chat"}, enName: "Pinned messages", enDesc: "Opens the conversation's pinned messages", enAliases: []string{"pinned", "chat"}, esName: "Mensajes fijados", esDesc: "Abre los mensajes fijados de la conversación", esAliases: []string{"fijados", "chat"}},
	{id: commandChatTokensOpenID, route: "ui/chat/tokens/open", ptName: "Estatísticas de tokens", ptDesc: "Abre as estatísticas de tokens da conversa", ptAliases: []string{"tokens", "uso de tokens", "chat"}, enName: "Token statistics", enDesc: "Opens the conversation's token statistics", enAliases: []string{"tokens", "token usage", "chat"}, esName: "Estadísticas de tokens", esDesc: "Abre las estadísticas de tokens de la conversación", esAliases: []string{"tokens", "uso de tokens", "chat"}},
	{id: "chat.model.open", route: "ui/chat/model/open", ptName: "Abrir seletor de modelo", ptDesc: "Abre o seletor de modelo do chat", ptAliases: []string{"modelo", "modelos", "chat"}, enName: "Open model picker", enDesc: "Opens the chat model picker", enAliases: []string{"model", "models", "chat"}, esName: "Abrir selector de modelo", esDesc: "Abre el selector de modelo del chat", esAliases: []string{"modelo", "modelos", "chat"}},
	{id: "chat.history.open", route: "ui/chat/history/open", ptName: "Abrir histórico do chat", ptDesc: "Abre o histórico do chat", ptAliases: []string{"histórico", "chat"}, enName: "Open chat history", enDesc: "Opens chat history", enAliases: []string{"history", "chat"}, esName: "Abrir historial del chat", esDesc: "Abre el historial del chat", esAliases: []string{"historial", "chat"}},
	{id: "chat.profile.open", route: "ui/chat/profile/open", ptName: "Abrir seletor de perfil", ptDesc: "Abre o seletor de perfil do chat", ptAliases: []string{"perfil", "perfis", "chat"}, enName: "Open profile picker", enDesc: "Opens the chat profile picker", enAliases: []string{"profile", "profiles", "chat"}, esName: "Abrir selector de perfil", esDesc: "Abre el selector de perfil del chat", esAliases: []string{"perfil", "perfiles", "chat"}},
}

var commandProductEditorMenus = []commandUINavigation{
	{id: "editor.mermaid.open", route: "ui/editor/mermaid/open", ptName: "Editar diagrama Mermaid", ptDesc: "Abre o editor do diagrama Mermaid selecionado", ptAliases: []string{"diagrama", "Mermaid"}, enName: "Edit Mermaid diagram", enDesc: "Opens the selected Mermaid diagram editor", enAliases: []string{"diagram", "Mermaid"}, esName: "Editar diagrama Mermaid", esDesc: "Abre el editor del diagrama Mermaid seleccionado", esAliases: []string{"diagrama", "Mermaid"}},
	{id: "editor.menu.insert.open", route: "ui/editor/menu/insert/open", ptName: "Abrir menu Inserir do editor", ptDesc: "Abre o menu Inserir do editor", ptAliases: []string{"inserir", "editor"}, enName: "Open Insert menu", enDesc: "Opens the editor Insert menu", enAliases: []string{"insert", "editor"}, esName: "Abrir menú Insertar", esDesc: "Abre el menú Insertar del editor", esAliases: []string{"insertar", "editor"}},
	{id: "editor.menu.file.open", route: "ui/editor/menu/file/open", ptName: "Abrir menu Arquivo do editor", ptDesc: "Abre o menu Arquivo do editor", ptAliases: []string{"arquivo", "editor"}, enName: "Open File menu", enDesc: "Opens the editor File menu", enAliases: []string{"file", "editor"}, esName: "Abrir menú Archivo", esDesc: "Abre el menú Archivo del editor", esAliases: []string{"archivo", "editor"}},
	{id: "editor.menu.format.open", route: "ui/editor/menu/format/open", ptName: "Abrir menu Formatar do editor", ptDesc: "Abre o menu Formatar do editor", ptAliases: []string{"formatar", "editor"}, enName: "Open Format menu", enDesc: "Opens the editor Format menu", enAliases: []string{"format", "editor"}, esName: "Abrir menú Formato", esDesc: "Abre el menú Formato del editor", esAliases: []string{"formato", "editor"}},
	{id: "editor.menu.mode.open", route: "ui/editor/menu/mode/open", ptName: "Abrir menu Modo do editor", ptDesc: "Abre o menu de modo do editor", ptAliases: []string{"modo", "editor"}, enName: "Open Mode menu", enDesc: "Opens the editor Mode menu", enAliases: []string{"mode", "editor"}, esName: "Abrir menú Modo", esDesc: "Abre el menú de modo del editor", esAliases: []string{"modo", "editor"}},
	{id: "editor.slides.open", route: "ui/editor/slides/open", ptName: "Abrir seletor de slides do editor", ptDesc: "Abre a apresentação do documento no editor", ptAliases: []string{"slides", "apresentação", "editor"}, enName: "Open slides", enDesc: "Opens the document slides in the editor", enAliases: []string{"slides", "presentation", "editor"}, esName: "Abrir presentación", esDesc: "Abre la presentación del documento en el editor", esAliases: []string{"diapositivas", "presentación", "editor"}},
	{id: "editor.presentation.fullscreen", route: "ui/editor/presentation/fullscreen", ptName: "Alternar tela cheia da apresentação", ptDesc: "Alterna a apresentação do editor em tela cheia", ptAliases: []string{"tela cheia", "apresentação", "editor"}, enName: "Fullscreen presentation", enDesc: "Toggles the editor presentation fullscreen", enAliases: []string{"fullscreen", "presentation", "editor"}, esName: "Presentación en pantalla completa", esDesc: "Alterna la presentación del editor en pantalla completa", esAliases: []string{"pantalla completa", "presentación", "editor"}},
	{id: "editor.table.cell.next", route: "ui/editor/table/cell/next", ptName: "Próxima célula da tabela", ptDesc: "Avança para a próxima célula da tabela", ptAliases: []string{"próxima célula", "tabela", "editor"}, enName: "Next table cell", enDesc: "Moves to the next table cell", enAliases: []string{"next cell", "table", "editor"}, esName: "Siguiente celda de la tabla", esDesc: "Avanza a la siguiente celda de la tabla", esAliases: []string{"siguiente celda", "tabla", "editor"}},
	{id: "editor.table.cell.previous", route: "ui/editor/table/cell/previous", ptName: "Célula anterior da tabela", ptDesc: "Volta para a célula anterior da tabela", ptAliases: []string{"célula anterior", "tabela", "editor"}, enName: "Previous table cell", enDesc: "Moves to the previous table cell", enAliases: []string{"previous cell", "table", "editor"}, esName: "Celda anterior de la tabla", esDesc: "Vuelve a la celda anterior de la tabla", esAliases: []string{"celda anterior", "tabla", "editor"}},
}

func commandChatPickerRegistrations() []commandcatalog.Registration {
	return commandLocalUIRegistrations(commandProductChatPickers, "Chat", "Chat", "Chat", "chat-picker-v1")
}

func commandEditorMenuRegistrations() []commandcatalog.Registration {
	return commandLocalUIRegistrations(commandProductEditorMenus, "Editor", "Editor", "Editor", "editor-menu-v1")
}

func commandLocalUIRegistrations(items []commandUINavigation, ptCategory, enCategory, esCategory, presentationVersion string) []commandcatalog.Registration {
	registrations := make([]commandcatalog.Registration, 0, len(items))
	for _, item := range items {
		sources := []commandcatalog.Source{commandcatalog.Palette, commandcatalog.KeyboardLocal, commandcatalog.StreamDeck}
		if item.uiAction || commandExternalUICommandSupported(item.id) {
			sources = append(sources, commandcatalog.UI)
		}
		contract := commandcatalog.HandlerContract{Effect: commandcatalog.Read, Route: item.route, Classification: commandcatalog.HandlerUI}
		registrations = append(registrations, commandcatalog.Registration{
			Definition: commandcatalog.Definition{
				ID: item.id, Effect: commandcatalog.Read, Decision: commandcatalog.NoDecision,
				AllowedSources: sources, Context: commandcatalog.ContextPolicy{None: true},
				Presentation: &commandcatalog.Presentation{Version: presentationVersion, Locales: map[string]commandcatalog.LocalizedMetadata{
					"pt-BR": {Name: item.ptName, Description: item.ptDesc, Category: ptCategory, Aliases: item.ptAliases},
					"en":    {Name: item.enName, Description: item.enDesc, Category: enCategory, Aliases: item.enAliases},
					"es":    {Name: item.esName, Description: item.esDesc, Category: esCategory, Aliases: item.esAliases},
				}},
				ArgumentsSchema: &commandcatalog.Schema{Type: commandcatalog.SchemaObject}, ResultSchema: &commandcatalog.Schema{Type: commandcatalog.SchemaObject},
				Risk: commandcatalog.RiskLow, Persistence: commandcatalog.PersistencePolicy{Arguments: commandcatalog.PersistenceNever, Result: commandcatalog.PersistenceNever, Audit: commandcatalog.PersistenceNever},
				Scopes: []commandcatalog.Scope{commandcatalog.ScopeSession}, Availability: commandcatalog.Availability{Status: commandcatalog.Available}, HandlerRoute: contract.Route, HandlerClassification: contract.Classification,
			},
			Handler: contract,
		})
	}
	return registrations
}

// Este allowlist espelha o dispatcher externo do Topbar: somente as rotas
// declaradas em commandNavigation.ts e a navegação de abas têm executor ali.
// Comandos com handlers locais/contextuais continuam fora de AllowedSources UI
// até que o dispatcher os aceite explicitamente.
func commandExternalUICommandSupported(commandID string) bool {
	switch commandID {
	case "navigation.workspace.open", "navigation.history.open", "navigation.memories.open",
		"navigation.tasklists.open", "navigation.jobs.open", "navigation.profiles.open",
		"navigation.settings.open", "navigation.data.export.open", "navigation.data.import.open",
		"navigation.help.open", "navigation.about.open",
		commandWorkspaceTabNextID, commandWorkspaceTabPreviousID, commandWorkspaceTabFirstID,
		commandWorkspaceTabSecondID, commandWorkspaceTabThirdID, commandWorkspaceTabFourthID,
		commandWorkspaceTabFifthID, commandWorkspaceTabSixthID, commandWorkspaceTabSeventhID,
		commandWorkspaceTabEighthID, commandWorkspaceTabNinthID:
		return true
	default:
		return false
	}
}

// O foco é uma capacidade síncrona exclusivamente visual: o backend registra a
// intenção e entrega o handoff, enquanto a UI aplica a capacidade ao painel
// ativo no instante do efeito.
func commandWorkspacePanelFocusRegistration() (commandcatalog.Definition, commandcatalog.HandlerContract) {
	contract := commandcatalog.HandlerContract{Effect: commandcatalog.Read, Route: "ui/workspace/panel/focus", Classification: commandcatalog.HandlerUI}
	return commandcatalog.Definition{
		ID:             commandWorkspacePanelFocusID,
		Effect:         commandcatalog.Read,
		Decision:       commandcatalog.NoDecision,
		AllowedSources: []commandcatalog.Source{commandcatalog.Palette, commandcatalog.KeyboardLocal, commandcatalog.StreamDeck},
		Context:        commandcatalog.ContextPolicy{None: true},
		Presentation: &commandcatalog.Presentation{Version: "workspace-panel-focus-v1", Locales: map[string]commandcatalog.LocalizedMetadata{
			"pt-BR": {Name: "Focar painel ativo", Description: "Foca o conteúdo da aba ativa; disponível somente no workspace com painel pronto", Category: "Workspace", Aliases: []string{"foco", "painel"}},
			"en":    {Name: "Focus active panel", Description: "Focuses the active tab's content; available only in a workspace with a ready panel", Category: "Workspace", Aliases: []string{"focus", "panel"}},
			"es":    {Name: "Enfocar panel activo", Description: "Enfoca el contenido de la pestaña activa; disponible solo en el espacio de trabajo con un panel listo", Category: "Workspace", Aliases: []string{"enfoque", "panel"}},
		}},
		ArgumentsSchema:       &commandcatalog.Schema{Type: commandcatalog.SchemaObject},
		ResultSchema:          &commandcatalog.Schema{Type: commandcatalog.SchemaObject},
		Risk:                  commandcatalog.RiskLow,
		Persistence:           commandcatalog.PersistencePolicy{Arguments: commandcatalog.PersistenceNever, Result: commandcatalog.PersistenceNever, Audit: commandcatalog.PersistenceNever},
		Scopes:                []commandcatalog.Scope{commandcatalog.ScopeSession},
		Availability:          commandcatalog.Availability{Status: commandcatalog.Available},
		HandlerRoute:          contract.Route,
		HandlerClassification: contract.Classification,
	}, contract
}

// A ajuda é uma leitura apresentada na UI, não uma mutação de domínio.
// O alvo visual é revalidado localmente; nenhum foco é espelhado no FactBus Go.
func commandShortcutsRegistration() (commandcatalog.Definition, commandcatalog.HandlerContract) {
	contract := commandcatalog.HandlerContract{Effect: commandcatalog.Read, Route: "ui/help/shortcuts/show", Classification: commandcatalog.HandlerUI}
	return commandcatalog.Definition{
		ID: commandProductShortcutsShowID, Effect: commandcatalog.Read, Decision: commandcatalog.NoDecision,
		AllowedSources: []commandcatalog.Source{commandcatalog.Palette, commandcatalog.KeyboardLocal, commandcatalog.StreamDeck}, Context: commandcatalog.ContextPolicy{None: true},
		Presentation: &commandcatalog.Presentation{Version: "shortcuts-v1", Locales: map[string]commandcatalog.LocalizedMetadata{
			"pt-BR": {Name: "Mostrar atalhos de teclado", Description: "Abre a ajuda dos atalhos existentes", Category: "Ajuda"},
			"en":    {Name: "Show keyboard shortcuts", Description: "Opens help for existing keyboard shortcuts", Category: "Help"},
			"es":    {Name: "Mostrar atajos de teclado", Description: "Abre la ayuda de los atajos existentes", Category: "Ayuda"},
		}},
		ArgumentsSchema: &commandcatalog.Schema{Type: commandcatalog.SchemaObject},
		ResultSchema:    &commandcatalog.Schema{Type: commandcatalog.SchemaObject},
		Risk:            commandcatalog.RiskLow,
		Persistence:     commandcatalog.PersistencePolicy{Arguments: commandcatalog.PersistenceNever, Result: commandcatalog.PersistenceNever, Audit: commandcatalog.PersistenceNever},
		Scopes:          []commandcatalog.Scope{commandcatalog.ScopeSession}, Availability: commandcatalog.Availability{Status: commandcatalog.Available},
		HandlerRoute: contract.Route, HandlerClassification: contract.Classification,
	}, contract
}
