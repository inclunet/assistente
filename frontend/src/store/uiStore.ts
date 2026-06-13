import { create } from 'zustand';

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
  
  addToast: (message: string, type: Toast['type'], duration?: number, action?: ToastAction) => string;
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
  addToast: (message, type, duration = 5000, action) => {
    const id = `toast-${toastIdCounter++}`;
    set((state) => ({
      toasts: [...state.toasts, { id, message, type, duration, action }],
    }));
    
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
