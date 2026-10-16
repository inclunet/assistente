package commandexecution

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandbindings"
)

func TestHostJobProjectionIdentityGuardRefreshPreservesPointerAndVersions(t *testing.T) {
	state, principal, _ := readyHostForRebuild(t)
	configuration, _, _ := claimProjectionConfigurations(t, "persisted:identity")
	equivalentConfiguration := configuration.WithValidityDeadline(configuration.ValidUntil())
	if equivalentConfiguration == configuration || !configuration.Equivalent(equivalentConfiguration) {
		t.Fatal("fixture precisa fornecer snapshots distintos com configuração totalmente equivalente")
	}
	ctx := context.Background()
	oldGuardErr := errors.New("old projection guard is stale")
	oldGuardCurrent := true
	var oldGuardCalls int
	oldGuard := func(context.Context) error {
		oldGuardCalls++
		if !oldGuardCurrent {
			return oldGuardErr
		}
		return nil
	}
	if err := state.RebuildUserConfigurationGuarded(ctx,
		func(context.Context) (auth.LocalSessionPrincipal, error) { return principal, nil },
		func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
			return configuration, []string{"application"}, nil
		}, oldGuard); err != nil {
		t.Fatal(err)
	}
	before, err := state.Snapshot(ctx, principal)
	if err != nil {
		t.Fatal(err)
	}
	beforeConfiguration, _, err := state.UserConfiguration(ctx, principal.UserID)
	if err != nil {
		t.Fatal(err)
	}

	oldGuardCurrent = false
	if _, err := state.Snapshot(ctx, principal); !errors.Is(err, oldGuardErr) {
		t.Fatalf("snapshot deveria aplicar e rejeitar guard antigo: %v", err)
	}
	var newGuardCalls int
	newGuard := func(context.Context) error {
		newGuardCalls++
		return nil
	}
	if err := state.RebuildUserConfigurationForJobClaimProjection(ctx,
		func(context.Context) (auth.LocalSessionPrincipal, error) { return principal, nil },
		func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
			return equivalentConfiguration, []string{"application"}, nil
		}, newGuard, func() bool { return true }); err != nil {
		t.Fatal(err)
	}

	after, err := state.Snapshot(ctx, principal)
	if err != nil {
		t.Fatalf("snapshot não aplicou o guard novo: %v (old guard calls=%d)", err, oldGuardCalls)
	}
	afterConfiguration, _, err := state.UserConfiguration(ctx, principal.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if afterConfiguration != beforeConfiguration {
		t.Fatalf("refresh somente do guard substituiu configuração: before=%p after=%p", beforeConfiguration, afterConfiguration)
	}
	if after != before {
		t.Fatalf("refresh somente do guard alterou versões: before=%+v after=%+v", before, after)
	}
	if newGuardCalls == 0 {
		t.Fatal("guard novo não foi observado")
	}
}

func TestHostJobProjectionDeadlineIdentityRefreshReplacesPointerAndPreservesVersions(t *testing.T) {
	state, principal, _ := readyHostForRebuild(t)
	base, _, _ := claimProjectionConfigurations(t, "persisted:deadline-identity")
	initial := base.WithValidityDeadline(time.Now().Add(time.Minute))
	refreshed := base.WithValidityDeadline(time.Now().Add(2 * time.Minute))
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
	beforeConfiguration, _, err := state.UserConfiguration(ctx, principal.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if beforeConfiguration != initial {
		t.Fatalf("snapshot inicial=%p want=%p", beforeConfiguration, initial)
	}
	if err := state.RebuildUserConfigurationForJobClaimProjection(ctx,
		func(context.Context) (auth.LocalSessionPrincipal, error) { return principal, nil },
		func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
			return refreshed, []string{"application"}, nil
		}, func(context.Context) error { return nil }, func() bool { return true }); err != nil {
		t.Fatal(err)
	}
	after, err := state.Snapshot(ctx, principal)
	if err != nil {
		t.Fatal(err)
	}
	afterConfiguration, _, err := state.UserConfiguration(ctx, principal.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if afterConfiguration != refreshed || afterConfiguration == beforeConfiguration {
		t.Fatalf("refresh de deadline não substituiu somente o snapshot: before=%p after=%p want=%p", beforeConfiguration, afterConfiguration, refreshed)
	}
	if after != before {
		t.Fatalf("refresh de deadline alterou versões: before=%+v after=%+v", before, after)
	}
	if afterConfiguration.ValidUntil() != refreshed.ValidUntil() {
		t.Fatalf("deadline persistido=%v want=%v", afterConfiguration.ValidUntil(), refreshed.ValidUntil())
	}
}
