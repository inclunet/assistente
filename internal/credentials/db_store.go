package credentials

import (
	"context"
	"encoding/json"
	"errors"

	"assistente/internal/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrStoreNotReady = errors.New("store de credenciais não inicializado")

// DBStore persiste credenciais no banco local.
type DBStore struct {
	db *gorm.DB
}

// NewDBStore cria um store baseado no banco atual.
func NewDBStore() *DBStore {
	return &DBStore{db: database.DB()}
}

func (s *DBStore) ensureDB() (*gorm.DB, error) {
	if s == nil || s.db == nil {
		return nil, ErrStoreNotReady
	}
	return s.db, nil
}

func (s *DBStore) SaveCredential(ctx context.Context, cred StoredCredential) error {
	db, err := s.ensureDB()
	if err != nil {
		return err
	}
	if cred.Auth == nil {
		return errors.New("auth não pode ser nil")
	}

	headersJSON := ""
	if len(cred.Auth.Headers) > 0 {
		data, err := json.Marshal(cred.Auth.Headers)
		if err != nil {
			return err
		}
		headersJSON = string(data)
	}
	userID := cred.UserID
	if userID == "" {
		if scopedUserID, ok := database.UserIDFromContext(ctx); ok {
			userID = scopedUserID
		}
	}

	entry := database.CredentialEntry{
		UUIDModel: database.UUIDModel{
			ID: cred.ID,
		},
		UserID:          userID,
		Pattern:         cred.Pattern,
		AuthType:        cred.Auth.Type,
		TokenEnc:        cred.Auth.Token,
		Username:        cred.Auth.Username,
		PasswordEnc:     cred.Auth.Password,
		HeadersEnc:      headersJSON,
		ExpiresAt:       cred.Auth.ExpiresAt,
		RefreshTokenEnc: cred.Auth.RefreshURL,
		ClientIDEnc:     cred.Auth.ClientID,
		ClientSecretEnc: cred.Auth.ClientSecret,
	}

	if cred.ID != "" {
		return database.ScopeByUser(ctx, db.WithContext(ctx), "user_id").Save(&entry).Error
	}

	return db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "pattern"}},
		UpdateAll: true,
	}).Create(&entry).Error
}

func (s *DBStore) ListCredentials(ctx context.Context) ([]StoredCredential, error) {
	db, err := s.ensureDB()
	if err != nil {
		return nil, err
	}

	var entries []database.CredentialEntry
	query := database.ScopeByUser(ctx, db.WithContext(ctx), "user_id")
	if err := query.Find(&entries).Error; err != nil {
		return nil, err
	}

	result := make([]StoredCredential, 0, len(entries))
	for _, entry := range entries {
		headers := map[string]string{}
		if entry.HeadersEnc != "" {
			if err := json.Unmarshal([]byte(entry.HeadersEnc), &headers); err != nil {
				return nil, err
			}
		}

		auth := &AuthConfig{
			Type:         entry.AuthType,
			Token:        entry.TokenEnc,
			Username:     entry.Username,
			Password:     entry.PasswordEnc,
			Headers:      headers,
			ExpiresAt:    entry.ExpiresAt,
			RefreshURL:   entry.RefreshTokenEnc,
			ClientID:     entry.ClientIDEnc,
			ClientSecret: entry.ClientSecretEnc,
		}

		result = append(result, StoredCredential{
			ID:      entry.ID,
			UserID:  entry.UserID,
			Pattern: entry.Pattern,
			Auth:    auth,
		})
	}

	return result, nil
}

func (s *DBStore) DeleteCredential(ctx context.Context, pattern string) error {
	db, err := s.ensureDB()
	if err != nil {
		return err
	}
	query := database.ScopeByUser(ctx, db.WithContext(ctx), "user_id")
	return query.Where("pattern = ?", pattern).Delete(&database.CredentialEntry{}).Error
}

func (s *DBStore) SaveKeyWrap(ctx context.Context, wrap KeyWrap) error {
	db, err := s.ensureDB()
	if err != nil {
		return err
	}

	entry := database.CredentialKeyWrap{
		Kind:         wrap.Kind,
		Salt:         wrap.Salt,
		WrappedDEK:   wrap.WrappedDEK,
		ArgonTime:    wrap.ArgonTime,
		ArgonMemory:  wrap.ArgonMemory,
		ArgonThreads: wrap.ArgonThreads,
	}

	return db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "kind"}},
		UpdateAll: true,
	}).Create(&entry).Error
}

func (s *DBStore) GetKeyWrap(ctx context.Context, kind string) (*KeyWrap, error) {
	db, err := s.ensureDB()
	if err != nil {
		return nil, err
	}

	var entry database.CredentialKeyWrap
	if err := db.WithContext(ctx).Where("kind = ?", kind).First(&entry).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	return &KeyWrap{
		Kind:         entry.Kind,
		Salt:         entry.Salt,
		WrappedDEK:   entry.WrappedDEK,
		ArgonTime:    entry.ArgonTime,
		ArgonMemory:  entry.ArgonMemory,
		ArgonThreads: entry.ArgonThreads,
	}, nil
}

func (s *DBStore) HasKeyWrap(ctx context.Context, kind string) (bool, error) {
	db, err := s.ensureDB()
	if err != nil {
		return false, err
	}

	var count int64
	if err := db.WithContext(ctx).Model(&database.CredentialKeyWrap{}).Where("kind = ?", kind).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}
