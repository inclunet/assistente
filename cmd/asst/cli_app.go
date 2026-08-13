package main

import (
	"fmt"

	"assistente/controllers"
	"assistente/internal/apidto"
	"assistente/internal/app"
	"assistente/internal/profiles"
)

var errProfilesNotReady = fmt.Errorf("profiles controller não inicializado")
var errCredentialsNotReady = fmt.Errorf("credentials controller não inicializado")

// cliApp adapta *app.App para as interfaces da CLI após AEP-0088: métodos de
// profiles/credentials saíram do Bind Wails e vivem nos controllers / wailsapi.
type cliApp struct {
	*app.App
}

func asCLI(a *app.App) cliApp {
	return cliApp{App: a}
}

func (c cliApp) profiles() (*controllers.ProfilesController, error) {
	ctrl := app.ProfilesCtrl(c.App)
	if ctrl == nil {
		return nil, errProfilesNotReady
	}
	return ctrl, nil
}

func (c cliApp) credentials() (*controllers.CredentialsController, error) {
	ctrl := app.CredentialsCtrl(c.App)
	if ctrl == nil {
		return nil, errCredentialsNotReady
	}
	return ctrl, nil
}

func (c cliApp) GetProfiles() ([]profiles.ProfileInfo, error) {
	ctrl, err := c.profiles()
	if err != nil {
		return nil, err
	}
	return ctrl.GetProfiles()
}

func (c cliApp) GetActiveProfileSlug() string {
	ctrl, err := c.profiles()
	if err != nil {
		return ""
	}
	return ctrl.GetActiveProfileSlug()
}

func (c cliApp) GetProfile(slug string) (*profiles.Profile, error) {
	ctrl, err := c.profiles()
	if err != nil {
		return nil, err
	}
	return ctrl.GetProfile(slug)
}

func (c cliApp) GetActiveProfile() (*profiles.Profile, error) {
	ctrl, err := c.profiles()
	if err != nil {
		return nil, err
	}
	return ctrl.GetActiveProfile()
}

func (c cliApp) GetActiveProfileAndSlug() (*profiles.ActiveProfile, error) {
	ctrl, err := c.profiles()
	if err != nil {
		return nil, err
	}
	return ctrl.GetActiveProfileAndSlug()
}

func (c cliApp) SetActiveProfile(slug string) error {
	ctrl, err := c.profiles()
	if err != nil {
		return err
	}
	return ctrl.SetActiveProfile(slug)
}

func (c cliApp) CreateProfile(p profiles.Profile) (string, error) {
	ctrl, err := c.profiles()
	if err != nil {
		return "", err
	}
	return ctrl.CreateProfile(p)
}

func (c cliApp) UpdateProfile(slug string, p profiles.Profile) error {
	ctrl, err := c.profiles()
	if err != nil {
		return err
	}
	return ctrl.UpdateProfile(slug, p)
}

func (c cliApp) DuplicateProfile(slug string) (string, error) {
	ctrl, err := c.profiles()
	if err != nil {
		return "", err
	}
	return ctrl.DuplicateProfile(slug)
}

func (c cliApp) DeleteProfile(slug string) error {
	ctrl, err := c.profiles()
	if err != nil {
		return err
	}
	return ctrl.DeleteProfile(slug)
}

func (c cliApp) ListCredentials() ([]apidto.CredentialSummary, error) {
	ctrl, err := c.credentials()
	if err != nil {
		return nil, err
	}
	ctx, err := app.AuthenticatedContext(c.App)
	if err != nil {
		return nil, err
	}
	return ctrl.ListCredentialsWithContext(ctx)
}

func (c cliApp) UpsertCredential(input apidto.CredentialInput) error {
	ctrl, err := c.credentials()
	if err != nil {
		return err
	}
	ctx, err := app.AuthenticatedContext(c.App)
	if err != nil {
		return err
	}
	return ctrl.UpsertCredentialWithContext(ctx, input)
}

func (c cliApp) DeleteCredential(pattern string) error {
	ctrl, err := c.credentials()
	if err != nil {
		return err
	}
	ctx, err := app.AuthenticatedContext(c.App)
	if err != nil {
		return err
	}
	return ctrl.DeleteCredentialWithContext(ctx, pattern)
}
