import { create } from 'zustand';

export type ConfirmVariant = 'danger' | 'warning' | 'info';

export interface ConfirmOptions {
  title: string;
  message: string;
  confirmText?: string;
  cancelText?: string;
  variant?: ConfirmVariant;
  /**
   * Se false, não restaura o foco ao confirmar (útil antes de navegar).
   * Cancelar sempre restaura o foco. Default: true.
   */
  restoreFocusOnConfirm?: boolean;
}

export interface ConfirmRequest extends Required<Pick<ConfirmOptions, 'title' | 'message'>> {
  id: string;
  confirmText: string;
  cancelText: string;
  variant: ConfirmVariant;
}

interface ConfirmInternal {
  request: ConfirmRequest;
  resolve: (value: boolean) => void;
  restoreFocusTo: HTMLElement | null;
  restoreFocusOnConfirm: boolean;
}

const queue: ConfirmInternal[] = [];
let activeInternal: ConfirmInternal | null = null;

function nextId(): string {
  try {
    return crypto.randomUUID();
  } catch {
    return `confirm-${Date.now()}-${Math.random().toString(16).slice(2)}`;
  }
}

function safeRestoreFocus(el: HTMLElement | null) {
  requestAnimationFrame(() => {
    try {
      if (el && document.contains(el)) {
        el.focus?.();
      }
    } catch {
      // best-effort
    }
  });
}

function showNext() {
  if (activeInternal) return;
  const next = queue.shift();
  if (!next) return;
  activeInternal = next;
  useConfirmStore.setState({ active: next.request });
}

interface ConfirmUIState {
  active: ConfirmRequest | null;
  respond: (confirmed: boolean) => void;
  cancel: () => void;
  confirm: () => void;
}

export const useConfirmStore = create<ConfirmUIState>(() => ({
  active: null,
  respond: (confirmed) => {
    const internal = activeInternal;
    activeInternal = null;
    useConfirmStore.setState({ active: null });

    if (internal) {
      internal.resolve(confirmed);
      if (!confirmed || internal.restoreFocusOnConfirm) {
        safeRestoreFocus(internal.restoreFocusTo);
      }
    }

    showNext();
  },
  cancel: () => useConfirmStore.getState().respond(false),
  confirm: () => useConfirmStore.getState().respond(true),
}));

export function requestConfirm(options: ConfirmOptions): Promise<boolean> {
  const request: ConfirmRequest = {
    id: nextId(),
    title: options.title,
    message: options.message,
    confirmText: options.confirmText ?? 'Confirmar',
    cancelText: options.cancelText ?? 'Cancelar',
    variant: options.variant ?? 'danger',
  };

  const restoreFocusTo = (document.activeElement as HTMLElement | null) ?? null;

  return new Promise<boolean>((resolve) => {
    queue.push({
      request,
      resolve,
      restoreFocusTo,
      restoreFocusOnConfirm: options.restoreFocusOnConfirm ?? true,
    });
    showNext();
  });
}
