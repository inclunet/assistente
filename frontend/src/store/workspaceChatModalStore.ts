import { logger } from '../utils/logger';
import { create } from 'zustand';
import i18next from 'i18next';
import type { MediaFile } from '../services/mediaService';
import type { llm } from '../../wailsjs/go/models';
import { useWorkspaceStore } from './workspaceStore';
import { isModalOpen } from '../components/ui/Modal';
import { useUIStore } from './uiStore';
import { ensureWorkspaceTabConversationId } from '../lib/workspaceConversation';
import { ttsService } from '../services/tts';
import { messageAudioService } from '../services/messageAudio';
import {
  buildWorkspaceModalChatSurfaceId,
  createChatSurfaceIdentity,
  type ChatSurfaceIdentity,
} from '../services/chatSessionRegistry';

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

// Serializa a persistência do vínculo de conversa na aba (latest-wins POR ABA): as
// escritas são encadeadas e cada uma só executa se ainda for a mais recente
// solicitada para AQUELA aba. Sem a fila, trocas rápidas poderiam resolver fora de
// ordem no backend e deixar persistida uma conversa antiga; com o seq por aba,
// fechar/reabrir o modal em outra aba não invalida persistências pendentes da
// aba anterior (um seq global pularia escritas de abas diferentes indevidamente).
let persistConversationChain: Promise<void> = Promise.resolve();
const persistConversationSeqByTab = new Map<string, number>();

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
  /** Identidade da superfície de chat vinculada ao painel que abriu o modal. */
  boundSurface: ChatSurfaceIdentity | null;
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
    boundSurface: ChatSurfaceIdentity,
    send: WorkspaceChatModalAdapter['send'],
  ) => void;
  close: () => void;
  bumpFocus: () => void;
  setAdapterError: (msg: string | null) => void;
  requestOpen: (tabId: string) => Promise<void>;
  /**
   * Troca a conversa vinculada ao modal já aberto, recriando a superfície de chat
   * (a `ChatSessionProvider` deriva tudo de `surface.conversationId`, então é a
   * recriação da identidade que efetiva a troca na view embutida) e persistindo o
   * vínculo na aba. Painéis que observam `boundConversationId` (ex.: TaskListView)
   * reagem automaticamente. No-op se o modal estiver fechado ou a conversa for a mesma.
   */
  setBoundConversation: (conversationId: string) => void;
}

export const useWorkspaceChatModalStore = create<WorkspaceChatModalState>((set, get) => ({
  isOpen: false,
  boundTabId: null,
  boundConversationId: null,
  boundSurface: null,
  contextDisplay: '',
  sessionMeta: null,
  boundSend: null,
  focusNonce: 0,
  adapterError: null,

  open: (contextDisplay, meta, boundTabId, boundConversationId, boundSurface, send) => {
    set({
      isOpen: true,
      boundTabId,
      boundConversationId,
      boundSurface,
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
      boundSurface: null,
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

  requestOpen: async (tabId) => {
    const workspaceStore = useWorkspaceStore.getState();
    const tab = workspaceStore.workspace?.tabs.find((item) => item.id === tabId) ?? null;
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
      logger.error('[workspaceChatModal] prepare() falhou:', e);
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
      conversationId = await ensureWorkspaceTabConversationId(tab);
    } catch (e) {
      logger.error('[workspaceChatModal] falha ao garantir conversa:', e);
      useUIStore.getState().addToast(
        i18next.t('editor.chatModal.newConversationError'),
        'error',
      );
      return;
    }

    const boundSurface = createChatSurfaceIdentity({
      conversationId,
      surfaceId: buildWorkspaceModalChatSurfaceId(tab.id),
      surfaceType: 'modal',
      tabId: tab.id,
    });
    get().open(result.contextDisplay, result.meta, tab.id, conversationId, boundSurface, adapter.send);
  },

  setBoundConversation: (conversationId) => {
    const { isOpen, boundTabId, boundSurface } = get();
    if (!isOpen || !boundTabId || !boundSurface) return;
    if (boundSurface.conversationId === conversationId) return;

    const nextSurface = createChatSurfaceIdentity({
      conversationId,
      surfaceId: boundSurface.surfaceId,
      surfaceType: boundSurface.surfaceType,
      tabId: boundTabId,
    });
    set({ boundConversationId: conversationId, boundSurface: nextSurface });

    // Persiste o vínculo na aba para sobreviver à reabertura do modal. A troca visual
    // já foi aplicada otimisticamente acima; a persistência é serializada com
    // latest-wins POR ABA, para que trocas rápidas na mesma aba não resolvam fora de
    // ordem sem que abas diferentes invalidem persistências pendentes umas das outras.
    const seq = (persistConversationSeqByTab.get(boundTabId) ?? 0) + 1;
    persistConversationSeqByTab.set(boundTabId, seq);
    persistConversationChain = persistConversationChain.then(async () => {
      // Já há uma troca mais recente para esta mesma aba.
      if (seq !== persistConversationSeqByTab.get(boundTabId)) return;
      try {
        await useWorkspaceStore.getState().updateTab(boundTabId, { conversation_id: conversationId });
      } catch (e) {
        // A troca visual já foi aplicada e o toolbar anunciou sucesso; sem feedback,
        // reabrir o modal "voltaria" para a conversa anterior sem o usuário saber.
        logger.error('[workspaceChatModal] falha ao persistir troca de conversa:', e);
        useUIStore.getState().addToast(i18next.t('chat.switchError'), 'error');
      }
    });
  },
}));
