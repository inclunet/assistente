import { useTranslation } from 'react-i18next';
import { LLMProviderPicker } from '../pickers/LLMProviderPicker';
import { ModelPicker } from '../pickers/ModelPicker';
import { RangeSlider } from '../ui/RangeSlider';

interface PromptCacheValue {
  enabled?: boolean;
  provider_hints?: boolean;
  explicit_cache_control?: boolean;
}

interface DebugValue {
  enabled?: boolean;
  dump_requests?: boolean;
  dump_responses?: boolean;
  max_files?: number;
}

export interface ProfileChatSectionProps {
  llmProvider: string;
  model: string;
  temperature: number;
  maxTokens: number;
  maxTokensMode: string;
  contextWindow: number;
  maxContextMessages: number;
  minContextMessages: number;
  topP: number;
  responseTimeout: number;
  rateLimitEnabled?: boolean;
  rateLimitRpm?: number;
  rateLimitBurst?: number;
  reasoningEffort: string;
  promptCache?: PromptCacheValue;
  debug?: DebugValue;
  streamingRecoveryEnabled?: boolean;
  streamingRecoveryMaxAttempts?: number;
  streamingRecoveryShowContinue?: boolean;
  onChange: (
    field:
      | 'llm_provider'
      | 'model'
      | 'temperature'
      | 'max_tokens'
      | 'max_tokens_mode'
      | 'context_window'
      | 'max_context_messages'
      | 'min_context_messages'
      | 'top_p'
      | 'response_timeout'
      | 'rate_limit_enabled'
      | 'rate_limit_rpm'
      | 'rate_limit_burst'
      | 'reasoning_effort'
      | 'prompt_cache.enabled'
      | 'prompt_cache.provider_hints'
      | 'prompt_cache.explicit_cache_control'
      | 'debug.enabled'
      | 'debug.dump_requests'
      | 'debug.dump_responses'
      | 'debug.max_files'
      | 'streaming_recovery_enabled'
      | 'streaming_recovery_max_attempts'
      | 'streaming_recovery_show_continue',
    value: string | number | boolean
  ) => void;
  onMultiChange?: (updates: Record<string, unknown>) => void;
  disabled?: boolean;
  /**
   * agentProvider diz que o provedor escolhido é um agente de código. O turno
   * dele lê só o modelo: amostragem, cache, contexto e recuperação não chegam
   * a existir, e o editor não os mostra (AEP-0084, Fase 8). O que já estiver
   * gravado continua no perfil, esperando um provedor que o use.
   */
  agentProvider?: boolean;
}

const RATE_LIMIT_MIN = 0;
const RATE_LIMIT_MAX = 10000;

/**
 * O campo numérico aceita digitação fora de min/max, então o valor é ajustado
 * aqui antes de entrar no estado — assim o perfil nunca guarda algo que o
 * backend recusaria na validação.
 */
function clampRateLimit(raw: string): number {
  const parsed = parseInt(raw, 10);
  if (Number.isNaN(parsed)) return RATE_LIMIT_MIN;
  return Math.min(RATE_LIMIT_MAX, Math.max(RATE_LIMIT_MIN, parsed));
}

/**
 * Seção de configuração de chat (LLM) de um perfil.
 * Permite escolher provedor, modelo, parâmetros de geração e limites de contexto.
 */
export function ProfileChatSection({
  llmProvider,
  model,
  temperature,
  maxTokens,
  maxTokensMode,
  contextWindow,
  maxContextMessages,
  minContextMessages,
  topP,
  responseTimeout,
  rateLimitEnabled,
  rateLimitRpm,
  rateLimitBurst,
  reasoningEffort,
  promptCache,
  debug,
  streamingRecoveryEnabled,
  streamingRecoveryMaxAttempts,
  streamingRecoveryShowContinue,
  onChange,
  onMultiChange,
  disabled = false,
  agentProvider = false,
}: ProfileChatSectionProps) {
  const { t } = useTranslation();
  const temperatureValue = temperature ?? 0.7;
  const topPValue = topP ?? 1.0;
  const maxTokensValue = maxTokens ?? 4096;
  const maxTokensModeValue = maxTokensMode || 'legacy';
  const contextWindowValue = contextWindow ?? 0;
  const maxContextMessagesValue = maxContextMessages ?? 0;
  const minContextMessagesValue = minContextMessages ?? 0;
  const responseTimeoutValue = responseTimeout ?? 180;
  const rateLimitEnabledValue = rateLimitEnabled ?? true;
  const rateLimitRpmValue = rateLimitRpm ?? 0;
  const rateLimitBurstValue = rateLimitBurst ?? 0;
  const reasoningValue = reasoningEffort || 'off';
  const promptCacheEnabledValue = promptCache?.enabled ?? false;
  const promptCacheProviderHintsValue = promptCache?.provider_hints ?? false;
  const promptCacheExplicitCacheControlValue = promptCache?.explicit_cache_control ?? false;
  const debugEnabledValue = debug?.enabled ?? false;
  const debugDumpRequestsValue = debug?.dump_requests ?? true;
  const debugDumpResponsesValue = debug?.dump_responses ?? true;
  const debugMaxFilesValue = debug?.max_files ?? 200;
  const streamingRecoveryEnabledValue = streamingRecoveryEnabled ?? true;
  const streamingRecoveryMaxAttemptsValue = streamingRecoveryMaxAttempts ?? 3;
  const streamingRecoveryShowContinueValue = streamingRecoveryShowContinue ?? true;

  const handlePromptCacheEnabledChange = (enabled: boolean) => {
    if (!enabled) {
      if (onMultiChange) {
        onMultiChange({
          'prompt_cache.enabled': false,
          'prompt_cache.provider_hints': false,
          'prompt_cache.explicit_cache_control': false,
        });
        return;
      }
      onChange('prompt_cache.enabled', false);
      onChange('prompt_cache.provider_hints', false);
      onChange('prompt_cache.explicit_cache_control', false);
      return;
    }
    onChange('prompt_cache.enabled', enabled);
  };

  const handleDebugEnabledChange = (enabled: boolean) => {
    if (onMultiChange) {
      onMultiChange({
        'debug.enabled': enabled,
        'debug.max_files': debugMaxFilesValue,
      });
      return;
    }
    onChange('debug.enabled', enabled);
    if (enabled) {
      onChange('debug.max_files', debugMaxFilesValue);
    }
  };

  return (
    <div className="profiles-fields" data-testid="profile-chat-section">
      {/* ── Provedor e Modelo ── */}
      <fieldset className="profiles-field-group">
        <legend className="profiles-field-group__title">
          {t('profiles.chatSection.groupProvider')}
        </legend>

        <div className="profiles-field">
          <LLMProviderPicker
            value={llmProvider || ''}
            onChange={(value) => {
              if (onMultiChange) {
                onMultiChange({
                  llm_provider: value,
                  model: '',
                });
              } else {
                onChange('llm_provider', value);
                onChange('model', '');
              }
            }}
            label={t('profiles.chatSection.provider')}
            variant="form"
            disabled={disabled}
          />
        </div>

        <div className="profiles-field">
          <ModelPicker
            value={model || ''}
            onChange={(value) => {
              onChange('model', value);
            }}
            label={t('profiles.chatSection.model')}
            placeholder={t('profiles.chatSection.filterModels')}
            variant="form"
            disabled={disabled || !llmProvider}
            providerID={llmProvider}
            helpText={!llmProvider ? t('profiles.chatSection.selectProvider') : ''}
          />
        </div>

        {agentProvider && (
          <p className="profiles-field__hint" data-testid="profile-chat-agent-hint">
            {t('profiles.chatSection.agentOnlyModel')}
          </p>
        )}
      </fieldset>

      {!agentProvider && (
        <>
      {/* ── Parâmetros de Geração ── */}
      <fieldset className="profiles-field-group">
        <legend className="profiles-field-group__title">
          {t('profiles.chatSection.groupGeneration')}
        </legend>

        <div className="profiles-field">
          <RangeSlider
            id="chat-temperature"
            label={t('profiles.chatSection.temperature')}
            value={temperatureValue}
            min={0}
            max={2}
            step={0.05}
            onChange={(value) => onChange('temperature', value)}
            formatValue={(value) => value.toFixed(2)}
            disabled={disabled}
          />
        </div>

        <div className="profiles-field">
          <RangeSlider
            id="chat-top-p"
            label={t('profiles.chatSection.topP')}
            value={topPValue}
            min={0}
            max={1}
            step={0.05}
            onChange={(value) => onChange('top_p', value)}
            formatValue={(value) => value.toFixed(2)}
            disabled={disabled}
          />
        </div>

        <div className="profiles-field">
          <label htmlFor="chat-reasoning" className="profiles-field__label">
            {t('profiles.chatSection.reasoning')}
          </label>
          <select
            id="chat-reasoning"
            className="profiles-field__select"
            value={reasoningValue}
            onChange={(e) => onChange('reasoning_effort', e.target.value === 'off' ? '' : e.target.value)}
            disabled={disabled}
          >
            <option value="ollama">{t('profiles.chatSection.reasoningOllama')}</option>
            <option value="off">{t('profiles.chatSection.reasoningOff')}</option>
            <option value="none">{t('profiles.chatSection.reasoningNone')}</option>
            <option value="low">{t('profiles.chatSection.reasoningLow')}</option>
            <option value="medium">{t('profiles.chatSection.reasoningMedium')}</option>
            <option value="high">{t('profiles.chatSection.reasoningHigh')}</option>
            <option value="max">{t('profiles.chatSection.reasoningMax')}</option>
          </select>
          <span className="profiles-field__hint">
            {t('profiles.chatSection.reasoningHint')}
          </span>
        </div>

        <div className="profiles-field">
          <label htmlFor="chat-max-tokens" className="profiles-field__label">
            {t('profiles.chatSection.maxTokens')}
          </label>
          <input
            id="chat-max-tokens"
            type="number"
            className="profiles-field__input"
            min={1}
            max={128000}
            value={maxTokensValue}
            onChange={(e) => onChange('max_tokens', parseInt(e.target.value) || 4096)}
            disabled={disabled}
          />
        </div>

        <div className="profiles-field">
          <label htmlFor="chat-max-tokens-mode" className="profiles-field__label">
            {t('profiles.chatSection.maxTokensFormat')}
          </label>
          <select
            id="chat-max-tokens-mode"
            className="profiles-field__select"
            value={maxTokensModeValue}
            onChange={(e) => onChange('max_tokens_mode', e.target.value)}
            disabled={disabled}
          >
            <option value="legacy">{t('profiles.chatSection.maxTokensLegacy')}</option>
            <option value="completion_tokens">{t('profiles.chatSection.maxTokensCompletion')}</option>
          </select>
          <span className="profiles-field__hint">
            {t('profiles.chatSection.maxTokensHint')}
          </span>
        </div>
      </fieldset>

      {/* ── Prompt Cache ── */}
      <fieldset className="profiles-field-group">
        <legend className="profiles-field-group__title">
          {t('profiles.chatSection.groupPromptCache')}
        </legend>

        <div className="profiles-field profiles-field--checkbox">
          <label className="profiles-field__label" htmlFor="chat-prompt-cache-enabled">
            <input
              id="chat-prompt-cache-enabled"
              type="checkbox"
              checked={promptCacheEnabledValue}
              onChange={(e) => handlePromptCacheEnabledChange(e.target.checked)}
              disabled={disabled}
            />
            {t('profiles.chatSection.promptCacheEnabled')}
          </label>
        </div>
        <span className="profiles-field__hint">
          {t('profiles.chatSection.promptCacheEnabledHint')}
        </span>

        <div className="profiles-field profiles-field--checkbox">
          <label className="profiles-field__label" htmlFor="chat-prompt-cache-provider-hints">
            <input
              id="chat-prompt-cache-provider-hints"
              type="checkbox"
              checked={promptCacheProviderHintsValue}
              onChange={(e) => onChange('prompt_cache.provider_hints', e.target.checked)}
              disabled={disabled || !promptCacheEnabledValue}
            />
            {t('profiles.chatSection.promptCacheProviderHints')}
          </label>
        </div>
        <span className="profiles-field__hint">
          {t('profiles.chatSection.promptCacheProviderHintsHint')}
        </span>

        <div className="profiles-field profiles-field--checkbox">
          <label className="profiles-field__label" htmlFor="chat-prompt-cache-explicit-cache-control">
            <input
              id="chat-prompt-cache-explicit-cache-control"
              type="checkbox"
              checked={promptCacheExplicitCacheControlValue}
              onChange={(e) => onChange('prompt_cache.explicit_cache_control', e.target.checked)}
              disabled={disabled || !promptCacheEnabledValue}
            />
            {t('profiles.chatSection.promptCacheExplicitCacheControl')}
          </label>
        </div>
        <span className="profiles-field__hint">
          {t('profiles.chatSection.promptCacheExplicitCacheControlHint')}
        </span>
      </fieldset>

      {/* ── Debug LLM ── */}
      <fieldset className="profiles-field-group">
        <legend className="profiles-field-group__title">
          {t('profiles.chatSection.groupDebug')}
        </legend>

        <div className="profiles-field profiles-field--checkbox">
          <label className="profiles-field__label" htmlFor="chat-debug-dumps-enabled">
            <input
              id="chat-debug-dumps-enabled"
              type="checkbox"
              checked={debugEnabledValue}
              onChange={(e) => handleDebugEnabledChange(e.target.checked)}
              disabled={disabled}
            />
            {t('profiles.chatSection.debugDumpsEnabled')}
          </label>
        </div>
        <span className="profiles-field__hint">
          {t('profiles.chatSection.debugDumpsEnabledHint')}
        </span>

        <div className="profiles-field profiles-field--checkbox">
          <label className="profiles-field__label" htmlFor="chat-debug-dump-requests">
            <input
              id="chat-debug-dump-requests"
              type="checkbox"
              checked={debugDumpRequestsValue}
              onChange={(e) => onChange('debug.dump_requests', e.target.checked)}
              disabled={disabled || !debugEnabledValue}
            />
            {t('profiles.chatSection.debugDumpRequests')}
          </label>
        </div>

        <div className="profiles-field profiles-field--checkbox">
          <label className="profiles-field__label" htmlFor="chat-debug-dump-responses">
            <input
              id="chat-debug-dump-responses"
              type="checkbox"
              checked={debugDumpResponsesValue}
              onChange={(e) => onChange('debug.dump_responses', e.target.checked)}
              disabled={disabled || !debugEnabledValue}
            />
            {t('profiles.chatSection.debugDumpResponses')}
          </label>
        </div>

        <div className="profiles-field">
          <label htmlFor="chat-debug-max-files" className="profiles-field__label">
            {t('profiles.chatSection.debugMaxFiles')}
          </label>
          <input
            id="chat-debug-max-files"
            type="number"
            className="profiles-field__input"
            min={0}
            max={10000}
            value={debugMaxFilesValue}
            onChange={(e) => {
              const parsed = parseInt(e.target.value, 10);
              const nextValue = Number.isNaN(parsed) ? 200 : Math.min(10000, Math.max(0, parsed));
              onChange('debug.max_files', nextValue);
            }}
            disabled={disabled || !debugEnabledValue}
          />
          <span className="profiles-field__hint">
            {t('profiles.chatSection.debugMaxFilesHint')}
          </span>
        </div>
      </fieldset>

      {/* ── Contexto e Limites ── */}
      <fieldset className="profiles-field-group">
        <legend className="profiles-field-group__title">
          {t('profiles.chatSection.groupContext')}
        </legend>

        <div className="profiles-field">
          <label htmlFor="chat-context-window" className="profiles-field__label">
            {t('profiles.chatSection.contextWindow')}
          </label>
          <input
            id="chat-context-window"
            type="number"
            className="profiles-field__input"
            min={0}
            max={2000000}
            value={contextWindowValue}
            onChange={(e) => onChange('context_window', parseInt(e.target.value) || 0)}
            placeholder="0"
            disabled={disabled}
          />
          <span className="profiles-field__hint">
            {t('profiles.chatSection.contextWindowHint')}
          </span>
        </div>

        <div className="profiles-field">
          <label htmlFor="chat-max-context-messages" className="profiles-field__label">
            {t('profiles.chatSection.maxMessages')}
          </label>
          <input
            id="chat-max-context-messages"
            type="number"
            className="profiles-field__input"
            min={0}
            max={500}
            value={maxContextMessagesValue}
            onChange={(e) => onChange('max_context_messages', parseInt(e.target.value) || 0)}
            placeholder="50"
            disabled={disabled}
          />
          <span className="profiles-field__hint">
            {t('profiles.chatSection.maxMessagesHint')}
          </span>
        </div>

        <div className="profiles-field">
          <label htmlFor="chat-min-context-messages" className="profiles-field__label">
            {t('profiles.chatSection.minPreserved')}
          </label>
          <input
            id="chat-min-context-messages"
            type="number"
            className="profiles-field__input"
            min={0}
            max={100}
            value={minContextMessagesValue}
            onChange={(e) => onChange('min_context_messages', parseInt(e.target.value) || 0)}
            placeholder="10"
            disabled={disabled}
          />
          <span className="profiles-field__hint">
            {t('profiles.chatSection.minPreservedHint')}
          </span>
        </div>

        <div className="profiles-field">
          <label htmlFor="chat-timeout" className="profiles-field__label">
            {t('profiles.chatSection.timeout')}
          </label>
          <input
            id="chat-timeout"
            type="number"
            className="profiles-field__input"
            min={10}
            max={600}
            value={responseTimeoutValue}
            onChange={(e) => onChange('response_timeout', parseInt(e.target.value) || 180)}
            disabled={disabled}
          />
        </div>
      </fieldset>

      {/* ── Recuperação ── */}
      <fieldset className="profiles-field-group">
        <legend className="profiles-field-group__title">
          {t('profiles.chatSection.groupRecovery')}
        </legend>

        <div className="profiles-field profiles-field--checkbox">
          <label className="profiles-field__label" htmlFor="chat-streaming-recovery-enabled">
            <input
              id="chat-streaming-recovery-enabled"
              type="checkbox"
              checked={streamingRecoveryEnabledValue}
              onChange={(e) => onChange('streaming_recovery_enabled', e.target.checked)}
              disabled={disabled}
            />
            {t('profiles.chatSection.streamingRecoveryEnabled')}
          </label>
        </div>
        <span className="profiles-field__hint">
          {t('profiles.chatSection.streamingRecoveryEnabledHint')}
        </span>

        <div className="profiles-field">
          <label htmlFor="chat-streaming-recovery-max-attempts" className="profiles-field__label">
            {t('profiles.chatSection.streamingRecoveryMaxAttempts')}
          </label>
          <input
            id="chat-streaming-recovery-max-attempts"
            type="number"
            className="profiles-field__input"
            min={1}
            max={10}
            value={streamingRecoveryMaxAttemptsValue}
            onChange={(e) => onChange('streaming_recovery_max_attempts', parseInt(e.target.value) || 3)}
            disabled={disabled || !streamingRecoveryEnabledValue}
          />
          <span className="profiles-field__hint">
            {t('profiles.chatSection.streamingRecoveryMaxAttemptsHint')}
          </span>
        </div>

        <div className="profiles-field profiles-field--checkbox">
          <label className="profiles-field__label" htmlFor="chat-streaming-recovery-show-continue">
            <input
              id="chat-streaming-recovery-show-continue"
              type="checkbox"
              checked={streamingRecoveryShowContinueValue}
              onChange={(e) => onChange('streaming_recovery_show_continue', e.target.checked)}
              disabled={disabled || !streamingRecoveryEnabledValue}
            />
            {t('profiles.chatSection.streamingRecoveryShowContinue')}
          </label>
        </div>
        <span className="profiles-field__hint">
          {t('profiles.chatSection.streamingRecoveryShowContinueHint')}
        </span>
      </fieldset>
        </>
      )}

      {/* ── Limite local de chamadas ── */}
      <fieldset className="profiles-field-group">
        <legend className="profiles-field-group__title">
          {t('profiles.chatSection.groupRateLimit')}
        </legend>

        <div className="profiles-field profiles-field--checkbox">
          <label className="profiles-field__label" htmlFor="chat-rate-limit-enabled">
            <input
              id="chat-rate-limit-enabled"
              type="checkbox"
              checked={rateLimitEnabledValue}
              onChange={(e) => onChange('rate_limit_enabled', e.target.checked)}
              disabled={disabled}
            />
            {t('profiles.chatSection.rateLimitEnabled')}
          </label>
        </div>
        <span className="profiles-field__hint">
          {t('profiles.chatSection.rateLimitEnabledHint')}
        </span>

        <div className="profiles-field">
          <label htmlFor="chat-rate-limit-rpm" className="profiles-field__label">
            {t('profiles.chatSection.rateLimitRpm')}
          </label>
          <input
            id="chat-rate-limit-rpm"
            type="number"
            className="profiles-field__input"
            min={RATE_LIMIT_MIN}
            max={RATE_LIMIT_MAX}
            value={rateLimitRpmValue}
            onChange={(e) => onChange('rate_limit_rpm', clampRateLimit(e.target.value))}
            disabled={disabled || !rateLimitEnabledValue}
          />
          <span className="profiles-field__hint">
            {t('profiles.chatSection.rateLimitRpmHint')}
          </span>
        </div>

        <div className="profiles-field">
          <label htmlFor="chat-rate-limit-burst" className="profiles-field__label">
            {t('profiles.chatSection.rateLimitBurst')}
          </label>
          <input
            id="chat-rate-limit-burst"
            type="number"
            className="profiles-field__input"
            min={RATE_LIMIT_MIN}
            max={RATE_LIMIT_MAX}
            value={rateLimitBurstValue}
            onChange={(e) => onChange('rate_limit_burst', clampRateLimit(e.target.value))}
            disabled={disabled || !rateLimitEnabledValue}
          />
          <span className="profiles-field__hint">
            {t('profiles.chatSection.rateLimitBurstHint')}
          </span>
        </div>
      </fieldset>
    </div>
  );
}
