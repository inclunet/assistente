import { useTranslation } from 'react-i18next';
import {
  DecisionDialog,
  type DecisionSeverity,
} from './DecisionDialog';

export interface ConfirmDialogProps {
  isOpen: boolean;
  title: string;
  message: string;
  confirmText?: string;
  cancelText?: string;
  variant?: 'danger' | 'warning' | 'info';
  onConfirm: () => void;
  onCancel: () => void;
  /**
   * Se false, o chamador restaura o foco (ConfirmHost/confirmStore).
   * Default true para usos diretos (ex.: TerminalPage).
   */
  returnFocusOnClose?: boolean;
}

function toSeverity(variant: ConfirmDialogProps['variant']): DecisionSeverity {
  if (variant === 'danger') return 'destructive';
  if (variant === 'warning') return 'permission';
  return 'info';
}

/**
 * Confirmação binária (AEP-0091): wrapper sobre DecisionDialog.
 * Mantém a API de useConfirm / ConfirmHost.
 */
export function ConfirmDialog({
  isOpen,
  title,
  message,
  confirmText,
  cancelText,
  variant = 'danger',
  onConfirm,
  onCancel,
  returnFocusOnClose = true,
}: ConfirmDialogProps) {
  const { t } = useTranslation();
  const confirmLabel = confirmText ?? t('common.confirm');
  const cancelLabel = cancelText ?? t('common.cancel');

  return (
    <DecisionDialog
      isOpen={isOpen}
      title={title}
      description={message}
      severity={toSeverity(variant)}
      className={`confirm-dialog-modal confirm-dialog-modal--${variant}`}
      safeActionId="cancel"
      actions={[
        {
          id: 'confirm',
          label: confirmLabel,
          variant: variant === 'danger' ? 'danger' : 'primary',
          primary: true,
        },
        {
          id: 'cancel',
          label: cancelLabel,
          variant: 'outline',
        },
      ]}
      onAction={(id) => {
        if (id === 'confirm') onConfirm();
        else onCancel();
      }}
      onCancel={onCancel}
      returnFocusOnClose={returnFocusOnClose}
    />
  );
}
