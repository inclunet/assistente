package app

import (
	"assistente/internal/apidto"
	"assistente/internal/chat"
	"assistente/internal/database"
	"assistente/internal/events"
	"assistente/internal/wailsapi"
	"assistente/internal/workspace"
	"context"
	"github.com/google/uuid"
	"testing"
)

func TestCommandChatTimelineCopyUsesExactPublishedResponse(t *testing.T) {
	for _, scenario := range []string{"copy", "markdown", "close", "wrong-conversation", "revision", "pin", "source-generation", "discard-after-prepare"} {
		t.Run(scenario, func(t *testing.T) {
			a, _, cid := clearCommandFixture(t, workspace.TabTypeChat)
			db := database.DB()
			if err := db.AutoMigrate(&database.ToolCatalog{}, &database.ToolInvocation{}); err != nil {
				t.Fatal(err)
			}
			if err := database.MigrateChatTurns(db); err != nil {
				t.Fatal(err)
			}
			if err := database.MigrateChatExecutionContent(db); err != nil {
				t.Fatal(err)
			}
			if err := db.Exec("CREATE TABLE chat_timeline_versions (conversation_id TEXT PRIMARY KEY, incarnation TEXT NOT NULL, revision INTEGER NOT NULL)").Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Exec("INSERT INTO chat_timeline_versions VALUES (?, 'copy-incarnation', 1)", cid).Error; err != nil {
				t.Fatal(err)
			}
			ctx := database.WithUserID(context.Background(), a.currentUserID)
			question := messageCommandSeed(t, cid)
			reader := chat.NewDBTimelineReader(db)
			projection := chat.NewLiveProjection(reader)
			executionID, err := uuid.NewV7()
			if err != nil {
				t.Fatal(err)
			}
			eid := executionID.String()
			repo := database.NewChatTurnRepository(db)
			execution, err := projection.Admit(ctx, cid, question.ID, eid, func() (database.ChatTimelineVersion, error) {
				_, err := repo.BeginExecution(ctx, cid, question.ID, "", database.TurnModeNew, eid, "")
				if err != nil {
					return database.ChatTimelineVersion{}, err
				}
				return reader.ReadVersion(ctx, cid)
			})
			if err != nil {
				t.Fatal(err)
			}
			execution.StartStream(0).Append("resposta sem persistência")
			if err := execution.CommitTerminal(database.TurnFailed, "persistence_error", func() (database.ChatTimelineVersion, error) {
				return repo.FinishResponseWithVersion(ctx, cid, question.ID, eid, database.TurnFailed, "persistence_error", "", nil)
			}); err != nil {
				t.Fatal(err)
			}
			a.emitter = events.NoopEmitter{}
			api := wailsapi.NewChatTimeline()
			SetChatTimelineAPI(a, api)
			wailsapi.AttachChatTimeline(api, timelineWailsSession{wailsSession{app: a}}, projection, reader)
			t.Cleanup(func() { wailsapi.CloseChatTimeline(api) })
			opened, err := api.Open(apidto.OpenTimelineRequest{ConversationID: cid, SurfaceID: "copy", TimelineWindowPosition: apidto.TimelineWindowPosition{Anchor: "end", Direction: "before", Limit: 10}})
			if err != nil {
				t.Fatal(err)
			}
			var identity apidto.TimelineRecoveryItemRequest
			for _, item := range opened.Items {
				if item.RecoveryReference != nil {
					if item.RepresentativeMessageID != nil {
						t.Fatal("persisted representative")
					}
					identity = apidto.TimelineRecoveryItemRequest{Reference: *item.RecoveryReference, ItemID: item.ItemID, Epoch: opened.Epoch, Revision: item.Revision}
				}
			}
			if identity.ItemID == "" {
				t.Fatal("missing volatile response")
			}
			id := commandMessageCopyID
			if scenario == "markdown" {
				id = commandMessageCopyMarkdownID
			}
			if scenario == "pin" {
				id = commandMessagePinID
			}
			r := beginUICommand(t, a, id)
			if scenario == "revision" {
				identity.Revision = "999"
			}
			if scenario == "wrong-conversation" {
				identity.Reference.TurnID = uuid.NewString()
			}
			err = a.PrepareChatTimelineCopyCommand(r.Ticket, opened.WindowID, identity)
			if scenario == "pin" || scenario == "revision" || scenario == "wrong-conversation" {
				if err == nil {
					t.Fatal("invalid copy admitted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := a.PrepareChatTimelineCopyCommand(r.Ticket, opened.WindowID, identity); err == nil {
				t.Fatal("prepare replay")
			}
			if scenario == "close" {
				if err := api.Close(opened.WindowID); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "source-generation" {
				_, run, err := a.commandUIRun(r.Ticket)
				if err != nil {
					t.Fatal(err)
				}
				run.sourceValid = func() bool { return false }
			}
			if scenario == "discard-after-prepare" {
				if err := projection.DiscardRetained(ctx, cid, question.ID, eid, reader); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "close" || scenario == "source-generation" || scenario == "discard-after-prepare" {
				if _, err := a.TakeUICommand(r.Ticket); err == nil {
					t.Fatal("stale target handed off")
				}
				return
			}
			handoff := takeUICommandFor(t, a, r.Ticket, id)
			if err := a.CompleteUICommand(r.Ticket, handoff.HandoffID, "succeeded"); err != nil {
				t.Fatal(err)
			}
		})
	}
}
