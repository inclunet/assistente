package database

import (
	"errors"
	"testing"

	"gorm.io/gorm"
)

// bancoComProvedores devolve um banco com a tabela de provedores já no formato
// atual, que é o estado em que a v12 roda: ela é pós-AutoMigrate justamente
// porque depende da coluna acp_agent_id existir.
func bancoComProvedores(t *testing.T) *gorm.DB {
	t.Helper()
	database := newMigratorTestDB(t)
	if err := database.AutoMigrate(&LLMProvider{}); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	return database
}

func provedorGravado(t *testing.T, database *gorm.DB, p LLMProvider) {
	t.Helper()
	if err := database.Create(&p).Error; err != nil {
		t.Fatalf("gravar o provedor %s: %v", p.ID, err)
	}
}

func provedorLido(t *testing.T, database *gorm.DB, id string) LLMProvider {
	t.Helper()
	var p LLMProvider
	if err := database.First(&p, "id = ?", id).Error; err != nil {
		t.Fatalf("ler o provedor %s: %v", id, err)
	}
	return p
}

func TestOProvedorDeAgenteGanhaOTipoUnicoEOIDDoRegistro(t *testing.T) {
	database := bancoComProvedores(t)
	provedorGravado(t, database, LLMProvider{
		ID: "cursor-agent", Name: "Cursor", Type: "cursor", APIFormat: "acp",
		ACPCommand: "cursor-agent", ACPArgs: `["acp"]`,
	})
	provedorGravado(t, database, LLMProvider{
		ID: "claude-code-agent", Name: "Claude Code", Type: "claude-code", APIFormat: "acp",
		ACPCommand: "claude-agent-acp",
	})

	if err := migrateAgentProvidersToSingleType(database); err != nil {
		t.Fatalf("migração v12: %v", err)
	}

	cursor := provedorLido(t, database, "cursor-agent")
	if cursor.Type != "acp" || cursor.ACPAgentID != "cursor" {
		t.Errorf("Cursor ficou type=%q agente=%q", cursor.Type, cursor.ACPAgentID)
	}
	claude := provedorLido(t, database, "claude-code-agent")
	if claude.Type != "acp" || claude.ACPAgentID != "claude-acp" {
		t.Errorf("Claude Code ficou type=%q agente=%q", claude.Type, claude.ACPAgentID)
	}
}

func TestAMigracaoNaoEncostaNoComandoDoProvedor(t *testing.T) {
	// O comando é a escolha de quem configurou, e nem a detecção automática o
	// sobrescreve (AEP-0084 Fase 3). Trocar o rótulo do provedor não é motivo
	// para o agente passar a subir de outro jeito.
	database := bancoComProvedores(t)
	provedorGravado(t, database, LLMProvider{
		ID: "meu-cursor", Name: "Cursor de casa", Type: "cursor", APIFormat: "acp",
		ACPCommand: `C:\ferramentas\cursor\node.exe`,
		ACPArgs:    `["C:\\ferramentas\\cursor\\index.js","acp"]`,
		ACPEnv:     `{"CURSOR_HOME":"D:\\cursor"}`,
	})

	if err := migrateAgentProvidersToSingleType(database); err != nil {
		t.Fatalf("migração v12: %v", err)
	}

	p := provedorLido(t, database, "meu-cursor")
	if p.ACPCommand != `C:\ferramentas\cursor\node.exe` {
		t.Errorf("comando = %q", p.ACPCommand)
	}
	if p.ACPArgs != `["C:\\ferramentas\\cursor\\index.js","acp"]` {
		t.Errorf("argumentos = %q", p.ACPArgs)
	}
	if p.ACPEnv != `{"CURSOR_HOME":"D:\\cursor"}` {
		t.Errorf("ambiente = %q", p.ACPEnv)
	}
}

func TestOProvedorHTTPQueAlguemChamouDeCursorNaoEConvertido(t *testing.T) {
	// O tipo sozinho é ambíguo: nada impedia alguém de digitar "cursor" num
	// provedor HTTP. Trocar o tipo dele o faria passar por agente sem ter
	// comando nenhum para subir.
	database := bancoComProvedores(t)
	provedorGravado(t, database, LLMProvider{
		ID: "http-cursor", Name: "Um HTTP qualquer", Type: "cursor",
		APIFormat: "openai", BaseURL: "https://exemplo.invalido/v1",
	})

	if err := migrateAgentProvidersToSingleType(database); err != nil {
		t.Fatalf("migração v12: %v", err)
	}

	p := provedorLido(t, database, "http-cursor")
	if p.Type != "cursor" || p.ACPAgentID != "" {
		t.Errorf("provedor HTTP foi convertido: type=%q agente=%q", p.Type, p.ACPAgentID)
	}
}

func TestOAgenteConfiguradoAMaoContinuaSemIDDeRegistro(t *testing.T) {
	// Configurar comando e argumentos à mão é caminho válido (D3), e um
	// provedor assim não veio de linha nenhuma do catálogo. Inventar um
	// identificador para ele faria o app oferecer atualizar um agente que não
	// foi ele quem instalou.
	database := bancoComProvedores(t)
	provedorGravado(t, database, LLMProvider{
		ID: "agente-proprio", Name: "Meu agente", Type: "custom", APIFormat: "acp",
		ACPCommand: "/opt/meu-agente/bin/acp",
	})

	if err := migrateAgentProvidersToSingleType(database); err != nil {
		t.Fatalf("migração v12: %v", err)
	}

	p := provedorLido(t, database, "agente-proprio")
	if p.ACPAgentID != "" {
		t.Errorf("agente configurado à mão ganhou o identificador %q", p.ACPAgentID)
	}
}

func TestRodarAMigracaoDeNovoNaoMudaNada(t *testing.T) {
	// Uma migração adiada é retentada no próximo boot, então rodar duas vezes é
	// caminho normal, e não exceção.
	database := bancoComProvedores(t)
	provedorGravado(t, database, LLMProvider{
		ID: "cursor-agent", Name: "Cursor", Type: "cursor", APIFormat: "acp",
		ACPCommand: "cursor-agent",
	})

	for i := range 2 {
		if err := migrateAgentProvidersToSingleType(database); err != nil {
			t.Fatalf("migração v12 (execução %d): %v", i+1, err)
		}
	}

	p := provedorLido(t, database, "cursor-agent")
	if p.Type != "acp" || p.ACPAgentID != "cursor" {
		t.Errorf("segunda execução mudou o provedor: type=%q agente=%q", p.Type, p.ACPAgentID)
	}
}

func TestSemATabelaDeProvedoresAMigracaoNaoReclama(t *testing.T) {
	// Banco novo roda as migrações antes de ter conteúdo, e não ter o que
	// converter é desfecho normal.
	if err := migrateAgentProvidersToSingleType(newMigratorTestDB(t)); err != nil {
		t.Fatalf("migração v12 num banco sem provedores: %v", err)
	}
	if err := migrateAgentProvidersToSingleType(nil); err != nil {
		t.Fatalf("migração v12 sem banco: %v", err)
	}
}

func TestSemAColunaAMigracaoAdiaEmVezDeAbortarOBoot(t *testing.T) {
	// A coluna vem do AutoMigrate. Se ele não a criou, o provedor continua
	// subindo o mesmo comando com o rótulo antigo — situação para retentar no
	// próximo boot, e não para impedir o app de abrir.
	database := newMigratorTestDB(t)
	if err := database.Exec(`CREATE TABLE llm_providers (id TEXT PRIMARY KEY, type TEXT, api_format TEXT)`).Error; err != nil {
		t.Fatalf("criar a tabela antiga: %v", err)
	}

	err := migrateAgentProvidersToSingleType(database)

	if err == nil {
		t.Fatal("migração passou sem a coluna acp_agent_id")
	}
	if !errors.Is(err, errMigrationDeferred) {
		t.Errorf("erro = %v, queria um adiamento", err)
	}
}
