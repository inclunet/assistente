package portability

import (
	"encoding/json"
	"os"
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

// Linha ilegível no banco não pode virar export mudo: o arquivo sairia sem os
// argumentos e a máquina de destino subiria o agente em outro modo.
func TestExportacaoFalhaComArgumentosIlegiveis(t *testing.T) {
	if _, err := exportProvider(&database.LLMProvider{
		ID: "cursor", Name: "Cursor", APIFormat: "acp", ACPArgs: "{quebrado",
	}); err == nil {
		t.Fatal("esperava falha ao exportar argumentos ilegíveis")
	}
}
