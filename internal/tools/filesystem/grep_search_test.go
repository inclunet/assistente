package filesystem

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"assistente/internal/docextract"
)

func TestGrepSearch_Name(t *testing.T) {
	tool := NewGrepSearch("/tmp")
	if tool.Name() != "grep_search" {
		t.Errorf("expected 'grep_search', got '%s'", tool.Name())
	}
}

func TestGrepSearch_Parameters(t *testing.T) {
	tool := NewGrepSearch("/tmp")
	var schema map[string]interface{}
	if err := json.Unmarshal(tool.Parameters(), &schema); err != nil {
		t.Fatalf("Parameters() deve retornar JSON válido: %v", err)
	}
	if schema["type"] != "object" {
		t.Error("schema deve ter type=object")
	}
	props := schema["properties"].(map[string]interface{})
	if _, ok := props["pattern"]; !ok {
		t.Error("schema deve ter propriedade 'pattern'")
	}
}

func TestGrepSearch_LiteralSearch(t *testing.T) {
	dir := t.TempDir()

	// Cria arquivo de teste
	content := `package main

import "fmt"

func main() {
	fmt.Println("Hello, World!")
	fmt.Println("Goodbye, World!")
}
`
	_ = os.WriteFile(filepath.Join(dir, "main.go"), []byte(content), 0644)

	tool := NewGrepSearch(dir)
	args := `{"pattern": "Hello"}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if result.IsError {
		t.Fatalf("resultado é erro: %s", result.Content)
	}
	if !containsString(result.Content, "Hello") {
		t.Error("resultado deve conter 'Hello'")
	}
	if !containsString(result.Content, "main.go") {
		t.Error("resultado deve conter nome do arquivo")
	}
}

func TestGrepSearch_RegexSearch(t *testing.T) {
	dir := t.TempDir()

	content := `func Calculate(a, b int) int {
	return a + b
}

func Validate(input string) bool {
	return len(input) > 0
}
`
	_ = os.WriteFile(filepath.Join(dir, "util.go"), []byte(content), 0644)

	tool := NewGrepSearch(dir)
	args := `{"pattern": "func \\w+\\("}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if result.IsError {
		t.Fatalf("resultado é erro: %s", result.Content)
	}
	// Deve encontrar ambas as funções
	if !containsString(result.Content, "Calculate") {
		t.Error("deve encontrar Calculate")
	}
	if !containsString(result.Content, "Validate") {
		t.Error("deve encontrar Validate")
	}
}

func TestGrepSearch_CaseInsensitive(t *testing.T) {
	dir := t.TempDir()

	content := `Hello World
hello world
HELLO WORLD
`
	_ = os.WriteFile(filepath.Join(dir, "test.txt"), []byte(content), 0644)

	tool := NewGrepSearch(dir)
	args := `{"pattern": "hello", "case_sensitive": false}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if result.IsError {
		t.Fatalf("resultado é erro: %s", result.Content)
	}
	// Deve encontrar todas as 3 ocorrências
	if !containsString(result.Content, "Hello World") {
		t.Error("deve encontrar 'Hello World'")
	}
	if !containsString(result.Content, "HELLO WORLD") {
		t.Error("deve encontrar 'HELLO WORLD'")
	}
}

func TestGrepSearch_IncludeFilter(t *testing.T) {
	dir := t.TempDir()

	// Cria arquivos de tipos diferentes
	_ = os.WriteFile(filepath.Join(dir, "code.go"), []byte("func main() {}"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "code.py"), []byte("def main(): pass"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("main note"), 0644)

	tool := NewGrepSearch(dir)
	args := `{"pattern": "main", "include": "*.go"}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if result.IsError {
		t.Fatalf("resultado é erro: %s", result.Content)
	}
	// Deve encontrar apenas no .go
	if !containsString(result.Content, "code.go") {
		t.Error("deve encontrar em code.go")
	}
	if containsString(result.Content, "code.py") {
		t.Error("não deve encontrar em code.py")
	}
	if containsString(result.Content, "notes.txt") {
		t.Error("não deve encontrar em notes.txt")
	}
}

func TestGrepSearch_NoMatch(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello world"), 0644)

	tool := NewGrepSearch(dir)
	args := `{"pattern": "xyz_not_found"}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if result.IsError {
		t.Error("não deve ser erro, apenas zero resultados")
	}
	if !containsString(result.Content, "Nenhuma") {
		t.Error("deve indicar que não encontrou")
	}
}

func TestGrepSearch_SingleFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "single.txt")
	_ = os.WriteFile(filePath, []byte("line1\nfind_me\nline3"), 0644)

	tool := NewGrepSearch(dir)
	args := `{"pattern": "find_me", "path": "single.txt"}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if result.IsError {
		t.Fatalf("resultado é erro: %s", result.Content)
	}
	if !containsString(result.Content, "find_me") {
		t.Error("deve encontrar 'find_me'")
	}
}

func TestGrepSearch_MissingPattern(t *testing.T) {
	tool := NewGrepSearch(t.TempDir())
	args := `{}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	if !result.IsError {
		t.Error("deve ser erro sem pattern")
	}
}

func TestGrepSearch_SkipsBinaryFiles(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "image.png"), []byte{0x89, 'P', 'N', 'G', 0x00, 0xff}, 0644)
	_ = os.WriteFile(filepath.Join(dir, "code.go"), []byte("searchterm in code"), 0644)

	tool := NewGrepSearch(dir)
	args := `{"pattern": "searchterm"}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute retornou erro: %v", err)
	}
	// Deve encontrar em code.go mas não em image.png
	if !containsString(result.Content, "code.go") {
		t.Error("deve encontrar em code.go")
	}
	if containsString(result.Content, "image.png") {
		t.Error("não deve buscar em image.png (binário)")
	}
	if result.Metadata["files_considered"] != 2 || result.Metadata["files_scanned"] != 1 {
		t.Fatalf("limite do walk deve contar também o binário omitido: %v", result.Metadata)
	}
}

func TestGrepSearchContentOverridesBinaryExtension(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "nao-e-pdf.pdf"), []byte("agulha em texto puro\n"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := NewGrepSearch(dir).Execute(context.Background(), json.RawMessage(`{"pattern":"agulha"}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError || !strings.Contains(result.Content, "agulha em texto puro") {
		t.Fatalf("extensão binária escondeu conteúdo textual: %s", result.Content)
	}
	if strings.Contains(result.Content, "projeção Markdown") {
		t.Fatalf("texto foi tratado como documento: %s", result.Content)
	}
}

func TestGrepSearch_SearchesOpaqueDocumentProjection(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "manual.docx"), buildMinimalDOCX(t, "agulha documental"), 0644); err != nil {
		t.Fatal(err)
	}

	// nil explícito também deve selecionar o cache padrão do construtor.
	result, err := NewGrepSearch(dir, nil).Execute(context.Background(), json.RawMessage(`{"pattern":"agulha documental"}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatal(result.Content)
	}
	if !strings.Contains(result.Content, "manual.docx (projeção Markdown: docx)") {
		t.Fatalf("resultado não identifica a projeção: %s", result.Content)
	}
	if !strings.Contains(result.Content, "agulha documental") {
		t.Fatalf("conteúdo extraído ausente: %s", result.Content)
	}
	if result.Metadata["documents_extracted"] != 1 {
		t.Fatalf("metadata=%v", result.Metadata)
	}
}

func TestGrepSearch_TextFormatsStayRawUnlessMarkdownRequested(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "dados.csv"), []byte("nome,idade\nAna,30\n"), 0644); err != nil {
		t.Fatal(err)
	}
	tool := NewGrepSearch(dir)

	raw, err := tool.Execute(context.Background(), json.RawMessage(`{"pattern":"nome,idade"}`))
	if err != nil {
		t.Fatal(err)
	}
	if raw.IsError || !strings.Contains(raw.Content, "nome,idade") {
		t.Fatalf("CSV bruto não foi pesquisado: %s", raw.Content)
	}
	if strings.Contains(raw.Content, "projeção Markdown") {
		t.Fatalf("modo auto projetou CSV: %s", raw.Content)
	}

	projected, err := tool.Execute(context.Background(), json.RawMessage(`{"pattern":"| Ana","document_mode":"markdown"}`))
	if err != nil {
		t.Fatal(err)
	}
	if projected.IsError || !strings.Contains(projected.Content, "projeção Markdown: csv") {
		t.Fatalf("projeção pedida não foi pesquisada: %s", projected.Content)
	}
}

func TestGrepSearchSharesProjectionCacheWithReadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manual.docx")
	if err := os.WriteFile(path, buildMinimalDOCX(t, "cache compartilhado"), 0644); err != nil {
		t.Fatal(err)
	}
	cache := docextract.NewProjectionCache(docextract.DefaultCacheConfig())

	read, err := NewReadFile(dir, cache).Execute(context.Background(), json.RawMessage(`{"path":"manual.docx"}`))
	if err != nil || read.IsError {
		t.Fatalf("read_file: err=%v result=%s", err, read.Content)
	}
	if read.Metadata["cache_hit"] != false {
		t.Fatalf("primeira extração deveria ser miss: %v", read.Metadata)
	}

	grep, err := NewGrepSearch(dir, cache).Execute(context.Background(), json.RawMessage(`{"pattern":"cache compartilhado"}`))
	if err != nil || grep.IsError {
		t.Fatalf("grep_search: err=%v result=%s", err, grep.Content)
	}
	if grep.Metadata["document_cache_hits"] != 1 {
		t.Fatalf("grep não reutilizou a projeção: %v", grep.Metadata)
	}
}

func TestGrepSearchSkipsOversizedDocumentWithWarning(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "grande.pdf")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("%PDF-1.4\n"); err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(docextract.MaxExtractBytes + 1); err != nil {
		_ = file.Close()
		t.Skipf("não foi possível criar arquivo esparso: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	result, err := NewGrepSearch(dir).Execute(context.Background(), json.RawMessage(`{"pattern":"agulha","path":"grande.pdf"}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("falha de um documento não deve abortar a busca: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Avisos de documentos") || !strings.Contains(result.Content, "muito grande para extração") {
		t.Fatalf("aviso ausente: %s", result.Content)
	}
	if result.Metadata["document_warnings"] != 1 {
		t.Fatalf("metadata=%v", result.Metadata)
	}
}

func TestGrepSearchInvalidDocumentBecomesWarning(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "quebrado.docx"), []byte("PK\x03\x04conteúdo truncado"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := NewGrepSearch(dir).Execute(context.Background(), json.RawMessage(`{"pattern":"agulha"}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("um documento inválido derrubou a busca: %s", result.Content)
	}
	if !strings.Contains(result.Content, "quebrado.docx") || !strings.Contains(result.Content, "Avisos de documentos") {
		t.Fatalf("aviso agregável ausente: %s", result.Content)
	}
}

func TestGrepSearchDoesNotOpenGenericZIPs(t *testing.T) {
	dir := t.TempDir()
	var data bytes.Buffer
	zw := zip.NewWriter(&data)
	entry, err := zw.Create("conteudo.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("agulha dentro do ZIP")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "arquivo.zip"), data.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := NewGrepSearch(dir).Execute(context.Background(), json.RawMessage(`{"pattern":"agulha"}`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result.Content, "arquivo.zip") || result.Metadata["document_warnings"] != 0 {
		t.Fatalf("ZIP genérico deveria ser omitido silenciosamente: %s metadata=%v", result.Content, result.Metadata)
	}
}

func TestGrepSearchFindsDocumentDisguisedAsCSV(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "dados.csv"), buildMinimalDOCX(t, "documento disfarçado"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := NewGrepSearch(dir).Execute(context.Background(), json.RawMessage(`{"pattern":"documento disfarçado"}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError || !strings.Contains(result.Content, "projeção Markdown: docx") {
		t.Fatalf("documento disfarçado não foi projetado: %s", result.Content)
	}
}

func TestGrepSearchDoesNotTreatGenericZIPAsRTF(t *testing.T) {
	dir := t.TempDir()
	var data bytes.Buffer
	zw := zip.NewWriter(&data)
	entry, err := zw.Create("conteudo.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("agulha que não deve aparecer")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "disfarcado.rtf"), data.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := NewGrepSearch(dir).Execute(context.Background(), json.RawMessage(`{"pattern":"agulha"}`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result.Content, "agulha que não deve aparecer") || !strings.Contains(result.Content, "binário não suportado") {
		t.Fatalf("ZIP genérico foi tratado como RTF: %s", result.Content)
	}
}

func TestGrepSearchRejectsBinaryCSVInsteadOfSearchingRawBytes(t *testing.T) {
	dir := t.TempDir()
	content := append([]byte("prefixo válido\n"), []byte{'a', 0x00, 0xff, 'b'}...)
	if err := os.WriteFile(filepath.Join(dir, "dados.csv"), content, 0644); err != nil {
		t.Fatal(err)
	}

	result, err := NewGrepSearch(dir).Execute(context.Background(), json.RawMessage(`{"pattern":"prefixo"}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("arquivo inválido deve ser omitido com aviso: %s", result.Content)
	}
	if !strings.Contains(result.Content, "conteúdo não é texto UTF-8 válido") {
		t.Fatalf("CSV binário não foi recusado: %s", result.Content)
	}
	if strings.Contains(result.Content, "      1: prefixo") {
		t.Fatalf("grep devolveu match parcial de arquivo binário: %s", result.Content)
	}
}

func TestGrepSearchCountsAndWarnsInvalidTextAfterPrefix(t *testing.T) {
	dir := t.TempDir()
	content := append(bytes.Repeat([]byte("a"), docextract.DetectPrefixBytes+10), 0x00)
	if err := os.WriteFile(filepath.Join(dir, "quebrado.txt"), content, 0644); err != nil {
		t.Fatal(err)
	}

	result, err := NewGrepSearch(dir).Execute(context.Background(), json.RawMessage(`{"pattern":"aaa"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Content, "conteúdo não é texto UTF-8 válido") {
		t.Fatalf("erro depois do prefixo foi ocultado: %s", result.Content)
	}
	if result.Metadata["files_scanned"] != 1 {
		t.Fatalf("arquivo tentado não contou no limite: %v", result.Metadata)
	}
}

func TestGrepSearchWarnsWhenPrefixCannotBeRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sumiu.txt")
	if err := os.WriteFile(path, []byte("conteúdo"), 0644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	stats := &grepStats{}
	matches, searched := NewGrepSearch(dir).searchPath(
		context.Background(),
		path,
		info,
		regexp.MustCompile("conteúdo"),
		10,
		0,
		docextract.ModeAuto,
		stats,
	)
	if !searched || len(matches) != 0 {
		t.Fatalf("falha de prefixo deve contar como tentativa sem matches: searched=%v matches=%v", searched, matches)
	}
	if len(stats.warnings) != 1 || !strings.Contains(stats.warnings[0].Reason, "não foi possível ler o prefixo") {
		t.Fatalf("aviso ausente: %+v", stats.warnings)
	}
}

func TestGrepSearchCacheInvalidatesWhenFileChanges(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manual.docx")
	cache := docextract.NewProjectionCache(docextract.DefaultCacheConfig())
	tool := NewGrepSearch(dir, cache)
	if err := os.WriteFile(path, buildMinimalDOCX(t, "versão antiga"), 0644); err != nil {
		t.Fatal(err)
	}
	first, err := tool.Execute(context.Background(), json.RawMessage(`{"pattern":"versão antiga"}`))
	if err != nil || first.IsError || !strings.Contains(first.Content, "versão antiga") {
		t.Fatalf("primeira busca: err=%v result=%s", err, first.Content)
	}

	if err := os.WriteFile(path, buildMinimalDOCX(t, "versão nova com outro tamanho"), 0644); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	second, err := tool.Execute(context.Background(), json.RawMessage(`{"pattern":"versão nova"}`))
	if err != nil || second.IsError || !strings.Contains(second.Content, "versão nova") {
		t.Fatalf("busca após mudança: err=%v result=%s", err, second.Content)
	}
	if second.Metadata["document_cache_hits"] != 0 {
		t.Fatalf("identidade antiga foi reutilizada: %v", second.Metadata)
	}
}

func TestGrepSearchDoesNotAdvertiseOrRunOCR(t *testing.T) {
	tool := NewGrepSearch(t.TempDir())
	var schema struct {
		Properties map[string]struct {
			Enum []string `json:"enum"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(tool.Parameters(), &schema); err != nil {
		t.Fatal(err)
	}
	for _, value := range schema.Properties["document_mode"].Enum {
		if value == "ocr" {
			t.Fatal("OCR não deve aparecer no schema antes da Fase 3")
		}
	}

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"pattern":"x","document_mode":"ocr"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(result.Content, "Fase 3") {
		t.Fatalf("OCR deveria falhar explicitamente: %+v", result)
	}
}

func TestGrepSearchCancellationStopsDocumentSearch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "manual.docx"), buildMinimalDOCX(t, "conteúdo"), 0644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := NewGrepSearch(dir).Execute(ctx, json.RawMessage(`{"pattern":"conteúdo","path":"manual.docx"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(result.Content, "cancelada") {
		t.Fatalf("cancelamento não propagado: %+v", result)
	}
}

func TestMatchIncludePattern(t *testing.T) {
	tests := []struct {
		filename string
		pattern  string
		expected bool
	}{
		{"main.go", "*.go", true},
		{"main.py", "*.go", false},
		{"app.tsx", "*.{ts,tsx}", true},
		{"app.ts", "*.{ts,tsx}", true},
		{"app.js", "*.{ts,tsx}", false},
		{"test_main.py", "test_*", true},
		{"main.py", "test_*", false},
	}

	for _, tt := range tests {
		result := matchIncludePattern(tt.filename, tt.pattern)
		if result != tt.expected {
			t.Errorf("matchIncludePattern(%q, %q) = %v, want %v", tt.filename, tt.pattern, result, tt.expected)
		}
	}
}

func TestIsBinaryExtension(t *testing.T) {
	tests := []struct {
		filename string
		expected bool
	}{
		{"main.go", false},
		{"photo.png", true},
		{"doc.pdf", true},
		{"data.json", false},
		{"archive.zip", true},
		{"script.py", false},
	}

	for _, tt := range tests {
		result := isBinaryExtension(tt.filename)
		if result != tt.expected {
			t.Errorf("isBinaryExtension(%q) = %v, want %v", tt.filename, result, tt.expected)
		}
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
