package chat

import (
	"context"
	"encoding/json"
	"testing"

	"assistente/internal/database"
	"assistente/internal/llm"
)

// stubRepo é um MessageRepository mínimo para testes de MediaHistoryLoader.
type stubRepo struct {
	messages []database.ChatMessage
	summary  string
	sumUpTo  string
}

func (r *stubRepo) GetMessages(_ context.Context, _ string, _ *string) ([]database.ChatMessage, error) {
	return r.messages, nil
}

func (r *stubRepo) GetMessagesByTurnID(_ context.Context, _ string, _ *string, _ string, _ int) ([]database.ChatMessage, error) {
	return r.messages, nil
}
func (r *stubRepo) GetConversationSummary(_ context.Context, _ string) (string, string, error) {
	return r.summary, r.sumUpTo, nil
}
func (r *stubRepo) CreateMessage(_ context.Context, _ database.MessageOptions) (*database.ChatMessage, error) {
	return nil, nil
}

func (r *stubRepo) UpdateMessageContentAndReasoning(_ context.Context, _ string, _ string, _ string, _, _, _ int, _ string) error {
	return nil
}
func (r *stubRepo) GetMessage(_ context.Context, messageID string) (*database.ChatMessage, error) {
	for i := range r.messages {
		if r.messages[i].ID == messageID {
			msg := r.messages[i]
			return &msg, nil
		}
	}
	return nil, nil
}
func (r *stubRepo) GetDetailedTokenStats(_ context.Context, _ string, _ string) (*database.DetailedTokenStats, error) {
	return nil, nil
}
func (r *stubRepo) GetContextWindowUsage(_ context.Context, _ string, _ int) (float64, int, error) {
	return 0, 0, nil
}
func (r *stubRepo) GetRecentMessagesTokenCount(_ context.Context, _ string, _ int) (int, error) {
	return 0, nil
}
func (r *stubRepo) GetTurnTokenStats(_ context.Context, _ string, _ string) (*database.TokenStats, error) {
	return nil, nil
}
func (r *stubRepo) AddAssistantToolMessage(_ context.Context, _ string, _ string, _, _, _, _ string) (*database.ChatMessage, error) {
	return nil, nil
}
func (r *stubRepo) AddToolResultMessage(_ context.Context, _ string, _ string, _, _ string) (*database.ChatMessage, error) {
	return nil, nil
}
func (r *stubRepo) SearchMessages(_ context.Context, _ string, _ int) ([]database.MessageSearchResult, error) {
	return nil, nil
}

func mediaJSON(parts []map[string]interface{}) string {
	b, _ := json.Marshal(parts)
	return string(b)
}

func boolPtr(v bool) *bool { return &v }

// ─── WhisperFilename ──────────────────────────────────────────────────────────

func TestWhisperFilename(t *testing.T) {
	cases := []struct{ input, want string }{
		{"aac", "audio.m4a"},
		{"opus", "audio.ogg"},
		{"mp3", "audio.mp3"},
		{"wav", "audio.wav"},
		{"webm", "audio.webm"},
	}
	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			if got := WhisperFilename(c.input); got != c.want {
				t.Errorf("WhisperFilename(%q) = %q, want %q", c.input, got, c.want)
			}
		})
	}
}

// ─── MediaHistoryLoader media conversion ─────────────────────────────────────

func TestMediaHistoryLoader_PlainText(t *testing.T) {
	repo := &stubRepo{messages: []database.ChatMessage{{Role: "user", Content: "olá"}}}
	msgs, _, err := (&MediaHistoryLoader{Repo: repo, MaxMsgs: 100}).Load(context.Background(), "1")
	if err != nil {
		t.Fatal(err)
	}
	if msgs[0].Content != "olá" {
		t.Errorf("content = %v, want \"olá\"", msgs[0].Content)
	}
}

func TestMediaHistoryLoader_NaoConverteReasoningPersistidoEmExtensaoDeProtocolo(t *testing.T) {
	repo := &stubRepo{messages: []database.ChatMessage{
		{Role: "user", Content: "consulte os dados"},
		{Role: "assistant", Content: "resposta", Reasoning: "thinking genérico", Model: "qualquer-modelo"},
	}}
	msgs, _, err := (&MediaHistoryLoader{Repo: repo, MaxMsgs: 100}).Load(context.Background(), "1")
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("len(messages) = %d, want 2", len(msgs))
	}
	if msgs[1].ReasoningContent != "" {
		t.Fatalf("reasoning_content = %q; histórico persistido não conhece a capability do protocolo", msgs[1].ReasoningContent)
	}
}

func TestMediaHistoryLoader_NaoConverteReasoningDeToolCallLegada(t *testing.T) {
	repo := &stubRepo{messages: []database.ChatMessage{
		{Role: "user", Content: "consulte os dados"},
		{
			Role:      "assistant",
			Content:   "resposta",
			Reasoning: "preciso usar lookup",
			ToolCalls: `{"id":"call-1","type":"function","function":{"name":"lookup","arguments":"{}"},"result":"ok"}`,
		},
	}}
	msgs, _, err := (&MediaHistoryLoader{Repo: repo, MaxMsgs: 100}).Load(context.Background(), "1")
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("len(messages) = %d, want 2", len(msgs))
	}
	if msgs[1].ReasoningContent != "" {
		t.Fatalf("reasoning_content legado = %q; replay só existe no agentic loop corrente", msgs[1].ReasoningContent)
	}
}

func TestMediaHistoryLoader_DescartaToolCallLegadaSoComReasoning(t *testing.T) {
	repo := &stubRepo{messages: []database.ChatMessage{
		{Role: "user", Content: "consulte os dados"},
		{
			Role:      "assistant",
			Reasoning: "preciso usar lookup",
			ToolCalls: `{"id":"call-1","type":"function","function":{"name":"lookup","arguments":"{}"},"result":"ok"}`,
		},
	}}
	msgs, _, err := (&MediaHistoryLoader{Repo: repo, MaxMsgs: 100}).Load(context.Background(), "1")
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("len(messages) = %d, want 1; assistant vazia não pode chegar ao provider", len(msgs))
	}
}

func TestMediaHistoryLoader_Image(t *testing.T) {
	media := mediaJSON([]map[string]interface{}{
		{"type": "image/png", "data": "abc123", "name": "foto.png"},
	})
	repo := &stubRepo{messages: []database.ChatMessage{{Role: "user", Media: media}}}
	msgs, _, err := (&MediaHistoryLoader{Repo: repo, MaxMsgs: 100}).Load(context.Background(), "1")
	if err != nil {
		t.Fatal(err)
	}
	parts := msgs[0].Content.([]interface{})
	m := parts[0].(map[string]interface{})
	if m["type"] != "image_url" {
		t.Errorf("type = %v, want image_url", m["type"])
	}
}

func TestMediaHistoryLoader_AudioSupported(t *testing.T) {
	media := mediaJSON([]map[string]interface{}{
		{"type": "audio/wav", "data": "wavdata"},
	})
	repo := &stubRepo{messages: []database.ChatMessage{{Role: "user", Media: media}}}
	msgs, _, err := (&MediaHistoryLoader{Repo: repo, MaxMsgs: 100}).Load(context.Background(), "1")
	if err != nil {
		t.Fatal(err)
	}
	parts := msgs[0].Content.([]interface{})
	m := parts[0].(map[string]interface{})
	if m["type"] != "input_audio" {
		t.Errorf("type = %v, want input_audio", m["type"])
	}
}

func TestMediaHistoryLoader_AudioUnsupported_Transcribed(t *testing.T) {
	media := mediaJSON([]map[string]interface{}{
		{"type": "audio/aac", "data": "aacdata"},
	})
	repo := &stubRepo{messages: []database.ChatMessage{{Role: "user", Media: media}}}
	loader := &MediaHistoryLoader{
		Repo: repo,
		Transcribe: func(_ context.Context, audio, _ string) (string, error) {
			return "transcrição do áudio", nil
		},
		MaxMsgs: 100,
	}
	msgs, _, err := loader.Load(context.Background(), "1")
	if err != nil {
		t.Fatal(err)
	}
	parts := msgs[0].Content.([]interface{})
	m := parts[0].(map[string]interface{})
	if m["type"] != "text" || m["text"] != "transcrição do áudio" {
		t.Errorf("got type=%v text=%v", m["type"], m["text"])
	}
}

func TestMediaHistoryLoader_AudioUnsupported_NoTranscribe_Placeholder(t *testing.T) {
	media := mediaJSON([]map[string]interface{}{
		{"type": "audio/webm", "data": "webmdata"},
	})
	repo := &stubRepo{messages: []database.ChatMessage{{Role: "user", Media: media}}}
	msgs, _, err := (&MediaHistoryLoader{Repo: repo, Transcribe: nil, MaxMsgs: 100}).Load(context.Background(), "1")
	if err != nil {
		t.Fatal(err)
	}
	parts := msgs[0].Content.([]interface{})
	m := parts[0].(map[string]interface{})
	if m["type"] != "text" {
		t.Errorf("type = %v, want text (placeholder)", m["type"])
	}
	if m["text"] == "" {
		t.Error("placeholder text must not be empty")
	}
}

func TestMediaHistoryLoader_AudioSkippedWhenHasTextContent(t *testing.T) {
	// Quando Content != "" (transcrição já existe), parte de áudio deve ser pulada
	media := mediaJSON([]map[string]interface{}{
		{"type": "audio/wav", "data": "wavdata"},
	})
	repo := &stubRepo{messages: []database.ChatMessage{
		{Role: "user", Content: "transcrição anterior", Media: media},
	}}
	msgs, _, err := (&MediaHistoryLoader{Repo: repo, MaxMsgs: 100}).Load(context.Background(), "1")
	if err != nil {
		t.Fatal(err)
	}
	parts := msgs[0].Content.([]interface{})
	if len(parts) != 1 {
		t.Fatalf("expected 1 part (text only), got %d", len(parts))
	}
	if parts[0].(map[string]interface{})["type"] != "text" {
		t.Error("expected text part, not audio")
	}
}

func TestMediaHistoryLoader_Document(t *testing.T) {
	media := mediaJSON([]map[string]interface{}{
		{"type": "application/pdf", "data": "pdfdata", "name": "doc.pdf"},
	})
	repo := &stubRepo{messages: []database.ChatMessage{{Role: "user", Media: media}}}
	msgs, _, err := (&MediaHistoryLoader{Repo: repo, MaxMsgs: 100}).Load(context.Background(), "1")
	if err != nil {
		t.Fatal(err)
	}
	parts := msgs[0].Content.([]interface{})
	if parts[0].(map[string]interface{})["type"] != "file" {
		t.Errorf("expected file, got %v", parts[0].(map[string]interface{})["type"])
	}
}

func TestMediaHistoryLoader_Video(t *testing.T) {
	media := mediaJSON([]map[string]interface{}{
		{"type": "video/mp4", "data": "videodata", "name": "vid.mp4"},
	})
	repo := &stubRepo{messages: []database.ChatMessage{{Role: "user", Media: media}}}
	msgs, _, err := (&MediaHistoryLoader{Repo: repo, MaxMsgs: 100}).Load(context.Background(), "1")
	if err != nil {
		t.Fatal(err)
	}
	parts := msgs[0].Content.([]interface{})
	if parts[0].(map[string]interface{})["type"] != "video" {
		t.Errorf("expected video, got %v", parts[0].(map[string]interface{})["type"])
	}
}

func TestMediaHistoryLoader_UnknownType_Placeholder(t *testing.T) {
	media := mediaJSON([]map[string]interface{}{
		{"type": "application/zip", "data": "zipdata", "name": "file.zip"},
	})
	repo := &stubRepo{messages: []database.ChatMessage{{Role: "user", Media: media}}}
	msgs, _, err := (&MediaHistoryLoader{Repo: repo, MaxMsgs: 100}).Load(context.Background(), "1")
	if err != nil {
		t.Fatal(err)
	}
	parts := msgs[0].Content.([]interface{})
	if parts[0].(map[string]interface{})["type"] != "text" {
		t.Errorf("expected text placeholder, got %v", parts[0].(map[string]interface{})["type"])
	}
}

func TestMediaHistoryLoader_ReturnsSummary(t *testing.T) {
	repo := &stubRepo{
		messages: []database.ChatMessage{{Role: "user", Content: "hi"}},
		summary:  "resumo anterior",
	}
	_, summary, err := (&MediaHistoryLoader{Repo: repo, MaxMsgs: 100}).Load(context.Background(), "1")
	if err != nil {
		t.Fatal(err)
	}
	if summary != "resumo anterior" {
		t.Errorf("summary = %q, want \"resumo anterior\"", summary)
	}
}

func TestMediaHistoryLoader_FiltersTool(t *testing.T) {
	repo := &stubRepo{messages: []database.ChatMessage{
		{Role: "user", Content: "oi"},
		{Role: "tool", Content: "result"},
		{Role: "assistant", Content: "tudo bem"},
	}}
	msgs, _, err := (&MediaHistoryLoader{Repo: repo, MaxMsgs: 100}).Load(context.Background(), "1")
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2 (tool must be filtered)", len(msgs))
	}
	for _, m := range msgs {
		if m.Role == "tool" {
			t.Error("tool message must be filtered out")
		}
	}
}

func TestMediaHistoryLoader_FiltersEmptyAssistantToolCall(t *testing.T) {
	// Para que ToolCalls seja preservado pelo HistoryLoader, o tool call deve
	// estar respondido (não-órfão). O MediaHistoryLoader deve então filtrar
	// o assistant com content="" + tool_calls não-vazio.
	repo := &stubRepo{messages: []database.ChatMessage{
		{Role: "user", Content: "oi"},
		{UUIDModel: database.UUIDModel{ID: "2"}, Role: "assistant", Content: "", ToolCalls: `[{"id":"x"}]`},
		{Role: "tool", Content: "result", ToolCallID: "x"},
		{Role: "assistant", Content: "resposta final"},
	}}
	msgs, _, err := (&MediaHistoryLoader{Repo: repo, MaxMsgs: 100}).Load(context.Background(), "1")
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("got %d, want 2 (empty assistant+tool_calls + tool msg must both be filtered)", len(msgs))
	}
}

func TestMediaHistoryLoader_KeepsAssistantWithTextAndToolCalls(t *testing.T) {
	repo := &stubRepo{messages: []database.ChatMessage{
		{Role: "user", Content: "oi"},
		{Role: "assistant", Content: "Vou buscar...", ToolCalls: `[{"id":"x"}]`},
	}}
	msgs, _, err := (&MediaHistoryLoader{Repo: repo, MaxMsgs: 100}).Load(context.Background(), "1")
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("got %d, want 2 (assistant with text+tool_calls must be kept)", len(msgs))
	}
}

// ─── PreprocessMessages ───────────────────────────────────────────────────────

func audioMsg(format, data string) llm.Message {
	return llm.Message{
		Role: "user",
		Content: []interface{}{
			map[string]interface{}{
				"type": "input_audio",
				"input_audio": map[string]interface{}{
					"data": data, "format": format,
				},
			},
		},
	}
}

func fileMsg(filename, mime string) llm.Message {
	return llm.Message{
		Role: "user",
		Content: []interface{}{
			map[string]interface{}{
				"type": "file",
				"file": map[string]interface{}{
					"filename": filename, "mime_type": mime, "data": "abc",
				},
			},
		},
	}
}

func TestPreprocess_SupportedAudioPassthrough(t *testing.T) {
	result := PreprocessMessages(context.Background(), []llm.Message{audioMsg("wav", "d")}, nil, nil, nil)
	m := result[0].Content.([]interface{})[0].(map[string]interface{})
	if m["type"] != "input_audio" {
		t.Errorf("supported audio must pass through, got type=%v", m["type"])
	}
}

func TestPreprocess_UnsupportedAudio_Transcribed(t *testing.T) {
	called := false
	result := PreprocessMessages(context.Background(), []llm.Message{audioMsg("aac", "d")}, func(_ context.Context, _, _ string) (string, error) {
		called = true
		return "texto transcrito", nil
	}, nil, nil)
	if !called {
		t.Error("transcribe must be called for unsupported aac format")
	}
	m := result[0].Content.([]interface{})[0].(map[string]interface{})
	if m["type"] != "text" || m["text"] != "texto transcrito" {
		t.Errorf("expected text=texto transcrito, got type=%v text=%v", m["type"], m["text"])
	}
}

func TestPreprocess_UnsupportedAudio_NoTranscribe_Placeholder(t *testing.T) {
	result := PreprocessMessages(context.Background(), []llm.Message{audioMsg("ogg", "d")}, nil, nil, nil)
	m := result[0].Content.([]interface{})[0].(map[string]interface{})
	if m["type"] != "text" {
		t.Errorf("expected placeholder text, got type=%v", m["type"])
	}
}

func TestPreprocess_AudioSupportedFalse_ForcesWhisper(t *testing.T) {
	// Mesmo wav (suportado nativamente), audioSupported=false → Whisper obrigatório
	called := false
	result := PreprocessMessages(context.Background(), []llm.Message{audioMsg("wav", "d")}, func(_ context.Context, _, _ string) (string, error) {
		called = true
		return "whisper text", nil
	}, boolPtr(false), nil)
	if !called {
		t.Error("transcribe must be called when audioSupported=false")
	}
	m := result[0].Content.([]interface{})[0].(map[string]interface{})
	if m["type"] != "text" {
		t.Errorf("expected text after forced whisper, got type=%v", m["type"])
	}
}

func TestPreprocess_DocSupportedFalse_Placeholder(t *testing.T) {
	result := PreprocessMessages(context.Background(), []llm.Message{fileMsg("rel.pdf", "application/pdf")}, nil, nil, boolPtr(false))
	m := result[0].Content.([]interface{})[0].(map[string]interface{})
	if m["type"] != "text" || m["text"] == "" {
		t.Errorf("expected non-empty placeholder text, got type=%v text=%v", m["type"], m["text"])
	}
}

func TestPreprocess_DocSupportedTrue_Passthrough(t *testing.T) {
	result := PreprocessMessages(context.Background(), []llm.Message{fileMsg("rel.pdf", "application/pdf")}, nil, nil, boolPtr(true))
	m := result[0].Content.([]interface{})[0].(map[string]interface{})
	if m["type"] != "file" {
		t.Errorf("doc must pass through when docSupported=true, got type=%v", m["type"])
	}
}

func TestPreprocess_StringContentUnchanged(t *testing.T) {
	msgs := []llm.Message{{Role: "user", Content: "texto simples"}}
	result := PreprocessMessages(context.Background(), msgs, nil, nil, nil)
	if result[0].Content != "texto simples" {
		t.Errorf("string content must be unchanged, got %v", result[0].Content)
	}
}
