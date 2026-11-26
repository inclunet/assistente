package usecases_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/agent"
	"assistente/internal/chat"
	"assistente/internal/core/usecases"
	"assistente/internal/database"

	"assistente/internal/llm"
	"assistente/internal/pluginpackage"
	"assistente/internal/plugins"
	"assistente/internal/prompt"
	"assistente/internal/providers"
	"assistente/internal/skills"
	"assistente/internal/toolinvocations"
	"assistente/internal/tools"
	"gorm.io/gorm"
)

type countedAgentTool struct {
	name  string
	calls atomic.Int32
}

func (p *countedAgentTool) Name() string        { return p.name }
func (p *countedAgentTool) Description() string { return p.name }
func (p *countedAgentTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{}}`)
}
func (p *countedAgentTool) Execute(context.Context, json.RawMessage) (tools.ToolResult, error) {
	p.calls.Add(1)
	return tools.ToolResult{Content: "feito"}, nil
}

func TestActivatedPluginAgentPromptAndDispatch(t *testing.T) {
	setupTestDB(t)
	db := database.DB()
	ctx := database.WithUserID(t.Context(), "test-user")
	profileMgr := setupProfileDir(t)
	dir := t.TempDir()
	const replacement = "Instruções exclusivas da segunda revisão."
	const instructions = "Verifique a tarefa usando apenas leitura."
	for key, body := range map[string]string{".claude-plugin/plugin.json": `{"name":"suite"}`, "agents/check.md": "---\nname: review\ndescription: Review\ntools: Read\nmodel: inherit\n---\n" + instructions} {
		file := filepath.Join(dir, filepath.FromSlash(key))
		if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	inv := plugins.NewInventory(db)
	snap, err := pluginpackage.InspectLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	admit := func(_ *gorm.DB, commit func() error) error { return commit() }
	row, err := inv.InstallSource(ctx, snap.Origin, snap.Digest, "", nil, admit, func() (*pluginpackage.LocalSnapshot, error) { return snap, nil })
	if err != nil {
		t.Fatal(err)
	}
	if err = inv.SetAgentActive(ctx, row.ID, row.Digest, "agents/check.md", true, admit); err != nil {
		t.Fatal(err)
	}
	components, err := inv.InstalledComponents(ctx, row.ID, row.Digest)
	if err != nil || len(components) != 1 {
		t.Fatal(components, err)
	}
	generation, err := inv.AgentGeneration(ctx, components[0].Slug)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := database.CreateConversationWithContext(ctx, "Principal", "")
	if err != nil {
		t.Fatal(err)
	}
	conv, err := database.NewConversationRepository(db).CreateDelegatedConversation(ctx, "Revisor", parent.ID, generation, "local-test", "modelo-herdado", instructions)
	if err != nil {
		t.Fatal(err)
	}

	read := &countedAgentTool{name: "read_file"}
	write := &countedAgentTool{name: "write_file"}
	registry := tools.NewRegistry()
	registry.MustRegister(read)
	registry.MustRegister(write)
	var requests atomic.Int32
	var afterRevocation atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		var request struct {
			Model    string
			Tools    []struct{ Function struct{ Name string } }
			Messages []json.RawMessage
		}
		if err = json.Unmarshal(raw, &request); err != nil {
			t.Error(err)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		turn := requests.Add(1)
		if afterRevocation.Load() {
			if len(request.Tools) != 0 || !strings.Contains(string(raw), "Analise.") || request.Model != "modelo-herdado" || !strings.Contains(string(raw), instructions) || strings.Contains(string(raw), replacement) {
				t.Errorf("retomada após revogação: %s", raw)
			}
		}
		if turn <= 2 {
			name := read.name
			if turn == 1 {
				if request.Model != "modelo-herdado" || !strings.Contains(string(raw), instructions) || len(request.Tools) != 1 || request.Tools[0].Function.Name != read.name {
					t.Errorf("prompt ou catálogo incorretos: %s", raw)
				}
			} else {
				name = write.name
			}
			if _, err := fmt.Fprintf(w, "data: "+`{"id":"chatcmpl-test","object":"chat.completion.chunk","model":"teste","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call-%s","type":"function","function":{"name":"%s","arguments":"{}"}}]}}]}`+"\n\n", name, name); err != nil {
				t.Error(err)
				return
			}
			if _, err := fmt.Fprint(w, "data: "+`{"id":"chatcmpl-test","object":"chat.completion.chunk","model":"teste","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`+"\n\n"); err != nil {
				t.Error(err)
				return
			}
		} else {
			if _, err := fmt.Fprint(w, "data: "+`{"id":"chatcmpl-test","object":"chat.completion.chunk","model":"teste","choices":[{"index":0,"delta":{"content":"pronto"},"finish_reason":"stop"}]}`+"\n\n"); err != nil {
				t.Error(err)
				return
			}
		}
		if _, err := fmt.Fprint(w, "data: [DONE]\n\n"); err != nil {
			t.Error(err)
			return
		}
	}))
	defer server.Close()
	provider := &llm.ProviderConfig{ID: "local-test", Name: "Local", Type: llm.ProviderOpenAI, APIFormat: llm.APIFormatOpenAI, BaseURL: server.URL + "/v1"}
	llms := llm.NewProviderRegistry()
	if err := llms.Register(provider); err != nil {
		t.Fatal(err)
	}
	store := providers.NewDBStore()
	if err = store.Save(ctx, []*llm.ProviderConfig{provider}); err != nil {
		t.Fatal(err)
	}
	prov := providers.NewService(providers.ServiceConfig{Registry: llms, Store: store})
	profile := minValidProfile("Teste", provider.ID)
	profile.Chat.EnabledTools = []string{read.name, write.name}
	setupProfileWith(t, profileMgr, profile)
	emitter := &mcpPreparationEmitter{}
	messages := chat.NewDBMessageStore()
	interactor := chat.NewInteractor(chat.InteractorConfig{Emitter: emitter, Repo: messages, ConvRepo: chat.NewDBConversationStore(), ProviderSvc: prov, ProfileMgr: profileMgr, SkillMgr: skills.NewManager(), PromptBuilder: &prompt.Builder{Tools: registry}})
	exec := tools.NewExecutor(registry, tools.DefaultExecutorConfig())
	service := agent.NewService(agent.ServiceConfig{Emitter: emitter, MsgRepo: messages, Turns: database.NewChatTurnRepository(db), Timeline: chat.NewDBTimelineReader(db), ToolExecutor: exec, ToolInvocations: toolinvocations.NewService(toolinvocations.NewDBRepository(db), exec)})
	uc := usecases.NewSendMessageUseCase(usecases.SendMessageConfig{Emitter: emitter, StreamMgr: chat.NewStreamingManager(nil), ChatInteractor: interactor, ToolRegistry: registry, ProviderSvc: prov, AgentSvc: service})
	finished := make(chan struct{})
	_, err = uc.Execute(usecases.SendMessageRequest{Ctx: ctx, ConversationID: conv.ID, UserContent: "Analise.", Source: "subagent", OnFinished: func() { close(finished) }})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
	case <-time.After(10 * time.Second):
		t.Fatal("turno não terminou")
	}
	if requests.Load() < 3 || read.calls.Load() != 1 || write.calls.Load() != 0 {
		var turns []database.ChatTurn
		db.Find(&turns, "conversation_id = ?", conv.ID)
		t.Fatalf("pedidos=%d leitura=%d escrita=%d erros=%+v turnos=%+v", requests.Load(), read.calls.Load(), write.calls.Load(), emitter.errors, turns)
	}
	var before, after int64
	history := db.Model(&database.ChatMessage{}).Where("conversation_id = ?", conv.ID)
	if err := history.Count(&before).Error; err != nil || before == 0 {
		t.Fatal(before, err)
	}
	_, err = uc.Execute(usecases.SendMessageRequest{Ctx: ctx, ConversationID: conv.ID, UserContent: "Reset", Source: "subagent", ClearPluginHistory: true, Params: llm.ChatParams{Model: "outro"}})
	if err == nil || !strings.Contains(err.Error(), "modelo do turno de origem") {
		t.Fatal("retomada incompatível", err)
	}
	_, err = uc.Execute(usecases.SendMessageRequest{Ctx: ctx, ConversationID: conv.ID, UserContent: "/plugin:missing", Source: "subagent", ClearPluginHistory: true})
	if err == nil || !strings.Contains(err.Error(), "skill de plugin indisponível") {
		t.Fatal("skill indisponível", err)
	}
	if err := history.Count(&after).Error; err != nil || after != before {
		t.Fatalf("histórico passou de %d para %d: %v", before, after, err)
	}

	if err := os.WriteFile(filepath.Join(dir, "agents", "check.md"), []byte("---\nname: review\ndescription: Review\n---\n"+replacement), 0600); err != nil {
		t.Fatal(err)
	}
	updated, err := pluginpackage.InspectLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	next, err := inv.InstallSource(ctx, updated.Origin, updated.Digest, row.Digest, nil, admit, func() (*pluginpackage.LocalSnapshot, error) { return updated, nil })
	if err != nil {
		t.Fatal(err)
	}
	if err = inv.SetAgentActive(ctx, next.ID, next.Digest, "agents/check.md", true, admit); err != nil {
		t.Fatal(err)
	}
	afterRevocation.Store(true)
	resumed := make(chan struct{})
	earlier := requests.Load()
	_, err = uc.Execute(usecases.SendMessageRequest{Ctx: ctx, ConversationID: conv.ID, UserContent: "Continue a conversa.", Source: "wails", OnFinished: func() { close(resumed) }})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-resumed:
	case <-time.After(10 * time.Second):
		t.Fatal("retomada não terminou")
	}
	if requests.Load() != earlier+1 || read.calls.Load() != 1 || write.calls.Load() != 0 {
		t.Fatal("retomada não chegou ao provedor sem ferramentas")
	}
	other := *provider
	other.ID = "alternate"
	other.Name = "Alternate"
	if err = llms.Register(&other); err != nil {
		t.Fatal(err)
	}
	if err = store.Save(ctx, []*llm.ProviderConfig{provider, &other}); err != nil {
		t.Fatal(err)
	}
	alternateProfile := minValidProfile("Alternativo", other.ID)
	alternative, err := profileMgr.Create(&alternateProfile)
	if err != nil {
		t.Fatal(err)
	}
	_, err = uc.Execute(usecases.SendMessageRequest{Ctx: ctx, ConversationID: conv.ID, UserContent: "Continue.", Source: "wails", Params: llm.ChatParams{ProfileSlug: alternative}})
	if err == nil || !strings.Contains(err.Error(), "herdar o modelo") || requests.Load() != earlier+1 {
		t.Fatal("retomada mudou de provedor", err)
	}
}
