package llm

import (
	"fmt"
	"net/url"
	"strings"
)

// ProviderType representa o tipo de provedor LLM (label de marca).
//
// NOTE: valores são strings para facilitar serialização
// e compatibilidade com configs.
type ProviderType string

const (
	ProviderOpenAI     ProviderType = "openai"
	ProviderClaude     ProviderType = "claude"
	ProviderGrok       ProviderType = "grok"
	ProviderDeepSeek   ProviderType = "deepseek"
	ProviderMistral    ProviderType = "mistral"
	ProviderGroq       ProviderType = "groq"
	ProviderTogether   ProviderType = "together"
	ProviderFireworks  ProviderType = "fireworks"
	ProviderPerplexity ProviderType = "perplexity"
	ProviderOllama     ProviderType = "ollama"
	ProviderLocalAI    ProviderType = "localai"
	ProviderLlamaCPP   ProviderType = "llamacpp"
	ProviderCustom     ProviderType = "custom"

	// ProviderACP é o tipo de todo agente de código local, sem exceção
	// (AEP-0086 D11). Não existe tipo por agente: qual deles é aquele provedor
	// está no ACPAgentID, e o que ele executa está no ACPCommand.
	//
	// Ter um tipo só é o que faz o app tratar 38 agentes do mesmo jeito. Os
	// dois que ele conhece desde o AEP-0084 — Cursor e Claude Code — não são
	// exceção nenhuma aqui: a única coisa que os distingue é o app saber
	// procurá-los no disco, e isso é resposta de uma pergunta feita para todos.
	ProviderACP ProviderType = "acp"
)

// AuthMode descreve o tratamento de autenticação para o provedor.
//
// Modos:
//
//   - AuthModeRequired (default): a credencial é obrigatória. Ausência
//     dispara erro explícito ("credencial gerenciada não resolvida"),
//     evitando que o request vá para upstream sem chave e gere um 401
//     opaco. Este é o comportamento esperado para provedores cloud
//     (OpenAI, Anthropic, etc.).
//
//   - AuthModeOptional: a credencial pode ou não existir. Quando existe,
//     o transport injeta normalmente (Authorization header). Quando
//     ausente, o request segue sem header — útil para provedores que
//     suportam autenticação opcional (LocalAI, LiteLLM standalone,
//     Ollama com proxy custom).
//
//   - AuthModeNone: o provedor explicitamente não usa Authorization.
//     O SDK não injeta o placeholder "managed-by-credential-transport",
//     e o transport remove qualquer header Authorization residual.
//     Para Ollama/llama.cpp puros que rejeitam headers desconhecidos.
//
// O default vazio é tratado como AuthModeRequired apenas para CredentialPattern != "".
// Se CredentialPattern == "" e AuthMode == "", trata como AuthModeNone (compatibilidade
// com configs existentes onde "sem pattern" significava "sem auth").
type AuthMode string

const (
	AuthModeRequired AuthMode = "required"
	AuthModeOptional AuthMode = "optional"
	AuthModeNone     AuthMode = "none"
)

// ReasoningContentMode descreve uma extensão opcional do protocolo
// OpenAI-compatible. É capability persistida, não heurística por marca ou URL
// (AEP-0097).
type ReasoningContentMode string

const (
	// ReasoningContentDisabled não captura nem reenvia reasoning_content.
	ReasoningContentDisabled ReasoningContentMode = "disabled"
	// ReasoningContentReplayWithTools captura a extensão do stream e a reenvia
	// apenas na continuação do mesmo turno quando a request contém tools.
	ReasoningContentReplayWithTools ReasoningContentMode = "replay_with_tools"
)

// APIFormat determina qual SDK/protocolo usar para comunicação com o provedor.
// Independente de ProviderType (que é apenas um label de marca).
//
// Formatos disponíveis:
//
//   - "openai" (APIFormatOpenAI / APIFormatOpenAICompatible):
//     Chat Completions API only (/v1/chat/completions).
//     Para provedores OpenAI-compatible: OpenRouter, Ollama, Groq, Together, etc.
//     NÃO suporta MCP nativo. Valor legado, default para configs sem api_format.
//
//   - "openai_responses" (APIFormatOpenAIResponses):
//     Responses API first (/v1/responses).
//     Para OpenAI real (api.openai.com). Suporta MCP nativo (type:mcp),
//     reasoning summaries, e todas as features modernas da plataforma OpenAI.
//     Inferido automaticamente quando BaseURL contém "api.openai.com".
//
//   - "anthropic": SDK anthropic-sdk-go. Suporta MCP nativo via Beta Messages API.
//
//   - "google": SDK google.golang.org/genai. NÃO suporta MCP nativo.
type APIFormat string

const (
	// APIFormatOpenAI é o formato Chat Completions only (/v1/chat/completions).
	// Valor wire: "openai". Usado para provedores OpenAI-compatible
	// (OpenRouter, Ollama, Groq, Together, LiteLLM, Azure, etc).
	// NÃO suporta MCP nativo nem Responses API.
	// Este é o valor default para configs legadas sem api_format explícito.
	APIFormatOpenAI APIFormat = "openai"

	// APIFormatOpenAICompatible é um alias semântico para APIFormatOpenAI.
	// Idêntico em valor wire ("openai"). Existe apenas para clareza em código novo —
	// deixa explícito que o provider usa Chat Completions por ser compatível/legado.
	APIFormatOpenAICompatible = APIFormatOpenAI

	// APIFormatOpenAIResponses é o formato Responses API first (/v1/responses).
	// Valor wire: "openai_responses". Para OpenAI real (api.openai.com).
	// Suporta MCP nativo (type:mcp), reasoning summaries, tool_choice, e
	// todas as features modernas. Inferido automaticamente quando BaseURL
	// contém "api.openai.com" e api_format não está definido.
	APIFormatOpenAIResponses APIFormat = "openai_responses"

	APIFormatAnthropic APIFormat = "anthropic" // anthropic-sdk-go SDK — suporta MCP nativo
	APIFormatGoogle    APIFormat = "google"    // google.golang.org/genai SDK — sem MCP nativo

	// APIFormatACP é o Agent Client Protocol sobre stdio (AEP-0084). Aqui o
	// "provedor" é um agente de código instalado na máquina, iniciado por
	// comando, que fala JSON-RPC pela entrada e saída padrão. Não tem URL nem
	// credencial guardada pelo app: quem autentica é o CLI do agente, fora
	// dele. É o único formato em que `BaseURL` não faz sentido.
	APIFormatACP APIFormat = "acp"
)

// ProviderConfig descreve um provedor LLM
// Usado pelo ProviderRegistry para inicialização do cliente.
type ProviderConfig struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	Type              ProviderType      `json:"type"`
	APIFormat         APIFormat         `json:"api_format,omitempty"`
	BaseURL           string            `json:"base_url"`
	Model             string            `json:"model,omitempty"`
	DefaultModel      string            `json:"default_model,omitempty"`
	IsDefault         bool              `json:"is_default,omitempty"`
	Timeout           int               `json:"timeout,omitempty"`
	// StreamIdleTimeoutSeconds limita quanto tempo um streaming SSE pode ficar
	// sem entregar evento nenhum antes de a tentativa ser cancelada (watchdog
	// de ociosidade). Zero = padrão de 60s. Não é teto sobre o stream total:
	// cada chunk recebido reinicia a contagem.
	StreamIdleTimeoutSeconds int               `json:"stream_idle_timeout_seconds,omitempty"`
	Headers                  map[string]string `json:"headers,omitempty"`
	CredentialPattern string            `json:"credential_pattern,omitempty"`
	// AuthMode controla o tratamento de credenciais. Ver `AuthMode` para detalhes.
	// Vazio = inferido a partir de CredentialPattern (sem pattern → none, com pattern → required).
	AuthMode AuthMode `json:"auth_mode,omitempty"`
	// ReasoningContentMode controla a extensão reasoning_content sem inferência
	// por Type, URL ou modelo. Vazio equivale a disabled (AEP-0097).
	ReasoningContentMode ReasoningContentMode `json:"reasoning_content_mode,omitempty"`

	// ACPCommand, ACPArgs e ACPEnv dizem como subir o agente quando o formato
	// é acp. Guardamos comando e argumentos em vez de um caminho mágico
	// porque a instalação varia por máquina: no Windows o Cursor entrega um
	// wrapper, e o binário versionado troca de lugar a cada atualização
	// (AEP-0084 D15). ACPEnv acrescenta variáveis ao ambiente herdado; não o
	// substitui.
	ACPCommand string            `json:"acp_command,omitempty"`
	ACPArgs    []string          `json:"acp_args,omitempty"`
	ACPEnv     map[string]string `json:"acp_env,omitempty"`

	// ACPCredentialEnv diz quais variáveis do ambiente do agente recebem uma
	// credencial do cofre, e de qual entrada dele: a chave é o nome da
	// variável, e o valor é o padrão de domínio que a resolve no momento de
	// subir o processo (AEP-0086 D12).
	//
	// O que fica guardado aqui é a referência, e não o segredo. Colar o
	// segredo já era possível pelo ACPEnv, e é justamente o que este campo
	// existe para evitar: ali ele mora numa coluna comum, em texto claro,
	// fora do cofre, sem rotação num ponto só e sem aparecer no inventário de
	// segredos. A referência não é segredo, então ela viaja no arquivo
	// exportado e pode ser lida pela tela.
	//
	// Ele não reaproveita o CredentialPattern de propósito: aquele campo diz
	// que credencial autentica as chamadas HTTP do provedor, e um agente não
	// faz nenhuma — tanto que a normalização o zera em todo provedor ACP.
	// Reusá-lo faria o mesmo campo querer dizer duas coisas conforme o
	// formato.
	//
	// Vazio — que é o padrão — é o ambiente do agente exatamente como sempre
	// foi: quem autentica é o CLI do agente, e o app não injeta nada.
	ACPCredentialEnv map[string]string `json:"acp_credential_env,omitempty"`

	// ACPAgentID diz qual agente do registro é este provedor (AEP-0086 D11).
	//
	// Ele fica aqui, e não no Type, porque o Type é rótulo de marca e este é o
	// identificador de uma linha de um documento de terceiro: se o registro
	// renomear um agente, o que envelhece é um campo de dado, e não o
	// vocabulário do app. Vazio é agente configurado à mão — caminho que o D3
	// mantém aberto —, e nesse caso o app não tem o que oferecer de catálogo.
	ACPAgentID string `json:"acp_agent_id,omitempty"`
}

// IsACP diz se o provedor é um agente ACP local, e não um serviço HTTP. É a
// pergunta que separa os caminhos que presumem URL — descoberta de modelos,
// health, credencial por hostname — dos que falam com um processo.
func (p *ProviderConfig) IsACP() bool {
	return p != nil && p.GetAPIFormat() == APIFormatACP
}

// EffectiveAuthMode devolve o AuthMode resolvido, aplicando a inferência
// de compat: configs antigas sem AuthMode tinham `CredentialPattern: ""`
// para indicar "sem auth" (caso ollama). Mantemos esse contrato.
func (p *ProviderConfig) EffectiveAuthMode() AuthMode {
	if p == nil {
		return AuthModeRequired
	}
	if p.AuthMode != "" {
		return p.AuthMode
	}
	switch p.Type {
	case ProviderLocalAI:
		return AuthModeOptional
	case ProviderOllama, ProviderLlamaCPP:
		return AuthModeNone
	}
	if strings.TrimSpace(p.CredentialPattern) == "" {
		return AuthModeNone
	}
	return AuthModeRequired
}

// EffectiveReasoningContentMode aplica o default seguro para providers antigos.
func (p *ProviderConfig) EffectiveReasoningContentMode() ReasoningContentMode {
	if p != nil && p.ReasoningContentMode == ReasoningContentReplayWithTools {
		return ReasoningContentReplayWithTools
	}
	return ReasoningContentDisabled
}

// AssistantPrefillCapability descreve, de forma explícita, até onde um
// provider/modelo suporta continuação via trailing assistant ("assistant
// prefill"). Modela os três casos previstos no AEP-0064 / Issue #124:
//
//   - PrefillUnsupported: o provider não aceita um trailing assistant como
//     prefill. A continuação explícita deve usar o fallback por mensagem de
//     usuário ("continue a partir deste texto: ...").
//
//   - PrefillWithoutThinking: aceita prefill apenas quando o thinking/reasoning
//     está desativado. É o caso de servidores locais (Qwen via LocalAI/Ollama/
//     llama.cpp) que rejeitam um trailing assistant quando `enable_thinking`
//     está ligado. Como o pipeline atual não desliga thinking só para
//     continuar, tratamos esse caso de forma conservadora: NÃO enviamos
//     prefill incondicionalmente; o fallback por mensagem de usuário é usado,
//     mantendo compatibilidade independentemente do estado do thinking.
//
//   - PrefillWithThinking: aceita prefill mesmo com thinking/reasoning ativo.
//     É o caso da OpenAI real via Responses API.
type AssistantPrefillCapability string

const (
	PrefillUnsupported     AssistantPrefillCapability = "unsupported"
	PrefillWithoutThinking AssistantPrefillCapability = "without_thinking"
	PrefillWithThinking    AssistantPrefillCapability = "with_thinking"
)

// PrefillCapability classifica o provider em um dos três casos de capacidade
// de assistant prefill. Mantém a postura conservadora existente: apenas a
// OpenAI real (Responses API) recebe prefill incondicional; servidores locais
// (LocalAI/Ollama/llama.cpp) são marcados como "só sem thinking" — e na
// prática usam o fallback por mensagem de usuário; os demais são tratados como
// não suportados.
func PrefillCapability(p *ProviderConfig) AssistantPrefillCapability {
	if p == nil {
		return PrefillUnsupported
	}
	switch {
	case p.Type == ProviderOpenAI && p.GetAPIFormat() == APIFormatOpenAIResponses:
		return PrefillWithThinking
	case p.Type == ProviderLocalAI, p.Type == ProviderOllama, p.Type == ProviderLlamaCPP:
		return PrefillWithoutThinking
	default:
		return PrefillUnsupported
	}
}

// SupportsAssistantPrefill informa se é seguro enviar trailing assistant
// intencional para continuação explícita SEM nenhum tratamento adicional.
// Só é verdadeiro quando o provider aceita prefill mesmo com thinking ativo
// (PrefillWithThinking) — hoje, apenas OpenAI real via Responses API.
//
// Quando retorna false (inclui o caso "só sem thinking" de Qwen/LocalAI), a
// continuação explícita deve recorrer ao fallback por mensagem de usuário em
// vez de injetar um trailing assistant.
func SupportsAssistantPrefill(p *ProviderConfig) bool {
	return PrefillCapability(p) == PrefillWithThinking
}

// SupportsExplicitCacheControl informa se é seguro alterar o payload com
// mecanismos explícitos de cache_control para este provider concreto.
//
// OpenAI-compatible, Mistral, Qwen/DashScope e gateways ficam fora daqui porque
// o suporte ativo deles já é tratado como provider hint (prompt_cache_key) ou
// exige contrato/capability específico do gateway. Gemini/Vertex expõe
// cachedContent, mas isso requer ciclo de vida persistido do recurso de cache,
// não uma marcação stateless na request.
func SupportsExplicitCacheControl(p *ProviderConfig) bool {
	if p == nil || p.GetAPIFormat() != APIFormatAnthropic {
		return false
	}
	return isAnthropicOfficialURL(p.BaseURL)
}

func isAnthropicOfficialURL(baseURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return false
	}
	return strings.EqualFold(parsed.Hostname(), "api.anthropic.com")
}

// GetAPIFormat retorna o api_format efetivo.
//
// Precedência:
//  1. APIFormat explícito (se definido) — respeitado incondicionalmente.
//  2. Inferência por URL: api.openai.com → APIFormatOpenAIResponses.
//  3. Default conservador: APIFormatOpenAI (Chat Completions only).
//
// A inferência por URL garante que providers OpenAI reais criados antes
// da introdução de api_format usem automaticamente a Responses API,
// sem exigir migração manual de configs existentes.
func (p *ProviderConfig) GetAPIFormat() APIFormat {
	if p.APIFormat != "" {
		switch p.Type {
		case ProviderLocalAI, ProviderOllama, ProviderLlamaCPP:
			if p.APIFormat == APIFormatOpenAIResponses {
				return APIFormatOpenAI
			}
		}
		return p.APIFormat
	}
	switch p.Type {
	case ProviderLocalAI, ProviderOllama, ProviderLlamaCPP:
		return APIFormatOpenAI
	}
	if isOpenAIRealURL(p.BaseURL) {
		return APIFormatOpenAIResponses
	}
	return APIFormatOpenAI
}

// isOpenAIRealURL retorna true se a URL aponta para a API oficial da OpenAI.
func isOpenAIRealURL(baseURL string) bool {
	normalized := strings.ToLower(strings.TrimSuffix(baseURL, "/"))
	return strings.Contains(normalized, "api.openai.com")
}

// Validate verifica se o ProviderConfig é válido
func (p *ProviderConfig) Validate() error {
	if p == nil {
		return fmt.Errorf("provider config nil")
	}
	p.ID = strings.TrimSpace(p.ID)
	p.Name = strings.TrimSpace(p.Name)
	p.BaseURL = strings.TrimSpace(p.BaseURL)
	p.CredentialPattern = strings.TrimSpace(p.CredentialPattern)
	p.ACPCommand = strings.TrimSpace(p.ACPCommand)
	p.ReasoningContentMode = ReasoningContentMode(strings.TrimSpace(string(p.ReasoningContentMode)))

	if p.ID == "" {
		return fmt.Errorf("provider id vazio")
	}
	if p.Name == "" {
		return fmt.Errorf("provider name vazio")
	}
	switch p.ReasoningContentMode {
	case "", ReasoningContentDisabled, ReasoningContentReplayWithTools:
	default:
		return fmt.Errorf("provider %s tem reasoning_content_mode inválido: %q", p.ID, p.ReasoningContentMode)
	}
	if p.IsACP() {
		// O que endereça um agente é o comando, e é ele que passa a ser
		// obrigatório. Exigir URL aqui seria pedir um endereço que não existe.
		if p.ACPCommand == "" {
			return fmt.Errorf("provider acp sem comando: informe o executável do agente")
		}
		return p.validateCredentialEnv()
	}
	// Configuração de agente fora do formato acp é configuração que ninguém
	// lê: nenhum caminho HTTP sobe processo, e argumentos ou variáveis soltos
	// enganam tanto quanto o comando. Recusar é melhor do que guardar em
	// silêncio algo que a pessoa configurou esperando efeito.
	if p.ACPCommand != "" || len(p.ACPArgs) > 0 || len(p.ACPEnv) > 0 || len(p.ACPCredentialEnv) > 0 {
		return fmt.Errorf("provider %s tem configuração de agente mas api_format é %q; use %q", p.ID, p.GetAPIFormat(), APIFormatACP)
	}
	if p.BaseURL == "" {
		return fmt.Errorf("provider base_url vazio")
	}
	return nil
}

// validateCredentialEnv recusa par de variável e padrão do cofre que não dá
// para cumprir (AEP-0086 D12).
//
// Os dois lados são obrigatórios porque nenhum dos dois sozinho descreve o que
// fazer: variável sem padrão é uma credencial que não existe, e padrão sem
// variável é um segredo decifrado sem lugar para ir. Guardar qualquer um dos
// dois em silêncio faria o agente subir sem a credencial que alguém acha que
// configurou — e é justamente o que o D12 pede para não acontecer.
//
// O nome da variável também é conferido: `=` e byte zero não passam pelo
// `exec`, e um nome com espaço vira uma variável que o agente não encontra.
//
// O padrão é aparado aqui, como o comando é: a busca no cofre compara a string
// inteira, e um espaço colado num "api.openai.com " faria a entrada certa
// parecer inexistente na hora de subir o agente.
func (p *ProviderConfig) validateCredentialEnv() error {
	if len(p.ACPCredentialEnv) == 0 {
		return nil
	}
	for name, pattern := range p.ACPCredentialEnv {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("provider %s tem credencial do cofre sem o nome da variável de ambiente", p.ID)
		}
		if strings.ContainsAny(name, "=\x00 \t\r\n") {
			return fmt.Errorf("provider %s tem nome de variável inválido para a credencial do cofre: %q", p.ID, name)
		}
		trimmed := strings.TrimSpace(pattern)
		if trimmed == "" {
			return fmt.Errorf("provider %s: a variável %s não diz de que entrada do cofre vem a credencial", p.ID, name)
		}
		p.ACPCredentialEnv[name] = trimmed
	}
	return nil
}

// SupportsTTS retorna true se o SDK do provider suporta síntese de voz (TTS).
// Atualmente apenas o SDK OpenAI (formatos openai e openai_responses) tem endpoint /audio/speech.
func (p *ProviderConfig) SupportsTTS() bool {
	f := p.GetAPIFormat()
	return f == APIFormatOpenAI || f == APIFormatOpenAIResponses
}

// SupportsSTT retorna true se o SDK do provider suporta transcrição de voz (STT/Whisper).
// Atualmente apenas o SDK OpenAI (formatos openai e openai_responses) tem endpoint /audio/transcriptions.
func (p *ProviderConfig) SupportsSTT() bool {
	f := p.GetAPIFormat()
	return f == APIFormatOpenAI || f == APIFormatOpenAIResponses
}
