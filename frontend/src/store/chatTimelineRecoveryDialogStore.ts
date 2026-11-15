import { create } from 'zustand';

let lastOrigin: HTMLElement | null = null;
export function rememberRecoveryOrigin(target: HTMLElement | null): HTMLElement | null {
  if (target && target !== document.body && target !== document.documentElement && target.isConnected && !target.closest('[role="alertdialog"]')) lastOrigin = target;
  if (lastOrigin && !lastOrigin.isConnected) lastOrigin = null;
  return lastOrigin;
}
export function clearRecoveryOrigin() { lastOrigin = null; }

/** Serializa os avisos de recuperação entre superfícies e o host global. */
export const useChatTimelineRecoveryDialogStore = create<{
  owner: symbol | null;
  acquire(owner: symbol): boolean;
  release(owner: symbol): void;
}>((set, get) => ({
  owner: null,
  acquire: owner => {
    if (get().owner && get().owner !== owner) return false;
    set({ owner });
    return true;
  },
  release: owner => { if (get().owner === owner) set({ owner: null }); },
}));
