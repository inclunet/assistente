package portability

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"assistente/internal/database"
)

func executionExportID(s string) *string { return &s }

func executionHistoryFixture() ConversationExport {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	return ConversationExport{ID: "execution-export", Title: "Execuções", Messages: []MessageExport{
		{ID: "question", Role: "user", Content: "pergunta", CreatedAt: now},
		{ID: "old-response", Role: "assistant", TurnID: "question", Content: "anterior", ChatExecutionID: executionExportID("old"), CreatedAt: now.Add(time.Second)},
		{ID: "new-response", Role: "assistant", TurnID: "question", Content: "atual", ChatExecutionID: executionExportID("new"), CreatedAt: now.Add(2 * time.Second)},
	}, ChatTurns: []ChatTurnExport{{TurnID: "question", CurrentExecutionID: executionExportID("new"), ResponseMessageID: executionExportID("new-response"), State: "completed"}},
		ChatExecutions: []ChatExecutionExport{
			{ExecutionID: "old", TurnID: "question", Mode: "new", State: "completed", StartedAt: now, FinishedAt: &now, TerminalReason: "completed", ResponseMessageID: executionExportID("old-response")},
			{ExecutionID: "new", TurnID: "question", Mode: "retry", State: "completed", StartedAt: now, FinishedAt: &now, TerminalReason: "completed", ResponseMessageID: executionExportID("new-response")},
		}, ToolInvocations: []ToolInvocationExport{
			{ID: "old-tool", TurnID: "question", ChatExecutionID: executionExportID("old"), ToolCallID: "same-call", ToolName: "example", Output: "old tool", Status: "succeeded"},
			{ID: "new-tool", TurnID: "question", ChatExecutionID: executionExportID("new"), ToolCallID: "same-call", ToolName: "example", Output: "new tool", Status: "succeeded"},
		}}
}

func TestExecutionHistoryRoundTripPreservesSelectionAndAudit(t *testing.T) {
	setupPortabilityRuntimeTestDB(t)
	ctx := portabilityTestCtx()
	fixture := executionHistoryFixture()
	if _, err := importConversation(ctx, fixture, false); err != nil {
		t.Fatal(err)
	}
	for round := 0; round < 2; round++ {
		visible, err := database.GetMessagesWithContext(ctx, fixture.ID, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(visible) != 2 || visible[1].ID != "new-response" {
			t.Fatalf("seleção perdida: %+v", visible)
		}
		exports, err := buildConversationExports(ctx, []string{fixture.ID}, false)
		if err != nil {
			t.Fatal(err)
		}
		got := exports[0]
		if len(got.Messages) != 3 || len(got.ChatExecutions) != 2 || len(got.ToolInvocations) != 2 || len(got.ChatTurns) != 1 || !sameExportID(got.ChatTurns[0].CurrentExecutionID, executionExportID("new")) {
			t.Fatalf("histórico perdido: %+v", got)
		}
		for _, m := range got.Messages {
			if m.Role == "assistant" && m.ChatExecutionID == nil {
				t.Fatal("proveniência da mensagem perdida")
			}
		}
		for _, i := range got.ToolInvocations {
			if i.ChatExecutionID == nil {
				t.Fatal("proveniência da ferramenta perdida")
			}
		}
		raw, err := json.Marshal(got)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "lease") || strings.Contains(string(raw), "activeExecution") {
			t.Fatal("runtime exportado")
		}
		if _, err := importConversation(ctx, got, false); err != nil {
			t.Fatal(err)
		}
	}
}

func TestExecutionHistoryContinuationAndActiveImport(t *testing.T) {
	for _, unavailable := range []bool{false, true} {
		t.Run(map[bool]string{false: "base", true: "base indisponível"}[unavailable], func(t *testing.T) {
			setupPortabilityRuntimeTestDB(t)
			fixture := executionHistoryFixture()
			fixture.ChatExecutions[1].Mode = "continue"
			fixture.ChatExecutions[1].BaseExecutionID = executionExportID("old")
			if !unavailable {
				fixture.ChatExecutions[1].BaseResponseMessageID = executionExportID("old-response")
			}
			fixture.ChatExecutions[1].State = "streaming"
			fixture.ChatExecutions[1].FinishedAt = nil
			fixture.ChatExecutions[1].TerminalReason = ""
			fixture.ChatTurns[0].State = "streaming"
			fixture.ChatExecutions[0], fixture.ChatExecutions[1] = fixture.ChatExecutions[1], fixture.ChatExecutions[0]
			if _, err := importConversation(portabilityTestCtx(), fixture, false); err != nil {
				t.Fatal(err)
			}
			var row struct {
				State              string
				ActiveExecutionID  *string
				CurrentExecutionID *string
			}
			if err := database.DB().Table("chat_turns").Where("user_message_id=?", "question").Take(&row).Error; err != nil {
				t.Fatal(err)
			}
			if row.State != "interrupted" || row.ActiveExecutionID != nil || !sameExportID(row.CurrentExecutionID, executionExportID("new")) {
				t.Fatalf("runtime restaurado: %+v", row)
			}
			var attempt struct {
				TerminalReason string
				LeasePath      string
				FinishedAt     *time.Time
			}
			if err := database.DB().Table("chat_turn_attempts").Where("execution_id=?", "new").Take(&attempt).Error; err != nil {
				t.Fatal(err)
			}
			if attempt.TerminalReason != "restart" || attempt.LeasePath != "" || attempt.FinishedAt == nil {
				t.Fatalf("tentativa ativa: %+v", attempt)
			}
		})
	}
}

func TestExecutionHistoryRejectsInvalidGraphAndRollsBack(t *testing.T) {
	cases := map[string]func(*ConversationExport){
		"execução ausente": func(c *ConversationExport) { c.Messages[2].ChatExecutionID = executionExportID("missing") },
		"seleção ausente":  func(c *ConversationExport) { c.ChatTurns[0].CurrentExecutionID = nil },
		"resposta alheia":  func(c *ConversationExport) { c.ChatExecutions[1].ResponseMessageID = executionExportID("old-response") },
		"base ausente": func(c *ConversationExport) {
			c.ChatExecutions[1].Mode = "continue"
			c.ChatExecutions[1].BaseExecutionID = executionExportID("missing")
		},
		"ciclo": func(c *ConversationExport) {
			c.ChatExecutions[0].Mode = "continue"
			c.ChatExecutions[0].BaseExecutionID = executionExportID("new")
			c.ChatExecutions[1].Mode = "continue"
			c.ChatExecutions[1].BaseExecutionID = executionExportID("old")
		},
		"duplicada":         func(c *ConversationExport) { c.ChatExecutions = append(c.ChatExecutions, c.ChatExecutions[0]) },
		"estado inválido":   func(c *ConversationExport) { c.ChatExecutions[0].TerminalReason = "bogus" },
		"ferramenta alheia": func(c *ConversationExport) { c.ToolInvocations[0].TurnID = "wrong" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			setupPortabilityRuntimeTestDB(t)
			fixture := executionHistoryFixture()
			if _, err := importConversation(portabilityTestCtx(), fixture, false); err != nil {
				t.Fatal(err)
			}
			mutate(&fixture)
			if _, err := importConversation(portabilityTestCtx(), fixture, false); err == nil {
				t.Fatal("grafo inválido aceito")
			}
			var count int64
			if err := database.DB().Table("chat_messages").Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if count != 3 {
				t.Fatalf("overwrite parcial: %d", count)
			}
			visible, err := database.GetMessagesWithContext(portabilityTestCtx(), fixture.ID, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(visible) != 2 || visible[1].ID != "new-response" {
				t.Fatal("seleção alterada após rollback")
			}
		})
	}
}

func TestExecutionHistoryRequiresVersionThree(t *testing.T) {
	for _, version := range []int{1, 2} {
		file := ExportFile{Version: version, Resources: ExportResources{Conversations: []ConversationExport{executionHistoryFixture()}}}
		if err := normalizeExportVersion(&file); err == nil {
			t.Fatalf("v%d aceitou proveniência", version)
		}
		file.Resources.Conversations = nil
		if err := normalizeExportVersion(&file); err != nil {
			t.Fatal(err)
		}
	}
}

func TestExecutionHistoryRichExportKeepsToolsInTheirExecution(t *testing.T) {
	fixture := executionHistoryFixture()
	// An old/foreign display anchor must not attach the new tool to an old response.
	fixture.ToolInvocations[1].Metadata = `{"display":{"assistant_message_id":"old-response"}}`
	file := &ExportFile{Resources: ExportResources{Conversations: []ConversationExport{fixture}}}
	rendered := richRenderExportFile(file).Resources.Conversations[0]
	if !strings.Contains(rendered.Messages[1].ToolInvocationDetails, "old tool") || strings.Contains(rendered.Messages[1].ToolInvocationDetails, "new tool") || !strings.Contains(rendered.Messages[2].ToolInvocationDetails, "new tool") {
		t.Fatalf("execuções misturadas: %+v", rendered.Messages)
	}
	if file.Resources.Conversations[0].Messages[1].ToolInvocationDetails != "" {
		t.Fatal("export canônico alterado pelo renderer")
	}
}

func TestExecutionHistoryLegacyContinuationPreservesUnknownProvenance(t *testing.T) {
	setupPortabilityRuntimeTestDB(t)
	c := executionHistoryFixture()
	c.Messages[1].ChatExecutionID = nil
	c.ToolInvocations[0].ChatExecutionID = nil
	c.ChatExecutions = c.ChatExecutions[1:]
	c.ChatExecutions[0].Mode = "continue"
	c.ChatExecutions[0].BaseResponseMessageID = executionExportID("old-response")
	if _, err := importConversation(portabilityTestCtx(), c, false); err != nil {
		t.Fatal(err)
	}
	exports, err := buildConversationExports(portabilityTestCtx(), []string{c.ID}, false)
	if err != nil {
		t.Fatal(err)
	}
	got := exports[0]
	if got.Messages[1].ChatExecutionID != nil || got.ChatExecutions[0].BaseExecutionID != nil || !sameExportID(got.ChatExecutions[0].BaseResponseMessageID, executionExportID("old-response")) {
		t.Fatalf("proveniência inventada: %+v", got)
	}
	if _, err := importConversation(portabilityTestCtx(), got, false); err != nil {
		t.Fatal(err)
	}
}

func TestExecutionHistoryCollisionDoesNotOverwriteOtherConversation(t *testing.T) {
	setupPortabilityRuntimeTestDB(t)
	c := executionHistoryFixture()
	if _, err := importConversation(portabilityTestCtx(), c, false); err != nil {
		t.Fatal(err)
	}
	c.ID = "another-conversation"
	for i := range c.Messages {
		c.Messages[i].ID = "other-" + c.Messages[i].ID
		if c.Messages[i].TurnID != "" {
			c.Messages[i].TurnID = "other-" + c.Messages[i].TurnID
		}
	}
	for i := range c.ChatExecutions {
		c.ChatExecutions[i].TurnID = "other-question"
		c.ChatExecutions[i].ResponseMessageID = executionExportID("other-" + *c.ChatExecutions[i].ResponseMessageID)
	}
	c.ChatTurns[0].TurnID = "other-question"
	c.ChatTurns[0].ResponseMessageID = executionExportID("other-new-response")
	c.ToolInvocations = nil
	if _, err := importConversation(portabilityTestCtx(), c, false); err == nil {
		t.Fatal("IDs de execução colidentes aceitos")
	}
	var count int64
	if err := database.DB().Table("conversations").Where("id=?", c.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("conversa parcialmente criada")
	}
	var executions int64
	if err := database.DB().Table("chat_turn_attempts").Count(&executions).Error; err != nil {
		t.Fatal(err)
	}
	if executions != 2 {
		t.Fatal("execuções originais alteradas")
	}
}

func TestExecutionHistoryPreservesOpaqueIDs(t *testing.T) {
	setupPortabilityRuntimeTestDB(t)
	c := executionHistoryFixture()
	c.ID = " opaque conversation "
	c.Messages[0].ID = " question "
	c.Messages[1].ID = "answer"
	c.Messages[2].ID = " answer "
	c.Messages[1].TurnID = " question "
	c.Messages[2].TurnID = " question "
	c.Messages[2].ChatExecutionID = executionExportID(" old ")
	c.ChatTurns[0].TurnID = " question "
	c.ChatTurns[0].CurrentExecutionID = executionExportID(" old ")
	c.ChatTurns[0].ResponseMessageID = executionExportID(" answer ")
	for i := range c.ChatExecutions {
		c.ChatExecutions[i].TurnID = " question "
		c.ChatExecutions[i].ResponseMessageID = executionExportID(c.Messages[i+1].ID)
	}
	c.ChatExecutions[1].ExecutionID = " old "
	c.ChatExecutions[1].Mode = "continue"
	c.ChatExecutions[1].BaseExecutionID = executionExportID("old")
	c.ChatExecutions[1].BaseResponseMessageID = executionExportID("answer")
	for i := range c.ToolInvocations {
		c.ToolInvocations[i].TurnID = " question "
		c.ToolInvocations[i].ID = " " + c.ToolInvocations[i].ID + " "
		c.ToolInvocations[i].ToolCallID = " call "
		c.ToolInvocations[i].ChatExecutionID = c.Messages[i+1].ChatExecutionID
	}
	for round := 0; round < 2; round++ {
		if _, err := importConversation(portabilityTestCtx(), c, false); err != nil {
			t.Fatal(err)
		}
		exports, err := buildConversationExports(portabilityTestCtx(), []string{c.ID}, false)
		if err != nil {
			t.Fatal(err)
		}
		c = exports[0]
		if c.ID != " opaque conversation " || c.Messages[0].ID != " question " || c.Messages[1].ID != "answer" || c.Messages[2].ID != " answer " || c.ChatTurns[0].TurnID != " question " || *c.ChatTurns[0].CurrentExecutionID != " old " {
			t.Fatalf("IDs normalizados: %+v", c)
		}
		for _, i := range c.ToolInvocations {
			if i.ToolCallID != " call " || i.TurnID != " question " || !strings.HasPrefix(i.ID, " ") {
				t.Fatalf("ledger normalizado: %+v", i)
			}
		}
	}
}
