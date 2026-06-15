import { create } from 'zustand';
import { announceWithOrigin } from '../services/voiceAccessibility/announcerBroker';

/** Opções extras ao adicionar um toast. */
export interface AddToastOptions {
  /**
   * Não dispara o anúncio automático para leitor de tela. Usado por call sites
   * que já emitem um announce() próprio (ex.: listeners de conexão e de runtime
   * parcial), evitando fala duplicada.
   */
  suppressAnnounce?: boolean;
}

interface Modal {
  id: string;
  type: string;
  data?: unknown;
}

/** Ação opcional exibida no toast (ex.: "Tentar novamente"). */
export interface ToastAction {
  /** Rótulo acessível do botão (já traduzido). */
  label: string;
  /** Callback disparado ao acionar a ação. */
  onClick: () => void;
}

interface Toast {
  id: string;
  message: string;
  type: 'success' | 'error' | 'warning' | 'info';
  duration?: number;
  action?: ToastAction;
}

interface UIState {
  // Modals
  modals: Modal[];
  
  // Toasts/Notifications
  toasts: Toast[];
  
  // Loading states
  globalLoading: boolean;
  
  // Actions
  openModal: (type: string, data?: unknown) => string;
  closeModal: (id: string) => void;
  closeAllModals: () => void;
  
  addToast: (
    message: string,
    type: Toast['type'],
    duration?: number,
    action?: ToastAction,
    options?: AddToastOptions,
  ) => string;
  removeToast: (id: string) => void;
  
  setGlobalLoading: (loading: boolean) => void;
}

let toastIdCounter = 0;
let modalIdCounter = 0;

export const useUIStore = create<UIState>((set) => ({
  // Initial state
  modals: [],
  toasts: [],
  globalLoading: false,

  // Modal actions
  openModal: (type, data) => {
    const id = `modal-${modalIdCounter++}`;
    set((state) => ({
      modals: [...state.modals, { id, type, data }],
    }));
    return id;
  },
  
  closeModal: (id) =>
    set((state) => ({
      modals: state.modals.filter((m) => m.id !== id),
    })),
  
  closeAllModals: () => set({ modals: [] }),

  // Toast actions
  addToast: (message, type, duration = 5000, action, options) => {
    const id = `toast-${toastIdCounter++}`;
    set((state) => ({
      toasts: [...state.toasts, { id, message, type, duration, action }],
    }));

    // Anúncio único para leitor de tela. O Toast deixou de ser uma live region
    // (sem role/aria-live), então centralizamos a fala aqui — assim todo toast
    // é anunciado exatamente uma vez, inclusive os criados fora de componentes
    // React (useUIStore.getState().addToast). Erros/avisos vão como assertivos;
    // sucesso/info como polidos. Call sites que já anunciam por conta própria
    // passam suppressAnnounce para não duplicar.
    if (!options?.suppressAnnounce) {
      announceWithOrigin({
        message,
        announcePriority: type === 'error' || type === 'warning' ? 'assertive' : 'polite',
        eventType: 'user-action',
      });
    }

    // Auto remove after duration (duration <= 0 = persistente até ação/fechar)
    if (duration > 0) {
      setTimeout(() => {
        set((state) => ({
          toasts: state.toasts.filter((t) => t.id !== id),
        }));
      }, duration);
    }
    
    return id;
  },
  
  removeToast: (id) =>
    set((state) => ({
      toasts: state.toasts.filter((t) => t.id !== id),
    })),

  // Global loading
  setGlobalLoading: (loading) => set({ globalLoading: loading }),
}));
