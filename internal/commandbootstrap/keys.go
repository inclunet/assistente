package commandbootstrap

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"assistente/internal/credentials"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrKeys = errors.New("chaves de comandos indisponíveis ou incompatíveis")

// Diagnóstico estável, sem incluir a assinatura, versão ou segredo recebido.
var ErrFingerprintReference = fmt.Errorf("%w: referência de fingerprint inválida", ErrKeys)

// Só serializa operações raras de bootstrap/rotação neste processo. A criação
// da credencial e o CAS de versão também são atômicos no banco entre processos.
var keysMu sync.Mutex

type keyVersion struct {
	ID              string `gorm:"primaryKey"`
	Version, Digest string
	Active          int
}

func (keyVersion) TableName() string { return "command_key_versions" }

func versionNumber(version string) (uint64, error) {
	if !strings.HasPrefix(version, "v") {
		return 0, ErrKeys
	}
	n, err := strconv.ParseUint(strings.TrimPrefix(version, "v"), 10, 63)
	if err != nil || n == 0 || version != "v"+strconv.FormatUint(n, 10) {
		return 0, ErrKeys
	}
	return n, nil
}

func keyDigest(value string) (string, error) {
	key, err := base64.RawURLEncoding.Strict().DecodeString(value)
	defer clear(key)
	if err != nil || len(key) < 32 || base64.RawURLEncoding.EncodeToString(key) != value {
		return "", ErrKeys
	}
	digest := sha256.Sum256(key)
	return hex.EncodeToString(digest[:]), nil
}

func generateKey() (string, error) {
	key := make([]byte, 32)
	defer clear(key)
	if _, err := rand.Read(key); err != nil {
		return "", ErrKeys
	}
	return base64.RawURLEncoding.EncodeToString(key), nil
}

// PrepareKeys exige Migrate concluído e Manager persistente já carregado.
// Nunca usa chave efêmera nem recria versão perdida. Todas as versões antigas
// são conservadas (inclusive além do deadline); coleta futura pertence a I12.
func PrepareKeys(ctx context.Context, db *gorm.DB, manager *credentials.Manager) (string, error) {
	if ctx == nil || !rootDB(db) || manager == nil {
		return "", ErrKeys
	}
	keysMu.Lock()
	defer keysMu.Unlock()
	return prepareKeys(ctx, db, manager)
}

func prepareKeys(ctx context.Context, db *gorm.DB, manager *credentials.Manager) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	var versions []keyVersion
	if err := db.WithContext(ctx).Find(&versions).Error; err != nil {
		return "", ErrKeys
	}
	if len(versions) == 0 {
		value, err := manager.EnsureInstanceSecret(ctx, "internal-auth:command-request-hmac:v1", func() (string, error) {
			// Ausência de metadata não prova instalação nova: releases de
			// protótipo já podiam ter ledgers/receipts sem esse registro.
			for _, table := range []string{"command_idempotency_keys", "command_invocations", "command_decision_receipts", "command_config_mutations"} {
				var count int64
				if err := db.WithContext(ctx).Table(table).Count(&count).Error; err != nil || count != 0 {
					return "", ErrKeys
				}
			}
			return generateKey()
		})
		if err != nil {
			return "", keyError(ctx)
		}
		digest, err := keyDigest(value)
		if err != nil {
			return "", ErrKeys
		}
		row, err := newKeyVersion("v1", digest)
		if err != nil {
			return "", ErrKeys
		}
		if err := db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return "", ErrKeys
		}
		if err := db.WithContext(ctx).Find(&versions).Error; err != nil {
			return "", ErrKeys
		}
	}
	active := ""
	known := map[string]bool{}
	for _, version := range versions {
		if _, err := versionNumber(version.Version); err != nil {
			return "", ErrKeys
		}
		value, err := manager.EnsureInstanceSecret(ctx, "internal-auth:command-request-hmac:"+version.Version, func() (string, error) { return "", ErrKeys })
		if err != nil {
			return "", keyError(ctx)
		}
		digest, err := keyDigest(value)
		if err != nil || digest != version.Digest {
			return "", ErrKeys
		}
		known[version.Version] = true
		if version.Active == 1 {
			if active != "" {
				return "", ErrKeys
			}
			active = version.Version
		} else if version.Active != 0 {
			return "", ErrKeys
		}
	}
	if active == "" {
		return "", ErrKeys
	}
	for _, table := range []string{"command_idempotency_keys", "command_invocations"} {
		var referenced []string
		if err := db.WithContext(ctx).Table(table).Distinct("request_fingerprint_version").Pluck("request_fingerprint_version", &referenced).Error; err != nil {
			return "", ErrKeys
		}
		for _, version := range referenced {
			if !known[version] {
				return "", ErrKeys
			}
		}
	}
	for _, table := range []string{"command_decision_receipts", "command_config_mutations"} {
		var fingerprints []string
		if err := db.WithContext(ctx).Table(table).Distinct("request_fingerprint").Pluck("request_fingerprint", &fingerprints).Error; err != nil {
			return "", ErrKeys
		}
		for _, fingerprint := range fingerprints {
			if !storedFingerprintAvailable(fingerprint, known) {
				return "", ErrFingerprintReference
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return active, nil
}

// Os signers canônicos (configuração, grants e envelopes) retornam digest
// hexadecimal: a versão já participa do domínio HMAC. O signer anterior de
// configuração retorna vN:digest. Não confundir digest opaco com referência
// a versão inexistente. A verificação acima exige TODAS as chaves registradas;
// não inferimos versão para digest opaco, não coletamos chaves e não reescrevemos
// receipts. Isto só verifica prontidão: não autentica nem consome uma decisão.
func storedFingerprintAvailable(fingerprint string, known map[string]bool) bool {
	digest := fingerprint
	if version, value, tagged := strings.Cut(fingerprint, ":"); tagged {
		if !known[version] {
			return false
		}
		digest = value
	}
	if len(known) == 0 || len(digest) != sha256.Size*2 {
		return false
	}
	for _, c := range digest {
		if !(c >= '0' && c <= '9') && !(c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func keyError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return ErrKeys
}

// RotateKeys é manutenção interna explícita, não uma API de cliente. O host
// deve suspender novas admissões e publicar o novo executor/versão só após
// sucesso. A versão esperada faz duas solicitações concorrentes serem um único
// avanço. Não exclui chaves anteriores nem altera JWT, pepper ou a DEK.
func RotateKeys(ctx context.Context, db *gorm.DB, manager *credentials.Manager, expected string) (string, error) {
	if ctx == nil || !rootDB(db) || manager == nil {
		return "", ErrKeys
	}
	keysMu.Lock()
	defer keysMu.Unlock()
	active, err := prepareKeys(ctx, db, manager)
	if err != nil {
		return "", err
	}
	if active != expected {
		return "", ErrKeys
	}
	n, err := versionNumber(active)
	if err != nil || n >= 1<<63-1 {
		return "", ErrKeys
	}
	next := "v" + strconv.FormatUint(n+1, 10)
	value, err := manager.EnsureInstanceSecret(ctx, "internal-auth:command-request-hmac:"+next, generateKey)
	if err != nil {
		return "", keyError(ctx)
	}
	digest, err := keyDigest(value)
	if err != nil {
		return "", ErrKeys
	}
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		changed := tx.Model(&keyVersion{}).Where("version = ? AND active = 1", expected).Update("active", 0)
		if changed.Error != nil {
			return changed.Error
		}
		if changed.RowsAffected != 1 {
			return ErrKeys
		}
		row, err := newKeyVersion(next, digest)
		if err != nil {
			return err
		}
		return tx.Create(&row).Error
	})
	if err != nil {
		return "", keyError(ctx)
	}
	return next, nil
}

func newKeyVersion(version, digest string) (keyVersion, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return keyVersion{}, ErrKeys
	}
	return keyVersion{ID: id.String(), Version: version, Digest: digest, Active: 1}, nil
}

func rootDB(db *gorm.DB) bool {
	if db == nil || db.Statement == nil || db.Config == nil {
		return false
	}
	_, transaction := db.Statement.ConnPool.(gorm.TxCommitter)
	return !transaction
}
