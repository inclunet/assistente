package commandidentity

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/auth"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandsecurity"
)

// A barreira representa uma revogação concluída depois da autenticação com
// rede, mas antes da captura no gate. A callback deve reler o vínculo.
type beforeCaptureEpochs struct {
	EpochPort
	before func()
}

func (p *beforeCaptureEpochs) CaptureContextAuthenticated(ctx context.Context, authenticate func() (ContextPrincipal, error)) (Epoch, error) {
	p.before()
	return p.EpochPort.CaptureContextAuthenticated(ctx, authenticate)
}

func TestExternalRevalidatesMappingBeforeCapture(t *testing.T) {
	ctx := context.Background()
	db := commandIdentityDB(t)
	user := commandIdentityUser(t, db)
	repo := auth.NewExternalIdentityRepository(db)
	if _, err := repo.Create(ctx, auth.ExternalIdentityMappingParams{Issuer: "issuer", Subject: "subject", UserID: user.ID}); err != nil {
		t.Fatal(err)
	}
	verifier := &cachedClaimsStub{claims: &auth.ExternalClaims{Issuer: "issuer", Subject: "subject"}}
	external := auth.NewExternalCommandAuthenticator(verifier, repo)
	external.SetReadiness(repo.CheckReadiness)
	core, err := commandsecurity.NewEpochService(&commandsecurity.DispatchGate{})
	if err != nil {
		t.Fatal(err)
	}
	port, err := NewCoreEpochs(core)
	if err != nil {
		t.Fatal(err)
	}
	epochs := &beforeCaptureEpochs{EpochPort: port, before: func() {
		if err := repo.Revoke(ctx, "issuer", "subject"); err != nil {
			t.Fatal(err)
		}
	}}
	service, err := New(Config{External: external, Epochs: epochs})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ResolveExternalToken(ctx, ExternalTokenRequest{AccessToken: "token", Source: commandcatalog.Palette}); !errors.Is(err, auth.ErrUnauthenticatedExternalCommand) {
		t.Fatalf("capturou vínculo revogado: %v", err)
	}
}

func TestExternalGroupRevocationInvalidatesEveryTokenAndOldAdmissions(t *testing.T) {
	ctx := context.Background()
	db := commandIdentityDB(t)
	user := commandIdentityUser(t, db)
	repo := auth.NewExternalIdentityRepository(db)
	if _, err := repo.Create(ctx, auth.ExternalIdentityMappingParams{Issuer: "issuer", Subject: "subject", UserID: user.ID}); err != nil {
		t.Fatal(err)
	}
	verifier := &cachedClaimsStub{claims: &auth.ExternalClaims{Issuer: "issuer", Subject: "subject", Scope: "identity:admin"}}
	external := auth.NewExternalCommandAuthenticator(verifier, repo)
	external.SetReadiness(repo.CheckReadiness)
	admin, err := auth.NewExternalIdentityAdminService(verifier, repo, auth.ExternalIdentityAdminConfig{Issuer: "issuer", AdminScopes: []string{"identity:admin"}})
	if err != nil {
		t.Fatal(err)
	}
	core, err := commandsecurity.NewEpochService(&commandsecurity.DispatchGate{})
	if err != nil {
		t.Fatal(err)
	}
	port, err := NewCoreEpochs(core)
	if err != nil {
		t.Fatal(err)
	}
	service, err := New(Config{External: external, ExternalAdmin: admin, Epochs: port})
	if err != nil {
		t.Fatal(err)
	}
	var snapshots []commandsecurity.EpochSnapshot
	var identities []TrustedIdentity
	var watches []context.Context
	for _, token := range []string{"token-a", "token-b"} {
		identity, err := service.ResolveExternalToken(ctx, ExternalTokenRequest{AccessToken: token, Source: commandcatalog.Palette})
		if err != nil {
			t.Fatal(err)
		}
		identities = append(identities, identity)
		snapshot, err := core.CaptureContextAuthenticated(ctx, func(context.Context) (commandsecurity.ContextPrincipal, error) {
			return commandsecurity.ContextPrincipal{UserID: user.ID, Type: "external_token", ID: identity.AuthContextID, GroupID: auth.ExternalIdentityContextID("issuer", "subject")}, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if snapshot.AuthGeneration != identity.AuthGeneration {
			t.Fatal("epoch não compartilhado")
		}
		snapshots = append(snapshots, snapshot)
		watch, release, err := core.WatchEpoch(ctx, snapshot)
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		watches = append(watches, watch)
	}
	if identities[0].AuthContextID == identities[1].AuthContextID {
		t.Fatal("tokens compartilham identidade")
	}
	if err := service.RevokeExternal(ctx, "admin", "issuer", "subject"); err != nil {
		t.Fatal(err)
	}
	for _, watch := range watches {
		if watch.Err() == nil {
			t.Fatal("espera não cancelada")
		}
	}
	if _, err := service.ResolveExternalToken(ctx, ExternalTokenRequest{AccessToken: "token-a", Source: commandcatalog.Palette}); !errors.Is(err, auth.ErrUnauthenticatedExternalCommand) {
		t.Fatalf("token ainda resolve após revogação: %v", err)
	}
	if err := repo.Enable(ctx, "issuer", "subject"); err != nil {
		t.Fatal(err)
	}
	fresh, err := service.ResolveExternalToken(ctx, ExternalTokenRequest{AccessToken: "token-a", Source: commandcatalog.Palette})
	if err != nil {
		t.Fatal(err)
	}
	if fresh.AuthGeneration == identities[0].AuthGeneration {
		t.Fatal("reativação ressuscitou geração antiga")
	}
	for _, snapshot := range snapshots {
		err := core.Admit(ctx, snapshot, func(context.Context) error { return nil }, func() error { t.Fatal("despachou prova revogada"); return nil })
		if !errors.Is(err, commandsecurity.ErrStaleEpoch) {
			t.Fatalf("admissão antiga: %v", err)
		}
	}
}
