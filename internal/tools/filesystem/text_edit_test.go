package filesystem

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"assistente/internal/questionnaire"
	"assistente/internal/tools/invocationctx"
)

// fakeQuestionnaireRequester simula o gerenciador de questionários para os testes.
type fakeQuestionnaireRequester struct {
	lastPayload questionnaire.RequestPayload
	response    questionnaire.Response
	err         error
	called      bool
}

func (f *fakeQuestionnaireRequester) RequestQuestionnaire(ctx context.Context, payload questionnaire.RequestPayload) (questionnaire.Response, error) {
	f.called = true
	f.lastPayload = payload
	return f.response, f.err
}

// editorCtx cria um contexto de invocação de aba de editor com o arquivo ativo.
func editorCtx(activeFilePath string) context.Context {
	return invocationctx.With(context.Background(), invocationctx.InvocationContext{
		TabType:        "editor",
		ActiveFilePath: activeFilePath,
	})
}

func TestTextEdit_Name(t *testing.T) {
	tool := NewTextEdit("/tmp", nil)
	if tool.Name() != "text_edit" {
		t.Errorf("expected 'text_edit', got '%s'", tool.Name())
	}
}

func TestTextEdit_BasicReplace(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "doc.md")
	_ = os.WriteFile(filePath, []byte("# Título\n\nParágrafo antigo aqui.\n"), 0644)

	quest := &fakeQuestionnaireRequester{}
	tool := NewTextEdit(dir, quest)
	args := `{"original": "Parágrafo antigo aqui.", "replacement": "Parágrafo novo aqui."}`
	result, err := tool.Execute(editorCtx(filePath), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if result.IsError {
		t.Fatalf("resultado é erro: %s", result.Content)
	}
	if !quest.called {
		t.Error("questionário de confirmação deveria ter sido exibido")
	}

	data, _ := os.ReadFile(filePath)
	if !containsString(string(data), "Parágrafo novo aqui.") {
		t.Error("substituição não foi aplicada")
	}
	if containsString(string(data), "Parágrafo antigo aqui.") {
		t.Error("texto antigo ainda presente")
	}
}

func TestTextEdit_QuestionnaireShowsBeforeAfter(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "doc.md")
	_ = os.WriteFile(filePath, []byte("antes"), 0644)

	quest := &fakeQuestionnaireRequester{}
	tool := NewTextEdit(dir, quest)
	args := `{"original": "antes", "replacement": "depois", "notes": "melhoria de clareza"}`
	result, err := tool.Execute(editorCtx(filePath), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if result.IsError {
		t.Fatalf("resultado é erro: %s", result.Content)
	}

	if len(quest.lastPayload.Questions) != 2 {
		t.Fatalf("questionário deveria ter 2 perguntas (Antes/Depois), got %d", len(quest.lastPayload.Questions))
	}
	if quest.lastPayload.Questions[0].Content != "antes" || quest.lastPayload.Questions[1].Content != "depois" {
		t.Errorf("conteúdo Antes/Depois incorreto: %#v", quest.lastPayload.Questions)
	}
	if quest.lastPayload.SubmitLabel != "Aplicar" || quest.lastPayload.CancelLabel != "Rejeitar" {
		t.Errorf("labels incorretos: submit=%q cancel=%q", quest.lastPayload.SubmitLabel, quest.lastPayload.CancelLabel)
	}
	if !containsString(quest.lastPayload.Description, "melhoria de clareza") {
		t.Errorf("notes deveria aparecer na descrição: %q", quest.lastPayload.Description)
	}
}

func TestTextEdit_RejectedByUser(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "doc.md")
	_ = os.WriteFile(filePath, []byte("conteúdo original"), 0644)

	quest := &fakeQuestionnaireRequester{response: questionnaire.Response{Cancelled: true}}
	tool := NewTextEdit(dir, quest)
	args := `{"original": "conteúdo original", "replacement": "conteúdo novo"}`
	result, err := tool.Execute(editorCtx(filePath), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if !result.IsError {
		t.Fatal("deve ser erro quando o usuário rejeita")
	}
	if !containsString(result.Content, "rejeitada") {
		t.Errorf("mensagem deveria indicar rejeição: %s", result.Content)
	}

	data, _ := os.ReadFile(filePath)
	if string(data) != "conteúdo original" {
		t.Error("arquivo não deveria ter sido modificado após rejeição")
	}
}

func TestTextEdit_OutsideEditor(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "doc.md")
	_ = os.WriteFile(filePath, []byte("texto"), 0644)

	tool := NewTextEdit(dir, nil)
	args := `{"original": "texto", "replacement": "novo texto"}`

	// Sem contexto de invocação
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if !result.IsError {
		t.Fatal("deve ser erro fora de aba de editor")
	}
	if !containsString(result.Content, "edit_file") {
		t.Errorf("mensagem deveria orientar a usar edit_file: %s", result.Content)
	}

	// Aba de chat (não-editor)
	chatCtx := invocationctx.With(context.Background(), invocationctx.InvocationContext{TabType: "chat"})
	result, err = tool.Execute(chatCtx, json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if !result.IsError {
		t.Fatal("deve ser erro em aba de chat")
	}

	// Aba de editor sem arquivo ativo
	noFileCtx := invocationctx.With(context.Background(), invocationctx.InvocationContext{TabType: "editor"})
	result, err = tool.Execute(noFileCtx, json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if !result.IsError {
		t.Fatal("deve ser erro sem arquivo ativo")
	}
}

func TestTextEdit_OriginalNotFound(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "doc.md")
	_ = os.WriteFile(filePath, []byte("algum conteúdo"), 0644)

	tool := NewTextEdit(dir, &fakeQuestionnaireRequester{})
	args := `{"original": "texto inexistente", "replacement": "novo"}`
	result, err := tool.Execute(editorCtx(filePath), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if !result.IsError {
		t.Fatal("deve ser erro quando 'original' não é encontrado")
	}
	if !containsString(result.Content, "não encontrado") {
		t.Errorf("mensagem deveria indicar não encontrado: %s", result.Content)
	}
}

func TestTextEdit_MultipleOccurrences(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "doc.md")
	_ = os.WriteFile(filePath, []byte("repetido\noutra linha\nrepetido"), 0644)

	quest := &fakeQuestionnaireRequester{}
	tool := NewTextEdit(dir, quest)
	args := `{"original": "repetido", "replacement": "único"}`
	result, err := tool.Execute(editorCtx(filePath), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if !result.IsError {
		t.Fatal("deve ser erro quando 'original' ocorre mais de uma vez")
	}
	if !containsString(result.Content, "2 vezes") {
		t.Errorf("deve informar quantas vezes foi encontrado: %s", result.Content)
	}
	if !containsString(result.Content, "mais contexto") {
		t.Errorf("deve instruir o modelo a incluir mais contexto: %s", result.Content)
	}
	if quest.called {
		t.Error("questionário não deveria ser exibido quando a ocorrência é ambígua")
	}
}

func TestTextEdit_EmptyOriginal(t *testing.T) {
	tool := NewTextEdit(t.TempDir(), nil)
	args := `{"original": "", "replacement": "novo"}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if !result.IsError {
		t.Error("deve ser erro com 'original' vazio")
	}
}

func TestTextEdit_IdenticalStrings(t *testing.T) {
	tool := NewTextEdit(t.TempDir(), nil)
	args := `{"original": "igual", "replacement": "igual"}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if !result.IsError {
		t.Error("deve ser erro quando original == replacement")
	}
}

func TestTextEdit_InvalidFormat(t *testing.T) {
	tool := NewTextEdit(t.TempDir(), nil)
	args := `{"original": "a", "replacement": "b", "format": "html"}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if !result.IsError {
		t.Error("deve ser erro com format inválido")
	}
}

func TestTextEdit_NotifiesWriteObserver(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "doc.md")
	_ = os.WriteFile(filePath, []byte("antes"), 0644)

	var observedPath string
	tool := NewTextEdit(dir, nil, WithTextEditWriteObserver(func(path string) func(bool) {
		observedPath = path
		return nil
	}))
	args := `{"original": "antes", "replacement": "depois"}`
	result, err := tool.Execute(editorCtx(filePath), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if result.IsError {
		t.Fatalf("resultado é erro: %s", result.Content)
	}
	if observedPath != filePath {
		t.Fatalf("observer path = %q, want %q", observedPath, filePath)
	}
}

func TestTextEdit_BlocksSensitive(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	_ = os.WriteFile(envPath, []byte("SECRET=123"), 0644)

	tool := NewTextEdit(dir, nil)
	args := `{"original": "SECRET=123", "replacement": "SECRET=456"}`
	result, err := tool.Execute(editorCtx(envPath), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if !result.IsError {
		t.Error("deve bloquear edição de .env")
	}
}

func TestTextEdit_FileNotExists(t *testing.T) {
	dir := t.TempDir()
	tool := NewTextEdit(dir, nil)
	args := `{"original": "a", "replacement": "b"}`
	result, err := tool.Execute(editorCtx(filepath.Join(dir, "ghost.md")), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if !result.IsError {
		t.Error("deve ser erro para arquivo inexistente")
	}
}
