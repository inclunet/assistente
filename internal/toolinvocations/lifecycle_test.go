package toolinvocations

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"assistente/internal/database"
	"assistente/internal/tools"
)

type lifecycleRepository struct {
	*DBRepository
	fail            string
	completed       int
	deleted         int
	visibilityCalls int
	validationCalls int
	validationErr   error
	deleteErr       error
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

func (r *lifecycleRepository) Create(ctx context.Context, inv *Invocation, options ...CreateOptions) error {
	if err := r.check(ctx, "create"); err != nil {
		return err
	}
	return r.DBRepository.Create(ctx, inv, options...)
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
	if r.deleteErr != nil {
		return r.deleteErr
	}
	return r.DBRepository.Delete(ctx, id)
}

func (r *lifecycleRepository) IsToolCatalogIDVisible(context.Context, string) (bool, error) {
	r.visibilityCalls++
	return false, errors.New("unneeded visibility lookup failed")
}

func (r *lifecycleRepository) ResolveToolCatalogID(ctx context.Context, name string) (string, error) {
	if err := r.check(ctx, "catalog"); err != nil {
		return "", err
	}
	return r.DBRepository.ResolveToolCatalogID(ctx, name)
}

func (r *lifecycleRepository) ValidateChatOrigin(ctx context.Context, id string) error {
	r.validationCalls++
	if err := r.check(ctx, "validate"); err != nil {
		return err
	}
	if r.validationErr != nil {
		return r.validationErr
	}
	return r.DBRepository.ValidateChatOrigin(ctx, id)
}

func runLifecycleEntry(svc *Service, ctx context.Context, mode string) (Invocation, bool) {
	call := tools.ToolCall{ID: "call-shared", Function: tools.FunctionCall{Name: "echo", Arguments: `{"token":"secret","value":"ok"}`}}
	origin := Origin{Type: " chat ", ID: " turn-1 "}
	if mode == "execute" {
		result := svc.Execute(ctx, ExecuteRequest{Call: call, Origin: origin, Iteration: 3, ToolCatalogID: "untrusted-id"})
		return result.Invocation, result.Persisted
	}
	req := RecordRequest{Call: call, Origin: origin, Iteration: 3, ToolCatalogID: "untrusted-id", Result: tools.ToolResult{Content: "recorded"}, DurationMs: 5}
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
				if repo.visibilityCalls != 0 {
					t.Fatal("queried unused catalog hint")
				}
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

func TestLifecycleObservationValidationBeforeDatabase(t *testing.T) {
	for _, tc := range []struct{ name, payload, want string }{
		{"invalid", "{", "JSON object"},
		{"null", "null", "JSON object"},
		{"array", "[]", "JSON object"},
		{"oversized-invalid", strings.Repeat("{", tools.DefaultMaxResultSize+1), "size limit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base, user, _ := setupRepositoryTest(t)
			var metrics Metrics
			svc := NewService(base, nil, &metrics)
			_, err := svc.Record(user, RecordRequest{
				Call:        tools.ToolCall{Function: tools.FunctionCall{Name: "external"}},
				Origin:      Origin{Type: OriginChat, ID: "turn-1"},
				Observation: &ExternalObservation{CatalogName: "external__new", DisplayMetadata: []byte(tc.payload)},
			})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("unexpected error: %v", err)
			}
			var catalogs, invocations int64
			if err := database.DB().Model(&database.ToolCatalog{}).Where("name = ?", "external__new").Count(&catalogs).Error; err != nil {
				t.Fatal(err)
			}
			if err := database.DB().Model(&database.ToolInvocation{}).Count(&invocations).Error; err != nil {
				t.Fatal(err)
			}
			if catalogs != 0 || invocations != 0 || metrics.Snapshot().PersistenceFailures != 1 {
				t.Fatalf("invalid observation had side effects: catalogs=%d invocations=%d metrics=%+v", catalogs, invocations, metrics.Snapshot())
			}
		})
	}
}

func TestLifecycleUsesRepositoryOriginContractWithoutGlobalDatabase(t *testing.T) {
	for _, mode := range []string{"execute", "record", "observation"} {
		t.Run(mode, func(t *testing.T) {
			base, user, _ := setupRepositoryTest(t)
			// Origem válida por turn_id, sem uma mensagem cujo id seja o turno.
			turn := "turn-1"
			if err := database.DB().Delete(&database.ChatMessage{}, "id = ?", turn).Error; err != nil {
				t.Fatal(err)
			}
			if err := database.DB().Create(&database.ChatMessage{UUIDModel: database.UUIDModel{ID: "assistant-child"}, ConversationID: "conv-a", TurnID: &turn, Role: "assistant"}).Error; err != nil {
				t.Fatal(err)
			}
			// O repositório mantém a conexão; o serviço não deve consultar o global.
			database.SetDB(nil)
			repo := &lifecycleRepository{DBRepository: base}
			registry := tools.NewRegistry()
			registry.MustRegister(echoTool{})
			svc := NewService(repo, tools.NewExecutor(registry, tools.DefaultExecutorConfig()))
			inv, persisted := runLifecycleEntry(svc, user, mode)
			wantValidations := 1
			if mode == "execute" {
				wantValidations = 2
			}
			if !persisted || repo.validationCalls != wantValidations {
				t.Fatalf("repository validation ignored: persisted=%v calls=%d", persisted, repo.validationCalls)
			}
			if _, err := base.Get(user, inv.ID); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestLifecycleObservedResultSurvivesTransientPreflightFailure(t *testing.T) {
	for _, mode := range []string{"execute", "record", "observation"} {
		t.Run(mode, func(t *testing.T) {
			base, user, _ := setupRepositoryTest(t)
			repo := &lifecycleRepository{DBRepository: base, validationErr: errors.New("transient read failure")}
			calls := 0
			registry := tools.NewRegistry()
			registry.MustRegister(countingTool{calls: &calls})
			svc := NewService(repo, tools.NewExecutor(registry, tools.DefaultExecutorConfig()))
			_, persisted := runLifecycleEntry(svc, user, mode)
			if persisted != (mode != "execute") || calls != 0 {
				t.Fatalf("incorrect validation policy: persisted=%v calls=%d", persisted, calls)
			}
		})
	}
}

func TestLifecycleStartAndCleanupFailuresAreBothCounted(t *testing.T) {
	base, user, _ := setupRepositoryTest(t)
	repo := &lifecycleRepository{DBRepository: base, fail: "running", deleteErr: errors.New("delete unavailable")}
	calls := 0
	registry := tools.NewRegistry()
	registry.MustRegister(countingTool{calls: &calls})
	var metrics Metrics
	svc := NewService(repo, tools.NewExecutor(registry, tools.DefaultExecutorConfig()), &metrics)
	_, persisted := runLifecycleEntry(svc, user, "execute")
	if persisted || calls != 0 || repo.deleted != 1 || metrics.Snapshot().PersistenceFailures != 2 {
		t.Fatalf("failed cleanup hidden: persisted=%v calls=%d deletes=%d metrics=%+v", persisted, calls, repo.deleted, metrics.Snapshot())
	}
}

func TestRepositoryOriginValidationBoundsSchemaQueries(t *testing.T) {
	repo, user, _ := setupRepositoryTest(t)
	sqlDB, err := repo.db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	conn, err := sqlDB.Conn(user)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			t.Error(err)
		}
	}()
	ctx, cancel := context.WithTimeout(user, 30*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- repo.ValidateChatOrigin(ctx, "turn-1") }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("validation succeeded without a connection")
		}
	case <-time.After(time.Second):
		t.Fatal("schema lookup ignored context deadline")
	}
}

func TestLifecycleKeepsOperationalCauseOutOfPublicResult(t *testing.T) {
	for _, tc := range []struct {
		fail, stage string
		cleanup     bool
	}{
		{"validate", "validate_origin", false},
		{"catalog", "resolve_catalog", false},
		{"create", "create", false},
		{"running", "delete_after_start_failure", true},
		{"orphan", "delete_orphan", true},
	} {
		t.Run(tc.stage, func(t *testing.T) {
			base, user, _ := setupRepositoryTest(t)
			repo := &lifecycleRepository{DBRepository: base, fail: tc.fail}
			cause := "injected " + tc.fail
			if tc.cleanup {
				cause = "cleanup unavailable"
				repo.deleteErr = errors.New(cause)
			}
			var output bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
			defer slog.SetDefault(previous)
			registry := tools.NewRegistry()
			registry.MustRegister(echoTool{})
			svc := NewService(repo, tools.NewExecutor(registry, tools.DefaultExecutorConfig()))
			result := svc.Execute(user, ExecuteRequest{
				Call:   tools.ToolCall{ID: "failure-call", Function: tools.FunctionCall{Name: "echo", Arguments: `{"value":"private-payload"}`}},
				Origin: Origin{Type: OriginChat, ID: "turn-1"},
			})
			logs := output.String()
			if result.Persisted || !strings.Contains(logs, `"stage":"`+tc.stage+`"`) || !strings.Contains(logs, cause) ||
				!strings.Contains(logs, `"origin_id":"turn-1"`) || strings.Contains(logs, "private-payload") {
				t.Fatalf("missing/unsafe operational diagnostic: persisted=%v logs=%s", result.Persisted, logs)
			}
			if strings.Contains(result.Execution.Result.Content, cause) {
				t.Fatal("internal cause leaked into public tool result")
			}
		})
	}
}

func TestLifecycleArchivalCatalogRollsBackWithInvocation(t *testing.T) {
	for _, mode := range []string{"execute", "record", "observation"} {
		for _, failure := range []string{"missing-origin", "foreign-origin", "insert", "insert-existing-catalog"} {
			t.Run(mode+"/"+failure, func(t *testing.T) {
				base, user, _ := setupRepositoryTest(t)
				db := database.DB()
				// Força também os caminhos local e MCP a precisar de archival.
				if err := db.Delete(&database.ToolCatalog{}, "name = ?", "echo").Error; err != nil {
					t.Fatal(err)
				}
				name := "echo"
				if mode == "observation" {
					name = "external__echo"
				}
				var existingID string
				if failure == "insert-existing-catalog" {
					var err error
					existingID, err = base.ResolveOrCreateArchivalToolCatalogID(user, name)
					if err != nil {
						t.Fatal(err)
					}
				}
				switch failure {
				case "missing-origin":
					if err := db.Delete(&database.ChatMessage{}, "id = ?", "turn-1").Error; err != nil {
						t.Fatal(err)
					}
				case "foreign-origin":
					if err := db.Model(&database.ChatMessage{}).Where("id = ?", "turn-1").Update("conversation_id", "conv-b").Error; err != nil {
						t.Fatal(err)
					}
				default:
					// Falha depois da criação do catálogo dentro da transação.
					if err := db.Exec("CREATE TRIGGER reject_test_invocation BEFORE INSERT ON tool_invocations BEGIN SELECT RAISE(ABORT, 'injected invocation failure'); END").Error; err != nil {
						t.Fatal(err)
					}
				}
				calls := 0
				registry := tools.NewRegistry()
				registry.MustRegister(countingTool{calls: &calls})
				svc := NewService(base, tools.NewExecutor(registry, tools.DefaultExecutorConfig()))
				_, persisted := runLifecycleEntry(svc, user, mode)
				if persisted || calls != 0 {
					t.Fatalf("failed insert executed or persisted: persisted=%v calls=%d", persisted, calls)
				}
				var catalogs []database.ToolCatalog
				if err := db.Where("name = ?", name).Find(&catalogs).Error; err != nil {
					t.Fatal(err)
				}
				if existingID == "" && len(catalogs) != 0 {
					t.Fatalf("orphan catalogs: %+v", catalogs)
				}
				if existingID != "" && (len(catalogs) != 1 || catalogs[0].ID != existingID) {
					t.Fatal("rollback removed existing catalog")
				}
				var count int64
				if err := db.Model(&database.ToolInvocation{}).Count(&count).Error; err != nil {
					t.Fatal(err)
				}
				if count != 0 {
					t.Fatalf("failed invocation survived: %d", count)
				}
			})
		}
	}
}

func TestLifecycleArchivalCatalogAndInvocationCommitTogether(t *testing.T) {
	for _, mode := range []string{"execute", "record", "observation"} {
		t.Run(mode, func(t *testing.T) {
			base, user, _ := setupRepositoryTest(t)
			if err := database.DB().Delete(&database.ToolCatalog{}, "name = ?", "echo").Error; err != nil {
				t.Fatal(err)
			}
			registry := tools.NewRegistry()
			registry.MustRegister(echoTool{})
			svc := NewService(base, tools.NewExecutor(registry, tools.DefaultExecutorConfig()))
			first, ok := runLifecycleEntry(svc, user, mode)
			if !ok || first.ToolCatalogID == "" {
				t.Fatalf("atomic create failed: %+v", first)
			}
			second, ok := runLifecycleEntry(svc, user, mode)
			if !ok || second.ToolCatalogID != first.ToolCatalogID || second.Attempt != 2 {
				t.Fatalf("archival reuse failed: %+v", second)
			}
			var catalogs, invocations int64
			if err := database.DB().Model(&database.ToolCatalog{}).Count(&catalogs).Error; err != nil {
				t.Fatal(err)
			}
			if err := database.DB().Model(&database.ToolInvocation{}).Count(&invocations).Error; err != nil {
				t.Fatal(err)
			}
			if catalogs != 1 || invocations != 2 {
				t.Fatalf("catalogs=%d invocations=%d", catalogs, invocations)
			}
		})
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
