import { ReactNode, useEffect, useRef, useCallback, useId } from 'react';
import { createPortal } from 'react-dom';
import { useTranslation } from 'react-i18next';
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
  setGlobalModalEffects(OPEN_MODAL_STACK.length > 0);
}

export function isModalOpen(): boolean {
  return OPEN_MODAL_STACK.length > 0;
}

// Seletor para elementos focáveis
const FOCUSABLE_SELECTOR = 
  'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), ' +
  'textarea:not([disabled]), [tabindex]:not([tabindex="-1"]), [contenteditable]';

// Função helper para restaurar foco no grid (se existir na página)
function focusGridFirstCell() {
  requestAnimationFrame(() => {
    const grid = document.querySelector('[role="grid"]');
    if (grid) {
      // Procura a primeira célula focável (tabIndex=0) ou qualquer gridcell
      const focusableCell = grid.querySelector('[role="gridcell"][tabindex="0"]') as HTMLElement 
        || grid.querySelector('[role="gridcell"]') as HTMLElement;
      if (focusableCell) {
        focusableCell.focus();
      }
    }
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
  /** Se true, restaura foco no grid quando o modal fecha. Default: true */
  returnFocusToGrid?: boolean;
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
  returnFocusToGrid = true,
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

  // Restaura foco no grid quando isOpen transita de true → false
  useEffect(() => {
    if (prevOpenRef.current && !isOpen && returnFocusToGrid) {
      focusGridFirstCell();
    }
    prevOpenRef.current = isOpen;
  }, [isOpen, returnFocusToGrid]);

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
    <div
      className="modal-overlay"
      role="dialog"
      aria-modal="true"
      aria-labelledby={titleId}
      aria-describedby={ariaDescribedBy}
    >
      <div ref={modalRef} className={`modal-content ${size}${className ? ` ${className}` : ''}`}>
        <div className="modal-header">
          <h2 id={titleId} className="modal-title">{title}</h2>
          {allowClose && (
            <button 
              className="modal-close"
              onClick={onClose}
              aria-label={t('ui.modal.close')}
            >
              ✕
            </button>
          )}
        </div>
        <div className="modal-body">
          {children}
        </div>
      </div>
    </div>,
    document.body
  );
}

