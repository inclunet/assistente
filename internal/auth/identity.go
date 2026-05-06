package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"assistente/internal/database"

	"gorm.io/gorm"
)

var (
	ErrUserExists        = errors.New("usuário já existe")
	ErrInvalidCredential = errors.New("usuário ou senha inválidos")
	ErrInactiveUser      = errors.New("usuário inativo")
)

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
