package auth

import (
	"assistente/internal/logging"
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
			//
			// P2-1 do re-review do PR #94: sinalizar em log com nível
			// crítico para que o operador saiba que a defesa contra
			// enumeração por timing (M2) não está ativa. Em produção
			// isso só acontece em condições muito patológicas (ex.:
			// argon2 falhando por OOM real).
			logging.Errorf(context.Background(), "auth.identity", "[Auth] CRITICO: dummy_password_hash falhou na inicializacao - defesa contra enumeracao por timing DESATIVADA: %v", err)
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

// isUniqueConstraintError detecta violação de unique index em múltiplos
// dialects (M5 do review da Fatia 1).
//
// Hoje o projeto usa SQLite (`glebarez/sqlite` → `modernc.org/sqlite`)
// que retorna mensagens com "constraint failed: UNIQUE constraint
// failed". Para que a mesma função detecte conflitos quando rodando
// contra Postgres ou MySQL — exatamente o que o AEP-0052 prevê em
// deployment futuro — incluímos as strings desses drivers no fallback:
//
//   - SQLite (`modernc`): "UNIQUE constraint failed"
//   - Postgres (`pq`/`pgx`): "duplicate key value violates unique
//     constraint" (SQLSTATE 23505)
//   - MySQL: "Error 1062: Duplicate entry"
//
// Quando algum desses drivers virar dependência direta, trocar a
// heurística por `errors.As` contra o tipo concreto do driver
// (sqlite3.Error.ExtendedCode, pgconn.PgError.Code, MySQLError.Number).
func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") ||
		strings.Contains(msg, "constraint failed") ||
		strings.Contains(msg, "duplicate key") ||
		strings.Contains(msg, "duplicate entry")
}
