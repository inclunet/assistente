package jobs

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"assistente/internal/commandmaintenance"
	"assistente/internal/database"
	"assistente/internal/toolinvocations"
)

var (
	ErrCommandMaintenanceUnavailable          = errors.New("manutenção de comandos indisponível")
	ErrCommandMaintenanceContinuationRequired = errors.New("manutenção de comandos requer continuação bounded")
	ErrCommandMaintenanceBusy                 = errors.New("manutenção de comandos já está em execução")
)

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
	cursorMu        sync.Mutex
	cursors         map[string]maintenanceCursor
	admission       chan struct{}
}

type maintenanceCursor struct {
	policy      commandmaintenance.Policy
	initialized bool
	after       string
}

const (
	maintenanceJobsCursor       = "jobs"
	maintenanceToolsDryRuns     = "tools.dry_runs"
	maintenanceToolsOrphanChats = "tools.orphan_chats"
	maintenanceToolsOldChats    = "tools.old_chats"
)

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
	owner := &maintenanceOwner{
		manager:         manager,
		repository:      repository,
		toolInvocations: manager.cfg.ToolInvocations,
		cursors:         make(map[string]maintenanceCursor),
		admission:       make(chan struct{}, 1),
	}
	return &CommandMaintenanceAdapters{
		Jobs:       &JobsRetentionAdapter{owner: owner},
		Tools:      &ToolsRetentionAdapter{owner: owner},
		Compaction: &CompactionAdapter{owner: owner},
	}, nil
}

func (o *maintenanceOwner) valid() bool {
	return o != nil && o.manager != nil && o.repository != nil && o.repository.db != nil && o.toolInvocations != nil && o.toolInvocations.CanPersist() && database.DB() == o.repository.db
}

func (o *maintenanceOwner) nextUsers(ctx context.Context, operation string, policy commandmaintenance.Policy) ([]string, bool, error) {
	if !o.valid() || ctx == nil {
		return nil, false, ErrCommandMaintenanceUnavailable
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	limit := policy.BatchSize
	if limit == 0 {
		limit = commandmaintenance.DefaultBatchSize
	}
	o.cursorMu.Lock()
	cursor := o.cursors[operation]
	if !cursor.initialized || cursor.policy != policy {
		cursor = maintenanceCursor{policy: policy, initialized: true}
		o.cursors[operation] = cursor
	}
	after := cursor.after
	o.cursorMu.Unlock()

	var ids []string
	query := o.repository.db.WithContext(ctx).Model(&database.User{}).Order("id ASC").Limit(limit + 1)
	if after != "" {
		query = query.Where("id > ?", after)
	}
	if err := query.Pluck("id", &ids).Error; err != nil {
		return nil, false, err
	}
	for _, id := range ids {
		if strings.TrimSpace(id) == "" || strings.TrimSpace(id) != id {
			return nil, false, ErrCommandMaintenanceUnavailable
		}
	}
	more := len(ids) > limit
	if more {
		ids = ids[:limit]
	}
	return ids, more, nil
}

func (o *maintenanceOwner) retainUsers(ctx context.Context, operation string, policy commandmaintenance.Policy, fn func(context.Context) (int, error)) (int64, bool, error) {
	if !o.tryAdmit() {
		return 0, false, ErrCommandMaintenanceBusy
	}
	defer o.release()

	ids, more, err := o.nextUsers(ctx, operation, policy)
	if err != nil {
		return 0, false, err
	}
	var total int64
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return total, true, err
		}
		deleted, err := fn(database.WithUserID(ctx, id))
		if deleted < 0 {
			return total, true, commandmaintenance.ErrInvalidRetentionResult
		}
		total += int64(deleted)
		if err != nil {
			return total, true, err
		}
		o.advanceCursor(operation, policy, id)
	}
	if len(ids) == 0 {
		o.resetCursor(operation, policy)
	} else if !more {
		// Só reinicia após todos os usuários do último lote terem concluído.
		o.resetCursor(operation, policy)
	}
	if err := ctx.Err(); err != nil {
		return total, more, err
	}
	return total, more, nil
}

func (o *maintenanceOwner) tryAdmit() bool {
	if o == nil || o.admission == nil {
		return false
	}
	select {
	case o.admission <- struct{}{}:
		return true
	default:
		return false
	}
}

func (o *maintenanceOwner) release() {
	if o != nil && o.admission != nil {
		<-o.admission
	}
}

func (o *maintenanceOwner) advanceCursor(operation string, policy commandmaintenance.Policy, id string) {
	o.cursorMu.Lock()
	defer o.cursorMu.Unlock()
	cursor := o.cursors[operation]
	if cursor.policy != policy || !cursor.initialized {
		cursor = maintenanceCursor{policy: policy, initialized: true}
	}
	cursor.after = id
	o.cursors[operation] = cursor
}

func (o *maintenanceOwner) resetCursor(operation string, policy commandmaintenance.Policy) {
	o.cursorMu.Lock()
	o.cursors[operation] = maintenanceCursor{policy: policy, initialized: true}
	o.cursorMu.Unlock()
}

// JobsRetentionAdapter preserva a sequência legada de limpeza, mas a executa
// para um lote de usuários da instância com escopo explícito.
type JobsRetentionAdapter struct{ owner *maintenanceOwner }

func (a *JobsRetentionAdapter) Retain(ctx context.Context, policy commandmaintenance.Policy) (int64, error) {
	result, err := a.RetainBatch(ctx, policy)
	if err != nil {
		return result.Deleted, err
	}
	if result.More {
		return result.Deleted, ErrCommandMaintenanceContinuationRequired
	}
	return result.Deleted, nil
}

// RetainBatch processa no máximo policy.BatchSize usuários e conserva o
// cursor em memória para a próxima passagem do mesmo adapter. Mudança de
// política reinicia a varredura, garantindo que a nova política alcance todos
// os usuários.
func (a *JobsRetentionAdapter) RetainBatch(ctx context.Context, policy commandmaintenance.Policy) (commandmaintenance.RetentionResult, error) {
	if a == nil || !a.owner.valid() || ctx == nil {
		return commandmaintenance.RetentionResult{}, ErrCommandMaintenanceUnavailable
	}
	if err := ctx.Err(); err != nil {
		return commandmaintenance.RetentionResult{}, err
	}
	if err := policy.Validate(); err != nil {
		return commandmaintenance.RetentionResult{}, err
	}
	deleted, more, err := a.owner.retainUsers(ctx, maintenanceJobsCursor, policy, func(userCtx context.Context) (int, error) {
		var total int
		for _, clean := range []func(context.Context, time.Duration) (int, error){
			a.owner.repository.CleanOldRunEvents,
			a.owner.repository.CleanOldEvents,
			a.owner.repository.CleanOldRuns,
		} {
			deleted, err := clean(userCtx, policy.JobRetention)
			if deleted < 0 {
				return total, commandmaintenance.ErrInvalidRetentionResult
			}
			total += deleted
			if err != nil {
				return total, err
			}
		}
		deleted, err := a.owner.repository.CleanRunsExceedingCount(userCtx, policy.RunsPerJobKeep)
		if deleted < 0 {
			return total, commandmaintenance.ErrInvalidRetentionResult
		}
		return total + deleted, err
	})
	return commandmaintenance.RetentionResult{Deleted: deleted, More: more}, err
}

// ToolsRetentionAdapter adapta as operações reais do serviço de invocações.
// Cada método enumera a instância, mas chama o serviço com userID explícito.
type ToolsRetentionAdapter struct{ owner *maintenanceOwner }

func (a *ToolsRetentionAdapter) CleanOldDryRuns(ctx context.Context, policy commandmaintenance.Policy) (int64, error) {
	result, err := a.CleanOldDryRunsBatch(ctx, policy)
	if err != nil {
		return result.Deleted, err
	}
	if result.More {
		return result.Deleted, ErrCommandMaintenanceContinuationRequired
	}
	return result.Deleted, nil
}

func (a *ToolsRetentionAdapter) CleanOldDryRunsBatch(ctx context.Context, policy commandmaintenance.Policy) (commandmaintenance.RetentionResult, error) {
	if err := validateToolsAdapter(a, ctx, policy); err != nil {
		return commandmaintenance.RetentionResult{}, err
	}
	deleted, more, err := a.owner.retainUsers(ctx, maintenanceToolsDryRuns, policy, func(userCtx context.Context) (int, error) {
		return a.owner.toolInvocations.CleanOldDryRuns(userCtx, policy.JobRetention)
	})
	return commandmaintenance.RetentionResult{Deleted: deleted, More: more}, err
}

func (a *ToolsRetentionAdapter) CleanOrphanChat(ctx context.Context, policy commandmaintenance.Policy) (int64, error) {
	result, err := a.CleanOrphanChatBatch(ctx, policy)
	if err != nil {
		return result.Deleted, err
	}
	if result.More {
		return result.Deleted, ErrCommandMaintenanceContinuationRequired
	}
	return result.Deleted, nil
}

func (a *ToolsRetentionAdapter) CleanOrphanChatBatch(ctx context.Context, policy commandmaintenance.Policy) (commandmaintenance.RetentionResult, error) {
	if err := validateToolsAdapter(a, ctx, policy); err != nil {
		return commandmaintenance.RetentionResult{}, err
	}
	deleted, more, err := a.owner.retainUsers(ctx, maintenanceToolsOrphanChats, policy, func(userCtx context.Context) (int, error) {
		return a.owner.toolInvocations.CleanOrphanChat(userCtx)
	})
	return commandmaintenance.RetentionResult{Deleted: deleted, More: more}, err
}

func (a *ToolsRetentionAdapter) CleanOldChat(ctx context.Context, policy commandmaintenance.Policy) (int64, error) {
	result, err := a.CleanOldChatBatch(ctx, policy)
	if err != nil {
		return result.Deleted, err
	}
	if result.More {
		return result.Deleted, ErrCommandMaintenanceContinuationRequired
	}
	return result.Deleted, nil
}

func (a *ToolsRetentionAdapter) CleanOldChatBatch(ctx context.Context, policy commandmaintenance.Policy) (commandmaintenance.RetentionResult, error) {
	if err := validateToolsAdapter(a, ctx, policy); err != nil {
		return commandmaintenance.RetentionResult{}, err
	}
	deleted, more, err := a.owner.retainUsers(ctx, maintenanceToolsOldChats, policy, func(userCtx context.Context) (int, error) {
		return a.owner.toolInvocations.CleanOldChat(userCtx, policy.ChatRetention)
	})
	return commandmaintenance.RetentionResult{Deleted: deleted, More: more}, err
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
	if !a.owner.tryAdmit() {
		return ErrCommandMaintenanceBusy
	}
	defer a.owner.release()
	return a.owner.manager.compact(ctx, minFreeBytes)
}

var _ commandmaintenance.RetentionPort = (*JobsRetentionAdapter)(nil)
var _ commandmaintenance.BoundedRetentionPort = (*JobsRetentionAdapter)(nil)
var _ commandmaintenance.ToolRetentionPort = (*ToolsRetentionAdapter)(nil)
var _ commandmaintenance.BoundedToolRetentionPort = (*ToolsRetentionAdapter)(nil)
var _ commandmaintenance.CompactionPort = (*CompactionAdapter)(nil)
