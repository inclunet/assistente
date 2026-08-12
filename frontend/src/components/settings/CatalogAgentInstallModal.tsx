import { useTranslation } from 'react-i18next';
import { Modal } from '../ui/Modal';
import { AgentInstall } from './AgentInstall';

export interface CatalogAgentInstallModalProps {
  agentId: string;
  agentName: string;
  isOpen: boolean;
  onClose: () => void;
  /** Chamado quando a instalação (ou o "Usar" pós-instalação) resolve o comando. */
  onInstalled: () => void;
}

/**
 * Instalação disparada a partir do catálogo browse, sem abrir o formulário do
 * provedor. O bloco é o mesmo `AgentInstall` do formulário — confirmação,
 * progresso, digest e handshake — para não haver dois fluxos divergentes.
 */
export function CatalogAgentInstallModal({
  agentId,
  agentName,
  isOpen,
  onClose,
  onInstalled,
}: CatalogAgentInstallModalProps) {
  const { t } = useTranslation();

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={t('acpCatalog.installModal.title', { name: agentName })}
      size="md"
    >
      <p className="acp-catalog-install-modal__intro">
        {t('acpCatalog.installModal.intro', { name: agentName })}
      </p>
      <AgentInstall
        agentId={agentId}
        onResolved={() => {
          onInstalled();
        }}
      />
    </Modal>
  );
}
