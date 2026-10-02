package portability

import (
	"context"
	"errors"

	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/oauthflow"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// External MCP JSON supplies a resource URL, never a transferable OAuth grant.
// Create a pending authorization without discovery, registration or consent.
func importExternalMCPOAuth(ctx context.Context, manager *credentials.Manager, server MCPServerExport) (bool, error) {
	user, err := database.RequireUserID(ctx)
	if err != nil {
		return false, err
	}
	server = normalizeMCPServerExport(server)
	if server.Slug == "" {
		return false, codedErrorf(CodeMCPServerMissingSlug, nil, "slug do servidor MCP é obrigatório")
	}
	var existing database.MCPServer
	err = database.DB().WithContext(ctx).Where("user_id = ? AND slug = ?", user, server.Slug).First(&existing).Error
	if err == nil {
		return false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, err
	}
	if err := validateMCPServerExport(server); err != nil {
		return false, err
	}
	if manager == nil {
		return false, errors.New("oauth_vault_persistence_required")
	}
	store, err := manager.OAuthStore(ctx)
	if err != nil {
		return false, err
	}
	atomicStore, ok := store.(interface {
		CreateWithConsumer(context.Context, oauthflow.Record, func(*gorm.DB) error) error
	})
	if !ok {
		return false, oauthflow.ErrResource
	}
	row, err := mcpServerExportToModel(user, server)
	if err != nil {
		return false, err
	}
	row.ID = uuid.NewString()
	row.OAuth2Scopes = ""
	row.OAuthManaged, row.OAuthAuthorizationID = true, uuid.NewString()
	// Import must never trigger an interactive login through startup auto-connect.
	row.AutoConnect = false
	r := oauthflow.Record{
		Version: 1, Revision: 1, ID: row.OAuthAuthorizationID, UserID: user,
		ConsumerID: row.ID, Integration: "mcp", Resource: row.URL, Audience: row.URL,
		GrantType: "authorization_code", State: "pending",
		Client:   oauthflow.ClientRegistration{Method: "manual", AuthMethod: "none"},
		Callback: oauthflow.CallbackConfig{Path: "/callback", PortPolicy: "ephemeral"},
	}
	if _, err := oauthflow.NewConfigured(r, nil); err != nil {
		return false, err
	}
	err = atomicStore.CreateWithConsumer(ctx, r, func(tx *gorm.DB) error {
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		// GORM applies the database's true default to a false bool on Create.
		return tx.Model(&row).UpdateColumn("auto_connect", false).Error
	})
	return err == nil, err
}
