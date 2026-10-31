package portability

import (
	"testing"

	"assistente/internal/database"
	"assistente/internal/toolinvocations"
)

func TestTimelineVersionFollowsLedgerProjection(t *testing.T) {
	setupPortabilityRuntimeTestDB(t)
	ctx := portabilityTestCtx()
	conv, err := database.CreateConversationWithContext(ctx, "Projeção", "")
	if err != nil {
		t.Fatal(err)
	}
	question, err := database.CreateMessageWithContext(ctx, database.MessageOptions{ConversationID: conv.ID, Role: "user", Content: "pergunta"})
	if err != nil {
		t.Fatal(err)
	}
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	catalog := database.ToolCatalog{Name: "timeline-tool", DisplayName: "Nome antigo", Origin: "builtin"}
	if err := database.DB().Create(&catalog).Error; err != nil {
		t.Fatal(err)
	}
	invocation := database.ToolInvocation{UserID: userID, ToolCatalogID: catalog.ID, OriginType: "chat", TurnID: &question.ID, ToolCallID: "call", Status: "running"}
	if err := database.DB().Create(&invocation).Error; err != nil {
		t.Fatal(err)
	}
	read := func() (database.ChatTimelineVersion, toolinvocations.Summary) {
		t.Helper()
		version, err := database.ReadChatTimelineVersion(ctx, database.DB(), conv.ID)
		if err != nil {
			t.Fatal(err)
		}
		summaries, err := toolinvocations.LoadSummariesForTurnIDsWithUser(ctx, userID, []string{question.ID})
		if err != nil || len(summaries[question.ID]) != 1 {
			t.Fatalf("resumos: %+v %v", summaries, err)
		}
		return version, summaries[question.ID][0]
	}
	before, summary := read()
	if summary.Name != "Nome antigo" {
		t.Fatalf("catálogo não projetado: %+v", summary)
	}
	if err := database.DB().Exec("UPDATE tool_invocations SET output_preview='novo resultado',status='succeeded' WHERE id=?", invocation.ID).Error; err != nil {
		t.Fatal(err)
	}
	after, summary := read()
	if after.Revision <= before.Revision || summary.OutputPreview != "novo resultado" {
		t.Fatalf("vínculo nulo não invalidou: %+v %+v %+v", before, after, summary)
	}
	before = after
	if err := database.DB().Exec("UPDATE tool_catalog SET updated_at=CURRENT_TIMESTAMP WHERE id=?", catalog.ID).Error; err != nil {
		t.Fatal(err)
	}
	unchanged, _ := read()
	if unchanged != before {
		t.Fatalf("renovação sem mudança visual invalidou catálogo: %+v", unchanged)
	}
	if err := database.DB().Exec("UPDATE tool_catalog SET display_name='Nome novo',origin='mcp' WHERE id=?", catalog.ID).Error; err != nil {
		t.Fatal(err)
	}
	after, summary = read()
	if after.Revision <= before.Revision || summary.Name != "Nome novo" || summary.Origin != "mcp" {
		t.Fatalf("catálogo não invalidou: %+v %+v %+v", before, after, summary)
	}
	// Um conversation_id diferente não apaga a associação consumida por turn_id.
	other, err := database.CreateConversationWithContext(ctx, "Outra", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := database.DB().Exec("UPDATE tool_invocations SET conversation_id=? WHERE id=?", other.ID, invocation.ID).Error; err != nil {
		t.Fatal(err)
	}
	before, _ = read()
	if err := database.DB().Exec("UPDATE tool_invocations SET output_preview='terceiro resultado' WHERE id=?", invocation.ID).Error; err != nil {
		t.Fatal(err)
	}
	after, summary = read()
	if after.Revision <= before.Revision || summary.OutputPreview != "terceiro resultado" {
		t.Fatalf("vínculo divergente não invalidou: %+v %+v", after, summary)
	}
}
