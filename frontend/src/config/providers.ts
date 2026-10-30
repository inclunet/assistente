/** Definições locais de tipos e formatos de conexão. Modelos e vozes TTS vêm do catálogo. */

/** Configuração de um tipo de provedor */
export interface ProviderPreset {
  label: string;
  /**
   * Chave de tradução do rótulo, para o tipo cujo nome é uma frase e não uma
   * marca. "OpenAI" se escreve igual em qualquer idioma; "agente de código",
   * não — e o `label` acima vira o recurso de quando a chave não estiver nos
   * locales.
   */
  labelKey?: string;
  defaultUrl: string;
  urlEditable: boolean;
  apiKeyRequired: boolean;
  testRequiresApiKey: boolean;
  helpText?: string;
  defaultModel?: string;
  apiFormat?: string;
  reasoningContentMode?: string;
}


export const PROVIDER_CONFIG: Record<string, ProviderPreset> = {
  // --- Commercial API providers ---
  openai: {
    label: 'OpenAI',
    defaultUrl: 'https://api.openai.com/v1',
    urlEditable: false,
    apiKeyRequired: true,
    testRequiresApiKey: true,
    helpText: 'Get your API key from https://platform.openai.com/api-keys',
    defaultModel: 'gpt-4o-mini',
    apiFormat: 'openai_responses',
  },
  anthropic: {
    label: 'Anthropic',
    defaultUrl: 'https://api.anthropic.com',
    urlEditable: false,
    apiKeyRequired: true,
    testRequiresApiKey: true,
    helpText: 'Get your API key from https://console.anthropic.com',
    defaultModel: 'claude-sonnet-4-20250514',
    apiFormat: 'anthropic',
  },
  google: {
    label: 'Google (Gemini)',
    defaultUrl: 'https://generativelanguage.googleapis.com/v1beta/openai/',
    urlEditable: false,
    apiKeyRequired: true,
    testRequiresApiKey: true,
    helpText: 'Get your API key from https://aistudio.google.com',
    defaultModel: 'gemini-2.0-flash',
    apiFormat: 'google',
  },

  // --- OpenAI-compatible commercial ---
  openrouter: {
    label: 'OpenRouter',
    defaultUrl: 'https://openrouter.ai/api/v1',
    urlEditable: false,
    apiKeyRequired: true,
    testRequiresApiKey: true,
    helpText: 'Get your API key from https://openrouter.ai/keys',
    apiFormat: 'openai',
  },
  deepseek: {
    label: 'DeepSeek',
    defaultUrl: 'https://api.deepseek.com/v1',
    urlEditable: false,
    apiKeyRequired: true,
    testRequiresApiKey: true,
    helpText: 'Get your API key from https://platform.deepseek.com',
    defaultModel: 'deepseek-chat',
    apiFormat: 'openai',
    reasoningContentMode: 'replay_with_tools',
  },
  xai: {
    label: 'xAI (Grok)',
    defaultUrl: 'https://api.x.ai/v1',
    urlEditable: false,
    apiKeyRequired: true,
    testRequiresApiKey: true,
    helpText: 'Get your API key from https://console.x.ai',
    defaultModel: 'grok-3-mini',
    apiFormat: 'openai',
  },
  mistral: {
    label: 'Mistral AI',
    defaultUrl: 'https://api.mistral.ai/v1',
    urlEditable: false,
    apiKeyRequired: true,
    testRequiresApiKey: true,
    helpText: 'Get your API key from https://console.mistral.ai',
    defaultModel: 'mistral-small-latest',
    apiFormat: 'openai',
  },
  groq: {
    label: 'Groq',
    defaultUrl: 'https://api.groq.com/openai/v1',
    urlEditable: false,
    apiKeyRequired: true,
    testRequiresApiKey: true,
    helpText: 'Get your API key from https://console.groq.com',
    defaultModel: 'llama-3.3-70b-versatile',
    apiFormat: 'openai',
  },
  together: {
    label: 'Together AI',
    defaultUrl: 'https://api.together.xyz/v1',
    urlEditable: false,
    apiKeyRequired: true,
    testRequiresApiKey: true,
    helpText: 'Get your API key from https://api.together.ai/settings/api-keys',
    apiFormat: 'openai',
  },
  fireworks: {
    label: 'Fireworks AI',
    defaultUrl: 'https://api.fireworks.ai/inference/v1',
    urlEditable: false,
    apiKeyRequired: true,
    testRequiresApiKey: true,
    helpText: 'Get your API key from https://fireworks.ai/account/api-keys',
    apiFormat: 'openai',
  },
  perplexity: {
    label: 'Perplexity',
    defaultUrl: 'https://api.perplexity.ai',
    urlEditable: false,
    apiKeyRequired: true,
    testRequiresApiKey: true,
    helpText: 'Get your API key from https://www.perplexity.ai/settings/api',
    defaultModel: 'sonar',
    apiFormat: 'openai',
  },
  cohere: {
    label: 'Cohere',
    defaultUrl: 'https://api.cohere.ai/compatibility/v1',
    urlEditable: false,
    apiKeyRequired: true,
    testRequiresApiKey: true,
    helpText: 'Get your API key from https://dashboard.cohere.ai',
    apiFormat: 'openai',
  },

  // --- Local/self-hosted ---
  ollama: {
    label: 'Ollama (local)',
    defaultUrl: 'http://localhost:11434/v1',
    urlEditable: true,
    apiKeyRequired: false,
    testRequiresApiKey: false,
    helpText: 'Running locally. You can change the URL if using a different host/port',
    apiFormat: 'openai',
  },
  localai: {
    label: 'LocalAI',
    defaultUrl: 'http://localhost:8080',
    urlEditable: true,
    apiKeyRequired: false,
    testRequiresApiKey: false,
    helpText: 'Running locally. Configure URL and optional API key if needed',
    apiFormat: 'openai',
  },
  llamacpp: {
    label: 'llama.cpp (server)',
    defaultUrl: 'http://localhost:8080',
    urlEditable: true,
    apiKeyRequired: false,
    testRequiresApiKey: false,
    helpText: 'llama.cpp server (--api-server). API key is not required by default.',
    apiFormat: 'openai',
  },

  // --- Proxy ---
  litellm: {
    label: 'LiteLLM Proxy',
    defaultUrl: 'http://localhost:4000',
    urlEditable: true,
    apiKeyRequired: true,
    testRequiresApiKey: true,
    helpText: 'LiteLLM proxy server. Requires URL and API key',
    apiFormat: 'openai',
  },

  // --- Agentes de código locais (ACP, AEP-0084) ---
  // Uma entrada só para os 38 agentes do catálogo, e não uma por agente
  // (AEP-0086 D11): qual deles é o provedor se escolhe no catálogo, que tem
  // busca e sabe o que está instalado nesta máquina. Enumerá-los aqui, ao lado
  // de OpenAI e Ollama, seria uma lista que ninguém consegue ler e que
  // envelheceria a cada versão do registro.
  acp: {
    // Só o gênero da coisa: o nome do agente escolhido aparece no formulário,
    // vindo do registro.
    label: 'Agente de código (ACP)',
    labelKey: 'providerForm.agent.typeLabel',
    // Um agente não tem endereço: o que o endereça é o comando dele, e é o
    // formulário do agente que pede isso no lugar de URL e chave.
    defaultUrl: '',
    urlEditable: false,
    apiKeyRequired: false,
    testRequiresApiKey: false,
    // Sem `helpText`: ele só aparece como descrição do campo de URL, que
    // agente não tem. Quem explica o que instalar é o formulário do agente,
    // em texto traduzido.
    apiFormat: 'acp',
  },

  // --- Custom ---
  custom: {
    label: 'Custom',
    defaultUrl: '',
    urlEditable: true,
    apiKeyRequired: true,
    testRequiresApiKey: true,
    helpText: 'Configure your custom LLM provider',
  },
};

/**
 * Formato de API dos provedores que são agentes de código locais falando ACP
 * (AEP-0084). Serve de fonte única para os caminhos que precisam se comportar
 * diferente: não há URL para validar, credencial para guardar nem endpoint de
 * modelos para consultar.
 */
export const AGENT_API_FORMAT = 'acp';

/** Diz se o tipo de provedor é um agente de código local, e não um serviço HTTP. */
export function providerIsAgent(providerType: string): boolean {
  return PROVIDER_CONFIG[providerType]?.apiFormat === AGENT_API_FORMAT;
}
