package portability

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"assistente/internal/credentials"
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
	// conversa falharia sem explicação. O aviso sai como código, para a tela
	// dizer isso no idioma de quem importou.
	if len(result.Warnings) != 1 {
		t.Fatalf("avisos inesperados: %v", messageCodes(result.Warnings))
	}
	aviso := findMessageByCode(t, result.Warnings, CodeACPCommandNotFound)
	requireParam(t, aviso, "providerId", "cursor")
	requireParam(t, aviso, "command", "agente-que-nao-existe-nesta-maquina")
	if !strings.Contains(aviso.Message, "não foi encontrado nesta máquina") {
		t.Errorf("texto de reserva = %q", aviso.Message)
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

	aviso, gerou := acpCommandWarning(ProviderExport{ID: "cursor", APIFormat: "acp", ACPCommand: executavel})
	if gerou {
		t.Errorf("aviso indevido: %q", aviso)
	}
}

func TestProvedorHTTPNaoGeraAvisoDeAgente(t *testing.T) {
	aviso, gerou := acpCommandWarning(ProviderExport{
		ID: "openai", APIFormat: "openai", BaseURL: "https://api.openai.com/v1",
	})
	if gerou {
		t.Errorf("aviso indevido: %q", aviso)
	}
}

func TestImportacaoRecusaProvedorACPSemComandoEAgenteEmProvedorHTTP(t *testing.T) {
	setupPortabilityTestDB(t)

	casos := []struct {
		nome     string
		provider ProviderExport
		codigo   string
	}{
		{
			nome:     "acp sem comando",
			provider: ProviderExport{ID: "cursor", Name: "Cursor", Type: "custom", APIFormat: "acp"},
			codigo:   CodeProviderACPMissingCommand,
		},
		{
			nome: "comando em provedor http",
			provider: ProviderExport{
				ID: "openai", Name: "OpenAI", Type: "openai", APIFormat: "openai",
				BaseURL: "https://api.openai.com/v1", ACPCommand: "cursor-agent",
			},
			codigo: CodeProviderACPOutsideACPFormat,
		},
		{
			nome: "argumentos em provedor http",
			provider: ProviderExport{
				ID: "openai", Name: "OpenAI", Type: "openai", APIFormat: "openai",
				BaseURL: "https://api.openai.com/v1", ACPArgs: []string{"acp"},
			},
			codigo: CodeProviderACPOutsideACPFormat,
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
			if result.Failed != 1 || len(result.Errors) != 1 || result.Errors[0].Code != caso.codigo {
				t.Fatalf("resultado inesperado: %+v", result)
			}
			requireParam(t, result.Errors[0], "providerId", caso.provider.ID)
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

// A referência do cofre viaja, ao contrário do ambiente literal: o que está
// guardado ali é o nome da variável e a entrada que a preenche, e nenhum dos
// dois é segredo (AEP-0086 D12).
func TestExportacaoLevaAReferenciaDoCofreENaoOSegredo(t *testing.T) {
	exportado, err := exportProvider(&database.LLMProvider{
		ID: "codex", Name: "Codex", Type: "acp", APIFormat: "acp",
		ACPCommand:       "codex-acp",
		ACPCredentialEnv: `{"OPENAI_API_KEY":"api.openai.com"}`,
	})
	if err != nil {
		t.Fatalf("exportProvider() error = %v", err)
	}
	if exportado.ACPCredentialEnv["OPENAI_API_KEY"] != "api.openai.com" {
		t.Fatalf("credenciais do cofre = %#v", exportado.ACPCredentialEnv)
	}

	raw, err := json.Marshal(exportado)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if !strings.Contains(string(raw), "acpCredentialEnv") {
		t.Errorf("a referência não saiu no arquivo: %s", raw)
	}
}

// Uma coluna quebrada não pode virar provedor exportado pela metade: um arquivo
// sem a referência levaria a pessoa a montar a máquina nova achando que a
// credencial estava lá.
func TestExportacaoFalhaComReferenciaDoCofreIlegivel(t *testing.T) {
	_, err := exportProvider(&database.LLMProvider{
		ID: "codex", Name: "Codex", Type: "acp", APIFormat: "acp",
		ACPCommand:       "codex-acp",
		ACPCredentialEnv: `{quebrado`,
	})
	if err == nil {
		t.Fatal("esperava falha na exportação da coluna ilegível")
	}
}

// O provedor entra mesmo sem a entrada no cofre — a configuração é legítima, e
// o que falta é local —, mas entra dizendo o que falta: sem o aviso, a primeira
// conversa falharia pedindo autenticação sem ninguém entender por quê.
func TestImportacaoAvisaQuandoAEntradaDoCofreNaoExisteAqui(t *testing.T) {
	setupPortabilityTestDB(t)
	credMgr := credentials.NewManagerWithStoreAndPersistence(
		[]byte("test-key-exactly-32-bytes-long!!"), credentials.NewDBStore(), true)
	if err := credMgr.RegisterPatternWithContext(portabilityTestCtx(), "api.anthropic.com", &credentials.AuthConfig{
		Type: "bearer", Token: "sk-daqui",
	}); err != nil {
		t.Fatalf("registrar a credencial existente: %v", err)
	}

	executavel, err := os.Executable()
	if err != nil {
		t.Skipf("sem caminho do executável de teste: %v", err)
	}
	file := &ExportFile{
		Version:    ExportVersion,
		ExportedAt: time.Now().UTC(),
		Resources: ExportResources{
			Providers: []ProviderExport{{
				ID: "codex", Name: "Codex", Type: "acp", APIFormat: "acp",
				// O comando existe aqui para o único aviso do resultado ser o
				// do cofre.
				ACPCommand: executavel,
				ACPCredentialEnv: map[string]string{
					"ANTHROPIC_API_KEY": "api.anthropic.com",
					"OPENAI_API_KEY":    "api.openai.com",
				},
			}},
		},
	}
	raw, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	result, err := ImportConversationsWithContext(portabilityTestCtx(), string(raw), credMgr, "")
	if err != nil {
		t.Fatalf("ImportConversations() error = %v", err)
	}
	if result.Imported != 1 || result.Failed != 0 {
		t.Fatalf("resultado inesperado: %+v", result)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("avisos = %v, esperado só o da entrada que falta", messageCodes(result.Warnings))
	}
	aviso := findMessageByCode(t, result.Warnings, CodeACPCredentialMissing)
	requireParam(t, aviso, "providerId", "codex")
	requireParam(t, aviso, "pattern", "api.openai.com")
	requireParam(t, aviso, "variable", "OPENAI_API_KEY")

	// E o provedor entrou com a referência inteira, para bastar cadastrar a
	// credencial que falta.
	importado, err := database.GetLLMProviderWithContext(portabilityTestCtx(), "codex")
	if err != nil {
		t.Fatalf("GetLLMProvider() error = %v", err)
	}
	if !strings.Contains(importado.ACPCredentialEnv, "OPENAI_API_KEY") {
		t.Errorf("referência do cofre = %q", importado.ACPCredentialEnv)
	}
}

// O arquivo não passa pelo serviço de provedores, então a conferência do par
// variável/cofre é feita duas vezes no código — e as duas precisam concordar,
// senão o que a tela recusa entra pela importação e quebra na leitura seguinte.
func TestAImportacaoConfereOParDoCofreComoODominioConfere(t *testing.T) {
	casos := []struct {
		nome  string
		pares map[string]string
	}{
		{nome: "par completo", pares: map[string]string{"OPENAI_API_KEY": "api.openai.com"}},
		{nome: "sem entrada do cofre", pares: map[string]string{"OPENAI_API_KEY": "  "}},
		{nome: "sem nome de variável", pares: map[string]string{"   ": "api.openai.com"}},
		{nome: "nome com igual", pares: map[string]string{"OPENAI=KEY": "api.openai.com"}},
		{nome: "nome com espaço", pares: map[string]string{"OPENAI KEY": "api.openai.com"}},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			cfg := llm.ProviderConfig{
				ID: "codex", Name: "Codex", APIFormat: llm.APIFormatACP,
				ACPCommand: "codex-acp", ACPCredentialEnv: copiaDosPares(caso.pares),
			}
			_, aqui := normalizedCredentialEnv(ProviderExport{
				ID: "codex", Name: "Codex", Type: "acp", APIFormat: "acp",
				ACPCommand: "codex-acp", ACPCredentialEnv: copiaDosPares(caso.pares),
			})
			noDominio := cfg.Validate()

			if (aqui == nil) != (noDominio == nil) {
				t.Fatalf("importação = %v, domínio = %v: as duas conferências divergiram", aqui, noDominio)
			}
		})
	}
}

func copiaDosPares(pares map[string]string) map[string]string {
	out := make(map[string]string, len(pares))
	for k, v := range pares {
		out[k] = v
	}
	return out
}

// Espaço nas pontas do padrão faria a busca no cofre não achar a entrada que
// está lá: some na importação, como já some no domínio.
func TestOPadraoDoCofreEntraAparadoNaImportacao(t *testing.T) {
	setupPortabilityTestDB(t)

	executavel, err := os.Executable()
	if err != nil {
		t.Skipf("sem caminho do executável de teste: %v", err)
	}
	file := &ExportFile{
		Version:    ExportVersion,
		ExportedAt: time.Now().UTC(),
		Resources: ExportResources{
			Providers: []ProviderExport{{
				ID: "codex", Name: "Codex", Type: "acp", APIFormat: "acp",
				ACPCommand:       executavel,
				ACPCredentialEnv: map[string]string{"OPENAI_API_KEY": "  api.openai.com  "},
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

	importado, err := database.GetLLMProviderWithContext(portabilityTestCtx(), "codex")
	if err != nil {
		t.Fatalf("GetLLMProvider() error = %v", err)
	}
	if importado.ACPCredentialEnv != `{"OPENAI_API_KEY":"api.openai.com"}` {
		t.Errorf("referência do cofre = %q, esperada sem espaços", importado.ACPCredentialEnv)
	}
}

// Sem cofre em mãos não dá para afirmar que a entrada falta. Avisar assim mesmo
// encheria de alarme falso a importação feita antes de o cofre abrir.
func TestSemCofreAImportacaoNaoInventaEntradaFaltando(t *testing.T) {
	avisos := acpCredentialWarnings(portabilityTestCtx(), nil, ProviderExport{
		ID: "codex", APIFormat: "acp", ACPCommand: "codex-acp",
		ACPCredentialEnv: map[string]string{"OPENAI_API_KEY": "api.openai.com"},
	})
	if len(avisos) != 0 {
		t.Errorf("avisos indevidos: %v", avisos)
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
