import { useEffect, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { GetACPCatalog } from '@wailsjs/go/app/App';
import { Button } from '../';
import { Modal } from '../ui/Modal';
import { ACPAgentCatalog, type CatalogAgent } from './ACPAgentCatalog';
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
  const baseId = useId();
  const helpId = `${baseId}-help`;
  const chosenId = `${baseId}-chosen`;

  const [open, setOpen] = useState(false);
  const [name, setName] = useState('');

  const mountedRef = useRef(true);
  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  // O nome é enfeite em cima do `id`: se o catálogo não vier, a tela continua
  // dizendo qual agente é. Por isso a falha aqui não vira erro na tela.
  useEffect(() => {
    if (!agentId) {
      setName('');
      return;
    }
    let cancelado = false;
    void (async () => {
      try {
        const catalog = await GetACPCatalog();
        if (cancelado || !mountedRef.current) return;
        const achado = catalog?.agents?.find((agent) => agent.id === agentId);
        setName(achado?.name ?? '');
      } catch {
        if (cancelado || !mountedRef.current) return;
        setName('');
      }
    })();
    return () => {
      cancelado = true;
    };
  }, [agentId]);

  const handlePick = (agent: CatalogAgent) => {
    setName(agent.name);
    setOpen(false);
    onPick(agent);
  };

  return (
    <div className="agent-picker">
      <p className="agent-picker__chosen" id={chosenId}>
        <span className="agent-picker__term">{t('providerForm.agent.picker.chosenTerm')}</span>{' '}
        {agentId ? name || agentId : t('providerForm.agent.picker.none')}
      </p>

      <Button
        type="button"
        variant="secondary"
        onClick={() => setOpen(true)}
        aria-describedby={`${chosenId} ${helpId}`}
      >
        {agentId ? t('providerForm.agent.picker.changeBtn') : t('providerForm.agent.picker.pickBtn')}
      </Button>

      <p id={helpId} className="agent-picker__help">
        {t('providerForm.agent.picker.help')}
      </p>

      <Modal
        isOpen={open}
        onClose={() => setOpen(false)}
        title={t('providerForm.agent.picker.modalTitle')}
        size="lg"
      >
        <ACPAgentCatalog onSelect={handlePick} selectedId={agentId} />
      </Modal>
    </div>
  );
};
