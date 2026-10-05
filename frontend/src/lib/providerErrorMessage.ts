import i18next from 'i18next';

const categoryKeys: Record<string, string> = {
  unsupported_parameter: 'unsupportedParameter',
  invalid_value: 'invalidValue',
  invalid_request: 'invalidRequest',
  authentication: 'authentication',
  permission: 'permission',
  quota_or_rate_limit: 'quota',
  transient: 'transient',
  not_found: 'notFound',
  unrecognized: 'unrecognized',
};

const supportedFields = new Set([
  'max_tokens', 'max_completion_tokens', 'max_output_tokens',
  'temperature', 'top_p', 'reasoning_effort', 'reasoning.effort',
  'frequency_penalty', 'presence_penalty', 'seed', 'max_reasoning_tokens',
  'parallel_tool_calls', 'voice', 'speed', 'audio_format', 'language',
]);

export function translateProviderErrorMessage(message: string): string | null {
  const match = /^provider_error:v1:([a-z_]+):([a-z_.]*)(?::([1-5][0-9]{2}))?$/.exec(message);
  if (!match) return null;

  const [, category, rawField, status = ''] = match;
  const categoryKey = categoryKeys[category];
  if (!categoryKey) return null;
  const field = supportedFields.has(rawField) ? rawField : '';
  const statusLabel = status ? ` (HTTP ${status})` : '';
  if (category === 'unsupported_parameter' && !field) {
    return i18next.t('chat.errors.providerErrors.unsupportedParameterUnknown', { statusLabel });
  }
  if (category === 'invalid_value' && !field) {
    return i18next.t('chat.errors.providerErrors.invalidValueUnknown', { statusLabel });
  }

  return i18next.t(`chat.errors.providerErrors.${categoryKey}`, { field, statusLabel });
}
