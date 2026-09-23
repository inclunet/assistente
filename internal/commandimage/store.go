package commandimage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

const (
	maxStoredPNG = 80 << 10
	ownerQuota   = 16 << 20
)

var (
	ErrInvalidOwner  = errors.New("commandimage: owner inválido")
	ErrInvalidRef    = errors.New("commandimage: referência inválida")
	ErrQuotaExceeded = errors.New("commandimage: quota de imagens excedida")
	ErrNotFound      = errors.New("commandimage: imagem não encontrada")
	ErrIntegrity     = errors.New("commandimage: integridade da imagem comprometida")
)

const imageAssetsTableSQL = `CREATE TABLE IF NOT EXISTS command_image_assets (
	user_id TEXT NOT NULL CHECK (length(trim(user_id)) > 0 AND instr(user_id, char(0)) = 0),
	ref TEXT NOT NULL CHECK (length(ref) = 64 AND lower(ref) = ref AND ref NOT GLOB '*[^0-9a-f]*'),
	png BLOB NOT NULL CHECK (typeof(png) = 'blob' AND length(png) > 0 AND length(png) <= 81920),
	PRIMARY KEY (user_id, ref)
)`

// Migrate cria somente a tabela de assets de imagem. Ela não cria nem exige
// qualquer tabela de configuração; o chamador controla a ordem das migrações.
func Migrate(ctx context.Context, db *gorm.DB) error {
	if ctx == nil || db == nil {
		return fmt.Errorf("commandimage: contexto ou banco nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var objectType string
		if err := tx.Raw("SELECT type FROM sqlite_master WHERE name = ?", "command_image_assets").Scan(&objectType).Error; err != nil {
			return err
		}
		if objectType != "" && objectType != "table" {
			return fmt.Errorf("commandimage: objeto command_image_assets não é uma tabela")
		}
		return tx.Exec(imageAssetsTableSQL).Error
	})
}

// PutTx valida e grava asset dentro da transação fornecida. A chave é por
// owner, portanto o mesmo Ref pode existir em owners distintos sem vazamento.
func PutTx(ctx context.Context, tx *gorm.DB, userID string, asset Asset) error {
	if err := validateContextDBOwner(ctx, tx, userID); err != nil {
		return err
	}
	canonical, err := Normalize(asset.PNG)
	if err != nil {
		return err
	}
	if !ValidRef(asset.Ref) || asset.Ref != canonical.Ref || !bytes.Equal(asset.PNG, canonical.PNG) {
		return ErrIntegrity
	}
	if len(asset.PNG) > maxStoredPNG {
		return fmt.Errorf("%w: PNG excede 80 KiB", ErrQuotaExceeded)
	}

	var existing imageAssetRow
	query := tx.WithContext(ctx).Table("command_image_assets").Where("user_id = ? AND ref = ?", userID, asset.Ref).Take(&existing)
	if query.Error == nil {
		if !bytes.Equal(existing.PNG, asset.PNG) {
			return ErrIntegrity
		}
		return nil
	}
	if !errors.Is(query.Error, gorm.ErrRecordNotFound) {
		return query.Error
	}

	var total int64
	if err := tx.WithContext(ctx).Raw("SELECT COALESCE(SUM(length(png)), 0) FROM command_image_assets WHERE user_id = ?", userID).Scan(&total).Error; err != nil {
		return err
	}
	if total > ownerQuota-int64(len(asset.PNG)) {
		return ErrQuotaExceeded
	}

	result := tx.WithContext(ctx).Exec("INSERT INTO command_image_assets (user_id, ref, png) VALUES (?, ?, ?) ON CONFLICT(user_id, ref) DO NOTHING", userID, asset.Ref, asset.PNG)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		return nil
	}

	// A concurrent writer may have inserted the same key after the first
	// lookup. Preserve idempotence, while still rejecting a conflicting blob.
	query = tx.WithContext(ctx).Table("command_image_assets").Where("user_id = ? AND ref = ?", userID, asset.Ref).Take(&existing)
	if errors.Is(query.Error, gorm.ErrRecordNotFound) {
		return fmt.Errorf("commandimage: inserção idempotente não encontrou asset")
	}
	if query.Error != nil {
		return query.Error
	}
	if !bytes.Equal(existing.PNG, asset.PNG) {
		return ErrIntegrity
	}
	return nil
}

// Load carrega somente o asset pertencente ao userID. Ausência e referência
// pertencente a outro owner têm exatamente o mesmo erro observável.
func Load(ctx context.Context, db *gorm.DB, userID, ref string) ([]byte, error) {
	if err := validateContextDBOwner(ctx, db, userID); err != nil {
		return nil, err
	}
	if !ValidRef(ref) {
		return nil, ErrInvalidRef
	}

	var row imageAssetRow
	query := db.WithContext(ctx).Table("command_image_assets").Where("user_id = ? AND ref = ?", userID, ref).Take(&row)
	if errors.Is(query.Error, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if query.Error != nil {
		return nil, query.Error
	}
	if len(row.PNG) == 0 || len(row.PNG) > maxStoredPNG {
		return nil, ErrIntegrity
	}
	canonical, err := Normalize(row.PNG)
	if err != nil || canonical.Ref != ref || !bytes.Equal(canonical.PNG, row.PNG) {
		return nil, ErrIntegrity
	}
	return append([]byte(nil), row.PNG...), nil
}

// PruneTx remove, para um owner, assets que não aparecem em nenhum binding do
// owner, sem restringir a workspace. A tabela de bindings só é consultada
// nesta operação; Migrate e PutTx permanecem independentes dela.
func PruneTx(ctx context.Context, tx *gorm.DB, userID string) error {
	if err := validateContextDBOwner(ctx, tx, userID); err != nil {
		return err
	}
	return tx.WithContext(ctx).Exec(`DELETE FROM command_image_assets AS assets
		WHERE assets.user_id = ?
		  AND NOT EXISTS (
			SELECT 1
			FROM command_bindings AS bindings
			WHERE bindings.user_id = assets.user_id
			  AND json_extract(
					CASE WHEN json_valid(bindings.presentation) THEN bindings.presentation ELSE '{}' END,
					'$.image_ref'
				  ) = assets.ref
		  )`, userID).Error
}

type imageAssetRow struct {
	PNG []byte `gorm:"column:png"`
}

func validateContextDBOwner(ctx context.Context, db *gorm.DB, userID string) error {
	if ctx == nil || db == nil {
		return fmt.Errorf("commandimage: contexto ou banco nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(userID) == "" || strings.IndexByte(userID, 0) >= 0 {
		return ErrInvalidOwner
	}
	return nil
}
