import { create } from 'zustand';
import i18next from 'i18next';
import type { MediaFile } from '../services/mediaService';
import type { llm } from '../../wailsjs/go/models';
import { useWorkspaceStore } from './workspaceStore';
import { isModalOpen } from '../components/ui/Modal';
import { useUIStore } from './uiStore';
import { ensureWorkspaceTabHasConversation } from '../lib/workspaceConversation';
import { ttsService } from '../services/tts';
import { messageAudioService } from '../services/messageAudio';

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

export type MiniChatSession = {
  tabId: string;
  conversationId: number;
};

export type MiniChatSendPlan = {
  content: string;
  mediaFiles?: MediaFile[];
  paramsOverride?: Partial<llm.ChatParams>;
  afterSend?: () => Promise<void>;
  onSendError?: (error: unknown) => void;
} | null;

export interface MiniChatAdapter {
  prepare: () => Promise<MiniChatPrepareResult>;
  send: (
    instruction: string,
    media: MediaFile[] | undefined,
    meta: unknown,
    session: MiniChatSession,
  ) => Promise<MiniChatSendPlan>;
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
  /** Conversa garantida ao abrir o mini-chat; usada para recuperar o chatStore antes do envio. */
  boundConversationId: number | null;
  contextDisplay: string;
  sessionMeta: unknown;
  /** `adapter.send` capturado no `open()` — o envio não depende do mapa global no momento do clique. */
  boundSend: MiniChatAdapter['send'] | null;
  focusNonce: number;
  adapterError: string | null;
  open: (
    contextDisplay: string,
    meta: unknown,
    boundTabId: string,
    boundConversationId: number,
    send: MiniChatAdapter['send'],
  ) => void;
  close: () => void;
  bumpFocus: () => void;
  setAdapterError: (msg: string | null) => void;
  requestOpen: () => Promise<void>;
}

export const useMiniChatStore = create<MiniChatState>((set, get) => ({
  isOpen: false,
  boundTabId: null,
  boundConversationId: null,
  contextDisplay: '',
  sessionMeta: null,
  boundSend: null,
  focusNonce: 0,
  adapterError: null,

  open: (contextDisplay, meta, boundTabId, boundConversationId, send) => {
    set({
      isOpen: true,
      boundTabId,
      boundConversationId,
      contextDisplay,
      sessionMeta: meta,
      boundSend: send,
      adapterError: null,
      focusNonce: get().focusNonce + 1,
    });
  },

  close: () => {
    ttsService.stop();
    messageAudioService.stopCurrentAudio();
    set({
      isOpen: false,
      boundTabId: null,
      boundConversationId: null,
      contextDisplay: '',
      sessionMeta: null,
      boundSend: null,
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
      get().bumpFocus();
      return;
    }

    if (isModalOpen()) {
      useUIStore.getState().addToast(
        i18next.t('workspace.miniChat.modalBlocked'),
        'info',
      );
      return;
    }

    if (tab.type === 'chat') {
      const input = document.querySelector('.chat-page .chat-input textarea') as HTMLTextAreaElement | null;
      if (input) {
        input.focus();
      }
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

    let result: MiniChatPrepareResult;
    try {
      result = await adapter.prepare();
    } catch (e) {
      console.error('[miniChat] prepare() falhou:', e);
      useUIStore.getState().addToast(
        i18next.t('workspace.miniChat.prepareFailed'),
        'error',
      );
      return;
    }

    if (!result.ok) {
      if (result.message) {
        useUIStore.getState().addToast(result.message, 'info');
      }
      return;
    }

    let conversationId: number;
    try {
      conversationId = await ensureWorkspaceTabHasConversation(tab);
    } catch (e) {
      console.error('[miniChat] falha ao garantir conversa:', e);
      useUIStore.getState().addToast(
        i18next.t('editor.inlineChat.newConversationError'),
        'error',
      );
      return;
    }

    get().open(result.contextDisplay, result.meta, tab.id, conversationId, adapter.send);
  },
}));
