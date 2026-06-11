package credentials

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"assistente/internal/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrStoreNotReady = errors.New("store de credenciais não inicializado")

// ErrEmptyPatternDelete é retornado quando alguém tenta apagar uma
// credencial sem informar `pattern`. "Limpar tudo" precisa ser
// expressado como iteração explícita sobre a lista visível, não como
// um `DeletePattern("")` que parece inofensivo mas é ambíguo.
var ErrEmptyPatternDelete = errors.New("pattern vazio não é permitido em DeleteCredential — use ListCredentials + iterate para limpeza em massa")

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
	if strings.TrimSpace(cred.Pattern) == "" {
		return errors.New("pattern vazio não é permitido em SaveCredential")
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
	if IsInstanceSecretPattern(cred.Pattern) {
		userID = ""
	}
	if userID == "" && !IsInstanceSecretPattern(cred.Pattern) {
		return database.ErrUserScopeRequired
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
		if IsInstanceSecretPattern(cred.Pattern) {
			return db.WithContext(ctx).Where("user_id = '' AND id = ?", cred.ID).Save(&entry).Error
		}
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

func (s *DBStore) ListInstanceCredentials(ctx context.Context) ([]StoredCredential, error) {
	db, err := s.ensureDB()
	if err != nil {
		return nil, err
	}

	var entries []database.CredentialEntry
	if err := db.WithContext(ctx).
		Where("user_id = ''").
		Where("pattern LIKE ? OR pattern LIKE ?", "internal-auth:%", "internal-tls:%").
		Find(&entries).Error; err != nil {
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
		result = append(result, StoredCredential{
			ID:      entry.ID,
			UserID:  entry.UserID,
			Pattern: entry.Pattern,
			Auth: &AuthConfig{
				Type:         entry.AuthType,
				Token:        entry.TokenEnc,
				Username:     entry.Username,
				Password:     entry.PasswordEnc,
				Headers:      headers,
				ExpiresAt:    entry.ExpiresAt,
				RefreshURL:   entry.RefreshTokenEnc,
				ClientID:     entry.ClientIDEnc,
				ClientSecret: entry.ClientSecretEnc,
			},
		})
	}
	return result, nil
}

func (s *DBStore) HasAnyCredentials(ctx context.Context) (bool, error) {
	db, err := s.ensureDB()
	if err != nil {
		return false, err
	}

	var count int64
	if err := db.WithContext(ctx).Model(&database.CredentialEntry{}).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// ListAllCredentialsIgnoringScope retorna TODAS as credenciais
// independentemente do user_id no contexto. Existe APENAS para
// `Manager.scanUnreadableCredentialIDs` poder identificar credenciais
// órfãs (cifradas com DEK divergente). NÃO USE em código que serve
// requests do app — esse caminho viola intencionalmente o
// user-scope. Ver AEP-0061.
func (s *DBStore) ListAllCredentialsIgnoringScope(ctx context.Context) ([]StoredCredential, error) {
	db, err := s.ensureDB()
	if err != nil {
		return nil, err
	}
	var entries []database.CredentialEntry
	if err := db.WithContext(ctx).Find(&entries).Error; err != nil {
		return nil, err
	}
	result := make([]StoredCredential, 0, len(entries))
	for _, entry := range entries {
		headers := map[string]string{}
		if entry.HeadersEnc != "" {
			if err := json.Unmarshal([]byte(entry.HeadersEnc), &headers); err != nil {
				continue
			}
		}
		result = append(result, StoredCredential{
			ID:      entry.ID,
			UserID:  entry.UserID,
			Pattern: entry.Pattern,
			Auth: &AuthConfig{
				Type:         entry.AuthType,
				Token:        entry.TokenEnc,
				Username:     entry.Username,
				Password:     entry.PasswordEnc,
				Headers:      headers,
				ExpiresAt:    entry.ExpiresAt,
				RefreshURL:   entry.RefreshTokenEnc,
				ClientID:     entry.ClientIDEnc,
				ClientSecret: entry.ClientSecretEnc,
			},
		})
	}
	return result, nil
}

// DeleteCredentialsByID remove credenciais pela lista de IDs,
// independentemente de user_id ou pattern. Existe APENAS para o
// caminho de purge de credenciais ilegíveis (ver
// `Manager.PurgeUnreadableCredentials`); não use para fluxos
// normais — DeleteCredential pelo pattern com user-scope é o caminho
// canônico do produto.
func (s *DBStore) DeleteCredentialsByID(ctx context.Context, ids []string) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	db, err := s.ensureDB()
	if err != nil {
		return 0, err
	}
	res := db.WithContext(ctx).Where("id IN ?", ids).Delete(&database.CredentialEntry{})
	if res.Error != nil {
		return 0, res.Error
	}
	return int(res.RowsAffected), nil
}

// UpdateRefreshTokenEncByID regrava APENAS a coluna `refresh_token_enc`
// da credencial identificada por `id`, sem aplicar user-scope. Existe
// APENAS para a re-cifragem one-shot de refresh tokens legados em texto
// plano (`Manager.reencryptLegacyPlaintextRefreshTokens`, issue #236),
// que roda no boot antes de qualquer sessão — não use em fluxos que
// servem requests do app; o caminho canônico é SaveCredential com
// user-scope.
func (s *DBStore) UpdateRefreshTokenEncByID(ctx context.Context, id, value string) error {
	db, err := s.ensureDB()
	if err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" {
		return errors.New("id vazio não é permitido em UpdateRefreshTokenEncByID")
	}
	res := db.WithContext(ctx).Model(&database.CredentialEntry{}).
		Where("id = ?", id).
		Update("refresh_token_enc", value)
	if res.Error != nil {
		return fmt.Errorf("atualizar refresh_token_enc da credencial %s: %w", id, res.Error)
	}
	return nil
}

// DeleteCredential remove a credencial associada ao `pattern` exato,
// escopada pelo usuário do contexto. Para instance secrets
// (`internal-auth:*`/`internal-tls:*`) o escopo é `user_id = ''`.
//
// `pattern` vazio é erro: "limpar tudo" tem que ser expressado como
// iteração sobre a lista visível, não como uma chamada sem nome.
func (s *DBStore) DeleteCredential(ctx context.Context, pattern string) error {
	db, err := s.ensureDB()
	if err != nil {
		return err
	}
	if strings.TrimSpace(pattern) == "" {
		return ErrEmptyPatternDelete
	}

	if IsInstanceSecretPattern(pattern) {
		return db.WithContext(ctx).Where("user_id = '' AND pattern = ?", pattern).Delete(&database.CredentialEntry{}).Error
	}
	return database.ScopeByUser(ctx, db.WithContext(ctx), "user_id").
		Where("pattern = ?", pattern).
		Delete(&database.CredentialEntry{}).Error
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
		DekID:        wrap.DekID,
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
		DekID:        entry.DekID,
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
