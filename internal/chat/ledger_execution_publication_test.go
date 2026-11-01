package chat

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"assistente/internal/database"
	"assistente/internal/toolinvocations"
	"assistente/internal/tools"
)

type publicationBlockingTool struct{ started, release chan struct{} }

func (publicationBlockingTool) Name() string        { return "publication-blocking" }
func (publicationBlockingTool) Description() string { return "fixture" }
func (publicationBlockingTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}
func (b publicationBlockingTool) Execute(ctx context.Context, _ json.RawMessage) (tools.ToolResult, error) {
	close(b.started)
	select {
	case <-b.release:
		return tools.ToolResult{Content: "concluída"}, nil
	case <-ctx.Done():
		return tools.ToolResult{}, ctx.Err()
	}
}

type publicationStartFailureRepo struct {
	*toolinvocations.DBRepository
	deleted string
}

func (r *publicationStartFailureRepo) MarkRunning(context.Context, string, time.Time) error {
	return errors.New("início recusado")
}
func (r *publicationStartFailureRepo) Delete(ctx context.Context, id string) error {
	r.deleted = id
	return r.DBRepository.Delete(ctx, id)
}

func TestStoragePublicationToolExecutionReleasesConversationBarrier(t *testing.T) {
	f, p, e, _ := automaticFixture(t)
	started, release := make(chan struct{}), make(chan struct{})
	cleanupRecoveryGate(t, release)
	registry := tools.NewRegistry()
	registry.MustRegister(publicationBlockingTool{started, release})
	svc := toolinvocations.NewServiceWithCoordinator(toolinvocations.NewDBRepository(f.db), tools.NewExecutor(registry, tools.DefaultExecutorConfig()), NewLedgerMutationCoordinator(p, f.db))
	done := make(chan toolinvocations.ExecuteResult, 1)
	go func() {
		done <- svc.Execute(f.ctx, toolinvocations.ExecuteRequest{Origin: toolinvocations.Origin{Type: toolinvocations.OriginChat, ID: f.question, ChatExecutionID: &e.executionID}, Call: tools.ToolCall{ID: "blocked", Function: tools.FunctionCall{Name: "publication-blocking", Arguments: "{}"}}})
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("ferramenta não iniciou")
	}
	concurrent := make(chan error, 1)
	go func() {
		if _, err := p.ReadCanonicalWindow(f.ctx, database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 20}, f.reader); err != nil {
			concurrent <- err
			return
		}
		concurrent <- p.ApplyStorageMutation(f.ctx, f.conversation, []string{"message:" + f.question}, func() ([]string, error) {
			return nil, database.UpdateMessageTextWithContext(f.ctx, f.question, "editada durante execução")
		})
	}()
	select {
	case err := <-concurrent:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ferramenta retém barreira e bloqueia leitura/mutação")
	}
	select {
	case <-done:
		t.Fatal("ferramenta terminou antes de ser liberada")
	default:
	}
	close(release)
	select {
	case result := <-done:
		if !result.Persisted || result.Invocation.Status != toolinvocations.StatusSucceeded {
			t.Fatalf("conclusão não persistida: %+v", result)
		}
		item, _ := publicationItem(t, e)
		found := false
		for _, segment := range item.Segments {
			for _, call := range segment.Invocations {
				if call.InvocationID == result.Invocation.ID {
					found = call.Status == toolinvocations.StatusSucceeded && call.OutputPreview == result.Invocation.OutputPreview && call.OutputBytes > 0 && call.HasDetails
				}
			}
		}
		stored, err := toolinvocations.NewDBRepository(f.db).Get(f.ctx, result.Invocation.ID)
		if err != nil || !strings.Contains(string(stored.Output), "concluída") || !found {
			t.Fatal("resultado persistido e metadados publicados não reconciliados antes do retorno", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("conclusão bloqueada")
	}
}

func TestStoragePublicationStartFailureReconcilesCompensationBeforeReturn(t *testing.T) {
	f, p, e, _ := automaticFixture(t)
	started, release := make(chan struct{}), make(chan struct{})
	cleanupRecoveryGate(t, release)
	registry := tools.NewRegistry()
	registry.MustRegister(publicationBlockingTool{started, release})
	repo := &publicationStartFailureRepo{DBRepository: toolinvocations.NewDBRepository(f.db)}
	observed := make([]bool, 0, 3)
	r := &failingSelectionReader{DBTimelineReader: f.reader}
	r.afterSelection = func() {
		var count int64
		if err := f.db.Model(&database.ToolInvocation{}).Where("tool_call_id=?", "start-failure").Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		observed = append(observed, count > 0)
	}
	p.reader = r
	svc := toolinvocations.NewServiceWithCoordinator(repo, tools.NewExecutor(registry, tools.DefaultExecutorConfig()), NewLedgerMutationCoordinator(p, f.db))
	result := svc.Execute(f.ctx, toolinvocations.ExecuteRequest{Origin: toolinvocations.Origin{Type: toolinvocations.OriginChat, ID: f.question, ChatExecutionID: &e.executionID}, Call: tools.ToolCall{ID: "start-failure", Function: tools.FunctionCall{Name: "publication-blocking", Arguments: "{}"}}})
	if result.Persisted || repo.deleted == "" {
		t.Fatal("falha de início não compensada")
	}
	select {
	case <-started:
		t.Fatal("ferramenta executou apesar da falha de início")
	default:
	}
	if _, err := repo.Get(f.ctx, repo.deleted); err == nil {
		t.Fatal("registro removido continua no banco")
	}
	if len(observed) != 3 || !observed[0] || !observed[1] || observed[2] {
		t.Fatalf("create/falha/delete não reconciliados antes do retorno: %v", observed)
	}
	item, _ := publicationItem(t, e)
	encoded, err := json.Marshal(item)
	if err != nil || strings.Contains(string(encoded), repo.deleted) {
		t.Fatal("registro removido continua na publicação", err)
	}
}
