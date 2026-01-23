import { ReactNode, useEffect, useRef } from 'react';
import { createPortal } from 'react-dom';
import { useUIStore } from '../../store/uiStore';
import './Modal.css';

export interface ModalProps {
  id: string;
  title?: string;
  children: ReactNode;
  onClose?: () => void;
  footer?: ReactNode;
  size?: 'sm' | 'md' | 'lg' | 'xl';
}

export function Modal({ id, title, children, onClose, footer, size = 'md' }: ModalProps) {
  const { closeModal } = useUIStore();
  const modalRef = useRef<HTMLDivElement>(null);
  const previousActiveElement = useRef<HTMLElement | null>(null);

  const handleClose = () => {
    closeModal(id);
    onClose?.();
  };

  // Focus trap e restauração
  useEffect(() => {
    // Salva elemento focado antes
    previousActiveElement.current = document.activeElement as HTMLElement;

    // Foca no modal
    if (modalRef.current) {
      modalRef.current.focus();
    }

    // Esconde conteúdo de background para leitores de tela
    const appRoot = document.getElementById('root');
    if (appRoot) {
      appRoot.setAttribute('aria-hidden', 'true');
    }

    return () => {
      // Restaura visibilidade do background
      if (appRoot) {
        appRoot.removeAttribute('aria-hidden');
      }
      // Restaura foco
      previousActiveElement.current?.focus();
    };
  }, []);

  // Close on ESC e trap de foco
  useEffect(() => {
    const handleEsc = (e: KeyboardEvent) => {
      if (e.key === 'Escape') handleClose();
    };

    const handleTab = (e: KeyboardEvent) => {
      if (e.key !== 'Tab' || !modalRef.current) return;

      const focusableElements = modalRef.current.querySelectorAll(
        'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'
      );
      const firstElement = focusableElements[0] as HTMLElement;
      const lastElement = focusableElements[focusableElements.length - 1] as HTMLElement;

      if (e.shiftKey && document.activeElement === firstElement) {
        e.preventDefault();
        lastElement?.focus();
      } else if (!e.shiftKey && document.activeElement === lastElement) {
        e.preventDefault();
        firstElement?.focus();
      }
    };

    window.addEventListener('keydown', handleEsc);
    window.addEventListener('keydown', handleTab);
    return () => {
      window.removeEventListener('keydown', handleEsc);
      window.removeEventListener('keydown', handleTab);
    };
  }, []);

  return createPortal(
    <div className="modal-overlay" onClick={handleClose}>
      <div
        ref={modalRef}
        className={`modal modal--${size}`}
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-labelledby={title ? `modal-title-${id}` : undefined}
        tabIndex={-1}
      >
        {title && (
          <div className="modal__header">
            <h2 id={`modal-title-${id}`} className="modal__title">{title}</h2>
            <button className="modal__close" onClick={handleClose} aria-label="Fechar">
              ×
            </button>
          </div>
        )}
        <div className="modal__body">{children}</div>
        {footer && <div className="modal__footer">{footer}</div>}
      </div>
    </div>,
    document.body
  );
}

// Helper component for modal footer with actions
export function ModalActions({ children }: { children: ReactNode }) {
  return <div className="modal-actions">{children}</div>;
}
