package app

import "assistente/internal/database"

// SetDesktopDatabasePath fixa antes do startup o banco reservado pelo desktop.
// É função de composição, não método exportado para os bindings da interface.
func SetDesktopDatabasePath(a *App, path string) {
	a.desktopDatabasePath = path
}

func (a *App) initDesktopDatabase() error {
	if a.desktopDatabasePath != "" {
		return database.InitPath(a.desktopDatabasePath)
	}
	return InitDatabase()
}
