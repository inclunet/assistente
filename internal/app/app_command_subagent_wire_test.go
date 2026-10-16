package app

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"assistente/internal/chat"
	"assistente/internal/configdir"
	"assistente/internal/database"
	"assistente/internal/eventctx"
	"assistente/internal/jobprofilegrant"
	"assistente/internal/messaging"
	"assistente/internal/profileaccess"
	"assistente/internal/profiles"
	"assistente/internal/questionnaire"
	"assistente/internal/subagent"
	"assistente/internal/tools"
	"assistente/internal/tools/filesystem"
	"assistente/internal/tools/invocationctx"
	subagenttool "assistente/internal/tools/subagent"
)

// C09: registro produtivo -> tool real -> profileaccess -> Manager -> SQLite.
// Send e disponibilidade do provider são portas controladas: esta prova não
// executa LLM, SendMessageUseCase nem scheduler/executor de jobs. A origem job
// entra pelo contexto canônico e consulta um grant exato persistido.
func TestCommandSubagentWireRealManager(t *testing.T) {
	for _, scenario := range []string{"chat approved", "job exact grant", "job without grant"} {
		t.Run(scenario, func(t *testing.T) {
			a, _ := appLifecycleProductMountFixture(t)
			root := t.TempDir()
			t.Setenv("HOME", root)
			t.Setenv("USERPROFILE", root)
			t.Chdir(root)
			configdir.ResetForTests()
			t.Cleanup(configdir.ResetForTests)
			a.profileManager = profiles.NewManager()
			profile := profiles.DefaultProfile()
			profile.Name, profile.Active = "Wire target", false
			target, err := a.profileManager.Create(profile)
			if err != nil {
				t.Fatal(err)
			}
			if target == "lifecycle-profile" || target == "" {
				t.Fatal("profile alvo não é cross-profile")
			}
			db := database.DB()
			if err := db.AutoMigrate(&database.Conversation{}, &database.ChatMessage{}, &database.SubAgentRun{}, &database.ToolCatalog{}, &database.Job{}, &database.JobProfileGrant{}, &database.JobProfileGrantEpoch{}, &database.ProfileGrantRevocationIntent{}); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(database.WithUserID(context.Background(), a.currentUserID), 10*time.Second)
			defer cancel()
			parent := database.Conversation{UserID: a.currentUserID, Title: "Wire parent"}
			if err := db.Create(&parent).Error; err != nil {
				t.Fatal(err)
			}
			decisions := appCommandImportWailsCopyDecisions(t, a)
			a.jobGrantStore = jobprofilegrant.NewStore(db)
			a.profileAccessOnce.Do(func() {
				a.profileAccess = profileaccess.NewService(a.profileManager, a.questionnaireRouter(), func(ctx context.Context, source, conversationID string) questionnaire.Surface {
					return resolveProfileAccessSurface(ctx, source, conversationID, database.GetConversationInfoWithContext)
				}, func(context.Context, *profiles.Profile) bool { return true }).WithJobGrants(a.jobGrantStore).WithSessionValidator(a.validateProfileGrantSession)
			})
			// Registra antes do Manager para exercitar também o provider lazy real.
			a.msgRepo = chat.NewDBMessageStore()
			t.Cleanup(func() {
				filesystem.SetSandboxRootFunc(nil)
				if a.httpResponseArtifacts != nil {
					if err := a.httpResponseArtifacts.CleanupArtifacts(); err != nil {
						t.Errorf("limpar artefatos HTTP do registry: %v", err)
					}
				}
			})
			a.initToolRegistry()
			tool, ok := a.toolRegistry.Get("subagent")
			if !ok {
				t.Fatal("subagent ausente do registry produtivo")
			}
			if _, ok := tool.(*subagenttool.Tool); !ok {
				t.Fatalf("tool substituída: %T", tool)
			}
			notifier := messaging.NewResponseNotifier()
			t.Cleanup(notifier.Stop)
			sends := make(chan subagent.SendParams, 1)
			a.subagentMgr = subagent.NewManager(subagent.ManagerConfig{
				Repo: subagent.NewDBRepository(db), Notifier: notifier,
				Send: func(_ context.Context, p subagent.SendParams) (string, error) {
					sends <- p
					notifier.Notify(p.ConversationID, "wire completed", "wire-assistant-message")
					return p.ConversationID, nil
				},
			})
			parentID, turnID := parent.ID, "wire-parent-turn"
			if scenario == "chat approved" {
				ctx = invocationctx.With(ctx, invocationctx.InvocationContext{ConversationID: parentID, TurnID: turnID, ProfileSlug: "lifecycle-profile", Source: "wails"})
			} else {
				catalog := database.ToolCatalog{Name: "subagent", DisplayName: "Subagent", Origin: "builtin", AvailabilityStatus: "available", Schema: string(tool.Parameters())}
				if err := db.Create(&catalog).Error; err != nil {
					t.Fatal(err)
				}
				inputs, err := json.Marshal(map[string]any{"profile": target, "prompt": "wire task"})
				if err != nil {
					t.Fatal(err)
				}
				job := database.Job{UserID: a.currentUserID, Slug: "wire-job", Name: "Wire job", Enabled: true, ToolCatalogID: catalog.ID, ToolName: "subagent", Inputs: string(inputs)}
				if err := db.Create(&job).Error; err != nil {
					t.Fatal(err)
				}
				if scenario == "job exact grant" {
					snapshot, err := a.jobGrantStore.AuthorizationSnapshot(ctx, job.ID, target)
					if err != nil {
						t.Fatal(err)
					}
					fingerprint := jobprofilegrant.Fingerprint("subagent", target)
					if snapshot.Config.Fingerprint != fingerprint {
						t.Fatal("fingerprint não corresponde à delegação exata")
					}
					if err := a.jobGrantStore.Grant(ctx, job.ID, target, fingerprint, "wire-test", snapshot.Generation); err != nil {
						t.Fatal(err)
					}
				}
				ctx = eventctx.With(ctx, eventctx.Provenance{Source: "job", SourceJobID: job.ID})
				parentID, turnID = "", ""
			}
			args, err := json.Marshal(map[string]any{"profile": target, "prompt": "wire task"})
			if err != nil {
				t.Fatal(err)
			}
			type outcome struct {
				result tools.ToolResult
				err    error
			}
			done := make(chan outcome, 1)
			joined := make(chan struct{})
			go func() { defer close(joined); result, err := tool.Execute(ctx, args); done <- outcome{result, err} }()
			t.Cleanup(func() {
				cancel()
				select {
				case <-joined:
				case <-time.After(3 * time.Second):
					t.Error("tool não encerrou")
				}
			})
			if scenario == "chat approved" {
				decision := agentConfigDecision(t, decisions)
				var count int64
				if err := db.Model(&database.SubAgentRun{}).Count(&count).Error; err != nil || count != 0 {
					t.Fatalf("run antes da autorização: %d %v", count, err)
				}
				finishCommandDecision(t, a.questionnaireMgr, decision, map[string]any{questionnaire.AnswerActionID: profileaccess.ActionAllow}, false)
			}
			var out outcome
			select {
			case out = <-done:
			case decision := <-decisions:
				t.Fatalf("job abriu decisão: %+v", decision)
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			if out.err != nil {
				t.Fatal(out.err)
			}
			var runs []database.SubAgentRun
			if err := db.Find(&runs).Error; err != nil {
				t.Fatal(err)
			}
			var children []database.Conversation
			if err := db.Where("kind = ?", database.ConversationKindSubagent).Find(&children).Error; err != nil {
				t.Fatal(err)
			}
			if scenario == "job without grant" {
				if !out.result.IsError || out.result.Failure == nil || out.result.Failure.Code != "authorization_not_granted" {
					t.Fatalf("recusa inesperada: %+v", out.result)
				}
				if len(runs) != 0 || len(children) != 0 || len(sends) != 0 {
					t.Fatalf("sem grant criou efeito: runs=%+v children=%+v sends=%d", runs, children, len(sends))
				}
				return
			}
			var result subagent.RunResult
			if err := json.Unmarshal([]byte(out.result.Content), &result); err != nil {
				t.Fatal(err)
			}
			if out.result.IsError || result.Status != subagent.StatusSucceeded || result.RunID == "" || result.ConversationID == "" {
				t.Fatalf("resultado: %+v %+v", out.result, result)
			}
			if len(runs) != 1 || len(children) != 1 {
				t.Fatalf("efeito não é único: runs=%+v children=%+v", runs, children)
			}
			run, child := runs[0], children[0]
			if run.ID != result.RunID || run.ChildConversationID != child.ID || child.ID != result.ConversationID || run.UserID != a.currentUserID || child.UserID != a.currentUserID || run.ParentConversationID != parentID || child.ParentConversationID != parentID || run.ParentTurnID != turnID || run.Status != subagent.StatusSucceeded || run.ResultSummary != "wire completed" || run.CompletedAt == nil {
				t.Fatalf("persistência divergente: run=%+v child=%+v", run, child)
			}
			select {
			case sent := <-sends:
				if sent.ProfileSlug != target || sent.ConversationID != child.ID || sent.Source != subagent.Source || sent.Prompt != "wire task" {
					t.Fatalf("Send não recebeu alvo autorizado: %+v", sent)
				}
			default:
				t.Fatal("Manager não chamou Send")
			}
			if len(sends) != 0 {
				t.Fatal("Send duplicado")
			}
			select {
			case decision := <-decisions:
				t.Fatalf("decisão adicional: %+v", decision)
			default:
			}
		})
	}
}
