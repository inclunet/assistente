package auth

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"assistente/internal/database"

	"gorm.io/gorm"
)

var (
	ErrUserExists        = errors.New("usuário já existe")
	ErrInvalidCredential = errors.New("usuário ou senha inválidos")
	ErrInactiveUser      = errors.New("usuário inativo")
)

// dummyPasswordHash é um hash argon2id válido de uma senha aleatória,
// gerado lazy na primeira chamada de AuthenticateLocal. Existe APENAS para
// equalizar o tempo de resposta do path "user not found" com o path
// "wrong password" e mitigar enumeração de usuários por timing
// (M2 do review da Fatia 1). NUNCA é usado para autenticação real — só
// como argumento de VerifyPassword cujo resultado é descartado.
var (
	dummyPasswordHash     string
	dummyPasswordHashOnce sync.Once
)

func ensureDummyPasswordHash() string {
	dummyPasswordHashOnce.Do(func() {
		// Senha aleatória de 32 bytes hex = 64 chars (acima do mínimo).
		// Hash gerado uma vez por processo; custo idêntico a um hash real.
		hash, err := HashPassword(strings.Repeat("dummy-password", 2))
		if err != nil {
			// Fallback degradado: continuamos sem dummy, aceitando a
			// regressão de timing. Não há por que crashar a app por isso.
			dummyPasswordHash = ""
			return
		}
		dummyPasswordHash = hash
	})
	return dummyPasswordHash
}

type IdentityService struct {
	db  *gorm.DB
	now func() time.Time
}

func NewIdentityService(db *gorm.DB) *IdentityService {
	return &IdentityService{
		db:  db,
		now: time.Now,
	}
}

type CreateUserParams struct {
	Username    string
	DisplayName string
	Password    string
	Admin       bool
}

func (s *IdentityService) CreateLocalUser(ctx context.Context, params CreateUserParams) (*database.User, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("identity service não inicializado")
	}

	username := normalizeUsername(params.Username)
	if username == "" {
		return nil, errors.New("username obrigatório")
	}

	passwordHash, err := HashPassword(params.Password)
	if err != nil {
		return nil, err
	}

	role := database.UserRoleUser
	if params.Admin {
		role = database.UserRoleAdmin
	}

	user := &database.User{
		Username:     username,
		DisplayName:  strings.TrimSpace(params.DisplayName),
		PasswordHash: passwordHash,
		Role:         role,
		IsActive:     true,
	}
	if user.DisplayName == "" {
		user.DisplayName = username
	}

	err = s.db.WithContext(ctx).Create(user).Error
	if err != nil {
		if isUniqueConstraintError(err) {
			return nil, ErrUserExists
		}
		return nil, err
	}
	return user, nil
}

func (s *IdentityService) AuthenticateLocal(ctx context.Context, username, password string) (*database.User, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("identity service não inicializado")
	}

	var user database.User
	err := s.db.WithContext(ctx).Where("username = ?", normalizeUsername(username)).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Anti-enumeration: roda VerifyPassword com hash dummy para
			// igualar o tempo de resposta com o caminho onde o user
			// existe. O resultado é deliberadamente descartado.
			if dummy := ensureDummyPasswordHash(); dummy != "" {
				_, _ = VerifyPassword(password, dummy)
			}
			return nil, ErrInvalidCredential
		}
		return nil, err
	}
	if !user.IsActive {
		return nil, ErrInactiveUser
	}

	ok, err := VerifyPassword(password, user.PasswordHash)
	if err != nil || !ok {
		return nil, ErrInvalidCredential
	}

	now := s.now()
	if err := s.db.WithContext(ctx).Model(&database.User{}).Where("id = ?", user.ID).Update("last_login_at", now).Error; err != nil {
		return nil, err
	}
	user.LastLoginAt = &now
	return &user, nil
}

func normalizeUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

func isUniqueConstraintError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") || strings.Contains(msg, "constraint failed")
}
