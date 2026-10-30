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

function translationFunction(language?: string) {
  const normalized = language?.trim().toLowerCase();
  if (normalized?.startsWith('pt')) return i18next.getFixedT('pt-BR');
  if (normalized?.startsWith('es')) return i18next.getFixedT('es');
  if (normalized?.startsWith('en')) return i18next.getFixedT('en');
  return i18next.t.bind(i18next);
}

export function translateProviderErrorMessage(message: string, language?: string): string | null {
  const alias = /^model_alias_error:v1:(empty_id|missing_target):(.*)$/.exec(message);
  if (alias) {
    let id: string;
    try { id = decodeURIComponent(alias[2]); } catch { return null; }
    const key = alias[1] === 'empty_id' ? 'emptyID' : 'missingTarget';
    return translationFunction(language)(`providerForm.aliases.${key}`, { id });
  }
  const parameter = /^parameter_error:v1:(temperature|top_p|max_output_tokens|reasoning_effort|context_window):(choice|number|minimum|maximum|required)(?::(-?\d+(?:\.\d+)?(?:e[+-]?\d+)?))?$/.exec(message);
  if (parameter) {
    const [, field, code, limit] = parameter;
    if ((code === 'minimum' || code === 'maximum') !== (limit !== undefined)) return null;
    return translationFunction(language)(`chat.errors.parameterErrors.${code}`, { field, limit });
  }
  const match = /^provider_error:v1:([a-z_]+):([a-z_.]*)(?::([1-5][0-9]{2}))?$/.exec(message);
  if (!match) return null;

  const [, category, rawField, status = ''] = match;
  const t = translationFunction(language);
  const categoryKey = categoryKeys[category];
  if (!categoryKey) return null;
  const field = supportedFields.has(rawField) ? rawField : '';
  const statusLabel = status ? ` (HTTP ${status})` : '';
  if (category === 'unsupported_parameter' && !field) {
    return t('chat.errors.providerErrors.unsupportedParameterUnknown', { statusLabel });
  }
  if (category === 'invalid_value' && !field) {
    return t('chat.errors.providerErrors.invalidValueUnknown', { statusLabel });
  }

  return t(`chat.errors.providerErrors.${categoryKey}`, { field, statusLabel });
}

const mcpStages: Record<string, string> = {
  list_tools: 'chat.errors.mcpStages.listTools',
  call: 'chat.errors.mcpStages.call',
  handshake: 'chat.errors.mcpStages.handshake',
  unknown: 'chat.errors.mcpStages.unknown',
};

export function translateErrorMarkers(text: string, language?: string): string {
  const withProviderErrors = text.replace(
    /(?:provider_error:v1:[a-z_]+:[a-z_.]*(?::[1-5][0-9]{2})?|parameter_error:v1:[a-z_]+:[a-z_]+(?::-?\d+(?:\.\d+)?(?:e[+-]?\d+)?)?)/g,
    (marker) => translateProviderErrorMessage(marker, language) ?? marker,
  );
  return withProviderErrors.replace(/mcp_error:v1:([^:\s]+):(list_tools|call|handshake|unknown)/g, (marker, encodedServer: string, stage: string) => {
    const stageKey = mcpStages[stage];
    if (!stageKey) return marker;
    let server = encodedServer;
    try {
      server = decodeURIComponent(encodedServer);
    } catch {
      return marker;
    }
    const t = translationFunction(language);
    return t('chat.errors.mcpFailure', { server, stage: t(stageKey) });
  });
}

// Chat history stores backend diagnostics in a small set of explicit formats.
// Restricting translation here prevents marker-like text in user messages or
// assistant code samples from being rewritten during Markdown rendering.
export function translateBackendErrorDiagnostic(text: string, language?: string): string {
  const agentFailurePrefix = 'Falha na resposta do agente: ';
  if (text.startsWith(agentFailurePrefix)) {
    const translated = translateProviderErrorMessage(text.slice(agentFailurePrefix.length), language);
    if (translated) {
      return `${translationFunction(language)('chat.errors.agentResponseFailed')} ${translated}`;
    }
  }
  if (/^mcp_error:v1:[^:\s]+:(list_tools|call|handshake|unknown)$/.test(text)) {
    return translateErrorMarkers(text, language);
  }
  const prefixedMcpDiagnostic = /^Falha na resposta do agente: (mcp_error:v1:[^:\s]+:(?:list_tools|call|handshake|unknown))$/.exec(text);
  if (prefixedMcpDiagnostic) {
    return translateErrorMarkers(prefixedMcpDiagnostic[1], language);
  }
  return text;
}
