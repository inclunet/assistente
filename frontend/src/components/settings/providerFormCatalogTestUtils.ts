import { apidto, llmcatalog } from '@wailsjs/go/models';

const profiles: Array<[string, string, string, string, string, string]> = [
  ['openai', 'OpenAI', 'openai', 'openai_responses', 'https://api.openai.com/v1', 'bearer_api_key'],
  ['anthropic', 'Anthropic', 'anthropic', 'anthropic', 'https://api.anthropic.com', 'api_key_header'],
  ['google-gemini', 'Google Gemini', 'google', 'google', 'https://generativelanguage.googleapis.com/v1beta', 'api_key_header'],
  ['ollama', 'Ollama', 'ollama', 'openai', 'http://localhost:11434/v1', 'none'],
  ['localai-openai-compatible', 'LocalAI', 'localai', 'openai', 'http://localhost:8080/v1', 'optional_api_key'],
  ['litellm-openai-proxy', 'LiteLLM Proxy', 'litellm', 'openai', 'http://localhost:4000/v1', 'optional_api_key'],
  ['deepseek', 'DeepSeek', 'deepseek', 'openai', 'https://api.deepseek.com/v1', 'bearer_api_key'],
  ['xai', 'xAI', 'xai', 'openai', 'https://api.x.ai/v1', 'bearer_api_key'],
  ['mistral', 'Mistral', 'mistral', 'openai', 'https://api.mistral.ai/v1', 'bearer_api_key'],
  ['groq', 'Groq', 'groq', 'openai', 'https://api.groq.com/openai/v1', 'bearer_api_key'],
  ['together-ai', 'Together AI', 'together', 'openai', 'https://api.together.xyz/v1', 'bearer_api_key'],
  ['fireworks-ai', 'Fireworks AI', 'fireworks', 'openai', 'https://api.fireworks.ai/inference/v1', 'bearer_api_key'],
  ['perplexity', 'Perplexity', 'perplexity', 'openai', 'https://api.perplexity.ai', 'bearer_api_key'],
  ['cohere', 'Cohere', 'cohere', 'openai', 'https://api.cohere.ai/compatibility/v1', 'bearer_api_key'],
  ['openrouter', 'OpenRouter', 'openrouter', 'openai', 'https://openrouter.ai/api/v1', 'bearer_api_key'],
  ['llamacpp', 'llama.cpp', 'llamacpp', 'openai', 'http://localhost:8080/v1', 'none'],
  ['custom-openai-compatible', 'Custom', 'custom', 'openai', '', 'optional_api_key'],
  ['fixed-model-test', 'OpenAI com modelo fixo', 'openai', 'openai', 'https://api.openai.com/v1', 'bearer_api_key'],
  ['editable-model-test', 'OpenAI com modelo inicial editável', 'openai', 'openai', 'https://api.openai.com/v1', 'bearer_api_key'],
  ['editable-url-openai-test', 'OpenAI compatível com URL editável', 'openai', 'openai', 'https://api.openai.com/v1', 'none'],
  ['retired-profile-test', 'Perfil descontinuado', 'openai', 'openai', 'https://retired.example/v1', 'bearer_api_key'],
];

export const providerFormCatalogResponse = {
  document: llmcatalog.Document.createFrom({
    servers: profiles.map(([id, name, provider_family, api_format, base_url, authMode]) => ({
      id, name, provider_family, api_format, base_url, status: id === 'retired-profile-test' ? 'deprecated' : 'active', group: 'hosted',
      adapter_id: api_format, auth: { mode: authMode, required: ['bearer_api_key', 'api_key_header'].includes(authMode) },
      ...(['ollama', 'cohere'].includes(id) ? { endpoints: { chat: { path: '/chat/completions' } } } : {}),
      ...(id === 'editable-model-test' ? { default_model_id: '' } : {}),
      ...(id === 'openai' ? { signup_url: 'https://platform.openai.com/', api_key_url: 'https://platform.openai.com/api-keys' } : {}),
      fields: {
        base_url: { policy: ['openai', 'anthropic', 'google-gemini', 'deepseek', 'xai', 'mistral', 'groq', 'together-ai', 'fireworks-ai', 'perplexity', 'cohere', 'openrouter', 'fixed-model-test', 'retired-profile-test'].includes(id) ? 'fixed' : 'editable', default: base_url },
        api_format: { policy: id === 'custom-openai-compatible' ? 'editable' : 'fixed', default: api_format },
        'auth.mode': { policy: id === 'custom-openai-compatible' ? 'editable' : 'fixed', default: authMode },
        default_model_id: id === 'fixed-model-test' || id === 'retired-profile-test'
          ? { policy: 'fixed', default: 'gpt-4o-mini' }
          : id === 'editable-model-test'
          ? { policy: 'editable', default: 'gpt-4o-mini' }
          : id === 'deepseek'
          ? { policy: 'choice', default: 'deepseek-chat', choices: ['deepseek-chat', 'deepseek-reasoner'] }
          : { policy: 'editable' },
      },
    })).map(server => llmcatalog.Server.createFrom(server)),
    models: [],
    exposures: [],
    schema_version: 1,
    catalog_version: 'test',
    updated_at: '2026-10-17T00:00:00Z',
  }),
};

export const providerFormCatalogAPIResponse = apidto.LLMCatalogResponse.createFrom(providerFormCatalogResponse);

export const providerFormCatalogRefreshingAPIResponse = apidto.LLMCatalogResponse.createFrom({
  document: llmcatalog.Document.createFrom({
    servers: [], models: [], exposures: [], schema_version: 1,
    catalog_version: 'test', updated_at: '2026-10-17T00:00:00Z',
  }),
  refreshing: true,
});
