package credentials

import (
	"context"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"strings"

	"assistente/internal/database"
	"assistente/internal/oauthflow"
)

// OAuth storage reuses the vault's DEK, user scope and credential row. The
// encrypted envelope is also the compare-and-swap token across processes.
type oauthPersistence interface {
	LoadOAuth(context.Context, string) (string, error)
	CreateOAuth(context.Context, string, string) error
	SwapOAuth(context.Context, string, string, string) error
}
type oauthStore struct {
	manager *Manager
	userID  string
	epoch   uint64
	session context.Context
}

func (m *Manager) OAuthStore(ctx context.Context) (oauthflow.Store, error) {
	user, err := database.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.persist || m.store == nil {
		return nil, errors.New("oauth_vault_persistence_required")
	}
	if _, ok := m.store.(oauthPersistence); !ok {
		return nil, errors.New("oauth_store_not_supported")
	}
	if m.oauthContext == nil {
		m.oauthContext, m.oauthCancel = context.WithCancel(context.Background())
	}
	return &oauthStore{m, user, m.oauthEpoch, m.oauthContext}, nil
}
func (s *oauthStore) check(ctx context.Context) error {
	user, err := database.RequireUserID(ctx)
	if err != nil || user != s.userID || s.epoch != s.manager.oauthEpoch {
		return oauthflow.ErrConflict
	}
	return ctx.Err()
}
func (s *oauthStore) load(ctx context.Context, id string) (oauthflow.Record, string, error) {
	if err := s.check(ctx); err != nil {
		return oauthflow.Record{}, "", err
	}
	enc, err := s.manager.store.(oauthPersistence).LoadOAuth(ctx, id)
	if err != nil {
		return oauthflow.Record{}, "", err
	}
	data, err := s.manager.decrypt(enc)
	if err != nil {
		return oauthflow.Record{}, "", err
	}
	var r oauthflow.Record
	if err = json.Unmarshal([]byte(data), &r); err != nil {
		return r, "", err
	}
	if r.Version != 1 || r.ID != id || r.UserID != s.userID {
		return r, "", oauthflow.ErrConflict
	}
	return r, enc, nil
}
func (s *oauthStore) Load(ctx context.Context, id string) (oauthflow.Record, error) {
	s.manager.mu.RLock()
	defer s.manager.mu.RUnlock()
	r, _, err := s.load(ctx, id)
	return r, err
}
func (s *oauthStore) Create(ctx context.Context, r oauthflow.Record) error {
	return s.CreateWithConsumer(ctx, r, nil)
}

// CreateWithConsumer publishes the authorization and consumer in one transaction.
func (s *oauthStore) CreateWithConsumer(ctx context.Context, r oauthflow.Record, createConsumer func(*gorm.DB) error) error {
	m := s.manager
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := s.check(ctx); err != nil {
		return err
	}
	if r.ID == "" || r.UserID != s.userID || r.Revision != 1 || r.Version != 1 {
		return oauthflow.ErrConflict
	}
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	enc, err := m.encrypt(string(data))
	if err != nil {
		return err
	}
	if createConsumer == nil {
		err = m.store.(oauthPersistence).CreateOAuth(ctx, r.ID, enc)
	} else {
		persistence, ok := m.store.(*DBStore)
		if !ok {
			return errors.New("oauth_store_not_supported")
		}
		db, dbErr := persistence.ensureDB()
		if dbErr != nil {
			return dbErr
		}
		err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			entry := database.CredentialEntry{UUIDModel: database.UUIDModel{ID: r.ID}, UserID: s.userID, Pattern: "oauth:" + r.ID, Source: "oauth", AuthType: "bearer", OAuthEnc: enc}
			if err := tx.Create(&entry).Error; err != nil {
				return err
			}
			return createConsumer(tx)
		})
	}
	if err != nil {
		return err
	}
	m.credentials = append(m.credentials, &DomainCredential{ID: r.ID, UserID: r.UserID, Pattern: "oauth:" + r.ID, Auth: &AuthConfig{Source: "oauth", Type: "bearer", OAuthEnc: enc}})
	return nil
}
func (s *oauthStore) CompareAndSwap(ctx context.Context, r oauthflow.Record, revision uint64) error {
	m := s.manager
	m.mu.Lock()
	defer m.mu.Unlock()
	old, enc, err := s.load(ctx, r.ID)
	if err != nil {
		return err
	}
	if old.Revision != revision || r.Revision != revision+1 || r.UserID != s.userID || r.Version != 1 || r.Integration != old.Integration {
		return oauthflow.ErrConflict
	}
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	next, err := m.encrypt(string(data))
	if err != nil {
		return err
	}
	if err = m.store.(oauthPersistence).SwapOAuth(ctx, r.ID, enc, next); err != nil {
		return err
	}
	if r.State != "connected" {
		for _, cancel := range m.oauthRequests[r.ID] {
			cancel()
		}
	}
	for _, dc := range m.credentials {
		if dc.ID == r.ID && dc.UserID == s.userID {
			dc.Auth = &AuthConfig{Source: "oauth", Type: "bearer", OAuthEnc: next}
			break
		}
	}
	return nil
}
func (m *Manager) SetOAuthService(service *oauthflow.Service) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.oauthService = service
}
func (m *Manager) resolveOAuth(ctx context.Context, id, resource, rejected string) (*AuthConfig, error) {
	store, err := m.OAuthStore(ctx)
	if err != nil {
		return nil, err
	}
	m.mu.RLock()
	service := m.oauthService
	m.mu.RUnlock()
	if service == nil {
		return nil, ErrOAuthSourceUnavailable
	}
	r, err := service.Resolve(ctx, store, id, resource, rejected)
	if err != nil {
		return nil, err
	}
	// Recheck session generation after network IO, before exposing material.
	latest, err := store.Load(ctx, id)
	if err != nil {
		return nil, err
	}
	if latest.Revision != r.Revision || latest.State != "connected" || latest.RefreshPending {
		return nil, oauthflow.ErrConflict
	}
	return &AuthConfig{Source: "oauth", Type: "bearer", Token: r.Tokens.Access, OAuth: &r}, nil
}
func OAuthCredentialID(pattern string) string { return strings.TrimPrefix(pattern, "oauth:") }

func (s *DBStore) LoadOAuth(ctx context.Context, id string) (string, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return "", err
	}
	db, err := s.ensureDB()
	if err != nil {
		return "", err
	}
	var entry database.CredentialEntry
	err = database.ScopeByUser(ctx, db.WithContext(ctx), "user_id").Where("id = ? AND source = ?", id, "oauth").First(&entry).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", oauthflow.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return entry.OAuthEnc, nil
}
func (s *DBStore) CreateOAuth(ctx context.Context, id, enc string) error {
	user, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	db, err := s.ensureDB()
	if err != nil {
		return err
	}
	return db.WithContext(ctx).Create(&database.CredentialEntry{UUIDModel: database.UUIDModel{ID: id}, UserID: user, Pattern: "oauth:" + id, Source: "oauth", AuthType: "bearer", OAuthEnc: enc}).Error
}
func (s *DBStore) SwapOAuth(ctx context.Context, id, before, after string) error {
	if _, err := database.RequireUserID(ctx); err != nil {
		return err
	}
	db, err := s.ensureDB()
	if err != nil {
		return err
	}
	result := database.ScopeByUser(ctx, db.WithContext(ctx).Model(&database.CredentialEntry{}), "user_id").Where("id = ? AND source = ? AND oauth_enc = ?", id, "oauth", before).Update("oauth_enc", after)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return oauthflow.ErrConflict
	}
	return nil
}

// Called with Manager.mu held, for logout, account switch and DEK reset.
func (m *Manager) invalidateOAuthSession() {
	m.oauthEpoch++
	if m.oauthCancel != nil {
		m.oauthCancel()
	}
	m.oauthContext = nil
	m.oauthCancel = nil
}
func (s *oauthStore) SessionContext(ctx context.Context) (context.Context, context.CancelFunc) {
	child, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(s.session, cancel)
	if s.session.Err() != nil {
		cancel()
	}
	return child, func() { stop(); cancel() }
}

// DeleteOAuthAuthorization commits deletion of a disconnected authorization and
// its consumer together. Neither the cache nor the consumer changes on failure.
func (m *Manager) DeleteOAuthAuthorization(ctx context.Context, id string, deleteConsumer func(*gorm.DB) error) error {
	user, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	persistence, ok := m.store.(*DBStore)
	if !ok {
		return errors.New("oauth_store_not_supported")
	}
	db, err := persistence.ensureDB()
	if err != nil {
		return err
	}
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var entry database.CredentialEntry
		scoped := database.ScopeByUser(ctx, tx, "user_id")
		err := scoped.Where("id = ? AND source = ?", id, "oauth").First(&entry).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil {
			if !m.persist {
				return errors.New("oauth_vault_persistence_required")
			}
			data, err := m.decrypt(entry.OAuthEnc)
			if err != nil {
				return err
			}
			var record oauthflow.Record
			if err = json.Unmarshal([]byte(data), &record); err != nil {
				return err
			}
			if record.Version != 1 || record.ID != id || record.UserID != user || record.State != "disconnected" || record.RefreshPending {
				return oauthflow.ErrConflict
			}
			result := database.ScopeByUser(ctx, tx, "user_id").Where("id = ? AND source = ? AND oauth_enc = ?", id, "oauth", entry.OAuthEnc).Delete(&database.CredentialEntry{})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return oauthflow.ErrConflict
			}
		}
		// Imported providers without a local grant can be removed with a locked vault.
		// Read and consumer deletion share the transaction, so a concurrent grant
		// creation cannot slip between the absence check and deletion.
		return deleteConsumer(tx)
	})
	if err != nil {
		return err
	}
	for index, dc := range m.credentials {
		if dc.ID == id && dc.UserID == user {
			m.credentials = append(m.credentials[:index], m.credentials[index+1:]...)
			break
		}
	}
	for _, cancel := range m.oauthRequests[id] {
		cancel()
	}
	delete(m.oauthRequests, id)
	return nil
}
