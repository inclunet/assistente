package auth

import (
	"assistente/internal/logging"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	passwordHashPrefix = "$assistente-argon2id$v=1"
	argonTime          = 3
	argonMemory        = 64 * 1024
	argonThreads       = 2
	argonKeyLen        = 32
	argonSaltLen       = 16

	// MinPasswordLength reflete a baseline NIST SP 800-63B para senhas
	// memorizáveis. Não bloqueamos por complexidade (caracteres especiais
	// etc.) — comprimento + alta entropia random é mais eficaz contra
	// brute-force do que regras de composição (M4 do review da Fatia 1).
	MinPasswordLength = 8
)

var (
	ErrInvalidPasswordHash = errors.New("hash de senha inválido")
	ErrPasswordTooShort    = fmt.Errorf("senha precisa de pelo menos %d caracteres", MinPasswordLength)
)

func HashPassword(password string) (string, error) {
	if strings.TrimSpace(password) == "" {
		return "", errors.New("senha obrigatória")
	}
	if len(password) < MinPasswordLength {
		return "", ErrPasswordTooShort
	}

	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}

	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf(
		"%s$m=%d,t=%d,p=%d$%s$%s",
		passwordHashPrefix,
		argonMemory,
		argonTime,
		argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

func VerifyPassword(password, encodedHash string) (bool, error) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 || parts[1] != "assistente-argon2id" || parts[2] != "v=1" {
		return false, ErrInvalidPasswordHash
	}

	params, err := parseArgonParams(parts[3])
	if err != nil {
		return false, err
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, ErrInvalidPasswordHash
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, ErrInvalidPasswordHash
	}
	if len(expected) == 0 {
		return false, ErrInvalidPasswordHash
	}

	actual := argon2.IDKey([]byte(password), salt, params.time, params.memory, params.threads, uint32(len(expected)))
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}

type argonParams struct {
	memory  uint32
	time    uint32
	threads uint8
}

func parseArgonParams(raw string) (argonParams, error) {
	values := map[string]string{}
	for _, segment := range strings.Split(raw, ",") {
		key, value, ok := strings.Cut(segment, "=")
		if !ok {
			return argonParams{}, ErrInvalidPasswordHash
		}
		values[key] = value
	}

	memory, err := parsePositiveUint32(values["m"])
	if err != nil {
		return argonParams{}, ErrInvalidPasswordHash
	}
	timeCost, err := parsePositiveUint32(values["t"])
	if err != nil {
		return argonParams{}, ErrInvalidPasswordHash
	}
	threads, err := strconv.ParseUint(values["p"], 10, 8)
	if err != nil || threads == 0 {
		return argonParams{}, ErrInvalidPasswordHash
	}

	return argonParams{
		memory:  memory,
		time:    timeCost,
		threads: uint8(threads),
	}, nil
}

// parsePositiveUint32 parseia um uint32 e rejeita o valor zero por
// design — parâmetros Argon2 (memory, time, parallelism) com valor
// zero indicam hash corrompido (B1 do review da Fatia 1). Um user
// com hash assim precisa redefinir a senha; aqui sinalizamos em log
// para o operador (P2-2 do re-review do PR #94).
func parsePositiveUint32(raw string) (uint32, error) {
	value, err := strconv.ParseUint(raw, 10, 32)
	if err != nil {
		return 0, err
	}
	if value == 0 {
		logging.Infof(context.Background(), "auth.password", "[Auth] hash de senha com parametro Argon2 zero detectado - corrupcao provavel, user precisa redefinir senha")
		return 0, ErrInvalidPasswordHash
	}
	return uint32(value), nil
}
