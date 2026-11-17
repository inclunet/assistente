package app

import (
	"context"

	"assistente/controllers"
	"assistente/internal/authorization"
	"assistente/internal/database"
	"assistente/internal/questionnaire"
	"assistente/internal/tools/invocationctx"
	"assistente/internal/wailsapi"
)

func SetPermissionsAPI(a *App, api *wailsapi.Permissions) {
	if a != nil {
		a.permissionsAPI = api
	}
}

func (a *App) wirePermissions() {
	controller := &controllers.PermissionsController{
		Repository: authorization.NewRepository(database.DB),
		Sessions:   []controllers.SessionPermissions{a.netTrustMgr, a.fsTrustMgr, a.shellTrustMgr, a.acpTrust},
		Prepare:    a.preparePermissions,
	}
	wailsapi.AttachPermissions(a.permissionsAPI, wailsSession{app: a}, controller)
}

func (a *App) preparePermissions(ctx context.Context) []error {
	var failures []error
	if _, err := a.allowlistMgr.List(ctx); err != nil {
		failures = append(failures, &controllers.PermissionPreparationError{Domain: "shell", Err: err})
	}
	profiles, err := a.profileManager.List()
	if err != nil {
		return append(failures, &controllers.PermissionPreparationError{Domain: "profiles", Err: err})
	}
	// Somente perfis conhecidos e o workspace aberto. Não há busca em discos
	// nem troca de banco para encontrar autorizações esquecidas.
	slugs := []string{""}
	for _, profile := range profiles {
		slugs = append(slugs, profile.Slug)
	}
	for _, slug := range slugs {
		origin := invocationctx.With(ctx, invocationctx.InvocationContext{ProfileSlug: slug})
		if _, err := a.netTrustMgr.List(origin); err != nil {
			failures = append(failures, &controllers.PermissionPreparationError{Domain: "network", Profile: slug, Err: err})
		}
		if _, err := a.fsTrustMgr.List(origin); err != nil {
			failures = append(failures, &controllers.PermissionPreparationError{Domain: "filesystem", Profile: slug, Err: err})
		}
	}
	if _, err := a.acpTrust.Profiles(ctx); err != nil {
		failures = append(failures, &controllers.PermissionPreparationError{Domain: "acp", Err: err})
	}
	return failures
}

// O evento não carrega recurso, comando nem erro do banco. O log técnico fica
// no autorizador; a UI recebe só o aviso não bloqueante no idioma escolhido.
func (a *App) authorizationPersistenceFailed(_ context.Context, req authorization.Request, _ error) {
	if a.emitter != nil {
		a.emitter.Emit("authorization:notice", authorization.Notice{Domain: req.Domain})
	}
}

func (a *App) askOperationAuthorization(ctx context.Context, payload questionnaire.RequestPayload) (questionnaire.Response, error) {
	inv, _ := invocationctx.Get(ctx)
	surface := resolveProfileAccessSurface(ctx, inv.Source, inv.ConversationID, database.GetConversationInfoWithContext)
	return a.questionnaireRouter().Ask(ctx, surface, payload)
}
func (a *App) authorizationScopes(ctx context.Context, scopes []authorization.Scope) []authorization.Scope {
	inv, _ := invocationctx.Get(ctx)
	surface := resolveProfileAccessSurface(ctx, inv.Source, inv.ConversationID, database.GetConversationInfoWithContext)
	if !surface.AllowsPersistentAuthorization() {
		return []authorization.Scope{authorization.ScopeOnce}
	}
	return scopes
}
