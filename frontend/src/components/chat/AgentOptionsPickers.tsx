import React, { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { BasePicker } from '../pickers/BasePicker';
import type { ComboboxItem } from '../pickers/Combobox';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import {
  AGENT_OPTION_MODE,
  AGENT_OPTION_MODEL,
  agentModeLabel,
  optionByCategory,
  useAgentSessionOptions,
  type AgentConfigOption,
} from './useAgentSessionOptions';

export interface AgentOptionsPickersProps {
  conversationId?: string | null;
  disabled?: boolean;
}

/**
 * AgentOptionsPickers mostra em que modelo e modo o agente desta conversa está,
 * e deixa trocar os dois (AEP-0084 D6).
 *
 * Só aparece quando há agente com escolhas a oferecer: numa conversa de provedor
 * HTTP o modelo é o do perfil, e um seletor vazio na barra seria mais um controle
 * para o Tab atravessar sem nada a fazer.
 */
export const AgentOptionsPickers: React.FC<AgentOptionsPickersProps> = ({
  conversationId,
  disabled = false,
}) => {
  const { t } = useTranslation();
  const { announce } = useAnnouncer();
  const { options, changing, change } = useAgentSessionOptions(conversationId);

  const model = optionByCategory(options, AGENT_OPTION_MODEL);
  const mode = optionByCategory(options, AGENT_OPTION_MODE);

  const handleModelChange = useCallback(async (value: string) => {
    if (!model || value === model.currentValue) return;
    const applied = await change(model.id, value);
    if (!applied) return;
    // A troca vale do próximo turno em diante, e dizer isso evita a dúvida de
    // quem acabou de mandar uma mensagem e não sabe qual modelo a atendeu.
    //
    // O que é falado é o valor que voltou do agente, pelo rótulo que a lista
    // mostra: o identificador cru — `claude-sonnet-4-5-20250929` — é ilegível
    // falado, e o agente pode ter acomodado o pedido em outro valor.
    const current = optionByCategory(applied, AGENT_OPTION_MODEL) ?? model;
    announce(t('chat.agentOptions.modelChanged', {
      model: labelOfValue(current, current.currentValue || value),
    }));
  }, [announce, change, model, t]);

  const handleModeChange = useCallback(async (value: string) => {
    if (!mode || value === mode.currentValue) return;
    const applied = await change(mode.id, value);
    if (!applied) return;
    const current = optionByCategory(applied, AGENT_OPTION_MODE) ?? mode;
    announce(t('chat.agentOptions.modeChanged', {
      mode: labelOfValue(current, current.currentValue || value, (v) => agentModeLabel(t, v)),
    }));
  }, [announce, change, mode, t]);

  if (!model && !mode) return null;

  return (
    <>
      {model && (
        <BasePicker
          variant="toolbar"
          items={itemsOf(model)}
          selected={model.currentValue}
          onSelect={(value) => void handleModelChange(value)}
          label={model.name || t('chat.agentOptions.modelLabel')}
          description={t('chat.agentOptions.modelDescription')}
          maxWidth="180px"
          disabled={disabled || changing}
          onAnnounce={announce}
          showEmptyState={false}
        />
      )}
      {mode && (
        <BasePicker
          variant="toolbar"
          items={itemsOf(mode, (value) => agentModeLabel(t, value))}
          selected={mode.currentValue}
          onSelect={(value) => void handleModeChange(value)}
          label={mode.name || t('chat.agentOptions.modeLabel')}
          description={t('chat.agentOptions.modeDescription')}
          maxWidth="140px"
          disabled={disabled || changing}
          onAnnounce={announce}
          showEmptyState={false}
        />
      )}
    </>
  );
};

/**
 * itemsOf monta os itens do seletor. O rótulo é o que o agente mandou; quando
 * ele não manda nenhum — o modo não traz —, quem exibe traduz o valor, e o
 * último recurso é o valor cru: um item sem texto seria invisível ao leitor de
 * telas.
 */
function itemsOf(
  option: AgentConfigOption,
  translateValue?: (value: string) => string,
): ComboboxItem[] {
  return option.values.map((item) => ({
    value: item.value,
    label: labelOfValue(option, item.value, translateValue),
  }));
}

/**
 * labelOfValue é o texto de um valor da opção. Mora aqui, e é usado tanto pelos
 * itens quanto pelo anúncio, para o que a pessoa ouve ser o mesmo que está
 * escrito na lista.
 */
function labelOfValue(
  option: AgentConfigOption,
  value: string,
  translateValue?: (value: string) => string,
): string {
  const item = option.values.find((candidate) => candidate.value === value);
  return item?.name || translateValue?.(value) || value;
}