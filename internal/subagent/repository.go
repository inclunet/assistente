package subagent

import (
	"context"
	"fmt"
	"strings"
	"time"

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

// enforceRunOwnership garante que o run pertence ao usuário do contexto
// autenticado (AEP-0052): normaliza UserID vazio para esse usuário e rejeita
// qualquer divergência (tentativa de gravar/transferir o run sob outro
// user_id). Retorna o userID efetivo.
func enforceRunOwnership(ctx context.Context, run *database.SubAgentRun) (string, error) {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return "", err
	}
	if run == nil {
		return "", fmt.Errorf("run de sub-agente nulo")
	}
	current := strings.TrimSpace(run.UserID)
	if current != "" && current != userID {
		return "", fmt.Errorf("user_id do run de sub-agente diverge do usuário autenticado (violação de escopo AEP-0052)")
	}
	run.UserID = userID
	return userID, nil
}

// Create persiste um novo run, forçando o UserID ao usuário do contexto
// (AEP-0052): impede inserir um run sob outro user_id mesmo que o chamador
// preencha o campo incorretamente.
func (r *DBRepository) Create(ctx context.Context, run *database.SubAgentRun) error {
	if _, err := enforceRunOwnership(ctx, run); err != nil {
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

// ListByUser retorna todos os runs do usuário do contexto, ordenados do mais
// recente para o mais antigo. Usado pela UI (AEP-0068 F5) para derivar status,
// contagem de runs e custo por sub-conversa.
func (r *DBRepository) ListByUser(ctx context.Context) ([]database.SubAgentRun, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	var runs []database.SubAgentRun
	err := database.ScopeByUser(ctx, r.db.WithContext(ctx), "user_id").
		Order("created_at DESC, id DESC").
		Find(&runs).Error
	if err != nil {
		return nil, err
	}
	return runs, nil
}

// Update persiste alterações de um run existente, escopado ao usuário do
// contexto. Força o UserID ao usuário do contexto (AEP-0052) antes do Save para
// que um UserID alterado não troque o dono do registro (transferência de posse);
// o filtro ScopeByUser no WHERE garante que só o dono atual é atingido.
func (r *DBRepository) Update(ctx context.Context, run *database.SubAgentRun) error {
	if _, err := enforceRunOwnership(ctx, run); err != nil {
		return err
	}
	return database.ScopeByUser(ctx, r.db.WithContext(ctx), "user_id").
		Save(run).Error
}

// ReconcileOrphans marca como failed runs deixados em queued/running por um
// encerramento abrupto. É uma manutenção instance-wide de startup (não um
// pedido de usuário), por isso NÃO exige userID no ctx — alinhado a operações
// como migrações/reconciliação de jobs (AEP-0052: maintenance op, não leitura
// de dados de usuário). Atualiza apenas o status terminal de runs interrompidos.
func (r *DBRepository) ReconcileOrphans(ctx context.Context, now time.Time) (int64, error) {
	res := r.db.WithContext(ctx).Model(&database.SubAgentRun{}).
		Where("status IN ?", []string{database.SubAgentRunStatusQueued, database.SubAgentRunStatusRunning}).
		Updates(map[string]any{
			"status":       database.SubAgentRunStatusFailed,
			"error":        "interrompido: o app foi encerrado durante a execução do sub-agente",
			"completed_at": now,
			"updated_at":   now,
		})
	return res.RowsAffected, res.Error
}
