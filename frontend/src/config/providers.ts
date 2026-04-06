/**
 * Metadados de provedores LLM: configuração de conexão e capacidades TTS.
 *
 * Centraliza informações que antes estavam espalhadas em heurísticas
 * (isTTSModel, isOpenAILike, standardModelPrefixes, FetchVoices fallback, etc.)
 *
 * Cada preset declara explicitamente:
 * - Configuração de conexão (URL, API key, API format)
 * - Capacidades de TTS (suporte, vozes estáticas, modelos, listagem dinâmica)
 */

/** Separador usado em IDs compostos "voiceId::model" */
export const COMPOSITE_VOICE_SEPARATOR = '::';

/** Cria um ID composto "voiceId::model" para o picker */
export function makeCompositeVoiceId(voiceId: string, model: string): string {
  return `${voiceId}${COMPOSITE_VOICE_SEPARATOR}${model}`;
}

/** Faz parse de um ID composto. Retorna null se não for composto. */
export function parseCompositeVoiceId(compositeId: string): { voiceId: string; model: string } | null {
  if (!compositeId.includes(COMPOSITE_VOICE_SEPARATOR)) return null;
  const [voiceId, model] = compositeId.split(COMPOSITE_VOICE_SEPARATOR);
  return { voiceId, model };
}

/** Voz estática com modelo TTS associado (ex: "Alloy HD" = voice alloy + model tts-1-hd) */
export interface StaticVoice {
  /** ID composto para o picker (ex: "alloy::tts-1-hd") */
  id: string;
  /** Nome exibido no picker (ex: "Alloy HD") */
  name: string;
  /** ID da voz para a API (ex: "alloy") */
  voiceId: string;
  /** Modelo TTS associado (ex: "tts-1", "tts-1-hd") */
  model: string;
  /** Provedor TTS (ex: "openai") */
  provider: string;
  /** Idioma da voz (ex: "multilingual") */
  language: string;
}

/** Capacidades TTS de um tipo de provedor */
export interface TTSCapabilities {
  /** Provedor suporta TTS via /audio/speech */
  supportsTTS: boolean;
  /** Vozes estáticas com modelo embutido (ex: OpenAI alloy + tts-1-hd) */
  staticVoices: StaticVoice[];
  /** Backend suporta listagem dinâmica de vozes/modelos via /v1/models */
  supportsDynamicVoiceListing: boolean;
}

/** Configuração de um tipo de provedor */
export interface ProviderPreset {
  label: string;
  defaultUrl: string;
  urlEditable: boolean;
  apiKeyRequired: boolean;
  testRequiresApiKey: boolean;
  helpText?: string;
  defaultModel?: string;
  apiFormat?: string;
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

/** Vozes OpenAI: cada voz gera uma entrada Standard (tts-1) e uma HD (tts-1-hd) */
const OPENAI_VOICES: StaticVoice[] = OPENAI_VOICE_NAMES.flatMap(v => [
  { id: makeCompositeVoiceId(v.id, 'tts-1'), voiceId: v.id, name: v.name, model: 'tts-1', provider: 'openai', language: 'multilingual' },
  { id: makeCompositeVoiceId(v.id, 'tts-1-hd'), voiceId: v.id, name: `${v.name} HD`, model: 'tts-1-hd', provider: 'openai', language: 'multilingual' },
]);

/** Capacidades TTS: sem suporte */
const NO_TTS: TTSCapabilities = {
  supportsTTS: false,
  staticVoices: [],
  supportsDynamicVoiceListing: false,
};

/** Capacidades TTS: OpenAI real (vozes+modelos combinados no picker) */
const OPENAI_TTS: TTSCapabilities = {
  supportsTTS: true,
  staticVoices: OPENAI_VOICES,
  supportsDynamicVoiceListing: false,
};

/** Capacidades TTS: provedor local com listagem dinâmica (LocalAI, etc.) */
const DYNAMIC_TTS: TTSCapabilities = {
  supportsTTS: true,
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
    defaultUrl: 'https://api.groq.com/openai',
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

/** Retorna o preset TTS para um tipo de provedor */
export function getTTSCapabilities(providerType: string): TTSCapabilities {
  return PROVIDER_CONFIG[providerType]?.tts ?? DYNAMIC_TTS;
}

/** Retorna true se o tipo de provedor suporta TTS */
export function providerSupportsTTS(providerType: string): boolean {
  return getTTSCapabilities(providerType).supportsTTS;
}
