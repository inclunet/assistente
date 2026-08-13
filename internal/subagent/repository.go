package subagent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"assistente/internal/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
		Order("turn_index DESC, created_at DESC, id DESC").
		First(&run).Error
	if err != nil {
		return nil, err
	}
	return &run, nil
}

// DefaultRunListLimit é o tamanho de página padrão da listagem de runs quando o
// chamador não informa um limite; maxRunListLimit é o teto aceito, para uma
// chamada da UI não varrer a tabela inteira.
const (
	DefaultRunListLimit = 50
	maxRunListLimit     = 200
)

// ListRecent devolve os runs do usuário do contexto (AEP-0052) ordenados com os
// ATIVOS primeiro e, dentro de cada grupo, do mais recente para o mais antigo.
// Runs ativos vêm antes porque são os únicos acionáveis (cancelamento) e não
// podem sumir da superfície só por serem antigos — um run em background pode
// durar mais que a página de "recentes".
//
// O título vem de um LEFT JOIN com conversations (mesmo padrão da listagem
// unificada do histórico): é a sub-conversa que dá nome ao run na UI, e um join
// evita N leituras extras. O LEFT preserva o run mesmo que a sub-conversa tenha
// sido excluída.
func (r *DBRepository) ListRecent(ctx context.Context, limit int) ([]RunListItem, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = DefaultRunListLimit
	}
	if limit > maxRunListLimit {
		limit = maxRunListLimit
	}

	type row struct {
		database.SubAgentRun
		Title string
	}
	activeFirst := clause.OrderBy{Expression: gorm.Expr(
		"CASE WHEN sub_agent_runs.status IN (?,?) THEN 0 ELSE 1 END",
		database.SubAgentRunStatusQueued, database.SubAgentRunStatusRunning,
	)}

	var rows []row
	err := database.ScopeByUser(ctx, r.db.WithContext(ctx).Model(&database.SubAgentRun{}), "sub_agent_runs.user_id").
		Select("sub_agent_runs.*, conversations.title AS title").
		Joins("LEFT JOIN conversations ON conversations.id = sub_agent_runs.child_conversation_id").
		Order(activeFirst).
		Order("sub_agent_runs.created_at DESC, sub_agent_runs.id DESC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}

	items := make([]RunListItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, RunListItem{
			RunID:                r.ID,
			ConversationID:       r.ChildConversationID,
			ParentConversationID: r.ParentConversationID,
			Title:                r.Title,
			Status:               r.Status,
			Background:           r.Background,
			Active:               IsActiveStatus(r.Status),
			Error:                r.Error,
			CreatedAt:            r.CreatedAt,
			StartedAt:            r.StartedAt,
			CompletedAt:          r.CompletedAt,
		})
	}
	return items, nil
}

// Update persiste alterações de um run existente, escopado ao usuário do
// contexto. Força o UserID ao usuário do contexto (AEP-0052) antes do UPDATE
// para que um UserID alterado não troque o dono do registro (transferência de
// posse); o filtro ScopeByUser + WHERE por id garantem alvo único (o dono
// atual), nunca um update em massa.
func (r *DBRepository) Update(ctx context.Context, run *database.SubAgentRun) error {
	if _, err := enforceRunOwnership(ctx, run); err != nil {
		return err
	}
	// PK obrigatória: sem id, um Updates(struct) viraria UPDATE EM MASSA (ainda
	// que escopado por user_id), corrompendo todos os runs do usuário se um
	// caller passar um struct parcial/novo. Falha fechado com ErrRecordNotFound
	// em vez de executar um update amplo.
	id := strings.TrimSpace(run.ID)
	if id == "" {
		return gorm.ErrRecordNotFound
	}
	// Usa Updates (não Save): o Save do GORM faz fallback para INSERT quando o
	// UPDATE não casa nenhuma linha, mascarando o no-op (e podendo recriar um
	// run de outro usuário). Select("*").Omit("id","created_at") grava todos os
	// campos mutáveis (inclusive zero-values, como o Save fazia) sem tocar nos
	// imutáveis — padrão do codebase (internal/jobs/repository.go,
	// internal/mcp/repository.go). O WHERE explícito por id (além do ScopeByUser
	// por user_id, AEP-0052) garante alvo único e nunca um update em massa.
	tx := database.ScopeByUser(ctx, r.db.WithContext(ctx).Model(run), "user_id").
		Where("id = ?", id).
		Select("*").
		Omit("id", "created_at").
		Updates(run)
	if tx.Error != nil {
		return tx.Error
	}
	// Se nenhuma linha foi afetada (run inexistente ou de outro usuário, fora do
	// escopo), o Update é um no-op silencioso. Seguimos o padrão do códigobase
	// (internal/toolinvocations/repository.go) e reportamos ErrRecordNotFound
	// para o no-op não passar como sucesso.
	if tx.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ReconcileOrphans marca como failed runs deixados em queued/running por um
// encerramento abrupto. É uma manutenção instance-wide de startup (não um
// pedido de usuário), por isso NÃO exige userID no ctx — alinhado a operações
// como migrações/reconciliação de jobs (AEP-0052: maintenance op, não leitura
// de dados de usuário). Atualiza apenas o status terminal de runs interrompidos.
//
// cutoff é a fronteira temporal: só runs CRIADOS antes de cutoff são
// reconciliados. Como o startup roda em goroutine enquanto o app já pode
// aceitar requests, runs legítimos criados após o início (created_at >= cutoff)
// NÃO podem ser marcados como órfãos. now é o instante para carimbar o desfecho.
//
// SECURITY: instance-wide — atualiza runs de TODOS os usuários, sem filtro de
// userID. É deliberado: roda só no startup (internal/app/app.go), sem ator de
// usuário, para reconciliar órfãos de QUALQUER dono após um crash (deixar o run
// de outro usuário preso em running seria o bug). O WHERE é restrito a
// status IN (queued,running) AND created_at < cutoff — não toca runs terminais
// nem os criados após o início. Não há entrada por request de usuário.
func (r *DBRepository) ReconcileOrphans(ctx context.Context, cutoff, now time.Time) (int64, error) {
	res := r.db.WithContext(ctx).Model(&database.SubAgentRun{}).
		Where("status IN ? AND created_at < ?", []string{database.SubAgentRunStatusQueued, database.SubAgentRunStatusRunning}, cutoff).
		Updates(map[string]any{
			"status":       database.SubAgentRunStatusFailed,
			"error":        "interrompido: o app foi encerrado durante a execução do sub-agente",
			"completed_at": now,
			"updated_at":   now,
		})
	return res.RowsAffected, res.Error
}
