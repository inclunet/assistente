package app

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"assistente/controllers"
	"assistente/internal/chat"
	"assistente/internal/commandcatalog"
	"assistente/internal/core/ports"
	"assistente/internal/database"
	"assistente/internal/workspace"
	"github.com/google/uuid"
)

func TestCommandChatMessageEditSavePreservesMetadataAndEmitsOnce(t *testing.T) {
	for _, text := range []string{"private edited text\nçãΩ", "  preserved spaces  "} {
		t.Run(text, func(t *testing.T) {
			a, _, cid := clearCommandFixture(t, workspace.TabTypeChat)
			m := messageCommandSeed(t, cid)
			m.Model = "private-model"
			m.Reasoning = "private-reasoning"
			m.Audio = "private-audio"
			m.AudioMimeType = "audio/wav"
			m.PromptTokens = 20
			m.CompletionTokens = 30
			m.TotalTokens = 50
			m.CacheReadTokens = 10
			m.CacheWriteTokens = 2
			m.CacheMissTokens = 8
			m.Pinned = true
			if err := database.DB().Save(&m).Error; err != nil {
				t.Fatal(err)
			}
			// Compare persisted representations, including SQLite time encoding.
			if err := database.DB().First(&m, "id = ?", m.ID).Error; err != nil {
				t.Fatal(err)
			}
			events := make(chan ports.MessageUpdatedEvent, 2)
			a.authMu.Lock()
			a.conversationsCtrl = controllers.NewConversationsController(controllers.ConversationsControllerConfig{PrepareBatchDelete: a.prepareConversationDeletion, Emitter: commandOSBootstrapEmitter(func(name string, value any) {
				if name == "message:updated" {
					events <- value.(ports.MessageUpdatedEvent)
				}
			})})
			a.authMu.Unlock()
			r := beginUICommand(t, a, commandMessageEditSaveID)
			if err := a.PrepareChatMessageCommand(r.Ticket, m.ID); err == nil {
				t.Fatal("save accepted preparation without edit input")
			}
			if err := a.PrepareChatMessageEditCommand(r.Ticket, m.ID, m.Content, text); err != nil {
				t.Fatal(err)
			}
			if err := a.PrepareChatMessageEditCommand(r.Ticket, m.ID, m.Content, "replace frozen input"); err == nil {
				t.Fatal("prepare replay")
			}
			h := takeUICommandFor(t, a, r.Ticket, commandMessageEditSaveID)
			if err := a.CompleteUICommand(r.Ticket, h.HandoffID, "succeeded"); err == nil {
				t.Fatal("UI bypassed save")
			}
			if err := a.CommitChatMessageCommand(r.Ticket, h.HandoffID); err != nil {
				t.Fatal(err)
			}
			if err := a.CommitChatMessageCommand(r.Ticket, h.HandoffID); err == nil {
				t.Fatal("commit replay")
			}
			if got := getUIResultEventually(t, a, r.Ticket); got.Status != "succeeded" {
				t.Fatalf("%+v", got)
			}
			var current database.ChatMessage
			if err := database.DB().First(&current, "id = ?", m.ID).Error; err != nil {
				t.Fatal(err)
			}
			if current.Content != text {
				t.Fatal("content not saved")
			}
			current.Content = m.Content
			current.UpdatedAt = m.UpdatedAt
			if !reflect.DeepEqual(current, m) {
				t.Fatal("metadata changed")
			}
			select {
			case event := <-events:
				if event.ConversationID != cid || event.MessageID != m.ID || event.Content != text {
					t.Fatal("event target/content")
				}
			default:
				t.Fatal("missing event")
			}
			select {
			case <-events:
				t.Fatal("duplicate event")
			default:
			}
			_, run, err := a.commandUIRun(r.Ticket)
			if err != nil {
				t.Fatal(err)
			}
			<-run.done
			p := a.commandProduct.Load()
			p.mu.Lock()
			retained := run.messagePreparation.editedContent != nil
			p.mu.Unlock()
			if retained {
				t.Fatal("text retained after completion")
			}
			for _, private := range []string{m.Content, m.Media, m.Audio, m.Reasoning, text} {
				if private != "" {
					assertChatActionLedgerRedacted(t, r.InvocationID, private)
				}
			}
		})
	}
}

func TestCommandChatMessageEditSaveRejectsStaleOriginal(t *testing.T) {
	a, _, cid := clearCommandFixture(t, workspace.TabTypeChat)
	m := messageCommandSeed(t, cid)
	r := beginUICommand(t, a, commandMessageEditSaveID)
	if err := database.DB().Model(&m).Update("content", "edited while form was open").Error; err != nil {
		t.Fatal(err)
	}
	if err := a.PrepareChatMessageEditCommand(r.Ticket, m.ID, "private-message-batch91", "replacement"); err == nil {
		t.Fatal("stale original accepted")
	}
	if err := a.PrepareChatMessageEditCommand(r.Ticket, m.ID, m.Content, "replacement"); err == nil {
		t.Fatal("failed prepare replay")
	}
	if commandInvocationCount(t, commandMessageEditSaveID) != 0 {
		t.Fatal("admitted stale original")
	}
}

func TestCommandChatMessageEditSaveGuards(t *testing.T) {
	for _, scenario := range []string{"changed", "aba", "generation", "active", "tab", "binding", "session", "cancel", "rollback"} {
		t.Run(scenario, func(t *testing.T) {
			a, _, cid := clearCommandFixture(t, workspace.TabTypeChat)
			m := messageCommandSeed(t, cid)
			r := beginUICommand(t, a, commandMessageEditSaveID)
			if err := a.PrepareChatMessageEditCommand(r.Ticket, m.ID, m.Content, "sensitive-new-content"); err != nil {
				t.Fatal(err)
			}
			h := takeUICommandFor(t, a, r.Ticket, commandMessageEditSaveID)
			switch scenario {
			case "changed":
				if err := database.DB().Model(&m).Update("content", "other edit").Error; err != nil {
					t.Fatal(err)
				}
			case "aba":
				for _, text := range []string{"other edit", m.Content} {
					if err := database.UpdateMessageTextWithContext(database.WithUserID(context.Background(), a.currentUserID), m.ID, text); err != nil {
						t.Fatal(err)
					}
				}
			case "generation":
				if err := a.commandHost.SetActiveLayers(context.Background(), a.currentUserID, []string{"changed"}); err != nil {
					t.Fatal(err)
				}
			case "active":
				generationCtx, cancel := context.WithCancel(context.Background())
				defer cancel()
				generation := a.streamMgr.Register(cid, cancel)
				defer a.streamMgr.UnregisterIfCurrent(cid, generation)
				defer func() {
					if generationCtx.Err() != nil {
						t.Error("save silently cancelled generation")
					}
				}()
			case "tab":
				if err := a.workspaceMgr.AddTab(workspace.Tab{ID: uuid.NewString(), Type: workspace.TabTypeChat}); err != nil {
					t.Fatal(err)
				}
			case "binding":
				if err := a.workspaceMgr.UpdateTab(a.workspaceMgr.Active().Tabs.Active, map[string]any{"conversation_id": uuid.NewString()}); err != nil {
					t.Fatal(err)
				}
			case "session":
				a.authMu.Lock()
				old := a.currentAuthUser.SessionID
				a.currentAuthUser.SessionID = uuid.NewString()
				a.authMu.Unlock()
				defer func() { a.authMu.Lock(); a.currentAuthUser.SessionID = old; a.authMu.Unlock() }()
			case "cancel":
				if err := a.CancelUICommand(r.Ticket); err != nil {
					t.Fatal(err)
				}
			case "rollback":
				if err := database.DB().Exec("CREATE TRIGGER reject_edit BEFORE UPDATE OF content ON chat_messages BEGIN SELECT RAISE(ABORT, 'sensitive-new-content'); END").Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := a.CommitChatMessageCommand(r.Ticket, h.HandoffID); err == nil {
				t.Fatal("unsafe save accepted")
			}
			var current database.ChatMessage
			if err := database.DB().First(&current, "id = ?", m.ID).Error; err != nil || current.Content == "sensitive-new-content" {
				t.Fatal("unsafe text persisted", err)
			}
			if scenario == "rollback" {
				getUIResultEventually(t, a, r.Ticket)
				assertChatActionLedgerRedacted(t, r.InvocationID, "sensitive-new-content")
			}
		})
	}
}

func TestCommandChatMessageEditSaveContract(t *testing.T) {
	a, _, cid := clearCommandFixture(t, workspace.TabTypeChat)
	p := a.commandProduct.Load()
	d, ok := p.registry.Lookup(commandMessageEditSaveID)
	if !ok || d.Effect != commandcatalog.Write || d.Decision != commandcatalog.NoDecision || d.HandlerClassification != commandcatalog.HandlerBackend || isLocalUICommand(d.ID) || !localKeyboardCommandAllowed(d.ID) || !commandDeckLedgerCommand(d) {
		t.Fatal("classification")
	}
	names := []string{"Salvar edição da mensagem", "Save message edit", "Guardar edición del mensaje"}
	for i, locale := range []string{"pt-BR", "en", "es"} {
		if d.Presentation.Locales[locale].Name != names[i] {
			t.Fatal("label")
		}
	}
	if commandUIRunTimeout(d.ID) != 30*time.Second {
		t.Fatal("unexpected extended timeout")
	}
	m := messageCommandSeed(t, cid)
	r := beginUICommand(t, a, commandMessagePinID)
	if err := a.PrepareChatMessageEditCommand(r.Ticket, m.ID, m.Content, "text"); err == nil {
		t.Fatal("edit payload accepted by pin")
	}
	if err := a.CancelUICommand(r.Ticket); err != nil {
		t.Fatal(err)
	}
}

func TestCommandChatMessageEditSaveRejectsInvalidInputAndTargets(t *testing.T) {
	for _, scenario := range []string{"blank", "whitespace", "oversized", "assistant", "system", "tool", "reasoning", "internal", "foreign-owner", "other-conversation"} {
		t.Run(scenario, func(t *testing.T) {
			a, _, cid := clearCommandFixture(t, workspace.TabTypeChat)
			m := messageCommandSeed(t, cid)
			content := "valid new text"
			switch scenario {
			case "blank":
				content = ""
			case "whitespace":
				content = " \t\r\n\u2003"
			case "oversized":
				content = strings.Repeat("a", chat.MaxMessageContentSize+1)
			case "assistant", "system", "tool", "reasoning":
				m.Role = scenario
			case "internal":
				parent := messageCommandSeed(t, cid)
				m.ParentID = &parent.ID
			case "foreign-owner", "other-conversation":
				owner := a.currentUserID
				if scenario == "foreign-owner" {
					owner = uuid.NewString()
				}
				conv, err := database.CreateConversationWithContext(database.WithUserID(context.Background(), owner), "other", "")
				if err != nil {
					t.Fatal(err)
				}
				m.ConversationID = conv.ID
			}
			if err := database.DB().Save(&m).Error; err != nil {
				t.Fatal(err)
			}
			r := beginUICommand(t, a, commandMessageEditSaveID)
			if err := a.PrepareChatMessageEditCommand(r.Ticket, m.ID, m.Content, content); err == nil {
				t.Fatal("invalid edit accepted")
			}
			if commandInvocationCount(t, commandMessageEditSaveID) != 0 {
				t.Fatal("invalid edit admitted")
			}
		})
	}
}
