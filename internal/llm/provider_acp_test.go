package llm

import (
	"strings"
	"testing"
)

// Um agente ACP não tem endereço: quem o encontra é o comando. Exigir URL aqui
// impediria de registrar o provedor; não exigir comando deixaria passar uma
// configuração que não sobe nada (AEP-0084 D12).
func TestValidateDeProvedorACPTrocaURLPorComando(t *testing.T) {
	casos := []struct {
		nome     string
		cfg      ProviderConfig
		erro     bool
		contendo string
	}{
		{
			nome: "comando basta, sem url",
			cfg: ProviderConfig{
				ID: "cursor", Name: "Cursor", APIFormat: APIFormatACP,
				ACPCommand: "cursor-agent", ACPArgs: []string{"acp"},
			},
		},
		{
			nome: "sem comando não vale",
			cfg:  ProviderConfig{ID: "cursor", Name: "Cursor", APIFormat: APIFormatACP},
			erro: true, contendo: "acp sem comando",
		},
		{
			nome: "comando em provedor http é recusado",
			cfg: ProviderConfig{
				ID: "openai", Name: "OpenAI", BaseURL: "https://api.openai.com/v1",
				ACPCommand: "cursor-agent",
			},
			erro: true, contendo: "api_format",
		},
		{
			// Sem comando nada sobe, mas argumentos e ambiente guardados num
			// provedor HTTP são igualmente configuração sem leitor.
			nome: "argumentos soltos em provedor http são recusados",
			cfg: ProviderConfig{
				ID: "openai", Name: "OpenAI", BaseURL: "https://api.openai.com/v1",
				ACPArgs: []string{"acp"},
			},
			erro: true, contendo: "configuração de agente",
		},
		{
			nome: "ambiente solto em provedor http é recusado",
			cfg: ProviderConfig{
				ID: "openai", Name: "OpenAI", BaseURL: "https://api.openai.com/v1",
				ACPEnv: map[string]string{"CURSOR_LOG": "debug"},
			},
			erro: true, contendo: "configuração de agente",
		},
		{
			nome: "provedor http continua exigindo url",
			cfg:  ProviderConfig{ID: "openai", Name: "OpenAI"},
			erro: true, contendo: "base_url vazio",
		},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			cfg := caso.cfg
			err := cfg.Validate()
			if caso.erro {
				if err == nil {
					t.Fatalf("esperava recusa, obtive nil")
				}
				if !strings.Contains(err.Error(), caso.contendo) {
					t.Errorf("erro %q não menciona %q", err.Error(), caso.contendo)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate falhou: %v", err)
			}
		})
	}
}

// O par variável/cofre só serve se os dois lados estiverem lá e o nome couber
// num ambiente de processo. Meio par guardado em silêncio faria o agente subir
// sem a credencial que alguém acha que configurou (AEP-0086 D12).
func TestValidateConfereOParDeVariavelEEntradaDoCofre(t *testing.T) {
	casos := []struct {
		nome     string
		cfg      ProviderConfig
		erro     bool
		contendo string
	}{
		{
			nome: "par completo passa",
			cfg: ProviderConfig{
				ID: "codex", Name: "Codex", APIFormat: APIFormatACP,
				ACPCommand:       "codex-acp",
				ACPCredentialEnv: map[string]string{"OPENAI_API_KEY": "api.openai.com"},
			},
		},
		{
			nome: "variável sem entrada do cofre não vale",
			cfg: ProviderConfig{
				ID: "codex", Name: "Codex", APIFormat: APIFormatACP,
				ACPCommand:       "codex-acp",
				ACPCredentialEnv: map[string]string{"OPENAI_API_KEY": "   "},
			},
			erro: true, contendo: "de que entrada do cofre",
		},
		{
			nome: "entrada do cofre sem variável não vale",
			cfg: ProviderConfig{
				ID: "codex", Name: "Codex", APIFormat: APIFormatACP,
				ACPCommand:       "codex-acp",
				ACPCredentialEnv: map[string]string{"  ": "api.openai.com"},
			},
			erro: true, contendo: "sem o nome da variável",
		},
		{
			nome: "nome com igual não chega ao processo",
			cfg: ProviderConfig{
				ID: "codex", Name: "Codex", APIFormat: APIFormatACP,
				ACPCommand:       "codex-acp",
				ACPCredentialEnv: map[string]string{"OPENAI=KEY": "api.openai.com"},
			},
			erro: true, contendo: "nome de variável inválido",
		},
		{
			nome: "nome com espaço vira variável que o agente não acha",
			cfg: ProviderConfig{
				ID: "codex", Name: "Codex", APIFormat: APIFormatACP,
				ACPCommand:       "codex-acp",
				ACPCredentialEnv: map[string]string{"OPENAI KEY": "api.openai.com"},
			},
			erro: true, contendo: "nome de variável inválido",
		},
		{
			// O caminho HTTP não sobe processo nenhum, então não há ambiente
			// onde essa variável pudesse existir.
			nome: "cofre por variável em provedor http é recusado",
			cfg: ProviderConfig{
				ID: "openai", Name: "OpenAI", BaseURL: "https://api.openai.com/v1",
				ACPCredentialEnv: map[string]string{"OPENAI_API_KEY": "api.openai.com"},
			},
			erro: true, contendo: "configuração de agente",
		},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			cfg := caso.cfg
			err := cfg.Validate()
			if caso.erro {
				if err == nil {
					t.Fatalf("esperava recusa, obtive nil")
				}
				if !strings.Contains(err.Error(), caso.contendo) {
					t.Errorf("erro %q não menciona %q", err.Error(), caso.contendo)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate falhou: %v", err)
			}
		})
	}
}

// Espaço nas pontas do comando vira erro de execução difícil de ler; some aqui,
// como já some do resto.
func TestValidateAparaOComandoDoAgente(t *testing.T) {
	cfg := ProviderConfig{
		ID: "cursor", Name: "Cursor", APIFormat: APIFormatACP,
		ACPCommand: "  cursor-agent  ",
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate falhou: %v", err)
	}
	if cfg.ACPCommand != "cursor-agent" {
		t.Errorf("comando = %q, esperado sem espaços nas pontas", cfg.ACPCommand)
	}
}

// As capacidades que dependem de endpoint HTTP não podem responder sim para um
// agente local: nenhuma delas existe no ACP, e um sim aqui viraria chamada a
// uma URL que o provedor não tem.
func TestAgenteACPNaoAnunciaCapacidadeDeAPIHTTP(t *testing.T) {
	cfg := &ProviderConfig{
		ID: "cursor", Name: "Cursor", Type: ProviderCustom, APIFormat: APIFormatACP,
		ACPCommand: "cursor-agent",
	}
	if !cfg.IsACP() {
		t.Fatal("o provedor deveria ser reconhecido como agente ACP")
	}
	if cfg.GetAPIFormat() != APIFormatACP {
		t.Errorf("api_format efetivo = %q, esperado %q", cfg.GetAPIFormat(), APIFormatACP)
	}
	if cfg.SupportsTTS() || cfg.SupportsSTT() {
		t.Error("o agente não tem endpoint de áudio")
	}
	if SupportsAssistantPrefill(cfg) {
		t.Error("não há como injetar palavras na boca do agente pelo protocolo")
	}
	if SupportsExplicitCacheControl(cfg) {
		t.Error("cache_control é do formato Anthropic")
	}
}