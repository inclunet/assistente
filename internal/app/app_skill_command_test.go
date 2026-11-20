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
)

type skillTestTerminal struct {
	shell.SessionManager
	calls    int
	exitCode int
}

func (s *skillTestTerminal) RunEphemeral(ctx context.Context, dir, command string, timeout time.Duration, source string) (*terminal.HistoryEntry, error) {
	s.calls++
	entry := &terminal.HistoryEntry{ExitCode: s.exitCode, Output: "private-output", Command: command}
	if s.exitCode == -1 {
		return entry, context.DeadlineExceeded
	}
	return entry, nil
}

func TestSkillCommandInAdmittedTurn(t *testing.T) {
	for _, decision := range []string{"allow", "deny", "posix", "nonzero", "partial", "no-turn", "origin-denied", "ledger-unavailable"} {
		t.Run(decision, func(t *testing.T) {
			if decision == "posix" && runtime.GOOS == "windows" {
				t.Skip("PTY POSIX exercitado no CI Linux")
			}
			db := setupAuthAppTestDB(t)
			user := database.User{Username: "skill-review", PasswordHash: "unused", Role: database.UserRoleUser, IsActive: true}
			if err := db.Create(&user).Error; err != nil {
				t.Fatal(err)
			}
			a := &App{currentUserID: user.ID}
			conv, err := database.CreateConversationWithContext(database.WithUserID(context.Background(), user.ID), "skill", "")
			if err != nil {
				t.Fatal(err)
			}
			cid := conv.ID
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
			switch decision {
			case "partial":
				term.exitCode = -1
			case "nonzero":
				term.exitCode = 13
			case "no-turn":
				ctx = context.Background()
			case "origin-denied":
				ctx = tools.WithExecutionContext(ctx, tools.ExecutionContext{AllowedBash: []string{"other"}})
			case "ledger-unavailable":
				if err := db.Migrator().DropTable(&database.ToolInvocation{}); err != nil {
					t.Fatal(err)
				}
			}
			output, err := a.runSkillCommand(ctx, &skills.Skill{Slug: "example"}, command)
			switch decision {
			case "no-turn", "origin-denied", "ledger-unavailable":
				if err == nil || output != "" || prompts != 0 || term.calls != 0 {
					t.Fatalf("output=%q prompts=%d calls=%d err=%v", output, prompts, term.calls, err)
				}
				return
			case "nonzero", "partial":
				if err == nil || output != "" || term.calls != 1 {
					t.Fatalf("output=%q calls=%d err=%v", output, term.calls, err)
				}
			}
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
