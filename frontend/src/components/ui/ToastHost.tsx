import { useUIStore } from '../../store/uiStore';
import { Toast } from './Toast';
// Toast.css é importado por Toast.tsx (renderizado aqui); evita import redundante.

/**
 * Host global que renderiza os toasts da uiStore. Montado uma única vez na
 * árvore (em App). Os Toasts não são mais live regions (sem role/aria-live);
 * o anúncio para leitor de tela acontece uma única vez no uiStore.addToast,
 * via announcer único (ScreenReaderAnnouncer). Este host apenas posiciona a
 * pilha visual.
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
