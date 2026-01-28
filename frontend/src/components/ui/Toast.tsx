import { useEffect } from 'react';
import { useUIStore } from '../../store/uiStore';
import './Toast.css';

export function ToastContainer() {
  const { toasts, removeToast } = useUIStore();

  return (
    <div className="toast-container">
      {toasts.map((toast) => (
        <Toast key={toast.id} {...toast} onClose={() => removeToast(toast.id)} />
      ))}
    </div>
  );
}

interface ToastProps {
  id: string;
  message: string;
  type: 'success' | 'error' | 'warning' | 'info';
  onClose: () => void;
}

function Toast({ message, type, onClose }: ToastProps) {
  useEffect(() => {
    const timer = setTimeout(onClose, 5000);
    return () => clearTimeout(timer);
  }, [onClose]);

  const icons = {
    success: '✓',
    error: '✕',
    warning: '⚠',
    info: 'ℹ',
  };

  return (
    <div className={`toast toast--${type}`}>
      <span className="toast__icon">{icons[type]}</span>
      <span className="toast__message">{message}</span>
      <button className="toast__close" onClick={onClose}>
        ×
      </button>
    </div>
  );
}
