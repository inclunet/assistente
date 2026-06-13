import { useUIStore } from '../../store/uiStore';
import { Toast } from './Toast';
import './Toast.css';

/**
 * Host global que renderiza os toasts da uiStore. Montado uma única vez na
 * árvore (em App). Cada Toast já tem role="alert"/aria-live próprio; este host
 * apenas posiciona a pilha. Anúncios para leitor de tela continuam indo pelo
 * announcer único (ScreenReaderAnnouncer), não por este container.
 */
export function ToastHost() {
  const toasts = useUIStore((s) => s.toasts);
  const removeToast = useUIStore((s) => s.removeToast);

  if (toasts.length === 0) return null;

  return (
    <div className="toast-container">
      {toasts.map((toast) => (
        <Toast
          key={toast.id}
          message={toast.message}
          variant={toast.type}
          duration={toast.duration}
          action={toast.action}
          onClose={() => removeToast(toast.id)}
        />
      ))}
    </div>
  );
}
