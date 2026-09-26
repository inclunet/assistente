package credentials

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"assistente/internal/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	// ErrInstanceSecretPersistenceRequired indica que a criação de um segredo
	// de instância foi tentada sem um caminho de persistência durável.
	ErrInstanceSecretPersistenceRequired = errors.New("instance secret persistence is required")

	// ErrInstanceSecretStoreUnsupported indica que o Store não oferece as
	// operações exatas necessárias para o get-or-create atômico.
	ErrInstanceSecretStoreUnsupported = errors.New("instance secret store does not support atomic get-or-create")

	// ErrInstanceSecretIntegrity indica que o estado do cofre não está íntegro
	// o bastante para cifrar e persistir uma nova credencial.
	ErrInstanceSecretIntegrity = errors.New("credential vault integrity is not OK")

	// ErrInstanceSecretInvalid indica que já existe uma linha para o pattern,
	// mas ela não é um segredo de instância legível e utilizável.
	ErrInstanceSecretInvalid = errors.New("stored instance secret is invalid")

	// ErrInstanceSecretCreate é deliberadamente genérico: o erro arbitrário do
	// provider não deve atravessar esta API nem correr o risco de vazar dados.
	ErrInstanceSecretCreate = errors.New("instance secret generation failed")

	// ErrInstanceSecretStore representa qualquer falha de I/O do Store sem
	// devolver SQL, ciphertext ou outros detalhes internos ao caller.
	ErrInstanceSecretStore = errors.New("instance secret store operation failed")
)

// instanceSecretCreateStore é separado de Store para não enfraquecer o
// contrato existente de credenciais. As duas operações são estritamente
// instance-scoped; nenhuma consulta usa o user_id do contexto.
type instanceSecretCreateStore interface {
	GetInstanceCredential(ctx context.Context, pattern string) (StoredCredential, bool, error)
	InsertInstanceCredentialIfAbsent(ctx context.Context, cred StoredCredential) (bool, error)
}

// EnsureInstanceSecret retorna o segredo de instância já persistido ou cria e
// persiste exatamente um segredo para pattern.
//
// A ausência é sempre comprovada no Store, nunca no cache do Manager. A
// inserção é insert-if-absent e o valor retornado é sempre relido do banco,
// inclusive quando este Manager perdeu a corrida para outro Manager.
func (m *Manager) EnsureInstanceSecret(ctx context.Context, pattern string, create func() (string, error)) (string, error) {
	if m == nil {
		return "", ErrInstanceSecretPersistenceRequired
	}
	if ctx == nil {
		return "", errors.New("context is required")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}

	if pattern == "" || pattern != strings.TrimSpace(pattern) || !IsInstanceSecretPattern(pattern) {
		return "", ErrInstanceSecretInvalid
	}
	if create == nil {
		return "", ErrInstanceSecretCreate
	}

	// Reset também usa mu. Mantê-lo até depois da releitura e da atualização do
	// cache impede trocar encKey/persist no meio de uma cifra ou de um commit.
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return "", err
	}
	store, ok := m.store.(instanceSecretCreateStore)
	if !m.persist || m.store == nil {
		return "", ErrInstanceSecretPersistenceRequired
	}
	if !ok {
		return "", ErrInstanceSecretStoreUnsupported
	}

	if err := m.ensureInstanceSecretReadyLocked(ctx); err != nil {
		return "", err
	}

	// O cache não é consultado para provar ausência. Esta leitura é exata e
	// nunca considera uma credencial user-scoped, mesmo que o contexto tenha
	// um usuário ou que haja uma linha user-scoped com o mesmo pattern.
	existing, found, err := store.GetInstanceCredential(ctx, pattern)
	if err != nil {
		return "", sanitizeInstanceSecretStoreError(err)
	}
	if found {
		value, err := m.validateStoredInstanceSecretLocked(pattern, existing)
		if err != nil {
			return "", err
		}
		if err := m.cacheInstanceCredentialLocked(existing); err != nil {
			return "", err
		}
		return value, nil
	}

	// Revalida imediatamente antes de qualquer geração/cifra por leitura focada
	// do master wrap e do status do vault; não consulta keychainIO.
	if err := m.ensureInstanceSecretReadyLocked(ctx); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}

	value, providerErr := create()
	if providerErr != nil {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		return "", ErrInstanceSecretCreate
	}
	if value == "" {
		return "", ErrInstanceSecretCreate
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}

	ciphertext, err := m.encrypt(value)
	if err != nil {
		return "", ErrInstanceSecretCreate
	}
	candidate := StoredCredential{
		UserID:  "",
		Pattern: pattern,
		Auth: &AuthConfig{Source: "static",
			Type:  "secret",
			Token: ciphertext,
		},
	}

	// A checagem precisa ocorrer de novo após a geração: o callback pode ter
	// demorado e o contrato exige falha fechada se o cofre deixou de estar OK.
	if err := m.ensureInstanceSecretReadyLocked(ctx); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if _, err := store.InsertInstanceCredentialIfAbsent(ctx, candidate); err != nil {
		return "", sanitizeInstanceSecretStoreError(err)
	}

	// Mesmo quando esta Manager venceu, o resultado canônico vem do Store. Em
	// caso de corrida, o INSERT não atualiza a linha e esta releitura devolve o
	// vencedor sem sobrescrevê-lo.
	winner, found, err := store.GetInstanceCredential(ctx, pattern)
	if err != nil {
		return "", sanitizeInstanceSecretStoreError(err)
	}
	if !found {
		return "", ErrInstanceSecretIntegrity
	}
	if err := m.ensureInstanceSecretReadyLocked(ctx); err != nil {
		return "", err
	}
	winningValue, err := m.validateStoredInstanceSecretLocked(pattern, winner)
	if err != nil {
		return "", err
	}
	if err := m.cacheInstanceCredentialLocked(winner); err != nil {
		return "", err
	}
	return winningValue, nil
}

func (m *Manager) ensureInstanceSecretReadyLocked(ctx context.Context) error {
	if !m.persist || m.store == nil {
		return ErrInstanceSecretPersistenceRequired
	}
	if _, ok := m.store.(instanceSecretCreateStore); !ok {
		return ErrInstanceSecretStoreUnsupported
	}
	// Ensure não inicializa o cofre. O caller precisa ter passado pelo
	// carregamento/validação do vault (normalmente LoadInstanceSecrets), que
	// publicou um status OK. A revalidação abaixo é focada no master_wrap e não
	// faz scan, adoção de wrap legado ou keychainIO.
	if !m.integrity.get().OK {
		return ErrInstanceSecretIntegrity
	}
	wrap, err := m.store.GetKeyWrap(ctx, KeyWrapKindMaster)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return ErrInstanceSecretIntegrity
	}
	status := m.integrity.get()
	if status.KeychainDekID != DEKIdentity(m.encKey) || (wrap != nil) != status.HasMasterWrap {
		return ErrInstanceSecretIntegrity
	}
	if wrap != nil && (wrap.DekID == "" || wrap.DekID != DEKIdentity(m.encKey)) {
		return ErrInstanceSecretIntegrity
	}
	if !status.OK || !m.persist {
		return ErrInstanceSecretIntegrity
	}
	return nil
}

func sanitizeInstanceSecretStoreError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		if errors.Is(err, context.Canceled) {
			return context.Canceled
		}
		return context.DeadlineExceeded
	}
	return ErrInstanceSecretStore
}

func (m *Manager) validateStoredInstanceSecretLocked(pattern string, cred StoredCredential) (string, error) {
	if cred.UserID != "" || cred.Pattern != pattern || cred.Auth == nil || cred.Auth.Type != "secret" || cred.Auth.Token == "" {
		return "", ErrInstanceSecretInvalid
	}
	value, err := m.decrypt(cred.Auth.Token)
	if err != nil || value == "" {
		return "", ErrInstanceSecretInvalid
	}
	return value, nil
}

// cacheInstanceCredentialLocked atualiza apenas a entrada instance-scoped.
// O chamador mantém m.mu; uma entrada user-scoped nunca é substituída por
// este caminho.
func (m *Manager) cacheInstanceCredentialLocked(cred StoredCredential) error {
	regex, err := regexpForInstanceSecret(cred.Pattern)
	if err != nil {
		return err
	}
	for i, existing := range m.credentials {
		if existing.UserID == "" && existing.Pattern == cred.Pattern {
			m.credentials[i] = &DomainCredential{
				ID:      cred.ID,
				UserID:  "",
				Pattern: cred.Pattern,
				regex:   regex,
				Auth:    cred.Auth,
			}
			return nil
		}
	}
	m.credentials = append(m.credentials, &DomainCredential{
		ID:      cred.ID,
		UserID:  "",
		Pattern: cred.Pattern,
		regex:   regex,
		Auth:    cred.Auth,
	})
	return nil
}

func regexpForInstanceSecret(pattern string) (*regexp.Regexp, error) {
	return regexp.Compile("^" + regexp.QuoteMeta(pattern) + "$")
}

// GetInstanceCredential lê somente a linha instance-scoped de pattern.
// Ele existe fora de Store por ser uma operação deliberadamente mais estreita
// que ListCredentials, cujo comportamento histórico depende de contexto.
func (s *DBStore) GetInstanceCredential(ctx context.Context, pattern string) (StoredCredential, bool, error) {
	db, err := s.ensureDB()
	if err != nil {
		return StoredCredential{}, false, err
	}
	if pattern == "" || pattern != strings.TrimSpace(pattern) || !IsInstanceSecretPattern(pattern) {
		return StoredCredential{}, false, ErrInstanceSecretInvalid
	}

	var entry database.CredentialEntry
	err = db.WithContext(ctx).Where("user_id = '' AND pattern = ?", pattern).First(&entry).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return StoredCredential{}, false, nil
	}
	if err != nil {
		return StoredCredential{}, false, err
	}
	cred, err := storedCredentialFromDatabaseEntry(entry)
	if err != nil {
		return StoredCredential{}, false, err
	}
	return cred, true, nil
}

// InsertInstanceCredentialIfAbsent grava uma credencial instance-scoped sem
// atualizar a linha existente. A unicidade natural (user_id, pattern) é a
// autoridade de arbitragem entre Managers distintos.
func (s *DBStore) InsertInstanceCredentialIfAbsent(ctx context.Context, cred StoredCredential) (bool, error) {
	db, err := s.ensureDB()
	if err != nil {
		return false, err
	}
	if cred.UserID != "" || cred.Pattern == "" || cred.Pattern != strings.TrimSpace(cred.Pattern) || !IsInstanceSecretPattern(cred.Pattern) || cred.Auth == nil || cred.Auth.Type != "secret" || cred.Auth.Token == "" {
		return false, ErrInstanceSecretInvalid
	}

	headersJSON := ""
	if len(cred.Auth.Headers) > 0 {
		data, err := json.Marshal(cred.Auth.Headers)
		if err != nil {
			return false, err
		}
		headersJSON = string(data)
	}
	entry := database.CredentialEntry{
		UUIDModel:       database.UUIDModel{ID: cred.ID},
		UserID:          "",
		Pattern:         cred.Pattern,
		AuthType:        cred.Auth.Type,
		Source:          cred.Auth.Source,
		SourceConfigEnc: cred.Auth.SourceConfigEnc,
		TokenEnc:        cred.Auth.Token,
		Username:        cred.Auth.Username,
		PasswordEnc:     cred.Auth.Password,
		HeadersEnc:      headersJSON,
		ExpiresAt:       cred.Auth.ExpiresAt,
		RefreshTokenEnc: cred.Auth.RefreshURL,
		ClientIDEnc:     cred.Auth.ClientID,
		ClientSecretEnc: cred.Auth.ClientSecret,
	}

	result := db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "pattern"}},
		DoNothing: true,
	}).Create(&entry)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

func storedCredentialFromDatabaseEntry(entry database.CredentialEntry) (StoredCredential, error) {
	headers := map[string]string{}
	if entry.HeadersEnc != "" {
		if err := json.Unmarshal([]byte(entry.HeadersEnc), &headers); err != nil {
			return StoredCredential{}, err
		}
	}
	return StoredCredential{
		ID:      entry.ID,
		UserID:  entry.UserID,
		Pattern: entry.Pattern,
		Auth: &AuthConfig{Source: entry.Source, SourceConfigEnc: entry.SourceConfigEnc,
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
	}, nil
}
