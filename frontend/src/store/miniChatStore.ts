import { create } from 'zustand';
import i18next from 'i18next';
import type { MediaFile } from '../services/mediaService';
import { useWorkspaceStore } from './workspaceStore';
import { isModalOpen } from '../components/ui/Modal';
import { useUIStore } from './uiStore';
import { ensureWorkspaceTabHasConversation } from '../lib/workspaceConversation';

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
  /** Aba do workspace à qual este modal está vinculado (envio usa sempre este id). */
  boundTabId: string | null;
  contextDisplay: string;
  sessionMeta: unknown;
  focusNonce: number;
  adapterError: string | null;
  open: (contextDisplay: string, meta: unknown, boundTabId: string) => void;
  close: () => void;
  bumpFocus: () => void;
  setAdapterError: (msg: string | null) => void;
  requestOpen: () => Promise<void>;
}

export const useMiniChatStore = create<MiniChatState>((set, get) => ({
  isOpen: false,
  boundTabId: null,
  contextDisplay: '',
  sessionMeta: null,
  focusNonce: 0,
  adapterError: null,

  open: (contextDisplay, meta, boundTabId) => {
    set({
      isOpen: true,
      boundTabId,
      contextDisplay,
      sessionMeta: meta,
      adapterError: null,
      focusNonce: get().focusNonce + 1,
    });
  },

  close: () => {
    set({
      isOpen: false,
      boundTabId: null,
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

    if (get().isOpen) {
      return;
    }

    if (isModalOpen()) {
      return;
    }

    if (tab.type === 'chat') {
      const input = document.querySelector('.chat-page .chat-input textarea') as HTMLTextAreaElement | null;
      if (input) {
        input.focus();
      }
      return;
    }

    try {
      await ensureWorkspaceTabHasConversation(tab);
    } catch (e) {
      console.error('[miniChat] falha ao garantir conversa:', e);
      useUIStore.getState().addToast(
        i18next.t('editor.inlineChat.newConversationError'),
        'error',
      );
      return;
    }

    const adapter = getAdapter(tab.id);
    if (!adapter) {
      useUIStore.getState().addToast(
        i18next.t('workspace.miniChat.panelNotSupported'),
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

    get().open(result.contextDisplay, result.meta, tab.id);
  },
}));
