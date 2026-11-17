package authorization

import (
	"context"
	"errors"

	"assistente/internal/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrLegacyOwnerPending = errors.New("autorizações legadas aguardam atribuição administrativa de proprietário")

type LegacyOwnerOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (r *Repository) LegacyOwners(ctx context.Context) ([]LegacyOwnerOption, error) {
	db, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	owners := make([]LegacyOwnerOption, 0)
	err = db.Model(&database.User{}).Select("id, username AS name").Where("is_active = ?", true).Order("username").Scan(&owners).Error
	return owners, err
}

// A linha sem UserID e com versão zero é o registro administrativo da origem,
// nunca um marcador de importação de usuário. Apenas origens encontradas pelos
// leitores legados entram aqui; a API não recebe caminhos para ler arquivos.
func (r *Repository) AuthorizeLegacyImport(ctx context.Context, domain, source string, localOwner bool) (bool, error) {
	db, user, err := r.query(ctx)
	if err != nil {
		return false, err
	}
	var assigned string
	err = database.WithSQLiteBusyRetry(ctx, "authorization.claim_origin", func() error {
		return db.Transaction(func(tx *gorm.DB) error {
			owner, err := importedOwner(tx, domain, source)
			if err != nil {
				return err
			}
			var origin database.AuthorizationImport
			err = tx.Where("user_id = '' AND domain = ? AND source = ? AND version = 0", domain, source).First(&origin).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				assigned = owner
				if assigned == "" && localOwner {
					assigned = user
				}
				origin = database.AuthorizationImport{Domain: domain, Source: source, Version: 0, AssignedTo: assigned}
				if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&origin).Error; err != nil {
					return err
				}
				origin = database.AuthorizationImport{}
				if err := tx.Where("user_id = '' AND domain = ? AND source = ? AND version = 0", domain, source).First(&origin).Error; err != nil {
					return err
				}
			} else if err != nil {
				return err
			}
			if owner != "" && origin.AssignedTo != "" && origin.AssignedTo != owner {
				return errors.New("origem legada tem proprietários conflitantes")
			}
			if origin.AssignedTo == "" && owner != "" {
				if err := tx.Model(&origin).Update("assigned_to", owner).Error; err != nil {
					return err
				}
				origin.AssignedTo = owner
			}
			assigned = origin.AssignedTo
			return nil
		})
	})
	if err != nil {
		return false, err
	}
	if assigned == "" {
		return false, ErrLegacyOwnerPending
	}
	return assigned == user, nil
}

// Marcadores anteriores ao registro administrativo também fixam o dono.
func importedOwner(tx *gorm.DB, domain, source string) (string, error) {
	var owners []string
	if err := tx.Model(&database.AuthorizationImport{}).Where("domain = ? AND source = ? AND version > 0 AND user_id <> ''", domain, source).Distinct("user_id").Limit(2).Pluck("user_id", &owners).Error; err != nil {
		return "", err
	}
	if len(owners) > 1 {
		return "", errors.New("origem legada importada por mais de um usuário")
	}
	if len(owners) == 1 {
		return owners[0], nil
	}
	return "", nil
}

func (r *Repository) requireAdmin(ctx context.Context) (*gorm.DB, error) {
	db, user, err := r.query(ctx)
	if err != nil {
		return nil, err
	}
	var count int64
	if err := db.Model(&database.User{}).Where("id = ? AND role = ? AND is_active = ?", user, database.UserRoleAdmin, true).Count(&count).Error; err != nil {
		return nil, err
	}
	if count != 1 {
		return nil, errors.New("admin_required")
	}
	return db, nil
}

func (r *Repository) PendingOrigins(ctx context.Context) ([]database.AuthorizationImport, error) {
	db, err := r.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	rows := make([]database.AuthorizationImport, 0)
	err = db.Where("user_id = '' AND version = 0 AND (assigned_to = '' OR assigned_to IS NULL)").Order("domain, source").Find(&rows).Error
	return rows, err
}

func (r *Repository) AssignOrigin(ctx context.Context, id, userID string) error {
	db, err := r.requireAdmin(ctx)
	if err != nil {
		return err
	}
	return database.WithSQLiteBusyRetry(ctx, "authorization.assign_origin", func() error {
		return db.Transaction(func(tx *gorm.DB) error {
			var origin database.AuthorizationImport
			if err := tx.Where("id = ? AND user_id = '' AND version = 0 AND (assigned_to = '' OR assigned_to IS NULL)", id).First(&origin).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrNotFound
				}
				return err
			}
			owner, err := importedOwner(tx, origin.Domain, origin.Source)
			if err != nil {
				return err
			}
			if owner != "" && owner != userID {
				return errors.New("origem legada já pertence a outro usuário")
			}
			var count int64
			if err := tx.Model(&database.User{}).Where("id = ? AND is_active = ?", userID, true).Count(&count).Error; err != nil {
				return err
			}
			if count != 1 {
				return errors.New("usuário de destino inválido")
			}
			result := tx.Model(&database.AuthorizationImport{}).Where("id = ? AND user_id = '' AND version = 0 AND (assigned_to = '' OR assigned_to IS NULL)", id).Update("assigned_to", userID)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrNotFound
			}
			return nil
		})
	})
}
