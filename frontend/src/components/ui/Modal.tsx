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
import './Modal.css';

// Stack global simples para garantir que apenas o modal do topo
// trate Escape/Tab/click-outside quando há múltiplos modais abertos.
const OPEN_MODAL_STACK: string[] = [];

let previousBodyOverflow: string | null = null;

function setGlobalModalEffects(enabled: boolean) {
  const appRoot = document.getElementById('root');

  if (enabled) {
    if (appRoot) {
      appRoot.setAttribute('aria-hidden', 'true');
      appRoot.setAttribute('inert', '');
    }
    if (previousBodyOverflow === null) {
      previousBodyOverflow = document.body.style.overflow;
    }
    document.body.style.overflow = 'hidden';
    return;
  }

  if (appRoot) {
    appRoot.removeAttribute('aria-hidden');
    appRoot.removeAttribute('inert');
  }

  if (previousBodyOverflow !== null) {
    document.body.style.overflow = previousBodyOverflow;
    previousBodyOverflow = null;
  } else {
    document.body.style.overflow = '';
  }
}

function syncGlobalModalEffects() {
  // Safety net: se a stack diz que há modais abertos, mas nenhum overlay
  // está no DOM, a stack ficou dessincronizada (ex: erro de render ou
  // unmount inesperado). Limpa a stack para restaurar a interatividade.
  if (OPEN_MODAL_STACK.length > 0) {
    const actualOverlays = document.querySelectorAll('.modal-overlay').length;
    if (actualOverlays === 0) {
      OPEN_MODAL_STACK.length = 0;
    }
  }
  setGlobalModalEffects(OPEN_MODAL_STACK.length > 0);
}

export function isModalOpen(): boolean {
  return OPEN_MODAL_STACK.length > 0;
}

/**
 * Força a limpeza do estado de modal (inert/aria-hidden) quando a stack
 * ficou dessincronizada. Chamado ao navegar entre páginas como safety net.
 */
export function ensureModalCleanup() {
  const actualOverlays = document.querySelectorAll('.modal-overlay').length;
  if (actualOverlays === 0 && OPEN_MODAL_STACK.length > 0) {
    OPEN_MODAL_STACK.length = 0;
    setGlobalModalEffects(false);
  }
}

/**
 * Contexto que expõe, para os descendentes de um Modal, se aquele Modal é o
 * que está no topo da stack. Reutiliza o mesmo critério usado pelo Modal para
 * tratar ESC/Tab, garantindo que handlers de teclado de conteúdos internos
 * (ex.: setas/zoom do ImageViewerModal) só ajam quando o modal estiver ativo.
 */
const ModalTopmostContext = createContext<(() => boolean) | null>(null);

/**
 * Retorna uma função que indica se o Modal mais próximo (ancestral) é o do
 * topo da stack. Fora de um Modal, assume `true` (sem stack concorrente).
 */
export function useModalIsTopmost(): () => boolean {
  const ctx = useContext(ModalTopmostContext);
  return ctx ?? (() => true);
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
    const id = modalInstanceIdRef.current;
    return OPEN_MODAL_STACK.length > 0 && OPEN_MODAL_STACK[OPEN_MODAL_STACK.length - 1] === id;
  }, []);

  // Valor estável exposto via contexto para descendentes do Modal.
  const topmostValue = useMemo(() => isTopMost, [isTopMost]);

  // Mantém o stack em sync com abertura/fechamento.
  useEffect(() => {
    const id = modalInstanceIdRef.current;
    if (!isOpen) return;

    // Remove qualquer entrada antiga (best-effort), e empilha no topo.
    for (let i = OPEN_MODAL_STACK.length - 1; i >= 0; i--) {
      if (OPEN_MODAL_STACK[i] === id) OPEN_MODAL_STACK.splice(i, 1);
    }
    OPEN_MODAL_STACK.push(id);
    syncGlobalModalEffects();

    return () => {
      for (let i = OPEN_MODAL_STACK.length - 1; i >= 0; i--) {
        if (OPEN_MODAL_STACK[i] === id) OPEN_MODAL_STACK.splice(i, 1);
      }
      syncGlobalModalEffects();
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
    requestAnimationFrame(() => {
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
  }, [isOpen, getFocusableElements]);

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
          <div className="modal-body">
            {children}
          </div>
        </div>
      </div>
    </ModalTopmostContext.Provider>,
    document.body
  );
}

