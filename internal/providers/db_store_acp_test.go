package providers

import (
	"context"
	"testing"

	"assistente/internal/database"
	"assistente/internal/llm"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func acpTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("falha ao criar banco em memória: %v", err)
	}
	if err := db.AutoMigrate(&database.LLMProvider{}); err != nil {
		t.Fatalf("falha ao migrar tabela: %v", err)
	}
	database.SetDB(db)
	return db
}

// O agente vai e volta inteiro: comando, argumentos e ambiente. Perder um
// argumento no caminho é subir o agente em outro modo — `cursor-agent` sem
// `acp` abre uma sessão interativa, não um servidor de protocolo.
func TestProvedorACPSobreviveAoIdaEVoltaDoBanco(t *testing.T) {
	acpTestDB(t)
	ctx := database.WithUserID(context.Background(), "u1")
	store := NewDBStore()

	original := &llm.ProviderConfig{
		ID:         "cursor",
		Name:       "Cursor",
		Type:       llm.ProviderCustom,
		APIFormat:  llm.APIFormatACP,
		ACPCommand: "cursor-agent",
		ACPArgs:    []string{"acp", "--force"},
		ACPEnv:     map[string]string{"CURSOR_LOG": "debug"},
	}
	if err := store.Save(ctx, []*llm.ProviderConfig{original}); err != nil {
		t.Fatalf("Save falhou: %v", err)
	}

	volta, err := store.Get(ctx, "cursor")
	if err != nil {
		t.Fatalf("Get falhou: %v", err)
	}
	if volta.ACPCommand != "cursor-agent" {
		t.Errorf("comando = %q", volta.ACPCommand)
	}
	if len(volta.ACPArgs) != 2 || volta.ACPArgs[0] != "acp" || volta.ACPArgs[1] != "--force" {
		t.Errorf("argumentos = %#v", volta.ACPArgs)
	}
	if volta.ACPEnv["CURSOR_LOG"] != "debug" {
		t.Errorf("ambiente = %#v", volta.ACPEnv)
	}

	// Sem URL: a coluna aceita vazio, e é isso que dispensa recriar a tabela
	// só para trocar vazio por nulo.
	var linha database.LLMProvider
	if err := database.DB().Where("id = ?", "cursor").First(&linha).Error; err != nil {
		t.Fatalf("linha não encontrada: %v", err)
	}
	if linha.BaseURL != "" {
		t.Errorf("base_url = %q, esperado vazio", linha.BaseURL)
	}
}

// Provedor HTTP não ganha coluna de agente preenchida com "[]" ou "null": a
// linha dele continua legível para quem for depurar o banco na mão.
func TestProvedorHTTPNaoGanhaSujeiraDeAgente(t *testing.T) {
	acpTestDB(t)
	ctx := database.WithUserID(context.Background(), "u1")
	store := NewDBStore()

	if err := store.Save(ctx, []*llm.ProviderConfig{{
		ID: "openai", Name: "OpenAI", Type: llm.ProviderOpenAI,
		BaseURL: "https://api.openai.com/v1",
	}}); err != nil {
		t.Fatalf("Save falhou: %v", err)
	}

	var linha database.LLMProvider
	if err := database.DB().Where("id = ?", "openai").First(&linha).Error; err != nil {
		t.Fatalf("linha não encontrada: %v", err)
	}
	if linha.ACPArgs != "" || linha.ACPEnv != "" || linha.ACPCommand != "" {
		t.Errorf("colunas de agente sujas: comando=%q args=%q env=%q", linha.ACPCommand, linha.ACPArgs, linha.ACPEnv)
	}
}

// Uma linha com JSON quebrado não pode virar provedor pela metade — subir um
// agente de código sem os argumentos que definem o modo dele é pior do que ele
// não aparecer — nem derrubar a lista inteira junto.
func TestLinhaIlegivelSaiDaListaSemLevarAsOutras(t *testing.T) {
	db := acpTestDB(t)
	ctx := database.WithUserID(context.Background(), "u1")
	store := NewDBStore()

	if err := store.Save(ctx, []*llm.ProviderConfig{
		{ID: "cursor", Name: "Cursor", Type: llm.ProviderCustom, APIFormat: llm.APIFormatACP, ACPCommand: "cursor-agent", ACPArgs: []string{"acp"}},
		{ID: "openai", Name: "OpenAI", Type: llm.ProviderOpenAI, BaseURL: "https://api.openai.com/v1"},
	}); err != nil {
		t.Fatalf("Save falhou: %v", err)
	}
	if err := db.Exec(`UPDATE llm_providers SET acp_args = '{quebrado' WHERE id = 'cursor'`).Error; err != nil {
		t.Fatalf("falha ao corromper a linha: %v", err)
	}

	lista, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("Load falhou: %v", err)
	}
	if len(lista) != 1 || lista[0].ID != "openai" {
		t.Fatalf("esperava só o provedor íntegro, obtive %#v", lista)
	}

	// Pedir explicitamente o quebrado devolve o motivo, em vez de um provedor
	// silenciosamente sem argumentos.
	if _, err := store.Get(ctx, "cursor"); err == nil {
		t.Error("Get de linha ilegível deveria falhar")
	}
}
