package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"assistente/internal/acptrust"
	"assistente/internal/authorization"
	"assistente/internal/database"
	"assistente/internal/fstrust"
	"assistente/internal/logging"
	"assistente/internal/nettrust"
	"assistente/internal/shelltrust"
)

type SessionPermissions interface {
	SessionRules(context.Context) ([]database.AuthorizationRule, error)
	RevokeSession(context.Context, string) error
}

type PermissionView struct {
	ID           string `json:"id"`
	Domain       string `json:"domain"`
	Scope        string `json:"scope"`
	ScopeKey     string `json:"scopeKey"`
	Effect       string `json:"effect"`
	Origin       string `json:"origin"`
	Resource     string `json:"resource"`
	Operation    string `json:"operation"`
	ResourceKind string `json:"resourceKind"`
	CreatedAt    string `json:"createdAt"`
	RevokedAt    string `json:"revokedAt"`
	ExpiresAt    string `json:"expiresAt"`
}

type PermissionSnapshot struct {
	Rules []PermissionView `json:"rules"`
	// Erros de importação aparecem junto às regras já conhecidas, sem fingir
	// que a listagem está completa ou impedir a revogação do que já existe.
	MigrationErrors []PermissionDiagnostic `json:"migrationErrors"`
}

type PermissionDiagnostic struct {
	Code    string `json:"code"`
	Domain  string `json:"domain"`
	Profile string `json:"profile"`
}

type PermissionPreparationError struct {
	Domain  string
	Profile string
	Err     error
}

func (e *PermissionPreparationError) Error() string {
	return fmt.Sprintf("%s (%s): %v", e.Domain, e.Profile, e.Err)
}
func (e *PermissionPreparationError) Unwrap() error { return e.Err }

type PermissionsController struct {
	Repository *authorization.Repository
	Sessions   []SessionPermissions
	Prepare    func(context.Context) []error
}

func (c *PermissionsController) List(ctx context.Context) (PermissionSnapshot, error) {
	result := PermissionSnapshot{Rules: []PermissionView{}, MigrationErrors: []PermissionDiagnostic{}}
	if c.Prepare != nil {
		for _, err := range c.Prepare(ctx) {
			if err != nil {
				diagnostic := PermissionDiagnostic{Code: "source_unavailable"}
				if errors.Is(err, authorization.ErrLegacyOwnerPending) {
					diagnostic.Code = "owner_pending"
				}
				var preparation *PermissionPreparationError
				if errors.As(err, &preparation) {
					diagnostic.Domain, diagnostic.Profile = preparation.Domain, preparation.Profile
				}
				result.MigrationErrors = append(result.MigrationErrors, diagnostic)
				logging.Logger(ctx, "authorization").WarnContext(ctx, "Preparação de permissões incompleta", "error", err, "code", diagnostic.Code)
			}
		}
	}
	rows, err := c.Repository.ManagementRules(ctx)
	if err != nil {
		return result, err
	}
	for _, source := range c.Sessions {
		sessions, err := source.SessionRules(ctx)
		if err != nil {
			return result, err
		}
		rows = append(rows, sessions...)
	}
	for _, row := range rows {
		view, err := permissionView(row)
		if err != nil {
			return result, err
		}
		result.Rules = append(result.Rules, view)
	}
	return result, nil
}

func (c *PermissionsController) Revoke(ctx context.Context, id string) (err error) {
	defer func() {
		if err != nil {
			logging.Logger(ctx, "authorization").WarnContext(ctx, "Revogação de autorização não concluída", "rule_id", id)
		} else {
			logging.Logger(ctx, "authorization").InfoContext(ctx, "Autorização revogada", "rule_id", id)
		}
	}()
	if strings.HasPrefix(id, "session:") {
		for _, source := range c.Sessions {
			err := source.RevokeSession(ctx, id)
			if err == nil {
				return nil
			}
			if !errors.Is(err, authorization.ErrNotFound) {
				return err
			}
		}
		return authorization.ErrNotFound
	}
	return c.Repository.RevokeID(ctx, id)
}

func permissionView(row database.AuthorizationRule) (PermissionView, error) {
	view := PermissionView{ID: row.ID, Domain: row.Domain, Scope: row.Scope, ScopeKey: row.ScopeKey, Effect: row.Effect, Origin: row.Origin, Operation: row.Operation}
	created := row.CreatedAt
	if row.SelectorVersion != 1 {
		return view, errors.New("versão de permissão não suportada")
	}
	switch row.Domain {
	case "network":
		var entry nettrust.AllowlistEntry
		if err := json.Unmarshal([]byte(row.Selector), &entry); err != nil {
			return view, err
		}
		view.Resource = entry.Host
		if entry.Port != "" {
			view.Resource += ":" + entry.Port
		}
		if !entry.CreatedAt.IsZero() {
			created = entry.CreatedAt
		}
	case "filesystem":
		var entry fstrust.AllowlistEntry
		if err := json.Unmarshal([]byte(row.Selector), &entry); err != nil {
			return view, err
		}
		view.Resource = entry.Path
		view.ResourceKind = string(entry.Kind)
		if !entry.CreatedAt.IsZero() {
			created = entry.CreatedAt
		}
	case "shell":
		var entry shelltrust.Entry
		if err := json.Unmarshal([]byte(row.Selector), &entry); err != nil {
			return view, err
		}
		view.Resource = strings.Join(append([]string{entry.Selector.Program}, entry.Selector.Arguments...), " ") + " — " + entry.Selector.Directory
		if !entry.CreatedAt.IsZero() {
			created = entry.CreatedAt
		}
	case authorization.PluginHookDomain:
		var hook authorization.PluginHook
		if err := json.Unmarshal([]byte(row.Selector), &hook); err != nil {
			return view, err
		}
		view.Resource = hook.Name + " (" + hook.Origin + ") — " + hook.Component
		view.ResourceKind = hook.Digest
	case "acp":
		var entry acptrust.Entry
		if err := json.Unmarshal([]byte(row.Selector), &entry); err != nil {
			return view, err
		}
		view.Resource = entry.AgentID
		view.ResourceKind = entry.Kind
		if entry.AgentID == "" {
			view.ResourceKind = "legacy:" + entry.Kind
		}
		if !entry.GrantedAt.IsZero() {
			created = entry.GrantedAt
		}
	default:
		return view, fmt.Errorf("domínio de permissão desconhecido: %s", row.Domain)
	}
	if !created.IsZero() {
		view.CreatedAt = created.Format(time.RFC3339)
	}
	if row.RevokedAt != nil {
		view.RevokedAt = row.RevokedAt.Format(time.RFC3339)
	}
	if row.ExpiresAt != nil {
		view.ExpiresAt = row.ExpiresAt.Format(time.RFC3339)
	}
	return view, nil
}
