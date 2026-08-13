package main

import (
	"assistente/controllers"
	"assistente/internal/app"
	"assistente/internal/profiles"
)

// cliApp adapta *app.App para as interfaces da CLI após AEP-0088: métodos de
// profiles saíram do Bind Wails e vivem em ProfilesController / wailsapi.Profiles.
type cliApp struct {
	*app.App
}

func asCLI(a *app.App) cliApp {
	return cliApp{App: a}
}

func (c cliApp) profiles() *controllers.ProfilesController {
	return app.ProfilesCtrl(c.App)
}

func (c cliApp) GetProfiles() ([]profiles.ProfileInfo, error) {
	return c.profiles().GetProfiles()
}

func (c cliApp) GetActiveProfileSlug() string {
	return c.profiles().GetActiveProfileSlug()
}

func (c cliApp) GetProfile(slug string) (*profiles.Profile, error) {
	return c.profiles().GetProfile(slug)
}

func (c cliApp) GetActiveProfile() (*profiles.Profile, error) {
	return c.profiles().GetActiveProfile()
}

func (c cliApp) GetActiveProfileAndSlug() (*profiles.ActiveProfile, error) {
	return c.profiles().GetActiveProfileAndSlug()
}

func (c cliApp) SetActiveProfile(slug string) error {
	return c.profiles().SetActiveProfile(slug)
}

func (c cliApp) CreateProfile(p profiles.Profile) (string, error) {
	return c.profiles().CreateProfile(p)
}

func (c cliApp) UpdateProfile(slug string, p profiles.Profile) error {
	return c.profiles().UpdateProfile(slug, p)
}

func (c cliApp) DuplicateProfile(slug string) (string, error) {
	return c.profiles().DuplicateProfile(slug)
}

func (c cliApp) DeleteProfile(slug string) error {
	return c.profiles().DeleteProfile(slug)
}
