import { useId } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from './Button';
import { Modal } from './Modal';
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
  const { t } = useTranslation();
  const messageId = useId();

  return (
    <Modal
      isOpen={isOpen}
      onClose={onCancel}
      title={title}
      size="sm"
      className={`confirm-dialog-modal confirm-dialog-modal--${variant}`}
      ariaDescribedBy={messageId}
      returnFocusToGrid={false}
    >
      <div className="confirm-dialog__body">
        <p id={messageId} className="confirm-dialog__message">
          {message}
        </p>
      </div>

      <div className="confirm-dialog__footer">
        <Button variant="outline" onClick={onCancel}>
          {cancelText ?? t('common.cancel')}
        </Button>
        <Button variant={variant === 'danger' ? 'danger' : 'primary'} onClick={onConfirm}>
          {confirmText ?? t('common.confirm')}
        </Button>
      </div>
    </Modal>
  );
}
