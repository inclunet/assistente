import { useEffect, useRef } from 'react';
import { CheckOutlined, CloseOutlined, ExclamationOutlined, InfoOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import type { ToastAction } from '../../store/uiStore';
import './Toast.css';

export interface ToastProps {
  message: string;
  variant?: 'success' | 'error' | 'warning' | 'info';
  duration?: number;
  /** Ação opcional (ex.: "Tentar novamente"). Renderiza um botão no toast. */
  action?: ToastAction;
  onClose: () => void;
}

export function Toast({ message, variant = 'info', duration = 3000, action, onClose }: ToastProps) {
  const { t } = useTranslation();
  const onCloseRef = useRef(onClose);
  onCloseRef.current = onClose;

  useEffect(() => {
    // duration <= 0 mantém o toast persistente (ex.: avisos com ação que o
    // usuário precisa ver e decidir). Sem timer nesse caso.
    if (duration <= 0) return;
    const timer = setTimeout(() => onCloseRef.current(), duration);
    return () => clearTimeout(timer);
  }, [duration]);

  // Sem role/aria-live/aria-atomic: o anúncio para leitor de tela é
  // centralizado no announcer único (useAnnouncer), disparado uma única vez ao
  // adicionar o toast (ver uiStore.addToast). Manter uma live region aqui
  // duplicaria a fala quando o call site também chama announce(). O toast
  // permanece visual e operável (botões de ação/fechar continuam focáveis).
  return (
    <div className={`toast toast--${variant}`}>
      <div className="toast__icon" aria-hidden="true">
        {variant === 'success' && <CheckOutlined />}
        {variant === 'error' && <CloseOutlined />}
        {variant === 'warning' && <ExclamationOutlined />}
        {variant === 'info' && <InfoOutlined />}
      </div>
      <div className="toast__message">{message}</div>
      {action && (
        <button
          type="button"
          className="toast__action"
          onClick={action.onClick}
        >
          {action.label}
        </button>
      )}
      <button
        type="button"
        className="toast__close"
        onClick={onClose}
        aria-label={t('ui.toast.close')}
      >
        <CloseOutlined aria-hidden="true" />
      </button>
    </div>
  );
}
