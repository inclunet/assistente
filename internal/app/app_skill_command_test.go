package app

import (
	"context"
	"encoding/json"
	"errors"
	"runtime"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"assistente/internal/agent"
	"assistente/internal/allowlist"
	"assistente/internal/chat"
	"assistente/internal/contextprovider"
	"assistente/internal/database"
	"assistente/internal/events"
	"assistente/internal/messaging"
	"assistente/internal/questionnaire"
	"assistente/internal/shelltrust"
	"assistente/internal/skills"
	"assistente/internal/terminal"
	"assistente/internal/toolinvocations"
	"assistente/internal/tools"
	"assistente/internal/tools/invocationctx"
	"assistente/internal/tools/shell"
	"gorm.io/gorm"
)

type skillTestTerminal struct {
	shell.SessionManager
	calls    int
	exitCode int
}

func (s *skillTestTerminal) RunEphemeral(ctx context.Context, dir terminal.WorkingDirectory, command terminal.CommandText, timeout time.Duration, source string) (*terminal.HistoryEntry, error) {
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
		{name: "veto-after-approval", prompts: 1, refused: true, ledger: true},
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
			ctx := beginSkillTestTurn(t, db, "wails")
			inv, _ := invocationctx.Get(ctx)
			a := &App{}
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
			confirm := func(ctx context.Context, raw, dir string) (bool, error) {
				prompts++
				if raw != command {
					t.Errorf("unexpected command %q", raw)
					return false, nil
				}
				if tc.unavailable {
					return false, context.DeadlineExceeded
				}
				return !tc.refused, nil
			}
			if tc.name == "veto-after-approval" {
				active := &allowlist.Allowlist{DefaultAction: "confirm"}
				policy = func(context.Context) (*allowlist.Allowlist, error) { return active, nil }
				auth := shelltrust.Authorizer{Policy: policy, Prompt: func(context.Context, shelltrust.PromptRequest) (shelltrust.PromptDecision, error) {
					prompts++
					active.AlwaysDeny = []string{"example-tool"}
					return shelltrust.PromptDecision{Approve: true, Scope: "once"}, nil
				}}
				confirm = auth.Authorize
			}
			registry.MustRegister(shell.NewRunCommand(sessions, confirm, policy, t.TempDir()))
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
			if err := db.Where("origin_id = ?", inv.TurnID).Find(&entries).Error; err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 || entries[0].ConversationID == nil || *entries[0].ConversationID != inv.ConversationID {
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

func beginSkillTestTurn(t *testing.T, db *gorm.DB, source string) context.Context {
	t.Helper()
	user := database.User{Username: "skill-review", PasswordHash: "unused", Role: database.UserRoleUser, IsActive: true}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	conv, err := database.CreateConversationWithContext(database.WithUserID(context.Background(), user.ID), "skill", "")
	if err != nil {
		t.Fatal(err)
	}
	cid := conv.ID
	question := messageCommandSeed(t, cid)
	svc := agent.NewService(agent.ServiceConfig{Emitter: events.NoopEmitter{}, Turns: database.NewChatTurnRepository(db), Timeline: chat.NewDBTimelineReader(db)})
	ctx, err := svc.BeginTurn(database.WithUserID(context.Background(), user.ID), cid, question.ID, database.TurnModeNew)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { svc.EndUnfinishedTurn(ctx, cid, question.ID, nil) })
	return invocationctx.With(ctx, invocationctx.InvocationContext{ConversationID: cid, TurnID: question.ID, Source: source})
}

type channelConsentBaseSkill struct {
	skill skills.Skill
}

func (s channelConsentBaseSkill) GetAllSkillsFull() ([]skills.Skill, error) {
	return []skills.Skill{s.skill}, nil
}
func (channelConsentBaseSkill) GetSkillFiles(string) ([]string, error) { return nil, nil }

func TestBaseSkillChannelRefusalAtExecutorDeadline(t *testing.T) {
	for _, tc := range []struct {
		name               string
		denyBeforeDeadline bool
		wantCode           string
	}{
		{"negativa antes do prazo", true, shell.FailureCommandRefused},
		{"prazo antes da negativa", false, shell.FailureConfirmationUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			db := setupAuthAppTestDB(t)
			ctx := beginSkillTestTurn(t, db, "telegram")
			inv, _ := invocationctx.Get(ctx)
			synctest.Test(t, func(t *testing.T) {
				opened, release := make(chan struct{}), make(chan struct{})
				resume := sync.OnceFunc(func() { close(release) })
				defer resume()
				gateway := messaging.NewGateway(nil, nil, nil, nil, nil, nil)
				gateway.Register("telegram", &consentMessenger{send: func(msg messaging.OutgoingMessage) error {
					if !strings.Contains(msg.Text, "2 - Negar") {
						t.Errorf("opções: %s", msg.Text)
					}
					close(opened)
					<-release // A resposta pode chegar antes de o transporte confirmar o envio da pergunta.
					return nil
				}})
				questions := gateway.ChannelQuestions()
				router := questionnaire.NewRouter(nil, func() questionnaire.ChannelAsker { return questions })
				auth := shelltrust.Authorizer{Prompt: func(ctx context.Context, req shelltrust.PromptRequest) (shelltrust.PromptDecision, error) {
					return askShellConfirmation(ctx, router, questionnaire.ChannelSurface(inv.ConversationID, "telegram", "dono"), req)
				}}
				term := &skillTestTerminal{}
				reg := tools.NewRegistry()
				reg.MustRegister(shell.NewRunCommand(term, auth.Authorize, nil, dir))
				cfg := tools.DefaultExecutorConfig()
				cfg.ToolTimeout = time.Second
				a := &App{toolInvocationSvc: toolinvocations.NewService(toolinvocations.NewDBRepository(db), tools.NewExecutor(reg, cfg))}
				skill := skills.Skill{Slug: "base", Content: "!example check\ninstrução preservada"}
				skill.Tools = &skills.ToolPermissions{BashCommands: &skills.BashCommands{Allowed: []string{"example"}}}
				provider := skills.NewContextProvider(channelConsentBaseSkill{skill})
				turn := skills.WithCommandTurn(ctx, a.runSkillCommand)
				done := make(chan struct{})
				var blocks []contextprovider.Block
				var buildErr error
				go func() {
					defer close(done)
					blocks, buildErr = provider.Build(turn, contextprovider.BuildRequest{EnabledSkills: []string{"base"}, ToolCallingEnabled: true})
				}()
				select {
				case <-opened:
				case <-done:
					t.Fatalf("preparação terminou sem confirmação: %v %v", blocks, buildErr)
				}
				answer := func(owner string) messaging.AnswerResult {
					return questions.TryAnswer(t.Context(), inv.ConversationID, owner, "2")
				}
				if answer("terceiro") != messaging.AnswerIgnored {
					t.Fatal("resposta do terceiro aceita")
				}
				if tc.denyBeforeDeadline && answer("dono") != messaging.AnswerDelivered {
					t.Fatal("negativa do dono perdida")
				}
				<-done // O relógio virtual vence com o transporte ainda bloqueado.
				if !tc.denyBeforeDeadline && answer("dono") != messaging.AnswerIgnored {
					t.Fatal("negativa tardia aceita")
				}
				if tc.denyBeforeDeadline {
					if !errors.Is(buildErr, contextprovider.ErrCommandRefused) || len(blocks) != 0 || !errors.Is(skills.CommandTurnError(turn), contextprovider.ErrCommandRefused) {
						t.Fatalf("turno não interrompido: %v %v", blocks, buildErr)
					}
				} else if buildErr != nil || skills.CommandTurnError(turn) != nil || len(blocks) != 1 || !strings.Contains(blocks[0].Content, "instrução preservada") || strings.Contains(blocks[0].Content, "!example") {
					t.Fatalf("turno após confirmação indisponível: %v %v", blocks, buildErr)
				}
				resume()
				synctest.Wait()
				var entry database.ToolInvocation
				if err := db.Where("origin_id = ?", inv.TurnID).First(&entry).Error; err != nil || entry.ErrorCode != tc.wantCode || term.calls != 0 {
					t.Fatalf("código=%q despachos=%d erro=%v", entry.ErrorCode, term.calls, err)
				}
			})
		})
	}
}
