package subagent

import (
	"context"

	"assistente/internal/database"

	"gorm.io/gorm"
)

// DBRepository é a implementação GORM de Repository, escopada por usuário
// (AEP-0052) via database.ScopeByUser.
type DBRepository struct {
	db *gorm.DB
}

// NewDBRepository cria um DBRepository sobre a instância GORM informada.
func NewDBRepository(db *gorm.DB) *DBRepository {
	return &DBRepository{db: db}
}

// Create persiste um novo run. O UserID deve estar preenchido pelo chamador
// (derivado do ctx autenticado) — a coluna é NOT NULL.
func (r *DBRepository) Create(ctx context.Context, run *database.SubAgentRun) error {
	if _, err := database.RequireUserID(ctx); err != nil {
		return err
	}
	return r.db.WithContext(ctx).Create(run).Error
}

// Get retorna um run pelo ID, escopado ao usuário do contexto.
func (r *DBRepository) Get(ctx context.Context, id string) (*database.SubAgentRun, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	var run database.SubAgentRun
	err := database.ScopeByUser(ctx, r.db.WithContext(ctx), "user_id").
		First(&run, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &run, nil
}

// GetLatestByChildConversation retorna o run mais recente de uma sub-conversa,
// escopado ao usuário do contexto. Usado por status/cancel (fases futuras).
func (r *DBRepository) GetLatestByChildConversation(ctx context.Context, childConversationID string) (*database.SubAgentRun, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	var run database.SubAgentRun
	err := database.ScopeByUser(ctx, r.db.WithContext(ctx), "user_id").
		Where("child_conversation_id = ?", childConversationID).
		Order("turn_index DESC, created_at DESC").
		First(&run).Error
	if err != nil {
		return nil, err
	}
	return &run, nil
}

// Update persiste alterações de um run existente, escopado ao usuário do contexto.
func (r *DBRepository) Update(ctx context.Context, run *database.SubAgentRun) error {
	if _, err := database.RequireUserID(ctx); err != nil {
		return err
	}
	return database.ScopeByUser(ctx, r.db.WithContext(ctx), "user_id").
		Save(run).Error
}
