package app

import "context"

// wailsSession adapta *App para wailsapi.Session sem expor o método no Bind Wails.
type wailsSession struct {
	app *App
}

func (s wailsSession) AuthenticatedContext() (context.Context, error) {
	return s.app.requireAuthenticatedContext()
}

func (s wailsSession) AdminContext() (context.Context, error) {
	return s.app.requireAdminContext()
}
