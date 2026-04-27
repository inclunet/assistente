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

export type WorkspaceChatModalPrepareOk = {
  ok: true;
  contextDisplay: string;
  meta: unknown;
};

export type WorkspaceChatModalPrepareFail = {
  ok: false;
  message?: string;
};

export type WorkspaceChatModalPrepareResult =
  | WorkspaceChatModalPrepareOk
  | WorkspaceChatModalPrepareFail;

export type WorkspaceChatModalSession = {
  tabId: string;
  conversationId: string;
};

export type WorkspaceChatSendPlan = {
  content: string;
  mediaFiles?: MediaFile[];
  paramsOverride?: Partial<llm.ChatParams>;
  afterSend?: () => Promise<void>;
  onSendError?: (error: unknown) => void;
} | null;

export interface WorkspaceChatModalAdapter {
  prepare: () => Promise<WorkspaceChatModalPrepareResult>;
  send: (
    instruction: string,
    media: MediaFile[] | undefined,
    meta: unknown,
    session: WorkspaceChatModalSession,
  ) => Promise<WorkspaceChatSendPlan>;
}

const adapters = new Map<string, WorkspaceChatModalAdapter>();

export function registerWorkspaceChatModalAdapter(
  tabId: string,
  adapter: WorkspaceChatModalAdapter | null,
) {
  if (!adapter) {
    adapters.delete(tabId);
    return;
  }
  adapters.set(tabId, adapter);
}

export function getWorkspaceChatModalAdapter(
  tabId: string | undefined | null,
): WorkspaceChatModalAdapter | null {
  if (!tabId) return null;
  return adapters.get(tabId) ?? null;
}

interface WorkspaceChatModalState {
  isOpen: boolean;
  /** Aba do workspace à qual este modal de chat está vinculado. */
  boundTabId: string | null;
  /** Conversa garantida ao abrir o modal; usada para recuperar o chatStore antes do envio. */
  boundConversationId: string | null;
  contextDisplay: string;
  sessionMeta: unknown;
  /** `adapter.send` capturado no `open()` para não depender do mapa global no clique. */
  boundSend: WorkspaceChatModalAdapter['send'] | null;
  focusNonce: number;
  adapterError: string | null;
  open: (
    contextDisplay: string,
    meta: unknown,
    boundTabId: string,
    boundConversationId: string,
    send: WorkspaceChatModalAdapter['send'],
  ) => void;
  close: () => void;
  bumpFocus: () => void;
  setAdapterError: (msg: string | null) => void;
  requestOpen: () => Promise<void>;
}

export const useWorkspaceChatModalStore = create<WorkspaceChatModalState>((set, get) => ({
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
        i18next.t('workspace.chatModal.modalBlocked'),
        'info',
      );
      return;
    }

    if (tab.type === 'chat') {
      const input = document.querySelector('.chat-page .chat-input__textarea') as HTMLTextAreaElement | null;
      if (input) {
        input.focus();
      }
      return;
    }

    const adapter = getWorkspaceChatModalAdapter(tab.id);
    if (!adapter) {
      useUIStore.getState().addToast(
        i18next.t('workspace.chatModal.panelNotSupported'),
        'info',
      );
      return;
    }

    let result: WorkspaceChatModalPrepareResult;
    try {
      result = await adapter.prepare();
    } catch (e) {
      console.error('[workspaceChatModal] prepare() falhou:', e);
      useUIStore.getState().addToast(
        i18next.t('workspace.chatModal.prepareFailed'),
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

    let conversationId: string;
    try {
      conversationId = await ensureWorkspaceTabHasConversation(tab);
    } catch (e) {
      console.error('[workspaceChatModal] falha ao garantir conversa:', e);
      useUIStore.getState().addToast(
        i18next.t('editor.chatModal.newConversationError'),
        'error',
      );
      return;
    }

    get().open(result.contextDisplay, result.meta, tab.id, conversationId, adapter.send);
  },
}));
