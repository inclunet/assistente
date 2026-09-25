package toolinvocations

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"assistente/internal/tools"
	"gorm.io/gorm"
)

func TestServiceBeforeExecuteGuardSeesRunningInvocationAndDeniesWithoutTool(t *testing.T) {
	repo, user, _ := setupRepositoryTest(t)
	var calls int
	registry := tools.NewRegistry()
	registry.MustRegister(countingTool{calls: &calls})
	service := NewService(repo, tools.NewExecutor(registry, tools.DefaultExecutorConfig()))
	var observed Invocation
	result := service.Execute(user, ExecuteRequest{
		Call:   tools.ToolCall{ID: "guard-deny", Type: "function", Function: tools.FunctionCall{Name: "echo", Arguments: `{}`}},
		Origin: Origin{Type: OriginJobRun, ID: "guard-run"},
		BeforeExecute: func(ctx context.Context) error {
			id := CurrentInvocationID(ctx)
			var err error
			got, err := repo.Get(ctx, id)
			if err != nil {
				return err
			}
			observed = *got
			if observed.Status != StatusRunning {
				return errors.New("invocation ainda não está running")
			}
			return errors.New("segredo do guard não deve ser persistido")
		},
	})
	if result.Persisted == false || result.Execution.ErrorKind != tools.ErrorKindAuthorization || result.Execution.ErrorCode != "execution_guard_denied" {
		t.Fatalf("guard não produziu terminal persistido: %+v", result)
	}
	if calls != 0 || observed.ID == "" {
		t.Fatalf("tool executada ou invocação não observada: calls=%d observed=%+v", calls, observed)
	}
	if strings.Contains(result.Invocation.ErrorMessage, "segredo") {
		t.Fatalf("mensagem bruta do guard vazou: %q", result.Invocation.ErrorMessage)
	}
	got, err := repo.Get(user, result.Invocation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusFailed || got.ErrorCode != "execution_guard_denied" || got.Status == StatusRunning {
		t.Fatalf("ledger não terminal: %+v", got)
	}
}

func TestServiceBeforeExecuteGuardPanicAndCancellationAreTerminalGeneric(t *testing.T) {
	cases := []struct {
		name       string
		cancel     bool
		guard      func(context.Context) error
		wantStatus string
		wantKind   tools.ErrorKind
	}{
		{name: "panic", guard: func(context.Context) error { panic("segredo panic") }, wantStatus: StatusFailed, wantKind: tools.ErrorKindAuthorization},
		{name: "cancelamento", cancel: true, guard: func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }, wantStatus: StatusCancelled, wantKind: tools.ErrorKindCancelled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo, user, _ := setupRepositoryTest(t)
			var calls int
			registry := tools.NewRegistry()
			registry.MustRegister(countingTool{calls: &calls})
			service := NewService(repo, tools.NewExecutor(registry, tools.DefaultExecutorConfig()))
			ctx := user
			var cancel context.CancelFunc
			if tc.cancel {
				ctx, cancel = context.WithCancel(user)
				cancel()
			}
			result := service.Execute(ctx, ExecuteRequest{
				Call:   tools.ToolCall{ID: "guard-" + tc.name, Type: "function", Function: tools.FunctionCall{Name: "echo", Arguments: `{}`}},
				Origin: Origin{Type: OriginJobRun, ID: "guard-" + tc.name}, BeforeExecute: tc.guard,
			})
			if !result.Persisted || result.Execution.ErrorKind != tc.wantKind || result.Invocation.Status != tc.wantStatus {
				t.Fatalf("guard %s não foi terminal: %+v", tc.name, result)
			}
			if calls != 0 || strings.Contains(result.Invocation.ErrorMessage, "segredo") {
				t.Fatalf("efeito ou mensagem bruta no caso %s: calls=%d inv=%+v", tc.name, calls, result.Invocation)
			}
		})
	}
}

type guardCompleteFailureRepository struct {
	Repository
	completeErr error
	deletes     atomic.Int32
}

func (r *guardCompleteFailureRepository) Complete(context.Context, string, *Invocation) error {
	return r.completeErr
}

func (r *guardCompleteFailureRepository) Delete(ctx context.Context, id string) error {
	r.deletes.Add(1)
	return r.Repository.Delete(ctx, id)
}

func TestServiceBeforeExecuteGuardCompletionFailureRemovesRunningInvocation(t *testing.T) {
	base, user, _ := setupRepositoryTest(t)
	repo := &guardCompleteFailureRepository{Repository: base, completeErr: errors.New("complete unavailable")}
	var calls int
	registry := tools.NewRegistry()
	registry.MustRegister(countingTool{calls: &calls})
	service := NewService(repo, tools.NewExecutor(registry, tools.DefaultExecutorConfig()))
	result := service.Execute(user, ExecuteRequest{
		Call:          tools.ToolCall{ID: "guard-complete-error", Type: "function", Function: tools.FunctionCall{Name: "echo", Arguments: `{}`}},
		Origin:        Origin{Type: OriginJobRun, ID: "guard-complete-error"},
		BeforeExecute: func(context.Context) error { return errors.New("guard denied") },
	})
	if result.Persisted || calls != 0 || repo.deletes.Load() != 1 {
		t.Fatalf("falha de Complete deixou estado incorreto: persisted=%v calls=%d deletes=%d", result.Persisted, calls, repo.deletes.Load())
	}
	if _, err := base.Get(user, result.Invocation.ID); err == nil {
		t.Fatal("invocação guardada ficou órfã após falha de finalização")
	}
}

func TestServiceForwardsExpectedToolGenerationAndDoesNotReuseExecutor(t *testing.T) {
	repo, user, _ := setupRepositoryTest(t)
	var calls int
	registry := tools.NewRegistry()
	registry.MustRegister(countingTool{calls: &calls})
	service := NewService(repo, tools.NewExecutor(registry, tools.DefaultExecutorConfig()))
	oldGeneration, ok := registry.Generation("echo")
	if !ok || oldGeneration == 0 {
		t.Fatal("geração inicial da tool ausente")
	}
	if !registry.Unregister("echo") {
		t.Fatal("falha ao remover tool antiga")
	}
	registry.MustRegister(countingTool{calls: &calls})
	result := service.Execute(user, ExecuteRequest{
		Call:   tools.ToolCall{ID: "guard-generation", Type: "function", Function: tools.FunctionCall{Name: "echo", Arguments: `{}`}},
		Origin: Origin{Type: OriginJobRun, ID: "guard-generation"}, ExpectedToolGeneration: oldGeneration,
	})
	if !result.Persisted || result.Execution.ErrorKind != tools.ErrorKindAuthorization || result.Execution.Result.Failure == nil || result.Execution.Result.Failure.Code != "tool_generation_mismatch" {
		t.Fatalf("geração esperada não foi encaminhada: %+v", result)
	}
	if calls != 0 {
		t.Fatalf("executor antigo/reusado executou a tool substituída: %d", calls)
	}
}

func TestServiceIsBoundToRequiresExactDatabaseAndRegistry(t *testing.T) {
	repo, _, _ := setupRepositoryTest(t)
	registry := tools.NewRegistry()
	service := NewService(repo, tools.NewExecutor(registry, tools.DefaultExecutorConfig()))
	if !service.IsBoundTo(repo.db, registry) {
		t.Fatal("service deveria estar ligado ao banco e registry exatos")
	}
	if service.IsBoundTo(&gorm.DB{}, registry) {
		t.Fatal("banco diferente foi aceito")
	}
	if service.IsBoundTo(repo.db, tools.NewRegistry()) {
		t.Fatal("registry diferente foi aceito")
	}
	if service.IsBoundTo(nil, registry) || service.IsBoundTo(repo.db, nil) {
		t.Fatal("dependência nil foi aceita")
	}
}

func TestServiceIsBoundToRejectsNilAndNonConcreteDependencies(t *testing.T) {
	repo, _, _ := setupRepositoryTest(t)
	registry := tools.NewRegistry()
	if (*Service)(nil).IsBoundTo(repo.db, registry) {
		t.Fatal("service nil foi aceito")
	}
	if NewService(nil, tools.NewExecutor(registry, tools.DefaultExecutorConfig())).IsBoundTo(repo.db, registry) {
		t.Fatal("repo nil foi aceito")
	}
	if NewService(repo, nil).IsBoundTo(repo.db, registry) {
		t.Fatal("executor nil foi aceito")
	}
	if (&Service{repo: &guardCompleteFailureRepository{Repository: repo}, executor: tools.NewExecutor(registry, tools.DefaultExecutorConfig())}).IsBoundTo(repo.db, registry) {
		t.Fatal("repository não concreto foi aceito")
	}
}
