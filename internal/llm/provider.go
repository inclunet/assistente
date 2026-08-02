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
	Headers           map[string]string `json:"headers,omitempty"`
	CredentialPattern string            `json:"credential_pattern,omitempty"`
	// AuthMode controla o tratamento de credenciais. Ver `AuthMode` para detalhes.
	// Vazio = inferido a partir de CredentialPattern (sem pattern → none, com pattern → required).
	AuthMode AuthMode `json:"auth_mode,omitempty"`

	// ACPCommand, ACPArgs e ACPEnv dizem como subir o agente quando o formato
	// é acp. Guardamos comando e argumentos em vez de um caminho mágico
	// porque a instalação varia por máquina: no Windows o Cursor entrega um
	// wrapper, e o binário versionado troca de lugar a cada atualização
	// (AEP-0084 D15). ACPEnv acrescenta variáveis ao ambiente herdado; não o
	// substitui.
	ACPCommand string            `json:"acp_command,omitempty"`
	ACPArgs    []string          `json:"acp_args,omitempty"`
	ACPEnv     map[string]string `json:"acp_env,omitempty"`
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

	if p.ID == "" {
		return fmt.Errorf("provider id vazio")
	}
	if p.Name == "" {
		return fmt.Errorf("provider name vazio")
	}
	if p.IsACP() {
		// O que endereça um agente é o comando, e é ele que passa a ser
		// obrigatório. Exigir URL aqui seria pedir um endereço que não existe.
		if p.ACPCommand == "" {
			return fmt.Errorf("provider acp sem comando: informe o executável do agente")
		}
		return nil
	}
	// Comando fora do formato acp seria configuração que ninguém lê: nenhum
	// caminho HTTP sobe processo. Recusar é melhor do que guardar em silêncio
	// algo que a pessoa configurou esperando efeito.
	if p.ACPCommand != "" {
		return fmt.Errorf("provider %s tem comando de agente mas api_format é %q; use %q", p.ID, p.GetAPIFormat(), APIFormatACP)
	}
	if p.BaseURL == "" {
		return fmt.Errorf("provider base_url vazio")
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
