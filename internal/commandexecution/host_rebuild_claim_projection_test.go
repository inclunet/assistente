package commandexecution

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandbindings"
)

func claimProjectionConfigurations(t *testing.T, baseline string) (*commandbindings.Configuration, *commandbindings.Configuration, *commandbindings.ExecutionDependency) {
	t.Helper()
	selected := commandbindings.Candidate{ID: "binding.workspace-list", Trigger: "palette:workspace.list", CommandID: "workspace.list", ArgumentsKey: `{}`, ExecutionScopeKey: "global", Scope: commandbindings.Global, Enabled: true, LayerActive: true, LayerRef: "application"}
	base, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{selected})
	if err != nil {
		t.Fatal(err)
	}
	base, err = base.WithPersistedBaseline(baseline)
	if err != nil {
		t.Fatal(err)
	}
	result, err := base.Resolve(selected.Trigger, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	dependency, err := base.CaptureExecutionDependency(selected.Trigger, nil, nil, result)
	if err != nil {
		t.Fatal(err)
	}
	jobBinding := commandbindings.Candidate{ID: "binding.job-layer", Trigger: "palette:settings", CommandID: "settings.open", ArgumentsKey: `{}`, ExecutionScopeKey: "global", Scope: commandbindings.Global, Enabled: true, LayerActive: true, LayerRef: "job.layer"}
	next, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{selected, jobBinding})
	if err != nil {
		t.Fatal(err)
	}
	next, err = next.WithPersistedBaseline(baseline)
	if err != nil {
		t.Fatal(err)
	}
	return base, next, dependency
}

func TestHostClaimProjectionPreservesOnlyProvenExecutionAndRevalidatesAuthority(t *testing.T) {
	state, principal, _ := readyHostForRebuild(t)
	base, next, dependency := claimProjectionConfigurations(t, "persisted:unchanged")
	ctx := context.Background()
	if err := state.RebuildUserConfiguration(ctx,
		func(context.Context) (auth.LocalSessionPrincipal, error) { return principal, nil },
		func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
			return base, []string{"application"}, nil
		}); err != nil {
		t.Fatal(err)
	}
	before, err := state.Snapshot(ctx, principal)
	if err != nil {
		t.Fatal(err)
	}
	epoch, err := state.Epochs().Capture(ctx, principal.UserID, principal.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	var preserved, unproven context.Context
	releasePreserved, err := state.Epochs().AdmitExecutionWithProjectionProof(ctx, epoch, dependency.UnaffectedBy,
		func(context.Context) error { return nil }, func(run context.Context) error { preserved = run; return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer releasePreserved()
	releaseUnproven, err := state.Epochs().AdmitExecution(ctx, epoch,
		func(context.Context) error { return nil }, func(run context.Context) error { unproven = run; return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer releaseUnproven()
	authCalls := 0
	err = state.RebuildUserConfigurationForJobClaimProjection(ctx,
		func(context.Context) (auth.LocalSessionPrincipal, error) { authCalls++; return principal, nil },
		func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
			return next, []string{"application", "job.layer"}, nil
		},
		func(context.Context) error { return nil }, func() bool { return true })
	if err != nil || authCalls != 2 {
		t.Fatalf("rebuild seletivo = %v, autenticações=%d", err, authCalls)
	}
	select {
	case <-preserved.Done():
		t.Fatal("testemunho equivalente foi cancelado")
	default:
	}
	select {
	case <-unproven.Done():
	default:
		t.Fatal("watch sem testemunho sobreviveu ao refresh de claims")
	}
	_, layers, after, err := state.ResolutionSnapshot(ctx, principal)
	if err != nil || len(layers) != 2 || layers[1] != "job.layer" || before.GlobalConfig != after.GlobalConfig || before.ActiveLayers == after.ActiveLayers {
		t.Fatalf("snapshot após claim = layers=%v before=%+v after=%+v err=%v", layers, before, after, err)
	}
}

func TestHostClaimProjectionRejectsPersistedBaseChangeAndCancelsConservatively(t *testing.T) {
	state, principal, _ := readyHostForRebuild(t)
	base, next, dependency := claimProjectionConfigurations(t, "persisted:before")
	ctx := context.Background()
	if err := state.RebuildUserConfiguration(ctx,
		func(context.Context) (auth.LocalSessionPrincipal, error) { return principal, nil },
		func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
			return base, []string{"application"}, nil
		}); err != nil {
		t.Fatal(err)
	}
	changedBase, err := next.WithPersistedBaseline("persisted:after")
	if err != nil {
		t.Fatal(err)
	}
	epoch, err := state.Epochs().Capture(ctx, principal.UserID, principal.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	var run context.Context
	release, err := state.Epochs().AdmitExecutionWithProjectionProof(ctx, epoch, dependency.UnaffectedBy,
		func(context.Context) error { return nil }, func(watched context.Context) error { run = watched; return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	err = state.RebuildUserConfigurationForJobClaimProjection(ctx,
		func(context.Context) (auth.LocalSessionPrincipal, error) { return principal, nil },
		func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
			return changedBase, []string{"application", "job.layer"}, nil
		},
		func(context.Context) error { return nil }, func() bool { return true })
	if !errors.Is(err, ErrJobProjectionBaseChanged) {
		t.Fatalf("mudança da base persistida = %v", err)
	}
	select {
	case <-run.Done():
	default:
		t.Fatal("falha de publicação deixou watch antigo vivo")
	}
	configuration, layers, err := state.UserConfiguration(ctx, principal.UserID)
	if err != nil || configuration != base || len(layers) != 1 || layers[0] != "application" {
		t.Fatalf("falha parcial alterou estado do host: config=%p layers=%v err=%v", configuration, layers, err)
	}
}

func TestHostClaimProjectionReauthenticatesAfterBuild(t *testing.T) {
	state, principal, _ := readyHostForRebuild(t)
	base, next, dependency := claimProjectionConfigurations(t, "persisted:unchanged")
	ctx := context.Background()
	if err := state.RebuildUserConfiguration(ctx,
		func(context.Context) (auth.LocalSessionPrincipal, error) { return principal, nil },
		func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
			return base, []string{"application"}, nil
		}); err != nil {
		t.Fatal(err)
	}
	epoch, err := state.Epochs().Capture(ctx, principal.UserID, principal.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	var run context.Context
	release, err := state.Epochs().AdmitExecutionWithProjectionProof(ctx, epoch, dependency.UnaffectedBy,
		func(context.Context) error { return nil }, func(watched context.Context) error { run = watched; return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	other := principal
	other.SessionID = hostUUID(t)
	authCalls := 0
	err = state.RebuildUserConfigurationForJobClaimProjection(ctx,
		func(context.Context) (auth.LocalSessionPrincipal, error) {
			authCalls++
			if authCalls == 1 {
				return principal, nil
			}
			return other, nil
		},
		func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
			return next, []string{"application", "job.layer"}, nil
		},
		func(context.Context) error { return nil }, func() bool { return true })
	if err == nil || authCalls != 2 {
		t.Fatalf("autenticação mudou durante build: err=%v calls=%d", err, authCalls)
	}
	select {
	case <-run.Done():
		t.Fatal("reautenticação recusada cancelou execução antes da publicação")
	default:
	}
}

func TestHostClaimProjectionLeaseDeadlineRefreshKeepsGenerationAndWatch(t *testing.T) {
	state, principal, _ := readyHostForRebuild(t)
	base, _, dependency := claimProjectionConfigurations(t, "persisted:unchanged")
	now := time.Now()
	initial := base.WithValidityDeadline(now.Add(time.Minute))
	refreshed := base.WithValidityDeadline(now.Add(2 * time.Minute))
	ctx := context.Background()
	if err := state.RebuildUserConfiguration(ctx,
		func(context.Context) (auth.LocalSessionPrincipal, error) { return principal, nil },
		func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
			return initial, []string{"application"}, nil
		}); err != nil {
		t.Fatal(err)
	}
	before, err := state.Snapshot(ctx, principal)
	if err != nil {
		t.Fatal(err)
	}
	epoch, err := state.Epochs().Capture(ctx, principal.UserID, principal.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	var run context.Context
	release, err := state.Epochs().AdmitExecutionWithProjectionProof(ctx, epoch, dependency.UnaffectedBy,
		func(context.Context) error { return nil }, func(watched context.Context) error { run = watched; return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if err := state.RebuildUserConfigurationForJobClaimProjection(ctx,
		func(context.Context) (auth.LocalSessionPrincipal, error) { return principal, nil },
		func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
			return refreshed, []string{"application"}, nil
		}, func(context.Context) error { return nil }, func() bool { return true }); err != nil {
		t.Fatal(err)
	}
	after, err := state.Snapshot(ctx, principal)
	if err != nil || after != before {
		t.Fatalf("renewal de lease alterou gerações: before=%+v after=%+v err=%v", before, after, err)
	}
	configuration, _, err := state.UserConfiguration(ctx, principal.UserID)
	if err != nil || configuration != refreshed {
		t.Fatalf("snapshot não recebeu o novo deadline: config=%p want=%p err=%v", configuration, refreshed, err)
	}
	if configuration.ValidUntil() != refreshed.ValidUntil() {
		t.Fatalf("deadline persistido=%v want=%v", configuration.ValidUntil(), refreshed.ValidUntil())
	}
	select {
	case <-run.Done():
		t.Fatal("renovação do deadline cancelou execução")
	default:
	}
}
