package commandledger

import (
	"context"
	"errors"

	"assistente/internal/commandsecurity"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// SealDrainedGeneration é exclusivo do host. A prova vem do fechamento real
// do core e de seus executores, não de escopo/bool/marker enviado pelo cliente.
// Só aceita gerações de segurança efetivamente emitidas por aquele core.
// O marcador persistido serve ao writer de recuperação existente; sozinho
// continua insuficiente para construir uma ClosedGenerationProof.
func (s *Store) SealDrainedGeneration(ctx context.Context, drained commandsecurity.DrainedGenerations, scope GenerationScope) (ClosedGenerationProof, error) {
	if scope.UserID != nil {
		user := *scope.UserID
		scope.UserID = &user
	}
	if s == nil || s.db == nil || s.now == nil || ctx == nil || validGenerationScope(scope) != nil || !drained.Includes(scope.SecurityGeneration) {
		return ClosedGenerationProof{}, ErrInvalidRequest
	}
	var marker string
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		now := s.now().UTC()
		if now.IsZero() {
			return ErrInvalidRequest
		}
		query := tx.Where("auth_context_type = ? AND auth_context_id = ? AND security_generation = ?", scope.AuthContextType, scope.AuthContextID, scope.SecurityGeneration)
		if scope.UserID == nil {
			query = query.Where("user_id IS NULL")
		} else {
			query = query.Where("user_id = ?", *scope.UserID)
		}
		var existing closedGenerationRow
		err := query.Take(&existing).Error
		if err == nil {
			marker = existing.ID
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		id, err := uuid.NewV7()
		if err != nil {
			return err
		}
		row := closedGenerationRow{ID: id.String(), UserID: scope.UserID, AuthContextType: scope.AuthContextType, AuthContextID: scope.AuthContextID, SecurityGeneration: scope.SecurityGeneration, ClosedAt: now}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		marker = row.ID
		return ctx.Err()
	})
	if err != nil {
		return ClosedGenerationProof{}, err
	}
	return ClosedGenerationProof{scope: scope, marker: marker}, nil
}
