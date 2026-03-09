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
          label="Provedor LLM"
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
          label="Modelo"
          placeholder="Filtrar modelos..."
          variant="form"
          disabled={disabled || !llmProvider}
          providerID={llmProvider}
          helpText={!llmProvider ? 'Selecione um provedor primeiro' : ''}
        />
      </div>

      <div className="profiles-field">
        <RangeSlider
          id="chat-temperature"
          label="Temperatura"
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
          Max Tokens
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
          Formato do parâmetro max_tokens
        </label>
        <select
          id="chat-max-tokens-mode"
          className="profiles-field__select"
          value={maxTokensModeValue}
          onChange={(e) => onChange('max_tokens_mode', e.target.value)}
          disabled={disabled}
        >
          <option value="legacy">Legacy (max_tokens) - Padrão</option>
          <option value="completion_tokens">Completion Tokens (max_completion_tokens) - GPT-4o, o1, etc</option>
        </select>
        <span className="profiles-field__hint">
          "legacy" usa max_tokens (maioria dos modelos). "completion_tokens" usa max_completion_tokens (GPT-4o, o1, modelos novos OpenAI).
        </span>
      </div>

      <div className="profiles-field">
        <label htmlFor="chat-context-window" className="profiles-field__label">
          Janela de Contexto (tokens)
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
          Total de tokens suportados pelo modelo (ex: 128000). 0 = não definido. Quando definido, ativa sumarização automática.
        </span>
      </div>

      <div className="profiles-field">
        <label htmlFor="chat-max-context-messages" className="profiles-field__label">
          Máx. Mensagens no Contexto
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
          Limite de mensagens enviadas ao modelo. 0 = padrão (50).
        </span>
      </div>

      <div className="profiles-field">
        <label htmlFor="chat-min-context-messages" className="profiles-field__label">
          Mín. Mensagens Preservadas
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
          Mínimo de mensagens mantidas após sumarização. 0 = padrão (10).
        </span>
      </div>

      <div className="profiles-field">
        <RangeSlider
          id="chat-top-p"
          label="Top P"
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
          Timeout (segundos)
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
          Raciocínio (Reasoning)
        </label>
        <select
          id="chat-reasoning"
          className="profiles-field__select"
          value={reasoningValue}
          onChange={(e) => onChange('reasoning_effort', e.target.value === 'off' ? '' : e.target.value)}
          disabled={disabled}
        >
          <option value="ollama">Ativado (Ollama)</option>
          <option value="off">Desativado</option>
          <option value="none">Mínimo (none)</option>
          <option value="low">Baixo (low)</option>
          <option value="medium">Médio (medium)</option>
          <option value="high">Alto (high)</option>
          <option value="max">Máximo (max)</option>
        </select>
        <span className="profiles-field__hint">
          Baixo/Médio/Alto envia reasoning_effort (OpenAI, Anthropic, LiteLLM). Ollama envia think=true.
        </span>
      </div>
    </div>
  );
}
