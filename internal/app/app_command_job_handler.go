package app

import (
	"context"

	"assistente/internal/commandcatalog"
	"assistente/internal/commandexecution"
	"assistente/internal/database"
	"assistente/internal/jobprofilegrant"
	"assistente/internal/jobs"
)

// newCommandJobHandler monta uma rota de alvo fixo do bootstrap. Não é uma
// API Wails nem aceita Authorize do chamador: sessão, owner, profile e grant
// são reconsultados nas dependências reais do App. Alterar alvo/definição ou
// reconceder grant exige remontar a rota e publicar nova versão do catálogo.
// O catálogo parametrizado job.run continua responsável por sua preparação.
func (a *App) newCommandJobHandler(ctx context.Context, definition commandcatalog.Definition, contract commandcatalog.HandlerContract, databaseID string, paths commandcatalog.SensitivePaths) (commandexecution.Handler, error) {
	if a == nil || ctx == nil || ctx.Err() != nil {
		return commandexecution.Handler{}, commandexecution.ErrInvalidConfiguration
	}
	principal, err := a.currentCommandPrincipal()
	if err != nil {
		return commandexecution.Handler{}, err
	}
	owner, err := database.RequireUserID(ctx)
	if err != nil || owner != principal.UserID {
		return commandexecution.Handler{}, commandexecution.ErrDenied
	}
	a.authMu.RLock()
	manager, sessions, credentials := a.jobMgr, a.sessionSvc, a.credMgr
	grants, access := a.jobGrantStore, a.profileAccess
	a.authMu.RUnlock()
	if manager == nil || sessions == nil || credentials == nil {
		return commandexecution.Handler{}, commandexecution.ErrInvalidConfiguration
	}
	job, err := manager.PrepareCommandJob(ctx, databaseID)
	if err != nil {
		return commandexecution.Handler{}, err
	}
	version, err := jobs.DefinitionFingerprint(job)
	if err != nil {
		return commandexecution.Handler{}, err
	}
	// Resolve o alvo antes da confirmação, sem transformar a expressão em
	// herança. O runtime confere o mesmo slug após resolver os inputs reais.
	var target, fingerprint string
	var generation uint64
	if job.Tool == jobprofilegrant.ToolSubagent {
		prepared, err := manager.PrepareCommandProfileTarget(ctx, databaseID)
		if err != nil || prepared.DefinitionFingerprint != version {
			return commandexecution.Handler{}, commandexecution.ErrDenied
		}
		target = prepared.Slug
		if target != "" {
			if grants == nil || access == nil || access.ValidateTarget(ctx, target) != nil {
				return commandexecution.Handler{}, commandexecution.ErrDenied
			}
			snapshot, err := grants.AuthorizationSnapshot(ctx, job.DatabaseID, target)
			if err != nil || snapshot.Config.JobID != job.DatabaseID || snapshot.Config.Fingerprint != jobprofilegrant.Fingerprint(job.Tool, prepared.Expression) {
				return commandexecution.Handler{}, commandexecution.ErrDenied
			}
			fingerprint, generation = snapshot.Config.Fingerprint, snapshot.Generation
		}
	}
	authorize := func(checkCtx context.Context, invocation commandexecution.Invocation, current *jobs.Job) error {
		user, err := database.RequireUserID(checkCtx)
		if err != nil || user != principal.UserID || invocation.Principal != principal || current == nil || current.DatabaseID != job.DatabaseID || !a.commandPrincipalMatches(sessions, credentials, principal) {
			return commandexecution.ErrDenied
		}
		revalidated, err := sessions.RevalidateLocalSession(checkCtx, principal)
		if err != nil || revalidated != principal {
			return commandexecution.ErrDenied
		}
		a.authMu.RLock()
		same := a.jobMgr == manager && (target == "" || a.jobGrantStore == grants && a.profileAccess == access)
		a.authMu.RUnlock()
		if !same {
			return commandexecution.ErrDenied
		}
		if target != "" {
			if access.ValidateTarget(checkCtx, target) != nil {
				return commandexecution.ErrDenied
			}
			valid, err := grants.HasValidGeneration(checkCtx, job.DatabaseID, target, fingerprint, generation)
			if err != nil || !valid {
				return commandexecution.ErrDenied
			}
		}
		if !a.commandPrincipalMatches(sessions, credentials, principal) {
			return commandexecution.ErrDenied
		}
		return checkCtx.Err()
	}
	// Préflight autoritativo; o handler repetirá a mesma política antes da fila
	// e de cada tentativa. Nenhuma confirmação concede um grant de profile.
	if err := authorize(ctx, commandexecution.Invocation{Principal: principal}, job); err != nil {
		return commandexecution.Handler{}, err
	}
	return manager.CommandHandler(jobs.CommandHandlerConfig{
		Definition: definition, Contract: contract, ToolSensitivePaths: paths,
		Target:        jobs.CommandJobTarget{DatabaseID: job.DatabaseID, Slug: job.ID, DefinitionFingerprint: version},
		ProfileTarget: target,
		Authorize:     authorize,
		ResolveOrigin: func(check context.Context, invocation commandexecution.Invocation) (jobs.CommandJobOrigin, error) {
			return a.resolveCommandJobOrigin(check, invocation, manager)
		},
	})
}
