package credentials

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"

	"assistente/internal/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrStoreNotReady = errors.New("store de credenciais não inicializado")

// ErrEmptyPatternDelete é retornado quando alguém tenta apagar uma
// credencial sem informar `pattern`. SQLite trata `WHERE pattern = ''`
// literalmente — só apaga rows com pattern realmente vazio — mas
// chamadores que confundem "limpar tudo" com `DeletePattern("")` deixam
// o sistema em um estado em que a operação parece sem efeito (nenhum
// erro, nada apagado) ou, com bug futuro de wildcard expansion, pode
// virar mass-delete silencioso. Falhar fechado força o caller a usar
// um caminho explícito de limpeza em massa (ListCredentials + iterate
// + DeletePattern) com escopo de usuário garantido.
//
// Histórico (incident report 10/05/2026, AEP-0053): o usuário perdeu 13
// credenciais durante o primeiro boot do AEP-0052; a investigação
// forensic mostrou que controllers/settings_controller.go:189 e
// internal/config/settings_service.go:140 chamavam DeletePattern com
// string vazia. Hoje o Manager rejeita, mas a defesa no DBStore garante
// que qualquer caller futuro (testes que mockam o Manager, novos
// callers, refatorações) não consegue burlar o invariante.
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
	// Defense-in-depth (incident report AEP-0053): pattern vazio em
	// SaveCredential cria uma row "fantasma" que vira target de qualquer
	// `WHERE pattern = ''` futuro — incluindo um eventual ClearAll
	// quebrado. Falhamos fechado para que o caller informe o pattern.
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

// DeleteCredential remove a credencial associada ao `pattern` exato,
// escopada pelo usuário do contexto. Para instance secrets
// (`internal-auth:*`/`internal-tls:*`) o escopo é `user_id = ''`.
//
// INVARIANTES (defense-in-depth — incident report AEP-0053):
//
//  1. `pattern` não pode ser vazio. `WHERE pattern = ''` parece
//     inofensivo, mas:
//     - rejeita silenciosamente intenções de "limpar tudo" (erro mudo);
//     - se o futuro DBStore mudar para aceitar wildcards/SQL building,
//     vira vetor de mass-delete acidental.
//     Falhamos fechado com `ErrEmptyPatternDelete` e logamos.
//
//  2. Cada chamada loga o `pattern`, o user scope e o número de rows
//     afetadas. Mass-delete acidental fica visível no log de produção
//     em vez de silenciosamente apagar credenciais.
//
//  3. Se RowsAffected > 1 emite WARN — `pattern` é exato, então o único
//     caso esperado é `pattern` órfão coexistindo com claimed (legacy
//     pré-AEP-0052), que é raro. Mais que isso é red flag.
func (s *DBStore) DeleteCredential(ctx context.Context, pattern string) error {
	db, err := s.ensureDB()
	if err != nil {
		return err
	}
	if strings.TrimSpace(pattern) == "" {
		log.Printf("[Credentials] BLOQUEADO: tentativa de DeleteCredential com pattern vazio (user_scoped=%v)", isUserScoped(ctx))
		return ErrEmptyPatternDelete
	}

	var res *gorm.DB
	scope := "instance"
	if IsInstanceSecretPattern(pattern) {
		res = db.WithContext(ctx).Where("user_id = '' AND pattern = ?", pattern).Delete(&database.CredentialEntry{})
	} else {
		scope = userScopeLabel(ctx)
		query := database.ScopeByUser(ctx, db.WithContext(ctx), "user_id")
		res = query.Where("pattern = ?", pattern).Delete(&database.CredentialEntry{})
	}
	if res.Error != nil {
		return res.Error
	}

	if res.RowsAffected > 1 {
		log.Printf("[Credentials] WARN: DeleteCredential pattern=%q scope=%s afetou %d rows (esperado <=1) — possível duplicata legacy ou bug de escopo", pattern, scope, res.RowsAffected)
	} else if res.RowsAffected == 1 {
		log.Printf("[Credentials] DeleteCredential pattern=%q scope=%s ok", pattern, scope)
	}
	return nil
}

func isUserScoped(ctx context.Context) bool {
	_, ok := database.UserIDFromContext(ctx)
	return ok
}

func userScopeLabel(ctx context.Context) string {
	if userID, ok := database.UserIDFromContext(ctx); ok {
		return "user=" + userID
	}
	return "user=<unscoped>"
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
