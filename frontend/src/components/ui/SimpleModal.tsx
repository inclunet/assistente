import { ReactNode, useEffect, useRef } from 'react';
import { createPortal } from 'react-dom';
import './SimpleModal.css';

export interface SimpleModalProps {
  isOpen: boolean;
  onClose: () => void;
  title: string;
  children: ReactNode;
  size?: 'sm' | 'md' | 'lg' | 'xl';
}

export function SimpleModal({ isOpen, onClose, title, children, size = 'md' }: SimpleModalProps) {
  const modalRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!isOpen) return;

    const handleEscape = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onClose();
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
    document.addEventListener('mousedown', handleClickOutside);
    
    // Prevent body scroll
    document.body.style.overflow = 'hidden';

    return () => {
      document.removeEventListener('keydown', handleEscape);
      document.removeEventListener('mousedown', handleClickOutside);
      document.body.style.overflow = '';
    };
  }, [isOpen, onClose]);

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
