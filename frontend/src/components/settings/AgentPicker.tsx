import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { GetACPCatalog, RefreshACPCatalog } from '@wailsjs/go/wailsapi/ACPRegistry';
import { BasePicker } from '../pickers/BasePicker';
import type { ComboboxItem } from '../pickers/Combobox';
import {
  catalogItemLabel,
  runtimeText,
  stateText,
  type CatalogAgent,
} from './ACPAgentCatalog';
import './AgentPicker.css';

export interface AgentPickerProps {
  /** O `id` da linha do registro que este provedor usa (AEP-0086 D11). */
  agentId: string;

  /** Chamado quando alguém escolhe um agente do catálogo. */
  onPick: (agent: CatalogAgent) => void;
}

/**
 * Escolhe qual agente de código o provedor é.
 *
 * Todo agente ACP é o mesmo tipo de provedor (D11), então o seletor de tipo não
 * tem mais uma entrada por agente: ele tem uma, e a escolha entre os 38 acontece
 * aqui, na mesma lista que a tela de provedores já mostra — com busca, estado
 * por máquina e pré-requisito de runtime. Uma lista mais curta feita só para
 * escolher faria a escolha ser feita com menos do que o app sabe.
 *
 * O nome do agente escolhido vem do catálogo, mas o `id` sozinho já descreve o
 * provedor: quem está sem rede vê o identificador, e não um espaço em branco.
 */
export const AgentPicker = ({ agentId, onPick }: AgentPickerProps) => {
  const { t } = useTranslation();
  const [agents, setAgents] = useState<CatalogAgent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const mountedRef = useRef(true);
  const tRef = useRef(t);
  tRef.current = t;

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  const loadCatalog = useCallback(async (refresh = false) => {
    setLoading(true);
    setError('');
    try {
      const catalog = refresh ? await RefreshACPCatalog() : await GetACPCatalog();
      if (!mountedRef.current) return;
      setAgents((catalog?.agents || []) as CatalogAgent[]);
    } catch (cause: unknown) {
      if (!mountedRef.current) return;
      const detail = cause instanceof Error ? cause.message : String(cause ?? '');
      setError(detail || tRef.current('providerForm.agent.picker.loadError'));
    } finally {
      if (mountedRef.current) setLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadCatalog();
  }, [loadCatalog]);

  const items = useMemo<ComboboxItem[]>(() => agents.map((agent) => ({
    value: agent.id,
    label: agent.name,
    sublabel: `${stateText(t, agent)} · ${runtimeText(t, agent)}`,
    searchText: [
      agent.id,
      agent.description,
      agent.license,
      ...(agent.authors || []),
    ].filter(Boolean).join(' '),
    accessibleLabel: catalogItemLabel(t, agent),
  })), [agents, t]);

  const handlePick = useCallback((value: string) => {
    const agent = agents.find((item) => item.id === value);
    if (agent) onPick(agent);
  }, [agents, onPick]);

  return (
    <div className="agent-picker">
      <BasePicker
        variant="form"
        items={items}
        selected={agentId}
        onSelect={handlePick}
        label={t('providerForm.agent.picker.label')}
        placeholder={t('providerForm.agent.picker.filterPlaceholder')}
        helpText={t('providerForm.agent.picker.help')}
        loading={loading}
        error={error}
        emptyLabel={t('providerForm.agent.picker.empty')}
        loadingLabel={t('providerForm.agent.picker.loading')}
        errorLabel={error || t('providerForm.agent.picker.loadError')}
        onRetry={() => void loadCatalog(true)}
        retryLabel={t('providerForm.agent.picker.retry')}
        maxWidth="100%"
        formClassName="agent-picker__field"
        helpTextClassName="agent-picker__help"
      />
    </div>
  );
};
