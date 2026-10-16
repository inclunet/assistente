package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"assistente/internal/database"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// BootstrapLegacy vincula o primeiro subject do issuer ao usuário local cujo
// ID é exatamente o sub legado. O JWT é validado antes da transação; a
// unicidade do registro de auditoria serializa tentativas concorrentes.
func (s *ExternalIdentityAdminService) BootstrapLegacy(ctx context.Context, token string) (*ExternalIdentityMapping, error) {
	claims, err := s.authorizeEnrollment(ctx, token)
	if err != nil {
		return nil, err
	}
	if !canonicalSessionUUID(claims.Subject) {
		return nil, ErrExternalTargetUserRequired
	}
	if err := s.repo.CheckReadiness(ctx); err != nil {
		return nil, ErrExternalIdentityNotReady
	}
	if !s.repo.db.Migrator().HasTable(&database.ExternalIdentityAdminAudit{}) {
		return nil, ErrExternalIdentityNotReady
	}
	user, err := activeExternalAdminUser(s.repo.db, ctx, claims.Subject)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	audit, err := newExternalIdentityAdminAudit(s.cfg.Issuer, claims.Subject, user.ID, "bootstrap", claims.Subject, user.ID, now)
	if err != nil {
		return nil, err
	}
	created, err := newExternalIdentityMapping(s.cfg.Issuer, claims.Subject, user.ID, now)
	if err != nil {
		return nil, err
	}
	err = s.repo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Esta deve ser a primeira escrita para que a unique index do bootstrap
		// arbitre concorrência sem promover uma leitura SQLite antiga a writer.
		if err := tx.Create(&audit).Error; err != nil {
			if isUniqueConstraintError(err) {
				return ErrExternalIdentityAlreadyMapped
			}
			return err
		}
		if _, err := activeExternalAdminUser(tx, ctx, claims.Subject); err != nil {
			return err
		}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&created)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrExternalIdentityAlreadyMapped
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &created, nil
}

// CreateMapped permite ao administrador já mapeado criar um vínculo explícito
// no issuer configurado. O mapa e a trilha de auditoria são uma única transação.
func (s *ExternalIdentityAdminService) CreateMapped(ctx context.Context, token string, params ExternalIdentityMappingParams) (*ExternalIdentityMapping, error) {
	claims, err := s.authorizeEnrollment(ctx, token)
	if err != nil {
		return nil, err
	}
	issuer, subject, userID, err := normalizeExternalMapping(params)
	if err != nil {
		return nil, err
	}
	if issuer != s.cfg.Issuer {
		return nil, ErrExternalIdentityNotMapped
	}
	if err := s.repo.CheckReadiness(ctx); err != nil {
		return nil, ErrExternalIdentityNotReady
	}
	if !s.repo.db.Migrator().HasTable(&database.ExternalIdentityAdminAudit{}) {
		return nil, ErrExternalIdentityNotReady
	}
	actorCandidate, err := s.repo.Resolve(ctx, s.cfg.Issuer, claims.Subject)
	if err != nil {
		return nil, ErrExternalAdministratorRequired
	}
	if _, err := activeExternalAdminUser(s.repo.db, ctx, actorCandidate.UserID); err != nil {
		return nil, ErrExternalAdministratorRequired
	}
	targetCandidate, err := activeExternalAdminUser(s.repo.db, ctx, userID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	audit, err := newExternalIdentityAdminAudit(issuer, claims.Subject, actorCandidate.UserID, "create", subject, targetCandidate.ID, now)
	if err != nil {
		return nil, err
	}
	created, err := newExternalIdentityMapping(issuer, subject, targetCandidate.ID, now)
	if err != nil {
		return nil, err
	}
	err = s.repo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Auditar primeiro evita a janela SQLite read→write; toda recusa abaixo
		// aborta a transação e remove esse registro junto com o vínculo.
		if err := tx.Create(&audit).Error; err != nil {
			return err
		}
		var bootstrapCount int64
		if err := tx.Model(&database.ExternalIdentityAdminAudit{}).
			Where("issuer = ? AND action = ?", s.cfg.Issuer, "bootstrap").Count(&bootstrapCount).Error; err != nil {
			return err
		}
		if bootstrapCount != 1 {
			return ErrExternalIdentityNotReady
		}
		var actor ExternalIdentityMapping
		if err := tx.Where("issuer = ? AND subject = ? AND enabled = ?", s.cfg.Issuer, claims.Subject, true).First(&actor).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrExternalAdministratorRequired
			}
			return err
		}
		if actor.UserID != actorCandidate.UserID {
			return ErrExternalAdministratorRequired
		}
		if _, err := activeExternalAdminUser(tx, ctx, actor.UserID); err != nil {
			return ErrExternalAdministratorRequired
		}
		target, err := activeExternalAdminUser(tx, ctx, userID)
		if err != nil || target.ID != targetCandidate.ID {
			return ErrExternalTargetUserRequired
		}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&created)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrExternalIdentityAlreadyMapped
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &created, nil
}

func (s *ExternalIdentityAdminService) authorizeEnrollment(ctx context.Context, token string) (*ExternalClaims, error) {
	if s == nil || s.verifier == nil || s.repo == nil || s.repo.db == nil || ctx == nil || strings.TrimSpace(token) == "" ||
		!validExternalIdentityPart(s.cfg.Issuer) || len(s.cfg.AdminScopes) == 0 {
		return nil, ErrExternalIdentityNotReady
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	claims, err := s.verifier.Validate(ctx, token)
	if err != nil || claims == nil {
		return nil, ErrExternalAdministratorRequired
	}
	if claims.Issuer != s.cfg.Issuer {
		return nil, ErrExternalAdministratorRequired
	}
	if !containsAll(splitExternalScopes(claims.Scope), s.cfg.AdminScopes) {
		return nil, ErrExternalAdminScopeRequired
	}
	return claims, nil
}

func activeExternalAdminUser(tx *gorm.DB, ctx context.Context, userID string) (*database.User, error) {
	if tx == nil || ctx == nil || !canonicalSessionUUID(userID) {
		return nil, ErrExternalTargetUserRequired
	}
	var user database.User
	err := tx.WithContext(ctx).Where("id = ? AND is_active = ?", userID, true).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrExternalTargetUserRequired
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func newExternalIdentityMapping(issuer, subject, userID string, now time.Time) (ExternalIdentityMapping, error) {
	issuer, subject, userID, err := normalizeExternalMapping(ExternalIdentityMappingParams{Issuer: issuer, Subject: subject, UserID: userID})
	if err != nil {
		return ExternalIdentityMapping{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return ExternalIdentityMapping{}, err
	}
	return ExternalIdentityMapping{ID: id.String(), Issuer: issuer, Subject: subject, UserID: userID, Enabled: true, CreatedAt: now, UpdatedAt: now}, nil
}

func newExternalIdentityAdminAudit(issuer, actorSubject, actorUserID, action, subject, userID string, now time.Time) (database.ExternalIdentityAdminAudit, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return database.ExternalIdentityAdminAudit{}, err
	}
	return database.ExternalIdentityAdminAudit{
		ID: id.String(), Issuer: issuer, ActorSubject: actorSubject, ActorUserID: actorUserID,
		Action: action, Subject: subject, UserID: userID, CreatedAt: now,
	}, nil
}
