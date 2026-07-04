import {
  ReactNode,
  createContext,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useCallback,
  useId,
} from 'react';
import { createPortal } from 'react-dom';
import { CloseOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { restoreDefaultFocus } from '../../hooks/useDefaultFocus';
import {
  registerOpenModal,
  unregisterOpenModal,
  isTopmostModal,
} from '../../lib/modalRegistry';
import './Modal.css';

// O registro de modais abertos (stack global + efeitos globais de inert/aria-hidden)
// vive no módulo neutro `lib/modalRegistry.ts`, consumido tanto por este componente
// quanto por stores/hooks. Reexportamos `isModalOpen`/`ensureModalCleanup` aqui para
// preservar os consumidores de UI já existentes, sem que a camada de estado precise
// importar deste componente React.
export { isModalOpen, ensureModalCleanup } from '../../lib/modalRegistry';

/**
 * Contexto que expõe, para os descendentes de um Modal, se aquele Modal é o
 * que está no topo da stack. Reutiliza o mesmo critério usado pelo Modal para
 * tratar ESC/Tab, garantindo que handlers de teclado de conteúdos internos
 * (ex.: setas/zoom do ImageViewerModal) só ajam quando o modal estiver ativo.
 */
const ModalTopmostContext = createContext<(() => boolean) | null>(null);

// Fallback estável por referência para uso fora de um Modal: sem stack
// concorrente, considera-se sempre o topo. Constante de módulo para não
// criar uma nova função a cada render (evita reexecução de useEffect/deps).
const ALWAYS_TOPMOST = () => true;

/**
 * Retorna uma função que indica se o Modal mais próximo (ancestral) é o do
 * topo da stack. Fora de um Modal, assume `true` (sem stack concorrente).
 */
export function useModalIsTopmost(): () => boolean {
  const ctx = useContext(ModalTopmostContext);
  return ctx ?? ALWAYS_TOPMOST;
}

// Seletor para elementos focáveis
const FOCUSABLE_SELECTOR = 
  'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), ' +
  'textarea:not([disabled]), [tabindex]:not([tabindex="-1"]), [contenteditable]';

function restorePageFocus() {
  requestAnimationFrame(() => {
    restoreDefaultFocus();
  });
}

export interface ModalProps {
  isOpen: boolean;
  onClose: () => void;
  title: string;
  children: ReactNode;
  size?: 'sm' | 'md' | 'lg' | 'xl';
  className?: string;
  ariaDescribedBy?: string;
  /** Se true, restaura foco na área padrão da página quando o modal fecha. Default: true */
  returnFocusOnClose?: boolean;
  /** Se false, desabilita fechamento (ESC, clique fora e botão X). Default: true */
  allowClose?: boolean;
  /**
   * Modais de leitura (ex.: detalhes de mensagem, estatísticas de tokens) recebem
   * `role="document"` no corpo, o que faz o NVDA alternar para modo de navegação
   * e permitir leitura linear do conteúdo. Modais de configuração/formulário NÃO
   * devem habilitar esta opção: eles permanecem com `role="application"` (modo
   * de foco do NVDA) para que setas e teclas sejam entregues aos controles.
   * Default: false
   */
  readingMode?: boolean;
}

export function Modal({
  isOpen,
  onClose,
  title,
  children,
  size = 'md',
  className,
  ariaDescribedBy,
  returnFocusOnClose = true,
  allowClose = true,
  readingMode = false,
}: ModalProps) {
  const { t } = useTranslation();
  const modalRef = useRef<HTMLDivElement>(null);
  const prevOpenRef = useRef(false);
  const titleId = useId();
  const modalInstanceIdRef = useRef<string>(
    (() => {
      try {
        return crypto.randomUUID();
      } catch {
        return `modal-${Date.now()}-${Math.random().toString(16).slice(2)}`;
      }
    })()
  );

  const isTopMost = useCallback(() => {
    return isTopmostModal(modalInstanceIdRef.current);
  }, []);

  // Valor estável exposto via contexto para descendentes do Modal.
  const topmostValue = useMemo(() => isTopMost, [isTopMost]);

  // Mantém o stack em sync com abertura/fechamento.
  useEffect(() => {
    const id = modalInstanceIdRef.current;
    if (!isOpen) return;

    registerOpenModal(id);

    return () => {
      unregisterOpenModal(id);
    };
  }, [isOpen]);

  // Retorna todos os elementos focáveis dentro do modal
  const getFocusableElements = useCallback(() => {
    if (!modalRef.current) return [];
    return Array.from(modalRef.current.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR))
      .filter(el => el.offsetParent !== null); // Filtra elementos visíveis
  }, []);

  // Restaura foco na área padrão quando isOpen transita de true → false
  useEffect(() => {
    if (prevOpenRef.current && !isOpen && returnFocusOnClose) {
      restorePageFocus();
    }
    prevOpenRef.current = isOpen;
  }, [isOpen, returnFocusOnClose]);

  // Auto-focus no primeiro elemento focável quando o modal abre
  useEffect(() => {
    if (!isOpen || !modalRef.current) return;

    // Aguarda o DOM renderizar completamente
    const frameId = requestAnimationFrame(() => {
      if (!isTopMost() || !modalRef.current) return;
      const focusableElements = getFocusableElements();
      // Procura primeiro um input/textarea/select, senão usa o primeiro focável
      const firstInput = focusableElements.find(el => 
        el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || el.tagName === 'SELECT'
      );
      const firstFocusable = firstInput || focusableElements[0];
      
      if (firstFocusable) {
        firstFocusable.focus();
      }
    });

    return () => cancelAnimationFrame(frameId);
  }, [isOpen, getFocusableElements, isTopMost]);

  useEffect(() => {
    if (!isOpen) return;

    const handleEscape = (e: KeyboardEvent) => {
      if (!isTopMost()) return;
      if (!allowClose) return;
      if (e.key === 'Escape') {
        e.stopPropagation();
        onClose();
      }
    };

    // Focus trap - mantém o foco dentro do modal
    const handleTab = (e: KeyboardEvent) => {
      if (!isTopMost()) return;
      if (e.key !== 'Tab') return;
      
      const focusableElements = getFocusableElements();
      if (focusableElements.length === 0) return;

      const firstElement = focusableElements[0];
      const lastElement = focusableElements[focusableElements.length - 1];

      if (e.shiftKey) {
        // Shift+Tab: se está no primeiro, vai para o último
        if (document.activeElement === firstElement) {
          e.preventDefault();
          lastElement.focus();
        }
      } else {
        // Tab: se está no último, volta para o primeiro
        if (document.activeElement === lastElement) {
          e.preventDefault();
          firstElement.focus();
        }
      }
    };

    const handleClickOutside = (e: MouseEvent) => {
      if (!isTopMost()) return;
      if (!allowClose) return;
      if (modalRef.current && !modalRef.current.contains(e.target as Node)) {
        const overlay = (e.target as HTMLElement).closest('.modal-overlay');
        if (overlay) {
          onClose();
        }
      }
    };

    document.addEventListener('keydown', handleEscape);
    document.addEventListener('keydown', handleTab);
    document.addEventListener('mousedown', handleClickOutside);

    return () => {
      document.removeEventListener('keydown', handleEscape);
      document.removeEventListener('keydown', handleTab);
      document.removeEventListener('mousedown', handleClickOutside);
    };
  }, [isOpen, onClose, getFocusableElements, allowClose, isTopMost]);

  if (!isOpen) return null;

  return createPortal(
    <ModalTopmostContext.Provider value={topmostValue}>
      <div
        className="modal-overlay"
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        aria-describedby={ariaDescribedBy}
      >
        <div ref={modalRef} className={`modal-content ${size}${className ? ` ${className}` : ''}`}>
          <div className="modal-header">
            {allowClose && (
              <button 
                className="modal-close"
                onClick={onClose}
                aria-label={t('ui.modal.close')}
              >
                <CloseOutlined aria-hidden="true" />
              </button>
            )}
            <h1 id={titleId} className="modal-title">{title}</h1>
          </div>
          <div
            className="modal-body"
            role={readingMode ? 'document' : 'application'}
          >
            {children}
          </div>
        </div>
      </div>
    </ModalTopmostContext.Provider>,
    document.body
  );
}

