package jobs

import (
	"context"
	"errors"
	"strings"

	"assistente/internal/commandcatalog"
	"assistente/internal/commandidentity"
	"assistente/internal/commandjobactivation"
	"assistente/internal/database"
	"assistente/internal/jobprofilegrant"
)

var errCommandJobServiceUnavailable = errors.New("identidade privada do job indisponível")

var _ commandidentity.JobRuntime = (*Manager)(nil)

type commandJobServiceMarkerKey struct{}

type commandJobServiceMarkerlessContext struct{ context.Context }

func (c commandJobServiceMarkerlessContext) Value(key any) any {
	if _, ok := key.(commandJobServiceMarkerKey); ok {
		return nil
	}
	return c.Context.Value(key)
}

func withoutCommandJobServiceMarker(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return commandJobServiceMarkerlessContext{Context: ctx}
}

// commandJobServiceMarker é criado somente pelo executor real. Nenhum campo
// de eventctx, invocationctx ou envelope público participa de sua montagem.
type commandJobServiceMarker struct {
	manager               *Manager
	lifetime              context.Context
	runID                 string
	jobDatabaseID         string
	jobSlug               string
	ownerUserID           string
	profileSlug           string
	definitionFingerprint string
	delegationFingerprint string
	grantGeneration       uint64
	runtimeIdentity       commandjobactivation.RuntimeIdentity

	capability commandidentity.JobServiceCapability
}

// commandJobServiceContext fixa o alvo persistido, o fingerprint executado,
// o grant e a identidade viva do run. Para jobs sem profile literal de
// subagent, preserva o comportamento legado sem instalar identidade.
func (m *Manager) commandJobServiceContext(ctx context.Context, job *Job, runID string, lifetime context.Context) (context.Context, error) {
	if m == nil || ctx == nil || job == nil || lifetime == nil || strings.TrimSpace(runID) == "" {
		return nil, errCommandJobServiceUnavailable
	}
	profile, delegationFingerprint, ok := literalJobProfile(job)
	if !ok {
		return ctx, nil
	}
	if m.cfg.JobProfileGrants == nil {
		return ctx, nil
	}

	owner, err := database.RequireUserID(ctx)
	managerOwner, managerOwnerOK := database.UserIDFromContext(m.context())
	if err != nil || !managerOwnerOK || managerOwner != owner {
		return nil, errCommandJobServiceUnavailable
	}
	m.commandRuntimeMu.Lock()
	entry, live := m.commandRuntime[runID]
	m.commandRuntimeMu.Unlock()
	if !live || entry.watchCtx == nil || entry.watchCtx.Err() != nil || entry.identity.UserID != owner {
		return ctx, nil
	}
	snapshot, err := m.PrepareCommandJob(ctx, job.DatabaseID)
	if err != nil || snapshot == nil || snapshot.ID != job.ID {
		return nil, errCommandJobServiceUnavailable
	}
	executedFingerprint, fingerprintErr := DefinitionFingerprint(job)
	definitionFingerprint, err := DefinitionFingerprint(snapshot)
	if fingerprintErr != nil || err != nil || definitionFingerprint != executedFingerprint {
		return nil, errCommandJobServiceUnavailable
	}
	snapshotProfile, snapshotDelegationFingerprint, ok := literalJobProfile(snapshot)
	if !ok || snapshotProfile != profile || snapshotDelegationFingerprint != delegationFingerprint {
		return nil, errCommandJobServiceUnavailable
	}
	authorization, err := m.cfg.JobProfileGrants.AuthorizationSnapshot(ctx, snapshot.DatabaseID, profile)
	if err != nil || authorization.Config.JobID != snapshot.DatabaseID || authorization.Config.JobSlug != snapshot.ID || authorization.Config.Fingerprint != delegationFingerprint {
		return nil, errCommandJobServiceUnavailable
	}
	marker := &commandJobServiceMarker{
		manager: m, lifetime: lifetime, runID: runID, jobDatabaseID: snapshot.DatabaseID,
		jobSlug: snapshot.ID, ownerUserID: owner, profileSlug: profile,
		definitionFingerprint: definitionFingerprint, delegationFingerprint: delegationFingerprint,
		grantGeneration: authorization.Generation, runtimeIdentity: entry.identity,
		capability: commandidentity.JobServiceCapabilityForRuntime(),
	}
	return context.WithValue(ctx, commandJobServiceMarkerKey{}, marker), nil
}

// CommandJobServiceRequest constrói a request da identidade somente a partir
// do marker privado do run. O capability é capturado por identidade, não por
// igualdade estrutural.
func (m *Manager) CommandJobServiceRequest(ctx context.Context, source commandcatalog.Source) (commandidentity.JobServiceRequest, error) {
	marker, err := m.commandJobServiceMarker(ctx)
	if err != nil || source != commandcatalog.Event {
		return commandidentity.JobServiceRequest{}, errCommandJobServiceUnavailable
	}
	return commandidentity.JobServiceRequest{
		Capability: marker.capability, JobDatabaseID: marker.jobDatabaseID,
		TargetProfileSlug: marker.profileSlug, RunID: marker.runID, Source: source,
	}, nil
}

func (m *Manager) ResolveCommandJob(ctx context.Context, capability commandidentity.JobServiceCapability, request commandidentity.JobServiceRequest) (commandidentity.TrustedJob, error) {
	marker, err := m.commandJobServiceMarker(ctx)
	if err != nil || request.Capability == nil || request.Capability != capability || request.Source != commandcatalog.Event {
		return commandidentity.TrustedJob{}, commandidentity.ErrJobExecutionDenied
	}
	if marker.capability != capability || request.JobDatabaseID != marker.jobDatabaseID || request.TargetProfileSlug != marker.profileSlug || request.RunID != marker.runID {
		return commandidentity.TrustedJob{}, commandidentity.ErrJobExecutionDenied
	}
	_, err = m.currentCommandJob(ctx, marker)
	if err != nil {
		return commandidentity.TrustedJob{}, commandidentity.ErrJobExecutionDenied
	}
	valid, err := m.cfg.JobProfileGrants.HasValidGeneration(database.WithUserID(ctx, marker.ownerUserID), marker.jobDatabaseID, marker.profileSlug, marker.delegationFingerprint, marker.grantGeneration)
	if err != nil || !valid {
		return commandidentity.TrustedJob{}, commandidentity.ErrJobExecutionDenied
	}
	if _, err := m.commandJobServiceMarker(ctx); err != nil {
		return commandidentity.TrustedJob{}, commandidentity.ErrJobExecutionDenied
	}
	return marker.trustedJob(), nil
}

func (m *Manager) RevalidateCommandJob(ctx context.Context, capability commandidentity.JobServiceCapability, trusted commandidentity.TrustedJob) error {
	marker, err := m.commandJobServiceMarker(ctx)
	if err != nil || capability == nil {
		return commandidentity.ErrJobExecutionDenied
	}
	if marker.capability != capability || !marker.matchesTrustedJob(trusted) {
		return commandidentity.ErrJobExecutionDenied
	}
	job, err := m.currentCommandJob(ctx, marker)
	if err != nil {
		return commandidentity.ErrJobExecutionDenied
	}
	valid, err := m.cfg.JobProfileGrants.HasValidGeneration(database.WithUserID(ctx, marker.ownerUserID), marker.jobDatabaseID, marker.profileSlug, marker.delegationFingerprint, marker.grantGeneration)
	if err != nil || !valid || job == nil {
		return commandidentity.ErrJobExecutionDenied
	}
	if _, err := m.commandJobServiceMarker(ctx); err != nil {
		return commandidentity.ErrJobExecutionDenied
	}
	return nil
}

func (m *Manager) commandJobServiceMarker(ctx context.Context) (*commandJobServiceMarker, error) {
	if m == nil || ctx == nil || ctx.Err() != nil {
		return nil, errCommandJobServiceUnavailable
	}
	marker, ok := ctx.Value(commandJobServiceMarkerKey{}).(*commandJobServiceMarker)
	if !ok || marker == nil || marker.manager != m || marker.lifetime == nil || marker.lifetime.Err() != nil {
		return nil, errCommandJobServiceUnavailable
	}
	owner, err := database.RequireUserID(ctx)
	if err != nil || owner != marker.ownerUserID {
		return nil, errCommandJobServiceUnavailable
	}
	m.commandRuntimeMu.Lock()
	entry, live := m.commandRuntime[marker.runID]
	m.commandRuntimeMu.Unlock()
	if !live || entry.watchCtx == nil || entry.watchCtx.Err() != nil || entry.identity != marker.runtimeIdentity {
		return nil, errCommandJobServiceUnavailable
	}
	return marker, nil
}

func (m *Manager) currentCommandJob(ctx context.Context, marker *commandJobServiceMarker) (*Job, error) {
	owner, err := database.RequireUserID(ctx)
	if err != nil || owner != marker.ownerUserID {
		return nil, errCommandJobServiceUnavailable
	}
	job, err := m.PrepareCommandJob(ctx, marker.jobDatabaseID)
	if err != nil || job == nil || job.ID != marker.jobSlug {
		return nil, errCommandJobServiceUnavailable
	}
	fingerprint, err := DefinitionFingerprint(job)
	if err != nil || fingerprint != marker.definitionFingerprint {
		return nil, errCommandJobServiceUnavailable
	}
	profile, delegationFingerprint, ok := literalJobProfile(job)
	if !ok || profile != marker.profileSlug || delegationFingerprint != marker.delegationFingerprint {
		return nil, errCommandJobServiceUnavailable
	}
	return job, nil
}

func (marker *commandJobServiceMarker) trustedJob() commandidentity.TrustedJob {
	return commandidentity.TrustedJob{DatabaseID: marker.jobDatabaseID, Slug: marker.jobSlug, OwnerUserID: marker.ownerUserID, TargetProfileSlug: marker.profileSlug, JobDefinitionFingerprint: marker.definitionFingerprint, DelegationFingerprint: marker.delegationFingerprint, GrantGeneration: marker.grantGeneration, RunID: marker.runID}
}

func (marker *commandJobServiceMarker) matchesTrustedJob(job commandidentity.TrustedJob) bool {
	return job.DatabaseID == marker.jobDatabaseID && job.Slug == marker.jobSlug && job.OwnerUserID == marker.ownerUserID && job.TargetProfileSlug == marker.profileSlug && job.JobDefinitionFingerprint == marker.definitionFingerprint && job.DelegationFingerprint == marker.delegationFingerprint && job.GrantGeneration == marker.grantGeneration && job.RunID == marker.runID
}

func literalJobProfile(job *Job) (string, string, bool) {
	if job == nil {
		return "", "", false
	}
	fingerprint, ok := jobprofilegrant.FingerprintForInputs(job.Tool, job.Inputs)
	if !ok {
		return "", "", false
	}
	profile, ok := job.Inputs["profile"].(string)
	profile = strings.TrimSpace(profile)
	return profile, fingerprint, ok && profile != "" && !strings.Contains(profile, "{{") && !strings.Contains(profile, "}}") && !strings.ContainsRune(profile, '\x00')
}
