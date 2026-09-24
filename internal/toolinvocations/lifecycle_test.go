package toolinvocations

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"assistente/internal/database"
	"assistente/internal/tools"
)

type lifecycleRepository struct {
	*DBRepository
	fail      string
	completed int
	deleted   int
}

func (r *lifecycleRepository) check(ctx context.Context, stage string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if _, ok := ctx.Deadline(); !ok {
		return errors.New("persist operation without deadline")
	}
	if r.fail == stage {
		return errors.New("injected " + stage)
	}
	return nil
}

func (r *lifecycleRepository) Create(ctx context.Context, inv *Invocation) error {
	if err := r.check(ctx, "create"); err != nil {
		return err
	}
	return r.DBRepository.Create(ctx, inv)
}
func (r *lifecycleRepository) MarkRunning(ctx context.Context, id string, at time.Time) error {
	if err := r.check(ctx, "running"); err != nil {
		return err
	}
	if r.fail == "orphan" {
		if err := database.DB().WithContext(ctx).Delete(&database.ChatMessage{}, "id = ?", "turn-1").Error; err != nil {
			return err
		}
	}
	return r.DBRepository.MarkRunning(ctx, id, at)
}
func (r *lifecycleRepository) Complete(ctx context.Context, id string, inv *Invocation) error {
	r.completed++
	if err := r.check(ctx, "complete"); err != nil {
		return err
	}
	return r.DBRepository.Complete(ctx, id, inv)
}
func (r *lifecycleRepository) Delete(ctx context.Context, id string) error {
	r.deleted++
	return r.DBRepository.Delete(ctx, id)
}

func runLifecycleEntry(svc *Service, ctx context.Context, mode string) (Invocation, bool) {
	call := tools.ToolCall{ID: "call-shared", Function: tools.FunctionCall{Name: "echo", Arguments: `{"token":"secret","value":"ok"}`}}
	origin := Origin{Type: " chat ", ID: " turn-1 "}
	if mode == "execute" {
		result := svc.Execute(ctx, ExecuteRequest{Call: call, Origin: origin, Iteration: 3})
		return result.Invocation, result.Persisted
	}
	req := RecordRequest{Call: call, Origin: origin, Iteration: 3, Result: tools.ToolResult{Content: "recorded"}, DurationMs: 5}
	if mode == "observation" {
		req.Observation = &ExternalObservation{
			CatalogName: "external__echo", Summary: "Resumo observado",
			StartedAt:       time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
			DisplayMetadata: []byte(`{"version":1,"name":"echo","origin":"external"}`),
		}
	}
	inv, err := svc.Record(ctx, req)
	return inv, err == nil
}

func TestLifecycleSharedPersistenceFailures(t *testing.T) {
	for _, mode := range []string{"execute", "record", "observation"} {
		for _, stage := range []string{"", "create", "running", "complete", "orphan"} {
			t.Run(mode+"/"+stage, func(t *testing.T) {
				base, ctx, otherUser := setupRepositoryTest(t)
				repo := &lifecycleRepository{DBRepository: base, fail: stage}
				calls := 0
				registry := tools.NewRegistry()
				registry.MustRegister(countingTool{calls: &calls})
				var metrics Metrics
				svc := NewService(repo, tools.NewExecutor(registry, tools.DefaultExecutorConfig()), &metrics)
				inv, persisted := runLifecycleEntry(svc, WithParentInvocationID(ctx, "parent-inv"), mode)
				wantPersisted := stage == "" || (stage == "running" && mode != "execute")
				if persisted != wantPersisted {
					t.Fatalf("persisted=%v want=%v invocation=%+v", persisted, wantPersisted, inv)
				}
				wantCalls := 0
				if mode == "execute" && (stage == "" || stage == "complete" || stage == "orphan") {
					wantCalls = 1
				}
				if calls != wantCalls {
					t.Fatalf("calls=%d want=%d", calls, wantCalls)
				}
				if stage != "" && stage != "orphan" && metrics.Snapshot().PersistenceFailures != 1 {
					t.Fatalf("missing failure metric: %+v", metrics.Snapshot())
				}
				if stage == "running" && mode == "execute" && repo.deleted != 1 {
					t.Fatal("failed local start must remove queued invocation")
				}
				if stage == "orphan" {
					if repo.deleted != 1 || repo.completed != 0 {
						t.Fatal("deleted origin must remove invocation without completing")
					}
					if _, err := base.Get(ctx, inv.ID); err == nil {
						t.Fatal("orphan invocation survived")
					}
				}
				if !persisted {
					return
				}
				stored, err := base.Get(ctx, inv.ID)
				if err != nil {
					t.Fatal(err)
				}
				if stored.Status != StatusSucceeded || stored.ConversationID != "conv-a" || stored.TurnID != "turn-1" ||
					stored.OriginType != OriginChat || stored.ModelIteration != 3 || stored.CompletedAt == nil || stored.ParentInvocationID != "parent-inv" {
					t.Fatalf("shared identity/finalization changed: %+v", stored)
				}
				if _, err := base.Get(otherUser, inv.ID); err == nil {
					t.Fatal("cross-user read allowed")
				}
				if mode == "observation" {
					if len(stored.Input) != 0 || len(stored.Output) != 0 || stored.InputHash != "" ||
						stored.OutputHash != "" || stored.ResultAvailability != "unavailable" ||
						stored.OutputPreview != "Resumo observado" || stored.CompletedAt.Sub(stored.QueuedAt) != 5*time.Millisecond {
						t.Fatalf("observation invented payload or changed timing: %+v", stored)
					}
					var catalog database.ToolCatalog
					if err := database.DB().First(&catalog, "id = ?", stored.ToolCatalogID).Error; err != nil {
						t.Fatal(err)
					}
					if catalog.Name != "external__echo" || catalog.Origin != ToolOriginArchival {
						t.Fatalf("observation reused executable catalog: %+v", catalog)
					}
				} else if strings.Contains(string(stored.Input), "secret") || stored.InputHash == "" ||
					stored.OutputHash == "" || stored.ResultAvailability != "available" {
					t.Fatalf("redaction/projection changed: %+v", stored)
				}
			})
		}
	}
}

func TestLifecycleExternalPersistenceSurvivesCancellation(t *testing.T) {
	for _, mode := range []string{"record", "observation"} {
		t.Run(mode, func(t *testing.T) {
			base, user, _ := setupRepositoryTest(t)
			ctx, cancel := context.WithCancel(user)
			cancel()
			svc := NewService(&lifecycleRepository{DBRepository: base}, nil)
			inv, persisted := runLifecycleEntry(svc, ctx, mode)
			if !persisted || inv.Status != StatusSucceeded {
				t.Fatalf("cancelled transport prevented archival: %+v", inv)
			}
		})
	}
}

func TestLifecycleRejectsMissingOriginForEveryEntry(t *testing.T) {
	for _, mode := range []string{"execute", "record", "observation"} {
		t.Run(mode, func(t *testing.T) {
			base, user, _ := setupRepositoryTest(t)
			if err := database.DB().Delete(&database.ChatMessage{}, "id = ?", "turn-1").Error; err != nil {
				t.Fatal(err)
			}
			calls := 0
			registry := tools.NewRegistry()
			registry.MustRegister(countingTool{calls: &calls})
			svc := NewService(base, tools.NewExecutor(registry, tools.DefaultExecutorConfig()))
			_, persisted := runLifecycleEntry(svc, user, mode)
			if persisted || calls != 0 {
				t.Fatalf("missing origin: persisted=%v calls=%d", persisted, calls)
			}
		})
	}
}
