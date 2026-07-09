package filesystem

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"assistente/internal/questionnaire"
	"assistente/internal/tools/invocationctx"
)

// fakeQuestionnaireRequester registra as solicitações e devolve uma resposta pré-configurada.
type fakeQuestionnaireRequester struct {
	calls     []questionnaire.RequestPayload
	cancelled bool
	err       error
}

func (f *fakeQuestionnaireRequester) RequestQuestionnaire(_ context.Context, payload questionnaire.RequestPayload) (questionnaire.Response, error) {
	f.calls = append(f.calls, payload)
	if f.err != nil {
		return questionnaire.Response{}, f.err
	}
	return questionnaire.Response{Cancelled: f.cancelled}, nil
}

func editorCtx(activeFilePath string) context.Context {
	return invocationctx.With(context.Background(), invocationctx.InvocationContext{
		TabType:        "editor",
		ActiveFilePath: activeFilePath,
	})
}

func writeArgs(t *testing.T, path, content string) json.RawMessage {
	t.Helper()
	args, err := json.Marshal(map[string]string{"path": path, "content": content})
	if err != nil {
		t.Fatalf("erro ao montar args: %v", err)
	}
	return args
}

func TestWriteFile_EditorActiveFile_ApprovedWrites(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "doc.md")
	_ = os.WriteFile(filePath, []byte("conteúdo antigo"), 0644)

	quest := &fakeQuestionnaireRequester{}
	tool := NewWriteFile(dir, WithWriteFileQuestionnaire(quest))

	result, err := tool.Execute(editorCtx(filePath), writeArgs(t, "doc.md", "conteúdo novo"))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if result.IsError {
		t.Fatalf("resultado é erro: %s", result.Content)
	}

	if len(quest.calls) != 1 {
		t.Fatalf("questionário deve ser exibido exatamente 1 vez, foi %d", len(quest.calls))
	}
	payload := quest.calls[0]
	if len(payload.Questions) != 2 {
		t.Fatalf("payload deve ter 2 questões (Antes/Depois), tem %d", len(payload.Questions))
	}
	if payload.Questions[0].Prompt != "Antes" || payload.Questions[0].Content != "conteúdo antigo" {
		t.Errorf("questão 'Antes' incorreta: %+v", payload.Questions[0])
	}
	if payload.Questions[1].Prompt != "Depois" || payload.Questions[1].Content != "conteúdo novo" {
		t.Errorf("questão 'Depois' incorreta: %+v", payload.Questions[1])
	}
	if payload.SubmitLabel != "Aplicar" || payload.CancelLabel != "Rejeitar" || !payload.AllowCancel {
		t.Errorf("labels/cancel incorretos: %+v", payload)
	}

	data, _ := os.ReadFile(filePath)
	if string(data) != "conteúdo novo" {
		t.Errorf("arquivo não foi gravado após aprovação: %s", string(data))
	}
}

func TestWriteFile_EditorActiveFile_RejectedDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "doc.md")
	_ = os.WriteFile(filePath, []byte("conteúdo antigo"), 0644)

	quest := &fakeQuestionnaireRequester{cancelled: true}
	tool := NewWriteFile(dir, WithWriteFileQuestionnaire(quest))

	result, err := tool.Execute(editorCtx(filePath), writeArgs(t, "doc.md", "conteúdo novo"))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if !result.IsError {
		t.Fatal("resultado deve ser erro quando rejeitado")
	}
	if !strings.Contains(result.Content, "Alteração rejeitada pelo usuário") {
		t.Errorf("mensagem de rejeição incorreta: %s", result.Content)
	}

	data, _ := os.ReadFile(filePath)
	if string(data) != "conteúdo antigo" {
		t.Errorf("arquivo não pode ser modificado após rejeição: %s", string(data))
	}
}

func TestWriteFile_EditorActiveFile_QuestionnaireErrorDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "doc.md")
	_ = os.WriteFile(filePath, []byte("conteúdo antigo"), 0644)

	quest := &fakeQuestionnaireRequester{err: fmt.Errorf("timeout")}
	tool := NewWriteFile(dir, WithWriteFileQuestionnaire(quest))

	result, err := tool.Execute(editorCtx(filePath), writeArgs(t, "doc.md", "conteúdo novo"))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if !result.IsError {
		t.Fatal("resultado deve ser erro quando o questionário falha")
	}

	data, _ := os.ReadFile(filePath)
	if string(data) != "conteúdo antigo" {
		t.Errorf("arquivo não pode ser modificado quando o questionário falha: %s", string(data))
	}
}

func TestWriteFile_EditorOtherFile_WritesDirect(t *testing.T) {
	dir := t.TempDir()
	quest := &fakeQuestionnaireRequester{}
	tool := NewWriteFile(dir, WithWriteFileQuestionnaire(quest))

	// Aba de editor, mas o arquivo ativo é outro
	ctx := editorCtx(filepath.Join(dir, "outro.md"))
	result, err := tool.Execute(ctx, writeArgs(t, "novo.md", "conteúdo"))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if result.IsError {
		t.Fatalf("resultado é erro: %s", result.Content)
	}
	if len(quest.calls) != 0 {
		t.Errorf("questionário não deve ser exibido para arquivo que não é o ativo, foi %d vez(es)", len(quest.calls))
	}

	data, _ := os.ReadFile(filepath.Join(dir, "novo.md"))
	if string(data) != "conteúdo" {
		t.Errorf("arquivo não foi gravado: %s", string(data))
	}
}

func TestWriteFile_OutsideEditor_WritesDirect(t *testing.T) {
	dir := t.TempDir()
	quest := &fakeQuestionnaireRequester{}
	tool := NewWriteFile(dir, WithWriteFileQuestionnaire(quest))

	result, err := tool.Execute(context.Background(), writeArgs(t, "livre.txt", "sem confirmação"))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if result.IsError {
		t.Fatalf("resultado é erro: %s", result.Content)
	}
	if len(quest.calls) != 0 {
		t.Errorf("questionário não deve ser exibido fora do editor, foi %d vez(es)", len(quest.calls))
	}
}

func TestWriteFile_NoQuestMgr_WritesDirect(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "doc.md")
	_ = os.WriteFile(filePath, []byte("antigo"), 0644)

	// Sem WithWriteFileQuestionnaire (contexto CLI/testes/não-UI)
	tool := NewWriteFile(dir)

	result, err := tool.Execute(editorCtx(filePath), writeArgs(t, "doc.md", "novo"))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if result.IsError {
		t.Fatalf("resultado é erro: %s", result.Content)
	}

	data, _ := os.ReadFile(filePath)
	if string(data) != "novo" {
		t.Errorf("sem questMgr deve gravar direto: %s", string(data))
	}
}

func TestWriteFile_ConfirmPreviewTruncated(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "grande.txt")

	var sb strings.Builder
	for i := 0; i < previewMaxLines+100; i++ {
		fmt.Fprintf(&sb, "linha %d\n", i)
	}
	bigOld := sb.String()
	_ = os.WriteFile(filePath, []byte(bigOld), 0644)

	bigNew := strings.Repeat("x", previewMaxBytes+1024)

	quest := &fakeQuestionnaireRequester{}
	tool := NewWriteFile(dir, WithWriteFileQuestionnaire(quest))

	result, err := tool.Execute(editorCtx(filePath), writeArgs(t, "grande.txt", bigNew))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if result.IsError {
		t.Fatalf("resultado é erro: %s", result.Content)
	}
	if len(quest.calls) != 1 {
		t.Fatalf("questionário deve ser exibido 1 vez, foi %d", len(quest.calls))
	}

	before := quest.calls[0].Questions[0].Content
	after := quest.calls[0].Questions[1].Content

	if !strings.HasSuffix(before, previewTruncationMarker) {
		t.Error("preview 'Antes' deve conter marcador de truncamento")
	}
	if got := strings.Count(strings.TrimSuffix(before, previewTruncationMarker), "\n"); got > previewMaxLines {
		t.Errorf("preview 'Antes' deve ter no máximo %d linhas, tem %d", previewMaxLines, got)
	}

	if !strings.HasSuffix(after, previewTruncationMarker) {
		t.Error("preview 'Depois' deve conter marcador de truncamento")
	}
	if got := len(after); got > previewMaxBytes+len(previewTruncationMarker) {
		t.Errorf("preview 'Depois' deve ter no máximo %d bytes, tem %d", previewMaxBytes+len(previewTruncationMarker), got)
	}
}

func TestWriteFile_EditorActiveFile_CaseInsensitivePathWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("comparação case-insensitive só se aplica ao Windows")
	}
	dir := t.TempDir()
	filePath := filepath.Join(dir, "doc.md")
	_ = os.WriteFile(filePath, []byte("antigo"), 0644)

	quest := &fakeQuestionnaireRequester{}
	tool := NewWriteFile(dir, WithWriteFileQuestionnaire(quest))

	// ActiveFilePath com capitalização diferente do path resolvido pela tool.
	ctx := editorCtx(strings.ToUpper(filePath))
	result, err := tool.Execute(ctx, writeArgs(t, "doc.md", "novo"))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if result.IsError {
		t.Fatalf("resultado é erro: %s", result.Content)
	}
	if len(quest.calls) != 1 {
		t.Errorf("capitalização diferente não pode burlar a confirmação: questionário foi exibido %d vez(es)", len(quest.calls))
	}
}

func TestSameFilePath_Normalization(t *testing.T) {
	if !sameFilePath(filepath.Join("a", "b", "..", "c.md"), filepath.Join("a", "c.md")) {
		t.Error("sameFilePath deve normalizar '..' via filepath.Clean")
	}
	if sameFilePath(filepath.Join("a", "c.md"), filepath.Join("a", "d.md")) {
		t.Error("sameFilePath não pode igualar arquivos diferentes")
	}
	if runtime.GOOS == "windows" {
		if !sameFilePath(`C:\Users\user\DOC.md`, `c:\users\user\doc.md`) {
			t.Error("sameFilePath deve ser case-insensitive no Windows")
		}
	}
}

func TestReadFilePrefixForPreview(t *testing.T) {
	dir := t.TempDir()

	small := filepath.Join(dir, "small.txt")
	_ = os.WriteFile(small, []byte("abc"), 0644)
	got, err := readFilePrefixForPreview(small)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if got != "abc" {
		t.Errorf("prefixo de arquivo pequeno deve ser o conteúdo inteiro, obtido %q", got)
	}

	big := filepath.Join(dir, "big.txt")
	_ = os.WriteFile(big, []byte(strings.Repeat("x", previewMaxBytes*4)), 0644)
	got, err = readFilePrefixForPreview(big)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(got) != previewMaxBytes+1 {
		t.Errorf("prefixo deve ler no máximo previewMaxBytes+1 bytes, leu %d", len(got))
	}
	// O preview final precisa sinalizar o corte.
	if !strings.HasSuffix(truncateForPreview(got), previewTruncationMarker) {
		t.Error("preview de arquivo grande deve conter marcador de truncamento")
	}
}

func TestReadFilePrefixForPreview_Error(t *testing.T) {
	dir := t.TempDir()
	if _, err := readFilePrefixForPreview(filepath.Join(dir, "inexistente.txt")); err == nil {
		t.Error("leitura de arquivo inexistente deve retornar erro")
	}
}

func TestTruncateForPreview(t *testing.T) {
	if got := truncateForPreview("curto"); got != "curto" {
		t.Errorf("conteúdo curto não deve ser alterado: %q", got)
	}

	// Corte por bytes não pode quebrar rune UTF-8 no meio
	multibyte := strings.Repeat("é", previewMaxBytes) // 2 bytes por rune
	got := truncateForPreview(multibyte)
	body := strings.TrimSuffix(got, previewTruncationMarker)
	if body == got {
		t.Fatal("conteúdo grande deve receber marcador de truncamento")
	}
	if len(body) > previewMaxBytes {
		t.Errorf("corpo truncado deve ter no máximo %d bytes, tem %d", previewMaxBytes, len(body))
	}
	for _, r := range body {
		if r != 'é' {
			t.Fatalf("corte quebrou rune UTF-8: encontrou %q", r)
		}
	}
}
