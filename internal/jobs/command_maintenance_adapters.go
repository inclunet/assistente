package jobs

import (
	"context"
	"errors"
	"strings"
	"time"

	"assistente/internal/commandmaintenance"
	"assistente/internal/database"
	"assistente/internal/toolinvocations"
)

var ErrCommandMaintenanceUnavailable = errors.New("manutenção de comandos indisponível")

// CommandMaintenanceAdapters agrupa as portas reais que o bootstrap pode
// conectar ao Coordinator. A capacidade é o Manager vivo; não há construtor
// que aceite owner, usuário ou bypass de escopo vindo do chamador.
type CommandMaintenanceAdapters struct {
	Jobs       *JobsRetentionAdapter
	Tools      *ToolsRetentionAdapter
	Compaction *CompactionAdapter
}

type maintenanceOwner struct {
	manager         *Manager
	repository      *DBRepository
	toolInvocations *toolinvocations.Service
}

// NewCommandMaintenanceAdapters cria adapters somente sobre dependências reais
// do Manager. A enumeração de usuários é instance-wide, mas cada operação de
// domínio recebe depois um contexto explícito com database.WithUserID, para
// que RequireUserID continue sendo a defesa efetiva dos repositórios.
func NewCommandMaintenanceAdapters(manager *Manager) (*CommandMaintenanceAdapters, error) {
	if manager == nil || manager.cfg.Repository == nil || manager.cfg.ToolInvocations == nil {
		return nil, ErrCommandMaintenanceUnavailable
	}
	repository, ok := manager.cfg.Repository.(*DBRepository)
	if !ok || repository.db == nil || database.DB() == nil || database.DB() != repository.db || !manager.cfg.ToolInvocations.CanPersist() {
		return nil, ErrCommandMaintenanceUnavailable
	}
	if !repository.db.Migrator().HasTable(&database.User{}) {
		return nil, ErrCommandMaintenanceUnavailable
	}
	owner := &maintenanceOwner{manager: manager, repository: repository, toolInvocations: manager.cfg.ToolInvocations}
	return &CommandMaintenanceAdapters{
		Jobs:       &JobsRetentionAdapter{owner: owner},
		Tools:      &ToolsRetentionAdapter{owner: owner},
		Compaction: &CompactionAdapter{owner: owner},
	}, nil
}

func (o *maintenanceOwner) valid() bool {
	return o != nil && o.manager != nil && o.repository != nil && o.repository.db != nil && o.toolInvocations != nil && o.toolInvocations.CanPersist() && database.DB() == o.repository.db
}

func (o *maintenanceOwner) userIDs(ctx context.Context) ([]string, error) {
	if !o.valid() || ctx == nil {
		return nil, ErrCommandMaintenanceUnavailable
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var ids []string
	if err := o.repository.db.WithContext(ctx).Model(&database.User{}).Order("id ASC").Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	for _, id := range ids {
		if strings.TrimSpace(id) == "" || strings.TrimSpace(id) != id {
			return nil, ErrCommandMaintenanceUnavailable
		}
	}
	return ids, nil
}

func (o *maintenanceOwner) forEachUser(ctx context.Context, fn func(context.Context) (int, error)) (int64, error) {
	ids, err := o.userIDs(ctx)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		deleted, err := fn(database.WithUserID(ctx, id))
		if err != nil {
			return 0, err
		}
		if deleted < 0 {
			return 0, commandmaintenance.ErrInvalidRetentionResult
		}
		total += int64(deleted)
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return total, nil
}

// JobsRetentionAdapter preserva a sequência legada de limpeza, mas a executa
// para todos os usuários da instância com escopo explícito.
type JobsRetentionAdapter struct{ owner *maintenanceOwner }

func (a *JobsRetentionAdapter) Retain(ctx context.Context, policy commandmaintenance.Policy) (int64, error) {
	if a == nil || !a.owner.valid() || ctx == nil {
		return 0, ErrCommandMaintenanceUnavailable
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if err := policy.Validate(); err != nil {
		return 0, err
	}
	return a.owner.forEachUser(ctx, func(userCtx context.Context) (int, error) {
		var total int
		for _, clean := range []func(context.Context, time.Duration) (int, error){
			a.owner.repository.CleanOldRunEvents,
			a.owner.repository.CleanOldEvents,
			a.owner.repository.CleanOldRuns,
		} {
			deleted, err := clean(userCtx, policy.JobRetention)
			if err != nil {
				return 0, err
			}
			total += deleted
		}
		deleted, err := a.owner.repository.CleanRunsExceedingCount(userCtx, policy.RunsPerJobKeep)
		if err != nil {
			return 0, err
		}
		return total + deleted, nil
	})
}

// ToolsRetentionAdapter adapta as operações reais do serviço de invocações.
// Cada método enumera a instância, mas chama o serviço com userID explícito.
type ToolsRetentionAdapter struct{ owner *maintenanceOwner }

func (a *ToolsRetentionAdapter) CleanOldDryRuns(ctx context.Context, policy commandmaintenance.Policy) (int64, error) {
	if err := validateToolsAdapter(a, ctx, policy); err != nil {
		return 0, err
	}
	return a.owner.forEachUser(ctx, func(userCtx context.Context) (int, error) {
		return a.owner.toolInvocations.CleanOldDryRuns(userCtx, policy.JobRetention)
	})
}

func (a *ToolsRetentionAdapter) CleanOrphanChat(ctx context.Context, policy commandmaintenance.Policy) (int64, error) {
	if a == nil || !a.owner.valid() || ctx == nil {
		return 0, ErrCommandMaintenanceUnavailable
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if err := policy.Validate(); err != nil {
		return 0, err
	}
	return a.owner.forEachUser(ctx, func(userCtx context.Context) (int, error) {
		return a.owner.toolInvocations.CleanOrphanChat(userCtx)
	})
}

func (a *ToolsRetentionAdapter) CleanOldChat(ctx context.Context, policy commandmaintenance.Policy) (int64, error) {
	if err := validateToolsAdapter(a, ctx, policy); err != nil {
		return 0, err
	}
	return a.owner.forEachUser(ctx, func(userCtx context.Context) (int, error) {
		return a.owner.toolInvocations.CleanOldChat(userCtx, policy.ChatRetention)
	})
}

func validateToolsAdapter(a *ToolsRetentionAdapter, ctx context.Context, policy commandmaintenance.Policy) error {
	if a == nil || !a.owner.valid() || ctx == nil {
		return ErrCommandMaintenanceUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return policy.Validate()
}

// CompactionAdapter reutiliza o mesmo throttle/gate do Manager. O caminho
// legado chama maybeCompact (best-effort), enquanto o adapter usa o núcleo que
// preserva o erro físico para o Coordinator.
type CompactionAdapter struct{ owner *maintenanceOwner }

func (a *CompactionAdapter) Compact(ctx context.Context, minFreeBytes int64) error {
	if a == nil || !a.owner.valid() || ctx == nil || minFreeBytes < 0 {
		return ErrCommandMaintenanceUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return a.owner.manager.compact(ctx, minFreeBytes)
}

var _ commandmaintenance.RetentionPort = (*JobsRetentionAdapter)(nil)
var _ commandmaintenance.ToolRetentionPort = (*ToolsRetentionAdapter)(nil)
var _ commandmaintenance.CompactionPort = (*CompactionAdapter)(nil)
