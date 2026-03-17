import { useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import './Toast.css';

export interface ToastProps {
  message: string;
  variant?: 'success' | 'error' | 'warning' | 'info';
  duration?: number;
  onClose: () => void;
}

export function Toast({ message, variant = 'info', duration = 3000, onClose }: ToastProps) {
  const { t } = useTranslation();
  const onCloseRef = useRef(onClose);
  onCloseRef.current = onClose;

  useEffect(() => {
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
      <div className="toast__icon">
        {variant === 'success' && '✓'}
        {variant === 'error' && '✕'}
        {variant === 'warning' && '⚠'}
        {variant === 'info' && 'ℹ'}
      </div>
      <div className="toast__message">{message}</div>
      <button
        className="toast__close"
        onClick={onClose}
        aria-label={t('ui.toast.close')}
      >
        ✕
      </button>
    </div>
  );
}
