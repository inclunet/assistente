package credentials

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"assistente/internal/database"
	"assistente/internal/oauthflow"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Transitional coordination only: this is not an authorization or a converted grant.
type legacyOAuthControl struct {
	Version    int
	ConsumerID string
	Attempt    string
	Until      time.Time
	Pending    bool
}

type legacyOAuthReceiptKey struct{}

type LegacyOAuthOperation struct {
	store                   *oauthStore
	rowID, pattern, control string
	consumerID              string
	until                   time.Time
	clientGrant             bool
}

// ClearLegacyOAuth is explicit local disconnection. It atomically removes the
// pair only after the live attempt ends; an uncertain inactive grant may be discarded.
func (m *Manager) ClearLegacyOAuth(ctx context.Context, slug, consumerID, hostname string) error {
	return m.ClearLegacyOAuthWithConsumer(ctx, slug, consumerID, hostname, nil, nil)
}

// ClearLegacyOAuthWithConsumer atomically detaches unmanaged MCP credentials and
// updates its consumer. Publish runs only after commit, under the vault lock.
func (m *Manager) ClearLegacyOAuthWithConsumer(ctx context.Context, slug, consumerID, hostname string, update func(*gorm.DB) error, publish func()) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	user, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	store, ok := m.store.(*DBStore)
	if !ok || !m.persist {
		return oauthflow.ErrResource
	}
	patterns := []string{"mcp-client:" + slug, "mcp-tokens:" + slug}
	if hostname != "" {
		patterns = append(patterns, hostname)
	}
	err = database.WithSQLiteImmediateTransactionOnce(ctx, time.Time{}, store.db, "credentials.legacy_oauth.clear", func(tx *gorm.DB) error {
		var consumer database.MCPServer
		if err := tx.Where("id = ? AND user_id = ? AND slug = ?", consumerID, user, slug).First(&consumer).Error; err != nil {
			return oauthflow.ErrConflict
		}
		if consumer.OAuthManaged || consumer.OAuthAuthorizationID != "" {
			return oauthflow.ErrConflict
		}
		var row database.CredentialEntry
		readErr := tx.Where("user_id = ? AND pattern = ?", user, "mcp-tokens:"+slug).First(&row).Error
		if readErr != nil && !errors.Is(readErr, gorm.ErrRecordNotFound) {
			return readErr
		}
		if row.LegacyOAuthControlEnc != "" {
			data, err := m.decrypt(row.LegacyOAuthControlEnc)
			if err != nil {
				return oauthflow.ErrConflict
			}
			var control legacyOAuthControl
			if json.Unmarshal([]byte(data), &control) != nil || control.Version != 1 || control.ConsumerID != consumerID {
				return oauthflow.ErrConflict
			}
			if time.Now().Before(control.Until) {
				return oauthflow.ErrTransient
			}
		}
		if err := tx.Where("user_id = ? AND pattern IN ?", user, patterns).Delete(&database.CredentialEntry{}).Error; err != nil {
			return err
		}
		if update != nil {
			return update(tx)
		}
		return nil
	})
	if err != nil {
		return err
	}
	kept := make([]*DomainCredential, 0, len(m.credentials))
	for _, entry := range m.credentials {
		remove := false
		for _, pattern := range patterns {
			if entry.UserID == user && entry.Pattern == pattern {
				remove = true
			}
		}
		if !remove {
			kept = append(kept, entry)
		} else {
			for _, cancel := range m.oauthRequests[entry.ID] {
				cancel()
			}
			entry.invalidateCommandCache()
		}
	}
	m.credentials = kept
	if publish != nil {
		publish()
	}
	return nil
}

// Context permits only writes belonging to this exact durable attempt.
func (o *LegacyOAuthOperation) Context(ctx context.Context) context.Context {
	return context.WithValue(database.WithUserID(ctx, o.store.userID), legacyOAuthReceiptKey{}, o)
}

// CheckLegacyOAuthMutation runs inside the caller's write transaction. Unknown
// encrypted control is fail-closed; ordinary saves must never erase it.
type mcpConsumerCheckKey struct{}

// WithMCPConsumerCheck fences UI mutations by the consumer snapshot they edited.
// The check must be read-only and runs inside each credential write transaction.
func WithMCPConsumerCheck(ctx context.Context, check func(*gorm.DB) error) context.Context {
	return context.WithValue(ctx, mcpConsumerCheckKey{}, check)
}

func checkMCPConsumer(ctx context.Context, tx *gorm.DB) error {
	if check, ok := ctx.Value(mcpConsumerCheckKey{}).(func(*gorm.DB) error); ok && check != nil {
		return check(tx)
	}
	return nil
}

func CheckLegacyOAuthMutation(ctx context.Context, tx *gorm.DB, user, slug string) error {
	if err := checkMCPConsumer(ctx, tx); err != nil {
		return err
	}
	if !tx.Migrator().HasTable(&database.CredentialEntry{}) {
		return nil
	}
	var row database.CredentialEntry
	err := tx.Where("user_id = ? AND pattern = ?", user, "mcp-tokens:"+slug).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if row.LegacyOAuthControlEnc == "" {
		return nil
	}
	if o, ok := ctx.Value(legacyOAuthReceiptKey{}).(*LegacyOAuthOperation); ok &&
		o.store.session.Err() == nil && o.store.userID == user && o.rowID == row.ID && o.control == row.LegacyOAuthControlEnc && time.Now().Before(o.until) {
		return nil
	}
	return oauthflow.ErrConflict
}

// InspectMCPAuthPresence reads metadata only; absence must never fall back to cache.
func (m *Manager) InspectMCPAuthPresence(ctx context.Context, slug, hostname string) (bool, error) {
	base, err := m.OAuthStore(ctx)
	if err != nil {
		return false, err
	}
	s := base.(*oauthStore)
	var count int64
	err = s.WithSession(ctx, func() error {
		store, ok := m.store.(*DBStore)
		if !ok {
			return oauthflow.ErrResource
		}
		return store.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := checkMCPConsumer(ctx, tx); err != nil {
				return err
			}
			patterns := []string{"mcp-client:" + slug, "mcp-tokens:" + slug}
			if hostname != "" {
				patterns = append(patterns, hostname)
			}
			return tx.Model(&database.CredentialEntry{}).Where("user_id = ? AND pattern IN ?", s.userID, patterns).Count(&count).Error
		})
	})
	return count > 0, err
}

// BeginLegacyOAuth acquires a durable attempt in the existing token row, after
// validating the persisted consumer. No lock/transaction spans protocol I/O.
// authorize is only supplied by an explicit user action when recovering pending refresh.
func (m *Manager) BeginLegacyOAuth(ctx context.Context, slug, consumerID string, authorize, recoverPending bool, validate func(*gorm.DB) error) (*LegacyOAuthOperation, *AuthConfig, error) {
	return m.beginLegacyOAuth(ctx, slug, consumerID, authorize, recoverPending, false, validate)
}

// Client Credentials obtains a new grant, never replays a rotating refresh token.
// It shares the same durable lease and mutation barriers as legacy PKCE.
func (m *Manager) BeginLegacyClientGrant(ctx context.Context, slug, consumerID string, validate func(*gorm.DB) error) (*LegacyOAuthOperation, *AuthConfig, error) {
	return m.beginLegacyOAuth(ctx, slug, consumerID, false, false, true, validate)
}

func (m *Manager) beginLegacyOAuth(ctx context.Context, slug, consumerID string, authorize, recoverPending, clientGrant bool, validate func(*gorm.DB) error) (*LegacyOAuthOperation, *AuthConfig, error) {
	base, err := m.OAuthStore(ctx)
	if err != nil {
		return nil, nil, err
	}
	s := base.(*oauthStore)
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := s.check(ctx); err != nil {
		return nil, nil, err
	}
	persistence, ok := m.store.(*DBStore)
	if !ok {
		return nil, nil, oauthflow.ErrResource
	}
	db, err := persistence.ensureDB()
	if err != nil {
		return nil, nil, err
	}
	var operation *LegacyOAuthOperation
	var auth *AuthConfig
	err = database.WithSQLiteImmediateTransaction(ctx, db, "credentials.legacy_oauth.begin", func(tx *gorm.DB) error {
		var consumer database.MCPServer
		if err := tx.Where("id = ? AND user_id = ? AND slug = ?", consumerID, s.userID, slug).First(&consumer).Error; err != nil {
			return oauthflow.ErrConflict
		}
		expectedType := "oauth2_pkce"
		if clientGrant {
			expectedType = "oauth2_client_credentials"
		}
		if consumer.OAuthManaged || consumer.OAuthAuthorizationID != "" || consumer.AuthType != expectedType {
			return oauthflow.ErrConflict
		}
		if validate != nil {
			if err := validate(tx); err != nil {
				return err
			}
		}
		var row database.CredentialEntry
		err := tx.Where("user_id = ? AND pattern = ?", s.userID, "mcp-tokens:"+slug).First(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) && (authorize || clientGrant) {
			row = database.CredentialEntry{UserID: s.userID, Pattern: "mcp-tokens:" + slug, Source: "static", AuthType: "oauth2"}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		} else if err != nil {
			return oauthflow.ErrReauthorize
		}
		if row.Source != "" && row.Source != "static" || row.AuthType != "oauth2" {
			return oauthflow.ErrConflict
		}
		pending := false
		if row.LegacyOAuthControlEnc != "" {
			data, err := m.decrypt(row.LegacyOAuthControlEnc)
			if err != nil {
				return oauthflow.ErrConflict
			}
			var previous legacyOAuthControl
			if json.Unmarshal([]byte(data), &previous) != nil || previous.Version != 1 || previous.ConsumerID != consumerID {
				return oauthflow.ErrConflict
			}
			if time.Now().Before(previous.Until) {
				return oauthflow.ErrTransient
			}
			if previous.Pending && !recoverPending {
				return oauthflow.ErrReauthorize
			}
			pending = previous.Pending
		}
		auth = &AuthConfig{Source: "static", Type: "oauth2", ExpiresAt: row.ExpiresAt}
		if row.TokenEnc != "" {
			auth.Token, err = m.decrypt(row.TokenEnc)
			if err != nil {
				return oauthflow.ErrConflict
			}
		}
		if row.RefreshTokenEnc != "" {
			auth.RefreshURL, err = m.decrypt(row.RefreshTokenEnc)
			if err != nil {
				return oauthflow.ErrConflict
			}
		}
		if !authorize && !clientGrant && auth.RefreshURL == "" {
			return oauthflow.ErrReauthorize
		}
		var client database.CredentialEntry
		clientErr := tx.Where("user_id = ? AND pattern = ?", s.userID, "mcp-client:"+slug).First(&client).Error
		if clientErr != nil && !errors.Is(clientErr, gorm.ErrRecordNotFound) {
			return clientErr
		}
		if clientErr == nil {
			auth.ClientGrantType = client.ClientGrantType
			if client.Source != "" && client.Source != "static" || clientGrant && (client.AuthType != "oauth2" || client.SourceConfigEnc != "" || client.OAuthEnc != "") {
				return oauthflow.ErrConflict
			}
			if client.ClientIDEnc != "" {
				auth.ClientID, err = m.decrypt(client.ClientIDEnc)
				if err != nil {
					return oauthflow.ErrConflict
				}
			}
			if client.ClientSecretEnc != "" {
				auth.ClientSecret, err = m.decrypt(client.ClientSecretEnc)
				if err != nil {
					return oauthflow.ErrConflict
				}
			}
		}
		if clientGrant && (clientErr != nil || auth.ClientSecret == "" || auth.RefreshURL != "") {
			return oauthflow.ErrConflict
		}
		var nonce [24]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			return err
		}
		duration := 30 * time.Second
		if authorize {
			duration = 10 * time.Minute
		}
		control := legacyOAuthControl{Version: 1, ConsumerID: consumerID, Attempt: hex.EncodeToString(nonce[:]), Until: time.Now().Add(duration), Pending: pending || (!authorize && !clientGrant)}
		data, err := json.Marshal(control)
		if err != nil {
			return err
		}
		enc, err := m.encrypt(string(data))
		if err != nil {
			return err
		}
		if err := tx.Model(&row).Update("legacy_oauth_control_enc", enc).Error; err != nil {
			return err
		}
		operation = &LegacyOAuthOperation{store: s, rowID: row.ID, pattern: row.Pattern, control: enc, consumerID: consumerID, until: control.Until, clientGrant: clientGrant}
		return nil
	})
	return operation, auth, err
}

func (o *LegacyOAuthOperation) Deadline() time.Time { return o.until }

// FinishClientGrant checks ownership and releases this non-rotating operation.
// Access tokens stay in the transport cache, never in a new persistence format.
func (o *LegacyOAuthOperation) FinishClientGrant(ctx context.Context) error {
	s, m := o.store, o.store.manager
	m.mu.Lock()
	defer m.mu.Unlock()
	if !o.clientGrant || !time.Now().Before(o.until) {
		return oauthflow.ErrConflict
	}
	if err := s.check(ctx); err != nil {
		return err
	}
	return database.WithSQLiteImmediateTransactionOnce(ctx, o.until, m.store.(*DBStore).db, "credentials.legacy_oauth.client_grant_finish", func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&database.MCPServer{}).Where("id = ? AND user_id = ? AND auth_type = ? AND oauth_managed = ? AND oauth_authorization_id = ''", o.consumerID, s.userID, "oauth2_client_credentials", false).Count(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			return oauthflow.ErrConflict
		}
		result := tx.Model(&database.CredentialEntry{}).Where("id = ? AND user_id = ? AND legacy_oauth_control_enc = ?", o.rowID, s.userID, o.control).Update("legacy_oauth_control_enc", "")
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return oauthflow.ErrConflict
		}
		return nil
	})
}

func (o *LegacyOAuthOperation) SessionContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return o.store.SessionContext(ctx)
}

// SaveClient fences both persistence and cache publication by the captured vault session.
func (o *LegacyOAuthOperation) SaveClient(ctx context.Context, auth *AuthConfig) error {
	return o.SaveClientWithConsumer(ctx, auth, nil, nil)
}

func (o *LegacyOAuthOperation) MatchesStore(store oauthflow.Store) bool {
	s, ok := store.(*oauthStore)
	return ok && s.manager == o.store.manager && s.userID == o.store.userID && s.epoch == o.store.epoch
}

// SaveClientWithConsumer commits DCR client and callback metadata together.
// Callbacks must remain local and must not reenter the credential manager.
func (o *LegacyOAuthOperation) SaveClientWithConsumer(ctx context.Context, auth *AuthConfig, update func(*gorm.DB) error, publish func()) error {
	if o.clientGrant {
		return oauthflow.ErrConflict
	}
	s, m := o.store, o.store.manager
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := s.check(ctx); err != nil {
		return err
	}
	enc, err := m.encryptAuth(auth)
	if err != nil {
		return err
	}
	pattern := "mcp-client:" + strings.TrimPrefix(o.pattern, "mcp-tokens:")
	var row database.CredentialEntry
	ctx = o.Context(ctx)
	err = database.WithSQLiteImmediateTransactionOnce(ctx, o.until, m.store.(*DBStore).db, "credentials.legacy_oauth.client", func(tx *gorm.DB) error {
		if err := guardLegacyMCPOAuthWrite(tx, s.userID, pattern); err != nil {
			return err
		}
		row = database.CredentialEntry{UserID: s.userID, Pattern: pattern, Source: "static", AuthType: "oauth2", ClientIDEnc: enc.ClientID, ClientSecretEnc: enc.ClientSecret, ClientGrantType: enc.ClientGrantType}
		if err := tx.Omit("legacy_oauth_control_enc").Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "user_id"}, {Name: "pattern"}}, UpdateAll: true}).Create(&row).Error; err != nil {
			return err
		}
		row = database.CredentialEntry{}
		if err := tx.Where("user_id = ? AND pattern = ?", s.userID, pattern).First(&row).Error; err != nil {
			return err
		}
		if update != nil {
			return update(tx)
		}
		return nil
	})
	if err != nil {
		return err
	}
	dc := &DomainCredential{ID: row.ID, UserID: s.userID, Pattern: pattern, Auth: enc, regex: regexp.MustCompile(wildcardToRegex(pattern))}
	for i, old := range m.credentials {
		if old.ID == row.ID || old.UserID == s.userID && old.Pattern == pattern {
			m.credentials[i] = dc
			if publish != nil {
				publish()
			}
			return nil
		}
	}
	m.credentials = append(m.credentials, dc)
	if publish != nil {
		publish()
	}
	return nil
}

// End releases only this live lease; uncertain refresh remains durable.
func (o *LegacyOAuthOperation) End() {
	s, m := o.store, o.store.manager
	m.mu.Lock()
	defer m.mu.Unlock()
	ctx := database.WithUserID(context.Background(), s.userID)
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if s.check(ctx) != nil {
		return
	}
	data, err := m.decrypt(o.control)
	if err != nil {
		return
	}
	var control legacyOAuthControl
	if json.Unmarshal([]byte(data), &control) != nil {
		return
	}
	enc := ""
	if control.Pending {
		control.Until = time.Time{}
		body, _ := json.Marshal(control)
		enc, err = m.encrypt(string(body))
		if err != nil {
			return
		}
	}
	db, err := m.store.(*DBStore).ensureDB()
	if err != nil {
		return
	}
	_ = db.WithContext(ctx).Model(&database.CredentialEntry{}).Where("id = ? AND user_id = ? AND legacy_oauth_control_enc = ?", o.rowID, s.userID, o.control).Update("legacy_oauth_control_enc", enc).Error
}

// ReadLegacyOAuthToken reads the authoritative pair, never the manager cache.
func (m *Manager) ReadLegacyOAuthToken(ctx context.Context, slug, consumerID string, validators ...func(*gorm.DB) error) (*AuthConfig, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	user, err := database.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	store, ok := m.store.(*DBStore)
	if !ok || !m.persist {
		return nil, oauthflow.ErrResource
	}
	var row database.CredentialEntry
	if err := store.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var consumer database.MCPServer
		if err := tx.Where("id = ? AND user_id = ? AND slug = ?", consumerID, user, slug).First(&consumer).Error; err != nil {
			return oauthflow.ErrConflict
		}
		if consumer.OAuthManaged || consumer.OAuthAuthorizationID != "" || consumer.AuthType != "oauth2_pkce" {
			return oauthflow.ErrConflict
		}
		for _, validate := range validators {
			if validate != nil {
				if err := validate(tx); err != nil {
					return err
				}
			}
		}
		if err := tx.Where("user_id = ? AND pattern = ?", user, "mcp-tokens:"+slug).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return oauthflow.ErrNotFound
			}
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if row.LegacyOAuthControlEnc != "" {
		data, err := m.decrypt(row.LegacyOAuthControlEnc)
		if err != nil {
			return nil, oauthflow.ErrConflict
		}
		var c legacyOAuthControl
		if json.Unmarshal([]byte(data), &c) != nil || c.Version != 1 || c.ConsumerID != consumerID {
			return nil, oauthflow.ErrConflict
		}
		if time.Now().Before(c.Until) {
			return nil, oauthflow.ErrTransient
		}
		if c.Pending {
			return nil, oauthflow.ErrReauthorize
		}
	}
	auth := &AuthConfig{Source: row.Source, Type: row.AuthType, ExpiresAt: row.ExpiresAt}
	if auth.Source != "" && auth.Source != "static" || auth.Type != "oauth2" {
		return nil, oauthflow.ErrConflict
	}
	if row.TokenEnc != "" {
		auth.Token, err = m.decrypt(row.TokenEnc)
		if err != nil {
			return nil, oauthflow.ErrConflict
		}
	}
	if row.RefreshTokenEnc != "" {
		auth.RefreshURL, err = m.decrypt(row.RefreshTokenEnc)
		if err != nil {
			return nil, oauthflow.ErrConflict
		}
	}
	return auth, nil
}

// ReadLegacyHostnameToken keeps the native compatibility fallback authoritative.
// The consumer and absence of its own grant are checked in the same read snapshot.
func (m *Manager) ReadLegacyHostnameToken(ctx context.Context, slug, hostname string, validate func(*gorm.DB) error) (*AuthConfig, error) {
	return m.readLegacyHostnameToken(ctx, slug, hostname, validate, ResolveSource)
}

func (m *Manager) readLegacyHostnameToken(ctx context.Context, slug, hostname string, validate func(*gorm.DB) error, resolve func(context.Context, *AuthConfig) (*AuthConfig, error)) (*AuthConfig, error) {
	base, err := m.OAuthStore(ctx)
	if err != nil {
		return nil, err
	}
	s := base.(*oauthStore)
	var auth *AuthConfig
	var row database.CredentialEntry
	checkConsumer := func(tx *gorm.DB) error {
		if validate == nil {
			return oauthflow.ErrConflict
		}
		if err := validate(tx); err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&database.CredentialEntry{}).Where("user_id = ? AND pattern = ?", s.userID, "mcp-tokens:"+slug).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return oauthflow.ErrConflict
		}
		return nil
	}
	err = s.WithSession(ctx, func() error {
		store, ok := m.store.(*DBStore)
		if !ok {
			return oauthflow.ErrResource
		}
		return store.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := checkConsumer(tx); err != nil {
				return err
			}
			if hostname == "" {
				return nil
			}
			if err := tx.Where("user_id = ? AND pattern = ?", s.userID, hostname).First(&row).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil
				}
				return err
			}
			var err error
			auth, err = m.decryptAuth(&AuthConfig{Source: row.Source, SourceConfigEnc: row.SourceConfigEnc, Type: row.AuthType, Token: row.TokenEnc, ExpiresAt: row.ExpiresAt})
			return err
		})
	})
	if err != nil || auth == nil {
		return auth, err
	}
	sourceCtx, cancel := s.SessionContext(ctx)
	defer cancel()
	auth, err = resolve(withDirectCommandDiagnostic(sourceCtx, row.ID), auth)
	if err != nil {
		return nil, err
	}
	if err = s.WithSession(ctx, func() error {
		return m.store.(*DBStore).db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := checkConsumer(tx); err != nil {
				return err
			}
			var current database.CredentialEntry
			if err := tx.Where("user_id = ? AND pattern = ?", s.userID, hostname).First(&current).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return oauthflow.ErrConflict
				}
				return err
			}
			if current.ID != row.ID || !current.UpdatedAt.Equal(row.UpdatedAt) || current.Source != row.Source || current.SourceConfigEnc != row.SourceConfigEnc || current.AuthType != row.AuthType || current.TokenEnc != row.TokenEnc || current.ExpiresAt != row.ExpiresAt || current.OAuthEnc != row.OAuthEnc || current.LegacyOAuthControlEnc != row.LegacyOAuthControlEnc {
				return oauthflow.ErrConflict
			}
			return nil
		})
	}); err != nil {
		return nil, err
	}
	return auth, nil
}

// Commit publishes the complete rotated pair and clears pending in one commit.
func (o *LegacyOAuthOperation) Commit(ctx context.Context, auth *AuthConfig) error {
	if o.clientGrant {
		return oauthflow.ErrConflict
	}
	s, m := o.store, o.store.manager
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := s.check(ctx); err != nil {
		return err
	}
	if !time.Now().Before(o.until) {
		return oauthflow.ErrConflict
	}
	if auth == nil || auth.Token == "" {
		return oauthflow.ErrReauthorize
	}
	auth = &AuthConfig{Source: "static", Type: "oauth2", Token: auth.Token, RefreshURL: auth.RefreshURL, ExpiresAt: auth.ExpiresAt}
	enc, err := m.encryptAuth(auth)
	if err != nil {
		return err
	}
	db, err := m.store.(*DBStore).ensureDB()
	if err != nil {
		return err
	}
	err = database.WithSQLiteImmediateTransaction(ctx, db, "credentials.legacy_oauth.commit", func(tx *gorm.DB) error {
		tx = tx.WithContext(o.Context(ctx))
		if err := guardLegacyMCPOAuthWrite(tx, s.userID, o.pattern); err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&database.MCPServer{}).Where("id = ? AND user_id = ? AND slug = ?", o.consumerID, s.userID, strings.TrimPrefix(o.pattern, "mcp-tokens:")).Count(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			return oauthflow.ErrConflict
		}
		result := tx.Model(&database.CredentialEntry{}).Where("id = ? AND user_id = ? AND legacy_oauth_control_enc = ?", o.rowID, s.userID, o.control).
			Updates(map[string]any{"token_enc": enc.Token, "refresh_token_enc": enc.RefreshURL, "expires_at": enc.ExpiresAt, "legacy_oauth_control_enc": ""})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return oauthflow.ErrConflict
		}
		return nil
	})
	if err != nil {
		return err
	}
	dc := &DomainCredential{ID: o.rowID, UserID: s.userID, Pattern: o.pattern, Auth: enc, regex: regexp.MustCompile(wildcardToRegex(o.pattern))}
	for i, old := range m.credentials {
		if old.ID == o.rowID || old.UserID == s.userID && old.Pattern == o.pattern {
			m.credentials[i] = dc
			return nil
		}
	}
	m.credentials = append(m.credentials, dc)
	return nil
}
