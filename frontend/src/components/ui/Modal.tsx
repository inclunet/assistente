import { ReactNode, useEffect } from 'react';
import { createPortal } from 'react-dom';
import { useUIStore } from '../../store/uiStore';
import { Button } from './Button';
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

  const handleClose = () => {
    closeModal(id);
    onClose?.();
  };

  // Close on ESC
  useEffect(() => {
    const handleEsc = (e: KeyboardEvent) => {
      if (e.key === 'Escape') handleClose();
    };
    window.addEventListener('keydown', handleEsc);
    return () => window.removeEventListener('keydown', handleEsc);
  }, []);

  return createPortal(
    <div className="modal-overlay" onClick={handleClose}>
      <div
        className={`modal modal--${size}`}
        onClick={(e) => e.stopPropagation()}
      >
        {title && (
          <div className="modal__header">
            <h2 className="modal__title">{title}</h2>
            <button className="modal__close" onClick={handleClose}>
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
