package llm

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"assistente/internal/credentials"
)

// countLogs conta records capturados em determinado nível cuja mensagem contém
// todos os substrings informados.
func countLogs(recs []slog.Record, level slog.Level, substrs ...string) int {
	n := 0
	for i := range recs {
		if recs[i].Level != level {
			continue
		}
		ok := true
		for _, s := range substrs {
			if !strings.Contains(recs[i].Message, s) {
				ok = false
				break
			}
		}
		if ok {
			n++
		}
	}
	return n
}

func streamResponsesForLog(t *testing.T, stream string) *logCaptureHandler {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(stream))
	}))
	defer server.Close()

	logHandler := &logCaptureHandler{}
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(logHandler))
	defer slog.SetDefault(oldLogger)

	provider := NewOpenAIResponsesProvider(&ProviderConfig{
		ID:           "responses-mcp-dedup",
		Name:         "Responses MCP Dedup",
		BaseURL:      server.URL + "/v1",
		APIFormat:    APIFormatOpenAIResponses,
		AuthMode:     AuthModeNone,
		DefaultModel: "gpt-test",
	}, credentials.NewManager(nil))

	handler := &mcpTrackingHandler{}
	provider.StreamChat(t.Context(), []Message{{Role: "user", Content: "oi"}},
		ChatParams{Model: "gpt-test"}, handler)
	return logHandler
}

// TestResponses_MCPFailure_ErroUnico_OrdemFailedAntesDoDone cobre a ordem
// observada no assistente.log: response.mcp_call.failed chega ANTES do
// response.output_item.done (que carrega o texto do erro). Antes da correção
// isso rendia dois ERROs por falha ("MCP call FAILED: itemID=..." e "MCP native
// call FAILED: ..."). Agora sai exatamente um ERRO, o detalhado, e o evento
// .failed vira Debug ("aguardando detalhe").
func TestResponses_MCPFailure_ErroUnico_OrdemFailedAntesDoDone(t *testing.T) {
	stream := "event: response.output_item.added\n" +
		`data: {"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"id":"mcp_1","type":"mcp_call","name":"addCommentToJiraIssue","server_label":"Atlassian"}}` + "\n\n" +
		"event: response.mcp_call.failed\n" +
		`data: {"type":"response.mcp_call.failed","sequence_number":2,"output_index":0,"item_id":"mcp_1"}` + "\n\n" +
		"event: response.output_item.done\n" +
		`data: {"type":"response.output_item.done","sequence_number":3,"output_index":0,"item":{"id":"mcp_1","type":"mcp_call","name":"addCommentToJiraIssue","server_label":"Atlassian","error":"comment body ausente"}}` + "\n\n" +
		"event: response.completed\n" +
		`data: {"type":"response.completed","sequence_number":4,"response":{"id":"resp_1","object":"response","created_at":1,"status":"completed","model":"gpt-test","output":[]}}` + "\n\n"

	recs := streamResponsesForLog(t, stream).records

	if got := countLogs(recs, slog.LevelError, "FAILED"); got != 1 {
		t.Fatalf("esperado exatamente 1 ERRO de falha MCP, obtido %d", got)
	}
	if got := countLogs(recs, slog.LevelError, "MCP native call FAILED", "comment body ausente"); got != 1 {
		t.Fatalf("o ERRO único deveria ser o detalhado (com o texto do erro), obtido %d", got)
	}
	if got := countLogs(recs, slog.LevelDebug, "aguardando detalhe", "mcp_1"); got != 1 {
		t.Fatalf("response.mcp_call.failed deveria virar Debug 'aguardando detalhe', obtido %d", got)
	}
}

// TestResponses_MCPFailure_ErroUnico_SemOutputItemDone cobre o proxy que só
// emite response.mcp_call.failed, sem response.output_item.done. O ERRO não sai
// mais no evento .failed (viraria Debug), mas o fallback de fim de stream
// garante exatamente um ERRO com servidor/tool — sem perder o sinal.
func TestResponses_MCPFailure_ErroUnico_SemOutputItemDone(t *testing.T) {
	stream := "event: response.output_item.added\n" +
		`data: {"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"id":"mcp_1","type":"mcp_call","name":"addCommentToJiraIssue","server_label":"Atlassian"}}` + "\n\n" +
		"event: response.mcp_call.failed\n" +
		`data: {"type":"response.mcp_call.failed","sequence_number":2,"output_index":0,"item_id":"mcp_1"}` + "\n\n" +
		"event: response.completed\n" +
		`data: {"type":"response.completed","sequence_number":3,"response":{"id":"resp_1","object":"response","created_at":1,"status":"completed","model":"gpt-test","output":[]}}` + "\n\n"

	recs := streamResponsesForLog(t, stream).records

	if got := countLogs(recs, slog.LevelError, "FAILED"); got != 1 {
		t.Fatalf("esperado exatamente 1 ERRO de falha MCP (fallback pós-stream), obtido %d", got)
	}
	if got := countLogs(recs, slog.LevelError, "MCP call FAILED", `server="Atlassian"`, `tool="addCommentToJiraIssue"`); got != 1 {
		t.Fatalf("o ERRO de fallback deveria conter servidor e tool, obtido %d", got)
	}
}
