package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"assistente/internal/commandexecution"
	"assistente/internal/commandui"
	"assistente/internal/database"
	"assistente/internal/workspace"
	"github.com/google/uuid"
)

func contextChatFixture(t *testing.T, kind workspace.TabType) *App {
	t.Helper()
	a := readyCommandProduct(t)
	if err := database.DB().AutoMigrate(&database.Conversation{}, &database.ChatMessage{}); err != nil {
		t.Fatal(err)
	}
	id := uuid.NewString()
	if err := a.workspaceMgr.AddTab(workspace.Tab{ID: id, Type: kind}); err != nil {
		t.Fatal(err)
	}
	if err := a.workspaceMgr.SetActiveTab(id); err != nil {
		t.Fatal(err)
	}
	return a
}

func contextChatCount(t *testing.T) int64 {
	t.Helper()
	var count int64
	if err := database.DB().Model(&database.Conversation{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	return count
}

func beginContextChat(t *testing.T, a *App) (commandui.Reservation, commandui.Handoff) {
	t.Helper()
	r := beginUICommand(t, a, commandWorkspaceChatOpenID)
	return r, takeUICommandFor(t, a, r.Ticket, commandWorkspaceChatOpenID)
}

func TestCommandWorkspaceContextChatCreatesAndReusesOwnedConversation(t *testing.T) {
	for _, kind := range []workspace.TabType{workspace.TabTypeEditor, workspace.TabTypeTerminal, workspace.TabTypeTasklist} {
		t.Run(string(kind), func(t *testing.T) {
			a := contextChatFixture(t, kind)
			emitter := wireCommandWorkspaceForTest(a)
			before := a.workspaceMgr.Active()
			r, h := beginContextChat(t, a)
			if err := a.CommitWorkspaceTabCommand(r.Ticket, h.HandoffID); err != nil {
				t.Fatal(err)
			}
			if result := getUIResultEventually(t, a, r.Ticket); result.Status != "succeeded" {
				t.Fatalf("result=%+v", result)
			}
			after := a.workspaceMgr.Active()
			if after.ID != before.ID || after.Tabs.Active != before.Tabs.Active || len(after.Tabs.Items) != len(before.Tabs.Items) {
				t.Fatal("navigation/tab mutation during contextual chat")
			}
			cid := after.FindTab(after.Tabs.Active).ConversationID
			if _, err := uuid.Parse(cid); err != nil {
				t.Fatal(err)
			}
			ctx := database.WithUserID(context.Background(), a.commandProduct.Load().principal.UserID)
			if _, err := database.GetConversationInfoWithContext(ctx, cid); err != nil {
				t.Fatal(err)
			}
			if contextChatCount(t) != 1 || len(emitter.find("workspace:conversation_bound")) != 1 {
				t.Fatal("expected one conversation and one binding event")
			}
			if err := a.CommitWorkspaceTabCommand(r.Ticket, h.HandoffID); err == nil {
				t.Fatal("replay accepted")
			}
			r, h = beginContextChat(t, a)
			if err := a.CommitWorkspaceTabCommand(r.Ticket, h.HandoffID); err != nil {
				t.Fatal(err)
			}
			if result := getUIResultEventually(t, a, r.Ticket); result.Status != "succeeded" {
				t.Fatalf("reuse=%+v", result)
			}
			if contextChatCount(t) != 1 || a.workspaceMgr.Active().FindTab(after.Tabs.Active).ConversationID != cid {
				t.Fatal("existing conversation replaced")
			}
		})
	}
}

func TestCommandWorkspaceContextChatChatPanelDoesNotCreateConversation(t *testing.T) {
	a := contextChatFixture(t, workspace.TabTypeChat)
	r, h := beginContextChat(t, a)
	if err := a.CommitWorkspaceTabCommand(r.Ticket, h.HandoffID); err != nil {
		t.Fatal(err)
	}
	if result := getUIResultEventually(t, a, r.Ticket); result.Status != "succeeded" {
		t.Fatalf("result=%+v", result)
	}
	if contextChatCount(t) != 0 {
		t.Fatal("focusing chat created a conversation")
	}
}

func TestCommandWorkspaceContextChatRejectsForeignConversation(t *testing.T) {
	a := contextChatFixture(t, workspace.TabTypeEditor)
	foreign, err := database.CreateConversationWithContext(database.WithUserID(context.Background(), "another-owner"), "private", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.workspaceMgr.UpdateTab(a.workspaceMgr.Active().Tabs.Active, map[string]any{"conversation_id": foreign.ID}); err != nil {
		t.Fatal(err)
	}
	r, h := beginContextChat(t, a)
	if err := a.CommitWorkspaceTabCommand(r.Ticket, h.HandoffID); err == nil {
		t.Fatal("foreign conversation accepted")
	}
	if contextChatCount(t) != 1 {
		t.Fatal("foreign conversation replaced/deleted")
	}
}

func TestCommandWorkspaceContextChatFailureCompensatesOnlyNewConversation(t *testing.T) {
	a := contextChatFixture(t, workspace.TabTypeEditor)
	owner := a.commandProduct.Load().principal.UserID
	kept, err := database.CreateConversationWithContext(database.WithUserID(context.Background(), owner), "keep", "")
	if err != nil {
		t.Fatal(err)
	}
	r, h := beginContextChat(t, a)
	// Fixture path is under t.TempDir. A nonempty directory forces publication
	// failure on Windows and Unix without permission tricks or production hooks.
	path := filepath.Join(a.workspaceMgr.ActivePath(), ".assistente", "workspace.yaml")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "marker"), []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := a.CommitWorkspaceTabCommand(r.Ticket, h.HandoffID); err == nil {
		t.Fatal("failed persistence accepted")
	}
	if result := getUIResultEventually(t, a, r.Ticket); result.Status == "succeeded" {
		t.Fatalf("failed binding reported success: %+v", result)
	}
	if contextChatCount(t) != 1 {
		t.Fatal("orphan conversation or unrelated deletion")
	}
	if _, err := database.GetConversationInfoWithContext(database.WithUserID(context.Background(), owner), kept.ID); err != nil {
		t.Fatal(err)
	}
	active := a.workspaceMgr.Active()
	if active.FindTab(active.Tabs.Active).ConversationID != "" {
		t.Fatal("failed binding retained in memory")
	}
}

func TestCommandWorkspaceContextChatGuards(t *testing.T) {
	for _, scenario := range []string{"cancel", "stale", "forged", "revoked", "raw"} {
		t.Run(scenario, func(t *testing.T) {
			a := contextChatFixture(t, workspace.TabTypeTasklist)
			if scenario == "raw" {
				result, err := a.ExecutePaletteCommand(commandWorkspaceChatOpenID, nil)
				if err == nil && result.Status == "succeeded" {
					t.Fatal("raw execution bypassed handoff")
				}
			} else {
				r, h := beginContextChat(t, a)
				switch scenario {
				case "cancel":
					if err := a.CancelUICommand(r.Ticket); err != nil {
						t.Fatal(err)
					}
				case "stale":
					if err := a.workspaceMgr.AddTab(workspace.Tab{ID: uuid.NewString(), Type: workspace.TabTypeEditor}); err != nil {
						t.Fatal(err)
					}
				case "forged":
					h.HandoffID += "-forged"
				case "revoked":
					if err := database.DB().Exec("UPDATE sessions SET revoked_at = CURRENT_TIMESTAMP WHERE id = ?", a.commandProduct.Load().principal.SessionID).Error; err != nil {
						t.Fatal(err)
					}
				}
				err := a.CommitWorkspaceTabCommand(r.Ticket, h.HandoffID)
				if err == nil {
					t.Fatal("invalid commit accepted")
				}
				if scenario == "stale" && !errors.Is(err, commandexecution.ErrStale) {
					t.Fatalf("stale=%v", err)
				}
			}
			if contextChatCount(t) != 0 {
				t.Fatal("rejected command created a conversation")
			}
		})
	}
}
