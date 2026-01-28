import { ReactNode, useEffect, useRef, useCallback } from 'react';
import { createPortal } from 'react-dom';
import './SimpleModal.css';

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
}

export function SimpleModal({ isOpen, onClose, title, children, size = 'md', returnFocusToGrid = true }: SimpleModalProps) {
  const modalRef = useRef<HTMLDivElement>(null);
  const firstFocusableRef = useRef<HTMLElement | null>(null);

  // Retorna todos os elementos focáveis dentro do modal
  const getFocusableElements = useCallback(() => {
    if (!modalRef.current) return [];
    return Array.from(modalRef.current.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR))
      .filter(el => el.offsetParent !== null); // Filtra elementos visíveis
  }, []);

  // Restaura foco no grid quando o componente desmonta (modal fecha)
  useEffect(() => {
    return () => {
      if (returnFocusToGrid) {
        focusGridFirstCell();
      }
    };
  }, [returnFocusToGrid]);

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
      if (e.key === 'Escape') {
        onClose();
      }
    };

    // Focus trap - mantém o foco dentro do modal
    const handleTab = (e: KeyboardEvent) => {
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
  }, [isOpen, onClose, getFocusableElements]);

  if (!isOpen) return null;

  return createPortal(
    <div className="simple-modal-overlay" role="dialog" aria-modal="true" aria-labelledby="modal-title">
      <div ref={modalRef} className={`simple-modal-content ${size}`}>
        <div className="simple-modal-header">
          <h2 id="modal-title" className="simple-modal-title">{title}</h2>
          <button 
            className="simple-modal-close"
            onClick={onClose}
            aria-label="Fechar modal"
          >
            ✕
          </button>
        </div>
        <div className="simple-modal-body">
          {children}
        </div>
      </div>
    </div>,
    document.body
  );
}
