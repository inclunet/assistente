package app

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"testing"
	"time"

	"assistente/internal/agent"
	"assistente/internal/chat"
	"assistente/internal/database"
	"assistente/internal/events"
	"assistente/internal/skills"
	"assistente/internal/terminal"
	"assistente/internal/toolinvocations"
	"assistente/internal/tools"
	"assistente/internal/tools/invocationctx"
	"assistente/internal/tools/shell"
	"assistente/internal/workspace"
)

type skillTestTerminal struct {
	shell.SessionManager
	calls int
}

func (s *skillTestTerminal) RunEphemeral(ctx context.Context, dir, command string, timeout time.Duration, source string) (*terminal.HistoryEntry, error) {
	s.calls++
	return &terminal.HistoryEntry{Output: "private-output", Command: command}, nil
}

func TestSkillCommandInAdmittedTurn(t *testing.T) {
	for _, decision := range []string{"allow", "deny", "posix"} {
		t.Run(decision, func(t *testing.T) {
			if decision == "posix" && runtime.GOOS == "windows" {
				t.Skip("PTY POSIX exercitado no CI Linux")
			}
			a, _, cid := clearCommandFixture(t, workspace.TabTypeChat)
			db := database.DB()
			if err := db.AutoMigrate(&database.ToolCatalog{}, &database.ToolInvocation{}); err != nil {
				t.Fatal(err)
			}
			if err := database.MigrateChatTurns(db); err != nil {
				t.Fatal(err)
			}
			if err := db.Exec("ALTER TABLE chat_turn_attempts ADD COLUMN lease_path TEXT").Error; err != nil {
				t.Fatal(err)
			}
			if err := database.MigrateChatExecutionContent(db); err != nil {
				t.Fatal(err)
			}
			if err := db.Exec("INSERT INTO chat_timeline_versions VALUES (?, 'skill-attempt', 1)", cid).Error; err != nil {
				t.Fatal(err)
			}
			question := messageCommandSeed(t, cid)
			svc := agent.NewService(agent.ServiceConfig{Emitter: events.NoopEmitter{}, Turns: database.NewChatTurnRepository(db), Timeline: chat.NewDBTimelineReader(db)})
			ctx, err := svc.BeginTurn(database.WithUserID(context.Background(), a.currentUserID), cid, question.ID, database.TurnModeNew)
			if err != nil {
				t.Fatal(err)
			}
			defer svc.EndUnfinishedTurn(ctx, cid, question.ID, nil)
			ctx = invocationctx.With(ctx, invocationctx.InvocationContext{ConversationID: cid, TurnID: question.ID, Source: "wails"})
			term := &skillTestTerminal{}
			prompts := 0
			command := "example-tool private-argument"
			var sessions shell.SessionManager = term
			if decision == "posix" {
				config := terminal.DefaultManagerConfig()
				config.DefaultShell = "/bin/sh"
				sessions = terminal.NewManager(config, nil)
				command = "printf private-output"
			}
			registry := tools.NewRegistry()
			registry.MustRegister(shell.NewRunCommand(sessions, func(ctx context.Context, raw, dir string) (bool, error) {
				prompts++
				if raw != command {
					t.Fatalf("unexpected command %q", raw)
				}
				return decision != "deny", nil
			}, nil, t.TempDir()))
			a.toolInvocationSvc = toolinvocations.NewService(toolinvocations.NewDBRepository(db), tools.NewExecutor(registry, tools.DefaultExecutorConfig()))
			output, err := a.runSkillCommand(ctx, &skills.Skill{Slug: "example"}, command)
			if prompts != 1 {
				t.Fatalf("prompts=%d err=%v", prompts, err)
			}
			if decision == "allow" && (err != nil || !strings.Contains(output, "private-output") || term.calls != 1) {
				t.Fatalf("allow: output=%q calls=%d err=%v", output, term.calls, err)
			}
			if decision == "posix" && (err != nil || !strings.Contains(output, "private-output")) {
				t.Fatalf("POSIX: output=%q err=%v", output, err)
			}
			if decision == "deny" && (err == nil || term.calls != 0) {
				t.Fatalf("deny: calls=%d err=%v", term.calls, err)
			}
			var entries []database.ToolInvocation
			if err := db.Where("origin_id = ?", question.ID).Find(&entries).Error; err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 || entries[0].ConversationID == nil || *entries[0].ConversationID != cid {
				t.Fatalf("ledger: %+v", entries)
			}
			data, err := json.Marshal(entries)
			if err != nil {
				t.Fatal(err)
			}
			for _, secret := range []string{"private-argument", "private-output"} {
				if strings.Contains(string(data), secret) {
					t.Fatalf("ledger contains %q", secret)
				}
			}
		})
	}
}
