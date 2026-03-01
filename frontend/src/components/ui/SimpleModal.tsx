import { ReactNode, useEffect, useRef, useCallback, useId } from 'react';
import { createPortal } from 'react-dom';
import './SimpleModal.css';

// Stack global simples para garantir que apenas o modal do topo
// trate Escape/Tab/click-outside quando há múltiplos modais abertos.
const OPEN_MODAL_STACK: string[] = [];

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

export interface SimpleModalProps {
  isOpen: boolean;
  onClose: () => void;
  title: string;
  children: ReactNode;
  size?: 'sm' | 'md' | 'lg' | 'xl';
  /** Se true, restaura foco no grid quando o modal fecha. Default: true */
  returnFocusToGrid?: boolean;
  /** Se false, desabilita fechamento (ESC, clique fora e botão X). Default: true */
  allowClose?: boolean;
}

export function SimpleModal({
  isOpen,
  onClose,
  title,
  children,
  size = 'md',
  returnFocusToGrid = true,
  allowClose = true,
}: SimpleModalProps) {
  const modalRef = useRef<HTMLDivElement>(null);
  const firstFocusableRef = useRef<HTMLElement | null>(null);
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

    return () => {
      for (let i = OPEN_MODAL_STACK.length - 1; i >= 0; i--) {
        if (OPEN_MODAL_STACK[i] === id) OPEN_MODAL_STACK.splice(i, 1);
      }
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

  // Esconde conteúdo de background para leitores de tela e evita foco fora do modal
  useEffect(() => {
    if (!isOpen) return;

    const appRoot = document.getElementById('root');
    if (appRoot) {
      appRoot.setAttribute('aria-hidden', 'true');
      appRoot.setAttribute('inert', '');
    }

    return () => {
      if (appRoot) {
        appRoot.removeAttribute('aria-hidden');
        appRoot.removeAttribute('inert');
      }
    };
  }, [isOpen]);

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
        firstFocusableRef.current = firstFocusable;
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
        const overlay = (e.target as HTMLElement).closest('.simple-modal-overlay');
        if (overlay) {
          onClose();
        }
      }
    };

    document.addEventListener('keydown', handleEscape);
    document.addEventListener('keydown', handleTab);
    document.addEventListener('mousedown', handleClickOutside);
    
    // Prevent body scroll
    document.body.style.overflow = 'hidden';

    return () => {
      document.removeEventListener('keydown', handleEscape);
      document.removeEventListener('keydown', handleTab);
      document.removeEventListener('mousedown', handleClickOutside);
      document.body.style.overflow = '';
    };
  }, [isOpen, onClose, getFocusableElements, allowClose, isTopMost]);

  if (!isOpen) return null;

  return createPortal(
    <div className="simple-modal-overlay" role="dialog" aria-modal="true" aria-labelledby={titleId}>
      <div ref={modalRef} className={`simple-modal-content ${size}`}>
        <div className="simple-modal-header">
          <h2 id={titleId} className="simple-modal-title">{title}</h2>
          {allowClose && (
            <button 
              className="simple-modal-close"
              onClick={onClose}
              aria-label="Fechar modal"
            >
              ✕
            </button>
          )}
        </div>
        <div className="simple-modal-body">
          {children}
        </div>
      </div>
    </div>,
    document.body
  );
}
