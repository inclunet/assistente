import { create } from 'zustand';
import { restoreDefaultFocus } from '../hooks/useDefaultFocus';
import { isModalOpen } from '../lib/modalRegistry';

export type ConfirmVariant = 'danger' | 'warning' | 'info';

export interface ConfirmOptions {
  title: string;
  message: string;
  confirmText?: string;
  cancelText?: string;
  variant?: ConfirmVariant;
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
    if (isModalOpen()) return;

    const activeElement = document.activeElement as HTMLElement | null;
    const focusIsIdle = !activeElement || activeElement === document.body || !document.contains(activeElement);
    if (!focusIsIdle && activeElement !== el) return;

    try {
      const targetIsUsable = Boolean(
        el
        && document.contains(el)
        && !el.matches?.(':disabled, [aria-disabled="true"]')
        && !el.closest?.('[hidden], [aria-hidden="true"], [inert]'),
      );
      if (targetIsUsable) {
        el?.focus();
        if (document.activeElement === el) return;
      }
    } catch {
      // O fallback abaixo cobre alvo inválido ou focus() rejeitado.
    }
    restoreDefaultFocus();
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
      safeRestoreFocus(internal.restoreFocusTo);
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
    queue.push({ request, resolve, restoreFocusTo });
    showNext();
  });
}
