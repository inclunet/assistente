import { create } from 'zustand';

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
