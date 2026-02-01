import { useEffect, useRef } from 'react';
import { Button } from './Button';
import './ConfirmDialog.css';

export interface ConfirmDialogProps {
  isOpen: boolean;
  title: string;
  message: string;
  confirmText?: string;
  cancelText?: string;
  variant?: 'danger' | 'warning' | 'info';
  onConfirm: () => void;
  onCancel: () => void;
}

export function ConfirmDialog({
  isOpen,
  title,
  message,
  confirmText = 'Confirmar',
  cancelText = 'Cancelar',
  variant = 'danger',
  onConfirm,
  onCancel,
}: ConfirmDialogProps) {
  const cancelButtonRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (isOpen) {
      // Focus cancel button by default (safer for destructive actions)
      cancelButtonRef.current?.focus();

      // Handle Escape key
      const handleEscape = (e: KeyboardEvent) => {
        if (e.key === 'Escape') {
          onCancel();
        }
      };

      document.addEventListener('keydown', handleEscape);
      return () => document.removeEventListener('keydown', handleEscape);
    }
  }, [isOpen, onCancel]);

  if (!isOpen) return null;

  return (
    <div
      className="confirm-dialog-overlay"
      onClick={onCancel}
      role="dialog"
      aria-modal="true"
      aria-labelledby="confirm-dialog-title"
      aria-describedby="confirm-dialog-message"
    >
      <div
        className={`confirm-dialog confirm-dialog--${variant}`}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="confirm-dialog__header">
          <h2 id="confirm-dialog-title" className="confirm-dialog__title">
            {title}
          </h2>
        </div>

        <div className="confirm-dialog__body">
          <p id="confirm-dialog-message" className="confirm-dialog__message">
            {message}
          </p>
        </div>

        <div className="confirm-dialog__footer">
          <Button
            ref={cancelButtonRef}
            variant="outline"
            onClick={onCancel}
          >
            {cancelText}
          </Button>
          <Button
            variant={variant === 'danger' ? 'danger' : 'primary'}
            onClick={onConfirm}
          >
            {confirmText}
          </Button>
        </div>
      </div>
    </div>
  );
}
