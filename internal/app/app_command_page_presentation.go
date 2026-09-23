package app

import "assistente/internal/commandcatalog"

// These commands only present existing controls; process and CRUD effects are excluded.
var commandProductPagePresentation = []commandUINavigation{
	{id: "tasklist.task.create.open", route: "ui/tasklist/task/create/open", uiAction: true, ptName: "Abrir criação de tarefa na lista", ptDesc: "Abrir formulário de tarefa na lista ativa do workspace", enName: "Open new task form in list", enDesc: "Open a task form in the active workspace list", esName: "Abrir creación de tarea en la lista", esDesc: "Abrir formulario de tarea en la lista activa del espacio de trabajo"},
	{id: "tasklists.create.open", route: "ui/tasklists/create/open", uiAction: true, ptName: "Abrir criação de lista de tarefas", ptDesc: "Abrir criação de lista de tarefas", enName: "Open new task list form", enDesc: "Open new task list form", esName: "Abrir formulario de lista de tareas", esDesc: "Abrir formulario de lista de tareas"},
	{id: "tasklists.edit.open", route: "ui/tasklists/edit/open", uiAction: true, ptName: "Editar lista de tarefas selecionada", ptDesc: "Editar lista de tarefas selecionada", enName: "Edit selected task list", enDesc: "Edit selected task list", esName: "Editar lista de tareas seleccionada", esDesc: "Editar lista de tareas seleccionada"},
	{id: "tasklists.search.focus", route: "ui/tasklists/search/focus", uiAction: true, ptName: "Focar busca de listas de tarefas", ptDesc: "Focar busca de listas de tarefas", enName: "Focus task list search", enDesc: "Focus task list search", esName: "Enfocar búsqueda de listas de tareas", esDesc: "Enfocar búsqueda de listas de tareas"},
	{id: "profiles.create.open", route: "ui/profiles/create/open", uiAction: true, ptName: "Abrir criação de perfil", ptDesc: "Abrir criação de perfil", enName: "Open new profile form", enDesc: "Open new profile form", esName: "Abrir formulario de perfil", esDesc: "Abrir formulario de perfil"},
	{id: "profiles.edit.open", route: "ui/profiles/edit/open", uiAction: true, ptName: "Editar perfil selecionado", ptDesc: "Editar perfil selecionado", enName: "Edit selected profile", enDesc: "Edit selected profile", esName: "Editar perfil seleccionado", esDesc: "Editar perfil seleccionado"},
	{id: "profiles.search.focus", route: "ui/profiles/search/focus", uiAction: true, ptName: "Focar busca de perfis", ptDesc: "Focar busca de perfis", enName: "Focus profile search", enDesc: "Focus profile search", esName: "Enfocar búsqueda de perfiles", esDesc: "Enfocar búsqueda de perfiles"},
	{id: "terminal.sessions.open", route: "ui/terminal/sessions/open", uiAction: true, ptName: "Abrir seletor de sessões do terminal", ptDesc: "Abrir seletor de sessões do terminal", enName: "Open terminal session picker", enDesc: "Open terminal session picker", esName: "Abrir selector de sesiones del terminal", esDesc: "Abrir selector de sesiones del terminal"},
	{id: "terminal.focus.input", route: "ui/terminal/focus/input", uiAction: true, ptName: "Focar entrada do terminal", ptDesc: "Focar entrada do terminal", enName: "Focus terminal input", enDesc: "Focus terminal input", esName: "Enfocar entrada del terminal", esDesc: "Enfocar entrada del terminal"},
	{id: "terminal.focus.history", route: "ui/terminal/focus/history", uiAction: true, ptName: "Focar histórico do terminal", ptDesc: "Focar histórico do terminal", enName: "Focus terminal history", enDesc: "Focus terminal history", esName: "Enfocar historial del terminal", esDesc: "Enfocar historial del terminal"},
}

func commandPagePresentationRegistrations() []commandcatalog.Registration {
	return commandLocalUIRegistrations(commandProductPagePresentation, "Página ativa", "Active page", "Página activa", "page-presentation-v1")
}
