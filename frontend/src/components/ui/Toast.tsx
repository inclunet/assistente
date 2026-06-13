import { useEffect, useRef } from 'react';
import { CheckOutlined, CloseOutlined, ExclamationOutlined, InfoOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import './Toast.css';

export interface ToastAction {
  label: string;
  onClick: () => void;
}

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

  return (
    <div
      className={`toast toast--${variant}`}
      role="alert"
      aria-live="assertive"
      aria-atomic="true"
    >
      <div className="toast__icon" aria-hidden="true">
        {variant === 'success' && <CheckOutlined />}
        {variant === 'error' && <CloseOutlined />}
        {variant === 'warning' && <ExclamationOutlined />}
        {variant === 'info' && <InfoOutlined />}
      </div>
      <div className="toast__message">{message}</div>
      {action && (
        <button
          className="toast__action"
          onClick={action.onClick}
        >
          {action.label}
        </button>
      )}
      <button
        className="toast__close"
        onClick={onClose}
        aria-label={t('ui.toast.close')}
      >
        <CloseOutlined aria-hidden="true" />
      </button>
    </div>
  );
}
