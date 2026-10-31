package chat

import (
	"assistente/internal/database"
	"reflect"
	"testing"
	"time"
)

func TestTimelineExecutionGroupsKeepRepeatedIterationsDistinct(t *testing.T) {
	calls := []TurnSegmentToolCall{
		{ExecutionID: "current", ExecutionDepth: 0, Iteration: 0, ID: "same", InvocationID: "new"},
		{ExecutionID: "base", ExecutionDepth: 1, Iteration: 1, ID: "same", InvocationID: "old-1"},
		{ExecutionID: "base", ExecutionDepth: 1, Iteration: 0, ID: "same", InvocationID: "old-0"},
	}
	groups := groupCanonicalInvocations(calls)
	if len(groups) != 3 {
		t.Fatalf("iterações de execuções distintas misturadas: %+v", groups)
	}
	for i, want := range []string{"old-0", "old-1", "new"} {
		if len(groups[i].calls) != 1 || groups[i].calls[0].InvocationID != want {
			t.Fatalf("ordem não seguiu cadeia: %+v", groups)
		}
	}
}

func TestTimelineExecutionSelectionMatchesWindowAndTerminal(t *testing.T) {
	f := newSnapshotFixture(t)
	turns := database.NewChatTurnRepository(f.db)
	execution, err := turns.Begin(f.ctx, f.conversation, f.question, "", database.TurnModeRetry)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.db.Exec("UPDATE chat_turns SET current_execution_id=?,response_message_id=NULL WHERE user_message_id=?", execution.ExecutionID, f.question).Error; err != nil {
		t.Fatal(err)
	}
	answer := "replacement-answer"
	if err := f.db.Table("chat_messages").Create(map[string]any{"id": answer, "conversation_id": f.conversation, "turn_id": f.question, "role": "assistant", "content": "nova resposta", "chat_execution_id": execution.ExecutionID, "created_at": time.Now(), "updated_at": time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Exec("UPDATE chat_turn_attempts SET response_message_id=? WHERE execution_id=?", answer, execution.ExecutionID).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Exec("UPDATE chat_turns SET response_message_id=? WHERE user_message_id=?", answer, f.question).Error; err != nil {
		t.Fatal(err)
	}
	if err := turns.Finish(f.ctx, f.conversation, f.question, execution.ExecutionID, database.TurnCompleted, "completed"); err != nil {
		t.Fatal(err)
	}
	window := f.window(t)
	terminal, err := f.reader.ReadTurn(f.ctx, f.conversation, f.question)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(window.Items[1], terminal.Response) {
		t.Fatal("janela e terminal selecionaram respostas diferentes")
	}
	response := terminal.Response
	if !reflect.DeepEqual(response.MessageIDs, []string{answer}) || len(response.Segments) != 1 || response.Segments[0].Text != "nova resposta" {
		t.Fatalf("conteúdo antigo reapareceu: %+v", response)
	}
	var original database.ChatMessage
	if err := f.db.First(&original, "id=?", f.answer).Error; err != nil || original.Content != "resposta antiga" {
		t.Fatalf("execução anterior perdida: %+v %v", original, err)
	}
}
