package commandidentity

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/auth"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontract"
	"assistente/internal/database"
)

func TestAuthorizeExternalRequiredRolesUseValidatedJWTClaims(t *testing.T) {
	tests := []struct {
		name          string
		localRole     string
		roles         []string
		scope         string
		requiredRoles []string
		requiredScope []string
		wantAllowed   bool
	}{
		{
			name:      "role local admin não substitui role ausente no token",
			localRole: database.UserRoleAdmin, roles: []string{"user"},
			requiredRoles: []string{"admin"},
		},
		{
			name:      "role admin validada permite usuário local comum",
			localRole: database.UserRoleUser, roles: []string{"admin"},
			requiredRoles: []string{"admin"}, wantAllowed: true,
		},
		{
			name:      "role válida na segunda posição permite",
			localRole: database.UserRoleUser, roles: []string{"user", "admin"},
			requiredRoles: []string{"admin"}, wantAllowed: true,
		},
		{
			name:      "roles exigidas são alternativas",
			localRole: database.UserRoleUser, roles: []string{"admin"},
			requiredRoles: []string{"operator", "admin"}, wantAllowed: true,
		},
		{
			name:      "token sem roles não atende roles exigidas",
			localRole: database.UserRoleAdmin, requiredRoles: []string{"admin"},
		},
		{
			name:      "roles não exigidas permitem quando scopes estão presentes",
			localRole: database.UserRoleUser, scope: "commands:read",
			requiredScope: []string{"commands:read"}, wantAllowed: true,
		},
		{
			name:          "scopes continuam obrigatórios sem roles exigidas",
			localRole:     database.UserRoleUser,
			requiredScope: []string{"commands:read"},
		},
		{
			name:      "token admin sem scope exigido é negado",
			localRole: database.UserRoleUser, roles: []string{"admin"},
			requiredRoles: []string{"admin"}, requiredScope: []string{"commands:read"},
		},
		{
			name:      "token admin com apenas parte dos scopes é negado",
			localRole: database.UserRoleUser, roles: []string{"admin"}, scope: "commands:read",
			requiredRoles: []string{"admin"}, requiredScope: []string{"commands:read", "commands:write"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			db := commandIdentityDB(t)
			user := commandIdentityUser(t, db)
			if err := db.Model(user).Update("role", tt.localRole).Error; err != nil {
				t.Fatal(err)
			}
			repo := auth.NewExternalIdentityRepository(db)
			if _, err := repo.Create(ctx, auth.ExternalIdentityMappingParams{Issuer: "issuer", Subject: "subject", UserID: user.ID}); err != nil {
				t.Fatal(err)
			}
			verifier := &cachedClaimsStub{claims: &auth.ExternalClaims{
				Issuer: "issuer", Subject: "subject", Scope: tt.scope, Roles: append([]string(nil), tt.roles...),
			}}
			external := auth.NewExternalCommandAuthenticator(verifier, repo)
			external.SetReadiness(repo.CheckReadiness)
			service, err := New(Config{
				External: external, Epochs: &epochPortStub{},
				AuthorizationRules: []AuthorizationRule{{
					CommandID: "maintenance.read", Actors: []commandcontract.ActorType{commandcontract.ActorUser},
					RequiredRoles: tt.requiredRoles, RequiredScopes: tt.requiredScope,
				}},
			})
			if err != nil {
				t.Fatal(err)
			}
			tokenRequest := ExternalTokenRequest{AccessToken: "test-token", Source: commandcatalog.Palette}
			identity, err := service.ResolveExternalToken(ctx, tokenRequest)
			if err != nil {
				t.Fatal(err)
			}
			err = service.Authorize(ctx, AuthorizationRequest{
				Identity: identity, Definition: safeCommandDefinition(), ExternalToken: &tokenRequest,
			})
			if tt.wantAllowed && err != nil {
				t.Fatalf("autorização deveria ser permitida: %v", err)
			}
			if !tt.wantAllowed && !errors.Is(err, ErrCommandNotAuthorized) {
				t.Fatalf("autorização deveria ser negada, erro = %v", err)
			}
		})
	}
}

func TestAuthorizeRejectsForgedExternalRolesAndScopesInProjection(t *testing.T) {
	for _, field := range []string{"roles", "scopes"} {
		t.Run(field, func(t *testing.T) {
			ctx := context.Background()
			db := commandIdentityDB(t)
			user := commandIdentityUser(t, db)
			repo := auth.NewExternalIdentityRepository(db)
			if _, err := repo.Create(ctx, auth.ExternalIdentityMappingParams{Issuer: "issuer", Subject: "subject", UserID: user.ID}); err != nil {
				t.Fatal(err)
			}
			verifier := &cachedClaimsStub{claims: &auth.ExternalClaims{
				Issuer: "issuer", Subject: "subject", Scope: "commands:read", Roles: []string{"admin"},
			}}
			external := auth.NewExternalCommandAuthenticator(verifier, repo)
			external.SetReadiness(repo.CheckReadiness)
			service, err := New(Config{
				External: external, Epochs: &epochPortStub{},
				AuthorizationRules: []AuthorizationRule{{
					CommandID: "maintenance.read", Actors: []commandcontract.ActorType{commandcontract.ActorUser},
					RequiredRoles: []string{"admin"}, RequiredScopes: []string{"commands:read"},
				}},
			})
			if err != nil {
				t.Fatal(err)
			}
			tokenRequest := ExternalTokenRequest{AccessToken: "test-token", Source: commandcatalog.Palette}
			identity, err := service.ResolveExternalToken(ctx, tokenRequest)
			if err != nil {
				t.Fatal(err)
			}
			if err := service.Authorize(ctx, AuthorizationRequest{
				Identity: identity, Definition: safeCommandDefinition(), ExternalToken: &tokenRequest,
			}); err != nil {
				t.Fatalf("baseline válido deveria autorizar antes da adulteração: %v", err)
			}
			switch field {
			case "roles":
				identity.Roles = []string{"admin", "roleextra"}
			case "scopes":
				identity.Scopes = []string{"commands:read", "scopeextra"}
			}
			if err := service.Authorize(ctx, AuthorizationRequest{
				Identity: identity, Definition: safeCommandDefinition(), ExternalToken: &tokenRequest,
			}); !errors.Is(err, ErrCommandNotAuthorized) {
				t.Fatalf("projeção forjada em %s não deveria autorizar: %v", field, err)
			}
		})
	}
}

func TestAuthorizeRejectsExternalRolesChangedAfterCapture(t *testing.T) {
	ctx := context.Background()
	db := commandIdentityDB(t)
	user := commandIdentityUser(t, db)
	repo := auth.NewExternalIdentityRepository(db)
	if _, err := repo.Create(ctx, auth.ExternalIdentityMappingParams{Issuer: "issuer", Subject: "subject", UserID: user.ID}); err != nil {
		t.Fatal(err)
	}
	claims := &auth.ExternalClaims{Issuer: "issuer", Subject: "subject", Roles: []string{"admin"}}
	verifier := &cachedClaimsStub{claims: claims}
	external := auth.NewExternalCommandAuthenticator(verifier, repo)
	external.SetReadiness(repo.CheckReadiness)
	service, err := New(Config{
		External: external, Epochs: &epochPortStub{},
		AuthorizationRules: []AuthorizationRule{{
			CommandID: "maintenance.read", Actors: []commandcontract.ActorType{commandcontract.ActorUser},
			RequiredRoles: []string{"admin", "operator"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	tokenRequest := ExternalTokenRequest{AccessToken: "test-token", Source: commandcatalog.Palette}
	identity, err := service.ResolveExternalToken(ctx, tokenRequest)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Authorize(ctx, AuthorizationRequest{
		Identity: identity, Definition: safeCommandDefinition(), ExternalToken: &tokenRequest,
	}); err != nil {
		t.Fatalf("baseline válido deveria autorizar antes da mudança das claims: %v", err)
	}
	claims.Roles[0] = "operator"
	if err := service.Authorize(ctx, AuthorizationRequest{
		Identity: identity, Definition: safeCommandDefinition(), ExternalToken: &tokenRequest,
	}); !errors.Is(err, ErrCommandNotAuthorized) {
		t.Fatalf("alteração das roles após captura não deveria autorizar: %v", err)
	}
}

func TestAuthorizeFreshLocalIdentityUsesOnlyLocalRole(t *testing.T) {
	service := &Service{rules: map[string]AuthorizationRule{
		"maintenance.read": {
			CommandID: "maintenance.read", Actors: []commandcontract.ActorType{commandcontract.ActorUser},
			RequiredRoles: []string{"admin"},
		},
	}}
	definition := safeCommandDefinition()
	identity := TrustedIdentity{
		AuthContextType: commandcontract.AuthLocalSession,
		ActorType:       commandcontract.ActorUser, Source: commandcatalog.Palette,
		Role: database.UserRoleUser, Roles: []string{"admin"},
	}
	if err := service.authorizeFresh(context.Background(), identity, definition); !errors.Is(err, ErrCommandNotAuthorized) {
		t.Fatalf("lista Roles não deve conceder role para identidade local: %v", err)
	}
	identity.Role = database.UserRoleAdmin
	identity.Roles = []string{"user"}
	if err := service.authorizeFresh(context.Background(), identity, definition); err != nil {
		t.Fatalf("role local preservada deveria autorizar: %v", err)
	}
}
