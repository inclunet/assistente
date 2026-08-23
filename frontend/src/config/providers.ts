/**
 * Metadados de provedores LLM: configuração de conexão e capacidades TTS.
 *
 * Centraliza informações que antes estavam espalhadas em heurísticas
 * (isTTSModel, isOpenAILike, standardModelPrefixes, etc.)
 *
 * Cada preset declara explicitamente:
 * - Configuração de conexão (URL, API key, API format)
 * - Capacidades de TTS (suporte, modelos, vozes, listagem dinâmica)
 */

import type { TTSSelectionMode } from '../services/tts/types';

/** Modelo TTS estático conhecido. */
export interface StaticTTSModel {
  id: string;
  name: string;
  provider: string;
  selectionMode: TTSSelectionMode;
  description?: string;
}

/** Voz estática associada a modelos que usam voz separada. */
export interface StaticVoice {
  /** ID da voz para a API (ex: "alloy") */
  id: string;
  /** Nome exibido no picker (ex: "Alloy HD") */
  name: string;
  /** Provedor TTS (ex: "openai") */
  provider: string;
  /** Idioma da voz (ex: "multilingual") */
  language: string;
}

/** Capacidades TTS de um tipo de provedor */
export interface TTSCapabilities {
  /** Provedor suporta TTS via /audio/speech */
  supportsTTS: boolean;
  /** Modelos estáticos conhecidos para provedores sem listagem dinâmica no backend. */
  staticModels: StaticTTSModel[];
  /** Vozes estáticas para modelos que exigem voz separada. */
  staticVoices: StaticVoice[];
  /** Backend suporta listagem dinâmica de modelos via /v1/models */
  supportsDynamicVoiceListing: boolean;
}

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
  tts: TTSCapabilities;
}

/** Nomes base das vozes OpenAI para gerar variantes Standard/HD */
const OPENAI_VOICE_NAMES = [
  { id: 'alloy', name: 'Alloy' },
  { id: 'ash', name: 'Ash' },
  { id: 'ballad', name: 'Ballad' },
  { id: 'coral', name: 'Coral' },
  { id: 'echo', name: 'Echo' },
  { id: 'fable', name: 'Fable' },
  { id: 'nova', name: 'Nova' },
  { id: 'onyx', name: 'Onyx' },
  { id: 'sage', name: 'Sage' },
  { id: 'shimmer', name: 'Shimmer' },
  { id: 'verse', name: 'Verse' },
];

const OPENAI_MODELS: StaticTTSModel[] = [
  { id: 'tts-1', name: 'tts-1', provider: 'openai', selectionMode: 'model_and_voice' },
  { id: 'tts-1-hd', name: 'tts-1-hd', provider: 'openai', selectionMode: 'model_and_voice' },
  { id: 'gpt-4o-mini-tts', name: 'gpt-4o-mini-tts', provider: 'openai', selectionMode: 'model_and_voice' },
];

const OPENAI_VOICES: StaticVoice[] = OPENAI_VOICE_NAMES.map(v => ({
  id: v.id,
  name: v.name,
  provider: 'openai',
  language: 'multilingual',
}));

/** Capacidades TTS: sem suporte */
const NO_TTS: TTSCapabilities = {
  supportsTTS: false,
  staticModels: [],
  staticVoices: [],
  supportsDynamicVoiceListing: false,
};

/** Capacidades TTS: OpenAI real (modelo e voz separados) */
const OPENAI_TTS: TTSCapabilities = {
  supportsTTS: true,
  staticModels: OPENAI_MODELS,
  staticVoices: OPENAI_VOICES,
  supportsDynamicVoiceListing: false,
};

/** Capacidades TTS: provedor local com listagem dinâmica (LocalAI, etc.) */
const DYNAMIC_TTS: TTSCapabilities = {
  supportsTTS: true,
  staticModels: [],
  staticVoices: [],
  supportsDynamicVoiceListing: true,
};

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
    tts: OPENAI_TTS,
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
    tts: NO_TTS,
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
    tts: NO_TTS,
  },

  // --- OpenAI-compatible commercial ---
  openrouter: {
    label: 'OpenRouter',
    defaultUrl: 'https://openrouter.ai/api',
    urlEditable: false,
    apiKeyRequired: true,
    testRequiresApiKey: true,
    helpText: 'Get your API key from https://openrouter.ai/keys',
    apiFormat: 'openai',
    tts: NO_TTS,
  },
  deepseek: {
    label: 'DeepSeek',
    defaultUrl: 'https://api.deepseek.com',
    urlEditable: false,
    apiKeyRequired: true,
    testRequiresApiKey: true,
    helpText: 'Get your API key from https://platform.deepseek.com',
    defaultModel: 'deepseek-chat',
    apiFormat: 'openai',
    reasoningContentMode: 'replay_with_tools',
    tts: NO_TTS,
  },
  xai: {
    label: 'xAI (Grok)',
    defaultUrl: 'https://api.x.ai',
    urlEditable: false,
    apiKeyRequired: true,
    testRequiresApiKey: true,
    helpText: 'Get your API key from https://console.x.ai',
    defaultModel: 'grok-3-mini',
    apiFormat: 'openai',
    tts: NO_TTS,
  },
  mistral: {
    label: 'Mistral AI',
    defaultUrl: 'https://api.mistral.ai',
    urlEditable: false,
    apiKeyRequired: true,
    testRequiresApiKey: true,
    helpText: 'Get your API key from https://console.mistral.ai',
    defaultModel: 'mistral-small-latest',
    apiFormat: 'openai',
    tts: NO_TTS,
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
    tts: NO_TTS,
  },
  together: {
    label: 'Together AI',
    defaultUrl: 'https://api.together.xyz',
    urlEditable: false,
    apiKeyRequired: true,
    testRequiresApiKey: true,
    helpText: 'Get your API key from https://api.together.ai/settings/api-keys',
    apiFormat: 'openai',
    tts: NO_TTS,
  },
  fireworks: {
    label: 'Fireworks AI',
    defaultUrl: 'https://api.fireworks.ai/inference',
    urlEditable: false,
    apiKeyRequired: true,
    testRequiresApiKey: true,
    helpText: 'Get your API key from https://fireworks.ai/account/api-keys',
    apiFormat: 'openai',
    tts: NO_TTS,
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
    tts: NO_TTS,
  },
  cohere: {
    label: 'Cohere',
    defaultUrl: 'https://api.cohere.ai',
    urlEditable: false,
    apiKeyRequired: true,
    testRequiresApiKey: true,
    helpText: 'Get your API key from https://dashboard.cohere.ai',
    apiFormat: 'openai',
    tts: NO_TTS,
  },

  // --- Local/self-hosted ---
  ollama: {
    label: 'Ollama (local)',
    defaultUrl: 'http://localhost:11434',
    urlEditable: true,
    apiKeyRequired: false,
    testRequiresApiKey: false,
    helpText: 'Running locally. You can change the URL if using a different host/port',
    apiFormat: 'openai',
    tts: NO_TTS,
  },
  localai: {
    label: 'LocalAI',
    defaultUrl: 'http://localhost:8080',
    urlEditable: true,
    apiKeyRequired: false,
    testRequiresApiKey: false,
    helpText: 'Running locally. Configure URL and optional API key if needed',
    apiFormat: 'openai',
    tts: DYNAMIC_TTS,
  },
  llamacpp: {
    label: 'llama.cpp (server)',
    defaultUrl: 'http://localhost:8080',
    urlEditable: true,
    apiKeyRequired: false,
    testRequiresApiKey: false,
    helpText: 'llama.cpp server (--api-server). API key is not required by default.',
    apiFormat: 'openai',
    tts: NO_TTS,
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
    tts: DYNAMIC_TTS,
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
    tts: NO_TTS,
  },

  // --- Custom ---
  custom: {
    label: 'Custom',
    defaultUrl: '',
    urlEditable: true,
    apiKeyRequired: true,
    testRequiresApiKey: true,
    helpText: 'Configure your custom LLM provider',
    tts: DYNAMIC_TTS,
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

/** Retorna o preset TTS para um tipo de provedor */
export function getTTSCapabilities(providerType: string): TTSCapabilities {
  return PROVIDER_CONFIG[providerType]?.tts ?? DYNAMIC_TTS;
}

/** Retorna true se o tipo de provedor suporta TTS */
export function providerSupportsTTS(providerType: string): boolean {
  return getTTSCapabilities(providerType).supportsTTS;
}
