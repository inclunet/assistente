import type { ReactNode } from 'react';
import './DialogActions.css';

export interface DialogActionsProps {
  /** Ação primária (Confirmar, Salvar, OK, Aplicar, Ir). Renderizada primeiro no DOM. */
  primary: ReactNode;
  /** Ação secundária (Cancelar e equivalentes). Renderizada depois da primária. */
  secondary?: ReactNode;
  className?: string;
}

/**
 * Rodapé de ações de diálogo/modal/formulário (AEP-0090).
 * Ordem DOM = Tab = NVDA: primária → secundária (cancelar).
 * Não usar row-reverse/order para mascarar essa ordem.
 */
export function DialogActions({ primary, secondary, className }: DialogActionsProps) {
  return (
    <div
      className={`dialog-actions${className ? ` ${className}` : ''}`}
      data-dialog-actions=""
    >
      {primary}
      {secondary}
    </div>
  );
}
