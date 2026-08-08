package portability

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"assistente/internal/database"
	"assistente/internal/llm"
)

// A constante local existe porque importar `llm` no código deste pacote fecha
// um ciclo. O teste pode importar, e é ele que impede a cópia de envelhecer.
func TestFormatoACPLocalAcompanhaODominio(t *testing.T) {
	if acpAPIFormat != string(llm.APIFormatACP) {
		t.Fatalf("acpAPIFormat = %q, mas o domínio usa %q", acpAPIFormat, llm.APIFormatACP)
	}
	if acpProviderType != string(llm.ProviderACP) {
		t.Fatalf("acpProviderType = %q, mas o domínio usa %q", acpProviderType, llm.ProviderACP)
	}
}

// O agente atravessa o arquivo inteiro: comando e argumentos são o que
// substitui a URL. Perder um argumento na viagem é subir o agente em outro
// modo na máquina de destino.
func TestImportaProvedorACPComComandoEArgumentos(t *testing.T) {
	setupPortabilityTestDB(t)

	file := &ExportFile{
		Version:    ExportVersion,
		ExportedAt: time.Now().UTC(),
		Resources: ExportResources{
			Providers: []ProviderExport{{
				ID:         "cursor",
				Name:       "Cursor",
				Type:       "custom",
				APIFormat:  "acp",
				ACPCommand: "agente-que-nao-existe-nesta-maquina",
				ACPArgs:    []string{"acp", "--force"},
				ACPEnv:     map[string]string{"CURSOR_LOG": "debug"},
			}},
		},
	}
	raw, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	result, err := ImportConversationsWithContext(portabilityTestCtx(), string(raw), nil, "")
	if err != nil {
		t.Fatalf("ImportConversations() error = %v", err)
	}
	if result.Imported != 1 || result.Failed != 0 {
		t.Fatalf("resultado inesperado: %+v", result)
	}

	imported, err := database.GetLLMProviderWithContext(portabilityTestCtx(), "cursor")
	if err != nil {
		t.Fatalf("GetLLMProvider() error = %v", err)
	}
	if imported.ACPCommand != "agente-que-nao-existe-nesta-maquina" {
		t.Errorf("comando = %q", imported.ACPCommand)
	}
	if imported.ACPArgs != `["acp","--force"]` {
		t.Errorf("argumentos = %q", imported.ACPArgs)
	}
	if imported.ACPEnv != `{"CURSOR_LOG":"debug"}` {
		t.Errorf("ambiente = %q", imported.ACPEnv)
	}

	// Caminho de binário é a parte que não viaja: sem aviso, a primeira
	// conversa falharia sem explicação.
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "não foi encontrado nesta máquina") {
		t.Fatalf("avisos inesperados: %v", result.Warnings)
	}
}

// Um arquivo escrito à mão pode trazer "acpArgs": [] num provedor HTTP. Isso
// não é configuração de agente, e não pode virar uma no banco: a coluna com o
// literal "[]" contradiz a convenção do store, onde vazio é vazio.
func TestColecaoVaziaDoAgenteNaoViraLiteralNoBanco(t *testing.T) {
	setupPortabilityTestDB(t)

	// O JSON é montado à mão de propósito: `ProviderExport` marca os campos do
	// agente com omitempty, então serializar a struct com coleções vazias
	// simplesmente não escreveria "acpArgs" — e o teste não exercitaria o caso.
	raw := `{
	  "version": ` + strconv.Itoa(ExportVersion) + `,
	  "exportedAt": "` + time.Now().UTC().Format(time.RFC3339) + `",
	  "options": {},
	  "resources": {
	    "providers": [
	      {
	        "id": "openai", "name": "OpenAI", "type": "openai", "apiFormat": "openai",
	        "baseUrl": "https://api.openai.com/v1",
	        "acpArgs": [], "acpEnv": {}
	      },
	      {
	        "id": "cursor", "name": "Cursor", "type": "custom", "apiFormat": "acp",
	        "acpCommand": "agente-que-nao-existe-nesta-maquina",
	        "acpArgs": [], "acpEnv": {}
	      }
	    ]
	  }
	}`

	result, err := ImportConversationsWithContext(portabilityTestCtx(), raw, nil, "")
	if err != nil {
		t.Fatalf("ImportConversations() error = %v", err)
	}
	if result.Imported != 2 || result.Failed != 0 {
		t.Fatalf("resultado inesperado: %+v", result)
	}

	for _, id := range []string{"openai", "cursor"} {
		imported, err := database.GetLLMProviderWithContext(portabilityTestCtx(), id)
		if err != nil {
			t.Fatalf("GetLLMProvider(%s) error = %v", id, err)
		}
		if imported.ACPArgs != "" {
			t.Errorf("provider %s guardou argumentos = %q, esperado coluna vazia", id, imported.ACPArgs)
		}
		if imported.ACPEnv != "" {
			t.Errorf("provider %s guardou ambiente = %q, esperado coluna vazia", id, imported.ACPEnv)
		}
	}
}

// Agente que existe aqui não vira alarme falso: aviso repetido em importação
// que está certa ensina a ignorar aviso.
func TestAgenteEncontradoNaMaquinaNaoGeraAviso(t *testing.T) {
	executavel, err := os.Executable()
	if err != nil {
		t.Skipf("sem caminho do executável de teste: %v", err)
	}

	aviso := acpCommandWarning(ProviderExport{ID: "cursor", APIFormat: "acp", ACPCommand: executavel})
	if aviso != "" {
		t.Errorf("aviso indevido: %q", aviso)
	}
}

func TestProvedorHTTPNaoGeraAvisoDeAgente(t *testing.T) {
	aviso := acpCommandWarning(ProviderExport{
		ID: "openai", APIFormat: "openai", BaseURL: "https://api.openai.com/v1",
	})
	if aviso != "" {
		t.Errorf("aviso indevido: %q", aviso)
	}
}

func TestImportacaoRecusaProvedorACPSemComandoEAgenteEmProvedorHTTP(t *testing.T) {
	setupPortabilityTestDB(t)

	casos := []struct {
		nome     string
		provider ProviderExport
		contendo string
	}{
		{
			nome:     "acp sem comando",
			provider: ProviderExport{ID: "cursor", Name: "Cursor", Type: "custom", APIFormat: "acp"},
			contendo: "sem acpCommand",
		},
		{
			nome: "comando em provedor http",
			provider: ProviderExport{
				ID: "openai", Name: "OpenAI", Type: "openai", APIFormat: "openai",
				BaseURL: "https://api.openai.com/v1", ACPCommand: "cursor-agent",
			},
			contendo: "configuração de agente",
		},
		{
			nome: "argumentos em provedor http",
			provider: ProviderExport{
				ID: "openai", Name: "OpenAI", Type: "openai", APIFormat: "openai",
				BaseURL: "https://api.openai.com/v1", ACPArgs: []string{"acp"},
			},
			contendo: "configuração de agente",
		},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			file := &ExportFile{
				Version:    ExportVersion,
				ExportedAt: time.Now().UTC(),
				Resources:  ExportResources{Providers: []ProviderExport{caso.provider}},
			}
			raw, err := json.Marshal(file)
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}

			result, err := ImportConversationsWithContext(portabilityTestCtx(), string(raw), nil, "")
			if err != nil {
				t.Fatalf("ImportConversations() error = %v", err)
			}
			if result.Failed != 1 || len(result.Errors) != 1 || !strings.Contains(result.Errors[0], caso.contendo) {
				t.Fatalf("resultado inesperado: %+v", result)
			}
		})
	}
}

// A exportação leva comando e argumentos e deixa o ambiente para trás — é onde
// token costuma parar, e o arquivo viaja entre máquinas. Mesma escolha já
// feita para o servidor MCP stdio.
func TestExportacaoLevaOAgenteMasNaoOAmbiente(t *testing.T) {
	exportado, err := exportProvider(&database.LLMProvider{
		ID: "cursor", Name: "Cursor", Type: "custom", APIFormat: "acp",
		ACPCommand: "cursor-agent",
		ACPArgs:    `["acp","--force"]`,
		ACPEnv:     `{"CURSOR_TOKEN":"segredo"}`,
	})
	if err != nil {
		t.Fatalf("exportProvider() error = %v", err)
	}
	if exportado.ACPCommand != "cursor-agent" {
		t.Errorf("comando = %q", exportado.ACPCommand)
	}
	if len(exportado.ACPArgs) != 2 || exportado.ACPArgs[0] != "acp" {
		t.Errorf("argumentos = %#v", exportado.ACPArgs)
	}
	if len(exportado.ACPEnv) != 0 {
		t.Errorf("o ambiente saiu no arquivo: %#v", exportado.ACPEnv)
	}

	raw, err := json.Marshal(exportado)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if strings.Contains(string(raw), "segredo") {
		t.Errorf("o segredo do ambiente vazou para o JSON: %s", raw)
	}
}

// Arquivo escrito antes da emenda do D11 nomeia o agente no tipo do provedor.
// Ele entra pelo vocabulário de hoje — tipo único e agente no campo próprio —
// porque um provedor com tipo que o app não oferece mais não teria como ser
// editado na tela: o seletor não teria a opção dele, e a primeira troca de tipo
// apagaria o comando.
func TestImportacaoTrazOProvedorDeTipoAntigoParaOTipoUnico(t *testing.T) {
	setupPortabilityTestDB(t)

	casos := []struct {
		tipo    string
		agente  string
		provID  string
		comando string
	}{
		{tipo: "cursor", agente: "cursor", provID: "cursor-1", comando: "cursor-agent"},
		{tipo: "claude-code", agente: "claude-acp", provID: "claude-1", comando: "node"},
	}

	for _, caso := range casos {
		t.Run(caso.tipo, func(t *testing.T) {
			file := &ExportFile{
				Version:    ExportVersion,
				ExportedAt: time.Now().UTC(),
				Resources: ExportResources{
					Providers: []ProviderExport{{
						ID:         caso.provID,
						Name:       caso.tipo,
						Type:       caso.tipo,
						APIFormat:  "acp",
						ACPCommand: caso.comando,
					}},
				},
			}
			raw, err := json.Marshal(file)
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}

			if _, err := ImportConversationsWithContext(portabilityTestCtx(), string(raw), nil, ""); err != nil {
				t.Fatalf("ImportConversations() error = %v", err)
			}

			imported, err := database.GetLLMProviderWithContext(portabilityTestCtx(), caso.provID)
			if err != nil {
				t.Fatalf("GetLLMProvider() error = %v", err)
			}
			if imported.Type != string(llm.ProviderACP) {
				t.Errorf("tipo = %q, queria %q", imported.Type, llm.ProviderACP)
			}
			if imported.ACPAgentID != caso.agente {
				t.Errorf("agente = %q, queria %q", imported.ACPAgentID, caso.agente)
			}
			if imported.ACPCommand != caso.comando {
				t.Errorf("comando = %q, queria %q", imported.ACPCommand, caso.comando)
			}
		})
	}
}

// Não é só o vocabulário aposentado que entra pelo tipo único: um arquivo pode
// nomear o agente de qualquer coisa, e importar aquilo como está gravaria, do
// lado de fora da migração, tipo que o app não oferece mais para agente.
func TestOAgenteImportadoComQualquerTipoEntraComOTipoUnico(t *testing.T) {
	setupPortabilityTestDB(t)

	file := &ExportFile{
		Version:    ExportVersion,
		ExportedAt: time.Now().UTC(),
		Resources: ExportResources{
			Providers: []ProviderExport{{
				ID:         "agente-da-casa",
				Name:       "Agente da casa",
				Type:       "custom",
				APIFormat:  "acp",
				ACPCommand: "meu-agente",
			}},
		},
	}
	raw, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	if _, err := ImportConversationsWithContext(portabilityTestCtx(), string(raw), nil, ""); err != nil {
		t.Fatalf("ImportConversations() error = %v", err)
	}

	imported, err := database.GetLLMProviderWithContext(portabilityTestCtx(), "agente-da-casa")
	if err != nil {
		t.Fatalf("GetLLMProvider() error = %v", err)
	}
	if imported.Type != string(llm.ProviderACP) {
		t.Errorf("tipo = %q, queria %q", imported.Type, llm.ProviderACP)
	}
	// Agente apontado à mão é caminho válido: sem `id` no arquivo, ele fica
	// sem nenhum, e não com um inventado a partir do tipo.
	if imported.ACPAgentID != "" {
		t.Errorf("agente = %q, queria nenhum", imported.ACPAgentID)
	}
	if imported.ACPCommand != "meu-agente" {
		t.Errorf("comando = %q, queria intacto", imported.ACPCommand)
	}
}

// O tipo antigo só diz qual agente é quando o provedor é um agente. Chamar de
// "cursor" um provedor HTTP é escolha de quem o cadastrou, e converter aquilo
// em agente do registro seria inventar configuração que ninguém pediu.
func TestOProvedorHTTPChamadoDeCursorNaoViraAgenteNaImportacao(t *testing.T) {
	setupPortabilityTestDB(t)

	file := &ExportFile{
		Version:    ExportVersion,
		ExportedAt: time.Now().UTC(),
		Resources: ExportResources{
			Providers: []ProviderExport{{
				ID:        "http-cursor",
				Name:      "Cursor pela API",
				Type:      "cursor",
				APIFormat: "openai",
				BaseURL:   "https://api.exemplo/v1",
			}},
		},
	}
	raw, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	if _, err := ImportConversationsWithContext(portabilityTestCtx(), string(raw), nil, ""); err != nil {
		t.Fatalf("ImportConversations() error = %v", err)
	}

	imported, err := database.GetLLMProviderWithContext(portabilityTestCtx(), "http-cursor")
	if err != nil {
		t.Fatalf("GetLLMProvider() error = %v", err)
	}
	if imported.Type != "cursor" {
		t.Errorf("tipo = %q, queria o que estava no arquivo", imported.Type)
	}
	if imported.ACPAgentID != "" {
		t.Errorf("ganhou agente do registro sem ser um agente: %q", imported.ACPAgentID)
	}
}

// O arquivo que já traz o agente no campo próprio manda nele: o tipo antigo é
// palpite sobre qual agente é, e o campo é a resposta.
func TestOAgenteDoArquivoPrevaleceSobreOTipoAntigo(t *testing.T) {
	setupPortabilityTestDB(t)

	file := &ExportFile{
		Version:    ExportVersion,
		ExportedAt: time.Now().UTC(),
		Resources: ExportResources{
			Providers: []ProviderExport{{
				ID:         "misto",
				Name:       "Agente",
				Type:       "cursor",
				APIFormat:  "acp",
				ACPCommand: "gemini",
				ACPAgentID: "gemini-cli",
			}},
		},
	}
	raw, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	if _, err := ImportConversationsWithContext(portabilityTestCtx(), string(raw), nil, ""); err != nil {
		t.Fatalf("ImportConversations() error = %v", err)
	}

	imported, err := database.GetLLMProviderWithContext(portabilityTestCtx(), "misto")
	if err != nil {
		t.Fatalf("GetLLMProvider() error = %v", err)
	}
	if imported.ACPAgentID != "gemini-cli" {
		t.Errorf("agente = %q, queria o do arquivo", imported.ACPAgentID)
	}
	if imported.Type != string(llm.ProviderACP) {
		t.Errorf("tipo = %q, queria %q", imported.Type, llm.ProviderACP)
	}
}

// Linha ilegível no banco não pode virar export mudo: o arquivo sairia sem os
// argumentos e a máquina de destino subiria o agente em outro modo.
func TestExportacaoFalhaComArgumentosIlegiveis(t *testing.T) {
	if _, err := exportProvider(&database.LLMProvider{
		ID: "cursor", Name: "Cursor", APIFormat: "acp", ACPArgs: "{quebrado",
	}); err == nil {
		t.Fatal("esperava falha ao exportar argumentos ilegíveis")
	}
}
