import { create } from 'zustand';
import type { MediaFile } from '../services/mediaService';
import { useWorkspaceStore } from './workspaceStore';
import { isModalOpen } from '../components/ui/Modal';
import { useUIStore } from './uiStore';

export type MiniChatPrepareOk = {
  ok: true;
  contextDisplay: string;
  meta: unknown;
};

export type MiniChatPrepareFail = {
  ok: false;
  message?: string;
};

export type MiniChatPrepareResult = MiniChatPrepareOk | MiniChatPrepareFail;

export interface MiniChatAdapter {
  prepare: () => Promise<MiniChatPrepareResult>;
  send: (instruction: string, media: MediaFile[] | undefined, meta: unknown) => Promise<void>;
}

const adapters = new Map<string, MiniChatAdapter>();

export function registerMiniChatAdapter(tabId: string, adapter: MiniChatAdapter | null) {
  if (!adapter) {
    adapters.delete(tabId);
    return;
  }
  adapters.set(tabId, adapter);
}

export function getAdapter(tabId: string | undefined | null): MiniChatAdapter | null {
  if (!tabId) return null;
  return adapters.get(tabId) ?? null;
}

interface MiniChatState {
  isOpen: boolean;
  contextDisplay: string;
  sessionMeta: unknown;
  focusNonce: number;
  adapterError: string | null;
  open: (contextDisplay: string, meta: unknown) => void;
  close: () => void;
  bumpFocus: () => void;
  setAdapterError: (msg: string | null) => void;
  requestOpen: () => Promise<void>;
}

export const useMiniChatStore = create<MiniChatState>((set, get) => ({
  isOpen: false,
  contextDisplay: '',
  sessionMeta: null,
  focusNonce: 0,
  adapterError: null,

  open: (contextDisplay, meta) => {
    set({
      isOpen: true,
      contextDisplay,
      sessionMeta: meta,
      adapterError: null,
      focusNonce: get().focusNonce + 1,
    });
  },

  close: () => {
    set({
      isOpen: false,
      contextDisplay: '',
      sessionMeta: null,
      adapterError: null,
    });
  },

  bumpFocus: () => {
    set({ focusNonce: get().focusNonce + 1 });
  },

  setAdapterError: (msg) => set({ adapterError: msg }),

  requestOpen: async () => {
    const tab = useWorkspaceStore.getState().getActiveTab();
    if (!tab) return;

    if (tab.type === 'chat') {
      const input = document.querySelector('.chat-page .chat-input textarea') as HTMLTextAreaElement | null;
      if (input) {
        input.focus();
      }
      return;
    }

    if (get().isOpen) {
      return;
    }

    if (isModalOpen()) {
      return;
    }

    const adapter = getAdapter(tab.id);
    if (!adapter) {
      useUIStore.getState().addToast(
        'Este painel ainda não suporta o mini-chat.',
        'info',
      );
      return;
    }

    const result = await adapter.prepare();
    if (!result.ok) {
      if (result.message) {
        useUIStore.getState().addToast(result.message, 'info');
      }
      return;
    }

    get().open(result.contextDisplay, result.meta);
  },
}));
