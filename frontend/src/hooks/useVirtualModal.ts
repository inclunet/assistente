import type { UseRenderedContentNavigationOptions } from './useRenderedContentNavigation';
import { useRenderedContentNavigation } from './useRenderedContentNavigation';

export interface UseVirtualModalOptions {
  /** Referência ao elemento que será o modal virtual. */
  elementRef: React.RefObject<HTMLElement | null>;
  /** Se o modo leitura está ativo. */
  isActive: boolean;
  /** Callback para desativar o modo leitura. */
  onClose: () => void;
  /** Labels já traduzidos para anúncio e nome do diálogo. */
  openAnnouncement: string;
  closeAnnouncement: string;
  dialogLabel: string;
  /** Seletor opcional do conteúdo que receberá role=document. */
  contentSelector?: string;
}

/**
 * Compatibilidade para o isolamento modal de mensagens.
 *
 * O contrato comum vive em useRenderedContentNavigation; este wrapper fixa o
 * perfil modal para manter a API histórica dos consumidores do chat.
 */
export function useVirtualModal({
  elementRef,
  isActive,
  onClose,
  openAnnouncement,
  closeAnnouncement,
  dialogLabel,
  contentSelector,
}: UseVirtualModalOptions) {
  const options: UseRenderedContentNavigationOptions = {
    elementRef,
    isActive,
    profile: 'modal',
    onEscape: onClose,
    openAnnouncement,
    closeAnnouncement,
    dialogLabel,
    contentSelector,
  };
  useRenderedContentNavigation(options);
}
