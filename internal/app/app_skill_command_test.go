package app

import (
	"context"
	"encoding/json"
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"

	"assistente/internal/agent"
	"assistente/internal/allowlist"
	"assistente/internal/chat"
	"assistente/internal/contextprovider"
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

func (s *skillTestTerminal) RunEphemeralWithDisplay(ctx context.Context, dir string, command terminal.CommandText, timeout time.Duration, source string) (*terminal.HistoryEntry, error) {
	s.calls++
	entry := &terminal.HistoryEntry{ExitCode: s.exitCode, Output: "private-output", Command: command.Display}
	if s.exitCode == -1 {
		return entry, context.DeadlineExceeded
	}
	return entry, nil
}

func TestSkillCommandInAdmittedTurn(t *testing.T) {
	cases := []struct {
		name, policy                                 string
		exitCode, prompts, calls                     int
		success, refused, ledger, posix, unavailable bool
	}{
		{name: "preapproved", policy: "approve", calls: 1, success: true, ledger: true},
		{name: "policy-denied", policy: "deny", refused: true, ledger: true},
		{name: "allow", prompts: 1, calls: 1, success: true, ledger: true},
		{name: "confirmation-unavailable", prompts: 1, unavailable: true, ledger: true},
		{name: "deny", prompts: 1, refused: true, ledger: true},
		{name: "posix", prompts: 1, success: true, ledger: true, posix: true},
		{name: "nonzero", exitCode: 13, prompts: 1, calls: 1, ledger: true},
		{name: "partial", exitCode: -1, prompts: 1, calls: 1, ledger: true},
		{name: "no-turn"}, {name: "origin-denied"}, {name: "ledger-unavailable"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.posix && runtime.GOOS == "windows" {
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
			term := &skillTestTerminal{exitCode: tc.exitCode}
			prompts := 0
			command := "example-tool private-argument"
			var sessions shell.SessionManager = term
			if tc.posix {
				config := terminal.DefaultManagerConfig()
				config.DefaultShell = "/bin/sh"
				sessions = terminal.NewManager(config, nil)
				command = "printf private-output"
			}
			var policy shell.GetAllowlistFunc
			if tc.policy != "" {
				policy = func(context.Context) (*allowlist.Allowlist, error) {
					return &allowlist.Allowlist{DefaultAction: "confirm", CommandRules: []allowlist.CommandRule{{Program: "example-tool", Subcommands: []string{"*"}, Decision: tc.policy}}}, nil
				}
			}
			registry := tools.NewRegistry()
			registry.MustRegister(shell.NewRunCommand(sessions, func(ctx context.Context, raw, dir string) (bool, error) {
				prompts++
				if raw != command {
					t.Errorf("unexpected command %q", raw)
					return false, nil
				}
				if tc.unavailable {
					return false, context.DeadlineExceeded
				}
				return !tc.refused, nil
			}, policy, t.TempDir()))
			a.toolInvocationSvc = toolinvocations.NewService(toolinvocations.NewDBRepository(db), tools.NewExecutor(registry, tools.DefaultExecutorConfig()))
			switch tc.name {
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
			if (err == nil) != tc.success || errors.Is(err, contextprovider.ErrCommandRefused) != tc.refused || errors.Is(err, contextprovider.ErrCommandNotAuthorized) != (tc.refused || tc.unavailable) || prompts != tc.prompts || term.calls != tc.calls {
				t.Fatalf("output=%q prompts=%d calls=%d err=%v", output, prompts, term.calls, err)
			}
			if tc.success && !strings.Contains(output, "private-output") || !tc.success && output != "" {
				t.Fatalf("output=%q", output)
			}
			if !tc.ledger {
				return
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
