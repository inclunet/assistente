import { useTranslation } from 'react-i18next';
import { LLMProviderPicker } from '../pickers/LLMProviderPicker';
import { ModelPicker } from '../pickers/ModelPicker';
import { RangeSlider } from '../ui/RangeSlider';

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
  reasoningEffort: string;
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
      | 'reasoning_effort',
    value: string | number
  ) => void;
  onMultiChange?: (updates: Record<string, any>) => void;
  disabled?: boolean;
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
  reasoningEffort,
  onChange,
  onMultiChange,
  disabled = false,
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
  const reasoningValue = reasoningEffort || 'off';

  return (
    <div className="profiles-fields" data-testid="profile-chat-section">
      <div className="profiles-field">
        <LLMProviderPicker
          value={llmProvider || ''}
          onChange={(value) => {
            // Atualiza provider e limpa model atomicamente
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
            console.log('[ProfileChatSection] Modelo selecionado:', value);
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
    </div>
  );
}
