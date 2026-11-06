package app

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"assistente/internal/chat"
	"assistente/internal/database"
	"assistente/internal/tools"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupMessageWindowAppTestDB(t testing.TB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&database.User{},
		&database.Conversation{},
		&database.ChatMessage{},
		&database.ToolCatalog{},
		&database.ToolInvocation{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := database.MigrateChatTurns(db); err != nil {
		t.Fatal(err)
	}
	if err := database.MigrateChatExecutionContent(db); err != nil {
		t.Fatal(err)
	}
	// Fixture imutável de leitura: a integração de versão/triggers usa InitPath
	// nos testes de snapshot. Preserva o ambiente SQLite em memória da baseline.
	if err := db.Exec(`CREATE TABLE chat_timeline_versions (conversation_id TEXT PRIMARY KEY, incarnation TEXT NOT NULL, revision INTEGER NOT NULL)`).Error; err != nil {
		t.Fatal(err)
	}
	database.SetDB(db)
	// Seed do usuário do teste (ToolInvocation tem FK para users).
	if err := db.Create(&database.User{
		UUIDModel:    database.UUIDModel{ID: messageWindowTestUserID},
		Username:     "message-window",
		DisplayName:  "Message Window",
		PasswordHash: "x",
		Role:         database.UserRoleUser,
		IsActive:     true,
		LastLoginAt:  nil,
		Sessions:     nil,
	}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	// Seed de uma tool builtin usada pelos testes.
	if err := db.Create(&database.ToolCatalog{
		Name:               "search",
		DisplayName:        "search",
		Origin:             tools.ToolOriginBuiltin,
		AvailabilityStatus: tools.ToolAvailabilityAvailable,
	}).Error; err != nil {
		t.Fatalf("seed tool catalog: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	})
}

const messageWindowTestUserID = "user-message-window"

func messageWindowTestCtx() context.Context {
	return database.WithUserID(context.Background(), messageWindowTestUserID)
}

type messageWindowTestReader struct {
	projection *chat.LiveProjection
	reader     *chat.DBTimelineReader
}
type messageWindowTestSession struct{}

func (messageWindowTestSession) AuthorizeTimeline(ctx context.Context) (context.Context, string, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, "", err
	}
	return ctx, "window-test-session", nil
}
func newMessageWindowTestReader() *messageWindowTestReader {
	return &messageWindowTestReader{chat.NewLiveProjection(nil), chat.NewDBTimelineReader(database.DB())}
}
func (r *messageWindowTestReader) Read(ctx context.Context, q database.MessageWindowQuery) (*chat.SubscribedTimelineSnapshot, error) {
	sub, snapshot, err := r.projection.SubscribeTimeline(ctx, messageWindowTestSession{}, "window-test", q, r.reader)
	if err != nil {
		return nil, err
	}
	defer sub.Close()
	return snapshot, nil
}
func windowItemText(item chat.CanonicalItem) string {
	var text strings.Builder
	for _, segment := range item.Segments {
		if segment.Kind == "text" {
			text.WriteString(segment.Text)
		}
	}
	return text.String()
}
func windowRepresentative(item chat.CanonicalItem) string {
	if item.RepresentativeMessageID == nil {
		return ""
	}
	return *item.RepresentativeMessageID
}

func createMessageWindowTestConversation(t testing.TB, title string) *database.Conversation {
	t.Helper()
	ctx := database.WithUserID(context.Background(), messageWindowTestUserID)
	conv, err := database.CreateConversationWithContext(ctx, title, "")
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	if err := database.DB().Exec("INSERT INTO chat_timeline_versions VALUES(?, ?, 0)", conv.ID, "0123456789abcdef0123456789abcdef").Error; err != nil {
		t.Fatal(err)
	}
	return conv
}

func addMessageWindowAssistant(t testing.TB, ctx context.Context, conversationID, turnID, content string) *database.ChatMessage {
	t.Helper()
	message, err := database.CreateMessageWithContext(ctx, database.MessageOptions{
		ConversationID: conversationID,
		TurnID:         &turnID,
		Role:           "assistant",
		Content:        content,
	})
	if err != nil {
		t.Fatalf("create assistant: %v", err)
	}
	return message
}

func TestGetConversationMessageWindow_ValidatesRequestShape(t *testing.T) {
	setupMessageWindowAppTestDB(t)
	ctrl := newMessageWindowTestReader()

	conv := createMessageWindowTestConversation(t, "Conversa")
	ctx := database.WithUserID(context.Background(), messageWindowTestUserID)
	if _, err := database.AddMessageWithContext(ctx, conv.ID, "user", "mensagem"); err != nil {
		t.Fatalf("create message: %v", err)
	}

	_, err := ctrl.Read(messageWindowTestCtx(), database.MessageWindowQuery{
		ConversationID: conv.ID,
		Direction:      "sideways",
		Limit:          10,
	})
	if err == nil || !strings.Contains(err.Error(), "direction") {
		t.Fatalf("expected direction validation error, got %v", err)
	}

	_, err = ctrl.Read(messageWindowTestCtx(), database.MessageWindowQuery{
		ConversationID: conv.ID,
		Direction:      chat.MessageWindowDirectionAround,
		Limit:          10,
	})
	if err == nil || !strings.Contains(err.Error(), "anchorMessageId") {
		t.Fatalf("expected around anchor validation error, got %v", err)
	}
}

func TestGetConversationMessageWindow_RejectsNestedThreadParent(t *testing.T) {
	setupMessageWindowAppTestDB(t)
	ctrl := newMessageWindowTestReader()

	conv := createMessageWindowTestConversation(t, "Conversa")
	ctx := database.WithUserID(context.Background(), messageWindowTestUserID)
	root, err := database.AddMessageWithContext(ctx, conv.ID, "assistant", "root")
	if err != nil {
		t.Fatalf("create root: %v", err)
	}
	child, err := database.AddChildMessageWithContext(ctx, conv.ID, root.ID, "assistant", "child", "")
	if err != nil {
		t.Fatalf("create child: %v", err)
	}

	_, err = ctrl.Read(messageWindowTestCtx(), database.MessageWindowQuery{
		ConversationID: conv.ID,
		ParentID:       &child.ID,
		Anchor:         chat.MessageWindowAnchorStart,
		Direction:      chat.MessageWindowDirectionAfter,
		Limit:          10,
	})
	if err == nil || !strings.Contains(err.Error(), "mensagem raiz") {
		t.Fatalf("expected root thread parent validation error, got %v", err)
	}
}

func TestGetConversationMessageWindow_NormalizesAnchorNotFound(t *testing.T) {
	setupMessageWindowAppTestDB(t)
	ctrl := newMessageWindowTestReader()

	conv := createMessageWindowTestConversation(t, "Conversa")
	ctx := database.WithUserID(context.Background(), messageWindowTestUserID)
	if _, err := database.AddMessageWithContext(ctx, conv.ID, "user", "mensagem"); err != nil {
		t.Fatalf("create message: %v", err)
	}

	_, err := ctrl.Read(messageWindowTestCtx(), database.MessageWindowQuery{
		ConversationID:  conv.ID,
		AnchorMessageID: "missing-message",
		Direction:       chat.MessageWindowDirectionAfter,
		Limit:           10,
	})
	if err == nil || !strings.Contains(err.Error(), "anchorMessageId inválido") {
		t.Fatalf("expected normalized anchor error, got %v", err)
	}
}

func TestGetConversationMessageWindow_ClampsOversizedLimit(t *testing.T) {
	setupMessageWindowAppTestDB(t)
	ctrl := newMessageWindowTestReader()

	conv := createMessageWindowTestConversation(t, "Conversa")
	ctx := database.WithUserID(context.Background(), messageWindowTestUserID)
	for i := 0; i < database.MaxMessageWindowRows+30; i++ {
		if _, err := database.AddMessageWithContext(ctx, conv.ID, "user", "mensagem"); err != nil {
			t.Fatalf("create message %d: %v", i, err)
		}
	}

	window, err := ctrl.Read(messageWindowTestCtx(), database.MessageWindowQuery{
		ConversationID: conv.ID,
		Anchor:         chat.MessageWindowAnchorEnd,
		Direction:      chat.MessageWindowDirectionBefore,
		Limit:          database.MaxMessageWindowRows + 60,
	})
	if err != nil {
		t.Fatalf("get window: %v", err)
	}
	if len(window.Items) != database.MaxMessageWindowRows {
		t.Fatalf("expected clamped window size %d, got %d", database.MaxMessageWindowRows, len(window.Items))
	}
}

func TestGetConversationMessageWindow_ReturnsCanonicalTimelineItems(t *testing.T) {
	setupMessageWindowAppTestDB(t)
	ctrl := newMessageWindowTestReader()

	conv := createMessageWindowTestConversation(t, "Conversa")
	ctx := database.WithUserID(context.Background(), messageWindowTestUserID)
	user, err := database.AddMessageWithContext(ctx, conv.ID, "user", "pergunta")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	intermediate := addMessageWindowAssistant(t, ctx, conv.ID, user.ID, "vou buscar")
	// Resultado técnico agora vem de tool_invocations (não role=tool messages).
	var catalog database.ToolCatalog
	if err := database.DB().WithContext(ctx).First(&catalog, "name = ?", "search").Error; err != nil {
		t.Fatalf("load tool catalog: %v", err)
	}
	if err := database.DB().WithContext(ctx).Create(&database.ToolInvocation{
		UUIDModel:      database.UUIDModel{ID: "inv-1"},
		UserID:         messageWindowTestUserID,
		ToolCatalogID:  catalog.ID,
		OriginType:     "chat",
		OriginID:       user.ID,
		ConversationID: &conv.ID,
		TurnID:         &user.ID,
		ToolCallID:     "tool-1",
		Status:         "succeeded",
		DryRun:         false,
		Output:         `{"content":"resultado","is_error":false}`,
		Metadata:       fmt.Sprintf(`{"display":{"version":1,"assistant_message_id":%q,"iteration":0}}`, intermediate.ID),
		QueuedAt:       time.Now(),
	}).Error; err != nil {
		t.Fatalf("create tool invocation: %v", err)
	}
	finalAssistant, err := database.AddMessageWithTokensWithContext(ctx, conv.ID, "assistant", "resposta final", 0, 0, 0, "")
	if err != nil {
		t.Fatalf("create final assistant: %v", err)
	}
	finalAssistant.TurnID = &user.ID
	if err := database.DB().Save(finalAssistant).Error; err != nil {
		t.Fatalf("save final assistant turn: %v", err)
	}
	if _, err := database.AddChildMessageWithContext(ctx, conv.ID, finalAssistant.ID, "assistant", "resposta filha", ""); err != nil {
		t.Fatalf("create child for consolidated representative: %v", err)
	}

	window, err := ctrl.Read(messageWindowTestCtx(), database.MessageWindowQuery{
		ConversationID: conv.ID,
		Anchor:         chat.MessageWindowAnchorEnd,
		Direction:      chat.MessageWindowDirectionBefore,
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("get window: %v", err)
	}

	if window.TotalCount != 2 {
		t.Fatalf("expected user item + consolidated turn item, got total=%d", window.TotalCount)
	}
	if len(window.Items) != 2 {
		t.Fatalf("expected 2 rendered timeline nodes, got %d", len(window.Items))
	}
	if windowRepresentative(window.Items[0]) != user.ID {
		t.Fatalf("expected first node to be user item, got %s", windowRepresentative(window.Items[0]))
	}
	turnNode := window.Items[1]
	if windowRepresentative(turnNode) != finalAssistant.ID {
		t.Fatalf("expected turn representative to be final assistant, got %s", windowRepresentative(turnNode))
	}
	if turnNode.Metadata.ChildCount != 1 {
		t.Fatalf("expected child count from consolidated representative only, got %d", turnNode.Metadata.ChildCount)
	}
	if window.StartIndex+1 != 1 {
		t.Fatalf("expected canonical originalIndex=1 for turn item, got %v", window.StartIndex+1)
	}
	if turnNode.Segments[len(turnNode.Segments)-1].Text != "resposta final" {
		t.Fatalf("expected final content, got %q", windowItemText(turnNode))
	}
	// Issue #150: o turno volta com segments cronológicos para que o frontend
	// renderize o turno inteiro em UMA única entrada acessível (cadeia de
	// raciocínio: texto → tool_calls → resposta final).
	if len(turnNode.Segments) != 3 {
		t.Fatalf("expected 3 segments (intermediate text → tool_calls → final text), got %+v", turnNode.Segments)
	}
	if turnNode.Segments[0].Kind != "text" || turnNode.Segments[0].Text != "vou buscar" {
		t.Fatalf("expected first segment to be intermediate text 'vou buscar', got %+v", turnNode.Segments[0])
	}
	if turnNode.Segments[1].Kind != "tool_invocations" || len(turnNode.Segments[1].Invocations) == 0 {
		t.Fatalf("expected second segment to be tool_calls, got %+v", turnNode.Segments[1])
	}
	if turnNode.Segments[2].Kind != "text" || turnNode.Segments[2].Text != "resposta final" {
		t.Fatalf("expected third segment to be final text, got %+v", turnNode.Segments[2])
	}
}

func TestGetConversationMessageWindow_HydratesToolCallsFromInvocationsWithoutMessageToolCalls(t *testing.T) {
	setupMessageWindowAppTestDB(t)
	ctrl := newMessageWindowTestReader()

	conv := createMessageWindowTestConversation(t, "Conversa")
	ctx := database.WithUserID(context.Background(), messageWindowTestUserID)
	user, err := database.AddMessageWithContext(ctx, conv.ID, "user", "pergunta")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	intermediate := addMessageWindowAssistant(t, ctx, conv.ID, user.ID, "vou buscar")
	finalAssistant, err := database.AddMessageWithTokensWithContext(ctx, conv.ID, "assistant", "resposta final", 0, 0, 0, "")
	if err != nil {
		t.Fatalf("create final assistant: %v", err)
	}
	finalAssistant.TurnID = &user.ID
	if err := database.DB().Save(finalAssistant).Error; err != nil {
		t.Fatalf("save final assistant turn: %v", err)
	}

	var catalog database.ToolCatalog
	if err := database.DB().WithContext(ctx).First(&catalog, "name = ?", "search").Error; err != nil {
		t.Fatalf("load tool catalog: %v", err)
	}
	if err := database.DB().WithContext(ctx).Create(&database.ToolInvocation{
		UUIDModel:      database.UUIDModel{ID: "inv-new-l3-free"},
		UserID:         messageWindowTestUserID,
		ToolCatalogID:  catalog.ID,
		OriginType:     "chat",
		OriginID:       user.ID,
		ConversationID: &conv.ID,
		TurnID:         &user.ID,
		ToolCallID:     "tool-1",
		Status:         "succeeded",
		DryRun:         false,
		Input:          `{"query":"foo"}`,
		Output:         `{"content":"resultado por invocacao","is_error":false}`,
		Metadata:       fmt.Sprintf(`{"display":{"version":1,"type":"function","name":"search","arguments":"{\"q\":\"foo\"}","origin":"builtin","assistant_message_id":%q,"iteration":1,"duration_ms":42},"search_result_presentation":{"version":1,"total":2},"security_signals":[{"version":1,"outcome":"approved"}]}`, intermediate.ID),
		QueuedAt:       time.Now(),
		DurationMs:     42,
	}).Error; err != nil {
		t.Fatalf("create tool invocation: %v", err)
	}
	window, err := ctrl.Read(messageWindowTestCtx(), database.MessageWindowQuery{
		ConversationID: conv.ID,
		Anchor:         chat.MessageWindowAnchorEnd,
		Direction:      chat.MessageWindowDirectionBefore,
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("get window: %v", err)
	}
	if len(window.Items) != 2 {
		t.Fatalf("expected user + consolidated assistant turn, got %d nodes", len(window.Items))
	}
	turnNode := window.Items[1]
	if len(turnNode.Segments) != 3 {
		t.Fatalf("expected text -> tool_calls -> final text segments, got %+v", turnNode.Segments)
	}
	call := turnNode.Segments[1].Invocations[0]
	if call.InvocationID != "inv-new-l3-free" || call.ID != "tool-1" || call.Name != "search" || call.DurationMs != 42 || !call.HasSearchResults || call.SearchResultCount != 2 || call.SecurityOutcome != "approved" {
		t.Fatalf("expected lightweight invocation summary, got %+v", call)
	}
}

func TestGetConversationMessageWindow_AnchorInsideTurnUsesTimelineItem(t *testing.T) {
	setupMessageWindowAppTestDB(t)
	ctrl := newMessageWindowTestReader()

	conv := createMessageWindowTestConversation(t, "Conversa")
	ctx := database.WithUserID(context.Background(), messageWindowTestUserID)
	user, err := database.AddMessageWithContext(ctx, conv.ID, "user", "pergunta")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	assistant := addMessageWindowAssistant(t, ctx, conv.ID, user.ID, "vou buscar")
	var catalog database.ToolCatalog
	if err := database.DB().WithContext(ctx).First(&catalog, "name = ?", "search").Error; err != nil {
		t.Fatalf("load tool catalog: %v", err)
	}
	if err := database.DB().WithContext(ctx).Create(&database.ToolInvocation{
		UUIDModel:      database.UUIDModel{ID: "inv-2"},
		UserID:         messageWindowTestUserID,
		ToolCatalogID:  catalog.ID,
		OriginType:     "chat",
		OriginID:       user.ID,
		ConversationID: &conv.ID,
		TurnID:         &user.ID,
		ToolCallID:     "tool-1",
		Status:         "succeeded",
		DryRun:         false,
		Output:         `{"content":"resultado","is_error":false}`,
		QueuedAt:       time.Now(),
	}).Error; err != nil {
		t.Fatalf("create tool invocation: %v", err)
	}
	nextUser, err := database.AddMessageWithContext(ctx, conv.ID, "user", "pergunta seguinte")
	if err != nil {
		t.Fatalf("create next user: %v", err)
	}

	window, err := ctrl.Read(messageWindowTestCtx(), database.MessageWindowQuery{
		ConversationID:  conv.ID,
		AnchorMessageID: assistant.ID,
		Direction:       chat.MessageWindowDirectionAfter,
		Limit:           1,
	})
	if err != nil {
		t.Fatalf("get window: %v", err)
	}
	if len(window.Items) != 1 || windowRepresentative(window.Items[0]) != nextUser.ID {
		t.Fatalf("expected anchor inside turn to page after the whole turn, got %+v", window.Items)
	}
}

func TestGetConversationMessageWindow_TurnWithEmptyAssistantKeepsLedgerSummary(t *testing.T) {
	setupMessageWindowAppTestDB(t)
	ctrl := newMessageWindowTestReader()

	conv := createMessageWindowTestConversation(t, "Conversa")
	ctx := database.WithUserID(context.Background(), messageWindowTestUserID)
	user, err := database.AddMessageWithContext(ctx, conv.ID, "user", "pergunta")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	assistant := addMessageWindowAssistant(t, ctx, conv.ID, user.ID, "")
	var catalog database.ToolCatalog
	if err := database.DB().WithContext(ctx).First(&catalog, "name = ?", "search").Error; err != nil {
		t.Fatalf("load tool catalog: %v", err)
	}
	if err := database.DB().WithContext(ctx).Create(&database.ToolInvocation{
		UUIDModel:      database.UUIDModel{ID: "inv-3"},
		UserID:         messageWindowTestUserID,
		ToolCatalogID:  catalog.ID,
		OriginType:     "chat",
		OriginID:       user.ID,
		ConversationID: &conv.ID,
		TurnID:         &user.ID,
		ToolCallID:     "tool-1",
		Status:         "succeeded",
		DryRun:         false,
		Output:         `{"content":"resultado preservado","is_error":false}`,
		QueuedAt:       time.Now(),
	}).Error; err != nil {
		t.Fatalf("create tool invocation: %v", err)
	}

	window, err := ctrl.Read(messageWindowTestCtx(), database.MessageWindowQuery{
		ConversationID: conv.ID,
		Anchor:         chat.MessageWindowAnchorStart,
		Direction:      chat.MessageWindowDirectionAfter,
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("get window: %v", err)
	}
	if window.TotalCount != 2 || len(window.Items) != 2 {
		t.Fatalf("expected user item + assistant turn item, got total=%d nodes=%d", window.TotalCount, len(window.Items))
	}
	turnNode := window.Items[1]
	if windowRepresentative(turnNode) != assistant.ID {
		t.Fatalf("expected assistant to remain representative, got %s", windowRepresentative(turnNode))
	}
	if turnNode.Metadata.Role != "assistant" || windowItemText(turnNode) != "" {
		t.Fatalf("expected empty assistant for ledger-only result, got role=%q content=%q", turnNode.Metadata.Role, windowItemText(turnNode))
	}
	if len(turnNode.Segments) != 1 ||
		len(turnNode.Segments[0].Invocations) != 1 ||
		turnNode.Segments[0].Invocations[0].ResultAvailability != "available" {
		t.Fatalf("expected lightweight ledger summary, got %+v", turnNode.Segments)
	}
	if window.StartIndex+1 != 1 {
		t.Fatalf("expected canonical originalIndex=1 for turn, got %v", window.StartIndex+1)
	}
}

func TestGetMessageChildrenUsesParentConversationForScope(t *testing.T) {
	setupMessageWindowAppTestDB(t)
	ctrl := newMessageWindowTestReader()

	conv := createMessageWindowTestConversation(t, "Conversa")
	ctx := database.WithUserID(context.Background(), messageWindowTestUserID)
	root, err := database.AddMessageWithContext(ctx, conv.ID, "assistant", "root")
	if err != nil {
		t.Fatalf("create root: %v", err)
	}
	child, err := database.AddChildMessageWithContext(ctx, conv.ID, root.ID, "assistant", "child", "")
	if err != nil {
		t.Fatalf("create child: %v", err)
	}

	snapshot, err := ctrl.Read(messageWindowTestCtx(), database.MessageWindowQuery{ConversationID: conv.ID, ParentID: &root.ID, Limit: 20})
	var nodes []chat.CanonicalItem
	if snapshot != nil {
		nodes = snapshot.Items
	}
	if err != nil {
		t.Fatalf("GetMessageChildren: %v", err)
	}
	if len(nodes) != 1 || windowRepresentative(nodes[0]) != child.ID {
		t.Fatalf("children: got %+v, want %s", nodes, child.ID)
	}
}

func TestGetMessageChildrenRejectsOtherUsersParent(t *testing.T) {
	setupMessageWindowAppTestDB(t)
	ctrl := newMessageWindowTestReader()

	otherCtx := database.WithUserID(context.Background(), "other-user")
	otherConv, err := database.CreateConversationWithContext(otherCtx, "Outra", "")
	if err != nil {
		t.Fatalf("create other conversation: %v", err)
	}
	root, err := database.AddMessageWithContext(otherCtx, otherConv.ID, "assistant", "root")
	if err != nil {
		t.Fatalf("create root: %v", err)
	}

	_, err = ctrl.Read(messageWindowTestCtx(), database.MessageWindowQuery{ConversationID: otherConv.ID, ParentID: &root.ID, Limit: 20})
	if err == nil {
		t.Fatal("expected cross-user message children to be rejected")
	}
}

func TestGetRecentMessages_HonorsLimitWithCanonicalTurns(t *testing.T) {
	setupMessageWindowAppTestDB(t)
	ctrl := newMessageWindowTestReader()

	conv := createMessageWindowTestConversation(t, "Conversa")
	ctx := database.WithUserID(context.Background(), messageWindowTestUserID)
	base := time.Date(2026, 5, 16, 10, 0, 0, 0, time.UTC)

	setTime := func(id string, at time.Time) {
		t.Helper()
		if err := database.DB().WithContext(ctx).Model(&database.ChatMessage{}).Where("id = ?", id).Update("created_at", at).Error; err != nil {
			t.Fatalf("set created_at %s: %v", id, err)
		}
	}

	// 4 turns; cada turno gera 2 itens de timeline (user + consolidated turn).
	for turn := 1; turn <= 4; turn++ {
		turnBase := base.Add(time.Duration(turn) * 10 * time.Second)
		userMsg, err := database.AddMessageWithContext(ctx, conv.ID, "user", "u"+string(rune('0'+turn)))
		if err != nil {
			t.Fatalf("create user %d: %v", turn, err)
		}
		setTime(userMsg.ID, turnBase)

		turnID := userMsg.ID
		assistant := addMessageWindowAssistant(t, ctx, conv.ID, turnID, "a"+string(rune('0'+turn)))
		setTime(assistant.ID, turnBase.Add(1*time.Second))
	}

	snapshot, err := ctrl.Read(messageWindowTestCtx(), database.MessageWindowQuery{ConversationID: conv.ID, Anchor: "end", Direction: "before", Limit: 6})
	var nodes []chat.CanonicalItem
	if snapshot != nil {
		nodes = snapshot.Items
	}
	if err != nil {
		t.Fatalf("GetRecentMessages: %v", err)
	}
	if len(nodes) != 6 {
		t.Fatalf("expected 6 nodes, got %d", len(nodes))
	}
	got := make([]string, 0, len(nodes))
	for _, n := range nodes {
		got = append(got, windowItemText(n))
	}
	// Espera os últimos 3 turns (u2/a2, u3/a3, u4/a4) em ordem cronológica.
	want := []string{"u2", "a2", "u3", "a3", "u4", "a4"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected nodes content: got=%v want=%v", got, want)
	}
}

func TestGetMessagesBefore_TrimsFromEndWithCanonicalTurns(t *testing.T) {
	setupMessageWindowAppTestDB(t)
	ctrl := newMessageWindowTestReader()

	conv := createMessageWindowTestConversation(t, "Conversa")
	ctx := database.WithUserID(context.Background(), messageWindowTestUserID)
	base := time.Date(2026, 5, 16, 10, 0, 0, 0, time.UTC)

	setTime := func(id string, at time.Time) {
		t.Helper()
		if err := database.DB().WithContext(ctx).Model(&database.ChatMessage{}).Where("id = ?", id).Update("created_at", at).Error; err != nil {
			t.Fatalf("set created_at %s: %v", id, err)
		}
	}

	var turn4UserID string
	for turn := 1; turn <= 4; turn++ {
		turnBase := base.Add(time.Duration(turn) * 10 * time.Second)
		userMsg, err := database.AddMessageWithContext(ctx, conv.ID, "user", "u"+string(rune('0'+turn)))
		if err != nil {
			t.Fatalf("create user %d: %v", turn, err)
		}
		setTime(userMsg.ID, turnBase)
		if turn == 4 {
			turn4UserID = userMsg.ID
		}

		turnID := userMsg.ID
		assistant := addMessageWindowAssistant(t, ctx, conv.ID, turnID, "a"+string(rune('0'+turn)))
		setTime(assistant.ID, turnBase.Add(1*time.Second))
	}
	if turn4UserID == "" {
		t.Fatal("missing turn4 user id")
	}

	// Pagina para trás a partir do início do turno 4; espera os últimos 2 turns completos antes dele.
	snapshot, err := ctrl.Read(messageWindowTestCtx(), database.MessageWindowQuery{ConversationID: conv.ID, AnchorMessageID: turn4UserID, Direction: "before", Limit: 4})
	var nodes []chat.CanonicalItem
	if snapshot != nil {
		nodes = snapshot.Items
	}
	if err != nil {
		t.Fatalf("GetMessagesBefore: %v", err)
	}
	if len(nodes) != 4 {
		t.Fatalf("expected 4 nodes, got %d", len(nodes))
	}
	got := make([]string, 0, len(nodes))
	for _, n := range nodes {
		got = append(got, windowItemText(n))
	}
	// Espera u2/a2, u3/a3 em ordem cronológica.
	want := []string{"u2", "a2", "u3", "a3"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected nodes content: got=%v want=%v", got, want)
	}
}
