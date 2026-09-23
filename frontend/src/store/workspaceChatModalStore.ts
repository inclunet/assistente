import { logger } from '../utils/logger';
import { create } from 'zustand';
import i18next from 'i18next';
import type { MediaFile } from '../services/mediaService';
import type { llm } from '../../wailsjs/go/models';
import { useWorkspaceStore } from './workspaceStore';
import { useAuthStore } from './authStore';
import { useEditorStore } from './editorStore';
import { getModalRegistrySnapshot, isModalOpen } from '../lib/modalRegistry';
import { isBackendId } from '../lib/idUtils';
import { useUIStore } from './uiStore';
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

export interface PreparedWorkspaceChatOpen {
  isCurrent(): boolean;
  dispose(): void;
  present(conversationID: string): boolean;
}

export type WorkspaceChatCommandDispatcher = (tabID: string) => Promise<void>;

const adapters = new Map<string, WorkspaceChatModalAdapter>();
const adapterGenerations = new Map<string, number>();
let nextAdapterGenerationID = 0;
let openingGeneration = 0;
let workspaceChatCommandDispatcher: WorkspaceChatCommandDispatcher | null = null;

// Serializa a persistência do vínculo de conversa na aba (latest-wins POR ABA): as
// escritas são encadeadas POR aba e cada uma só executa se ainda for a mais recente
// solicitada para AQUELA aba. Sem a fila, trocas rápidas poderiam resolver fora de
// ordem no backend e deixar persistida uma conversa antiga; cadeias independentes
// por aba evitam que uma persistência lenta em uma aba atrase as demais, e o seq
// por aba garante que fechar/reabrir o modal em outra aba não invalide
// persistências pendentes da aba anterior.
const persistConversationChainByTab = new Map<string, Promise<void>>();
const persistConversationSeqByTab = new Map<string, number>();

export function registerWorkspaceChatModalAdapter(
  tabId: string,
  adapter: WorkspaceChatModalAdapter | null,
) {
  if (!adapter) {
    adapters.delete(tabId);
    adapterGenerations.delete(tabId);
    return;
  }
  adapters.set(tabId, adapter);
  nextAdapterGenerationID += 1;
  adapterGenerations.set(tabId, nextAdapterGenerationID);
}

export function getWorkspaceChatModalAdapter(
  tabId: string | undefined | null,
): WorkspaceChatModalAdapter | null {
  if (!tabId) return null;
  return adapters.get(tabId) ?? null;
}

export function registerWorkspaceChatCommandDispatcher(
  dispatcher: WorkspaceChatCommandDispatcher | null,
): () => void {
  workspaceChatCommandDispatcher = dispatcher;
  return () => {
    if (workspaceChatCommandDispatcher === dispatcher) workspaceChatCommandDispatcher = null;
  };
}

function isVisibleWorkspaceLayout(): boolean {
  const layout = document.querySelector<HTMLElement>('.workspace-layout');
  if (!layout?.isConnected) return false;
  for (let current: HTMLElement | null = layout; current; current = current.parentElement) {
    if (current.hidden || current.hasAttribute('inert') || current.getAttribute('aria-hidden') === 'true') return false;
  }
  return true;
}

function currentWorkspaceTab(tabId: string) {
  const workspace = useWorkspaceStore.getState().workspace;
  if (!workspace || workspace.activeTabId !== tabId) return undefined;
  const tab = workspace.tabs.find((candidate) => candidate.id === tabId);
  return tab ? { workspace, tab } : undefined;
}

function isReadOnlyEditor(tabID: string): boolean {
  return useEditorStore.getState().documents[tabID]?.readOnly === true;
}

function activeWorkspacePanel(tabID: string): HTMLElement | null {
  const panel = [...document.querySelectorAll<HTMLElement>('.ws-content__panel[data-tab-id]')]
    .find((candidate) => candidate.dataset.tabId === tabID) ?? null;
  if (!panel?.isConnected || panel.hidden || panel.getAttribute('aria-hidden') === 'true' || panel.dataset.active === 'false') {
    return null;
  }
  return panel;
}

export function canPrepareWorkspaceChatOpen(tabID: string): boolean {
  if (typeof document === 'undefined' || isModalOpen() || !document.hasFocus() || !isVisibleWorkspaceLayout()) return false;
  const auth = useAuthStore.getState();
  const active = currentWorkspaceTab(tabID);
  if (!auth.isAuthenticated || !auth.user || !active) return false;
  if (active.tab.type === 'editor' && isReadOnlyEditor(tabID)) return false;
  return active.tab.type === 'chat' || getWorkspaceChatModalAdapter(tabID) !== null;
}

export async function prepareWorkspaceChatOpen(
  tabID: string,
  sourceIsCurrent: () => boolean = () => true,
): Promise<PreparedWorkspaceChatOpen | undefined> {
  const requestGeneration = ++openingGeneration;
  const isSourceCurrent = () => {
    try {
      return sourceIsCurrent();
    } catch {
      return false;
    }
  };
  if (!isSourceCurrent() || !canPrepareWorkspaceChatOpen(tabID)) return undefined;

  const auth = useAuthStore.getState();
  const active = currentWorkspaceTab(tabID);
  if (!auth.user || !active) return undefined;
  const adapter = active.tab.type === 'chat' ? null : getWorkspaceChatModalAdapter(tabID);
  const adapterGeneration = adapterGenerations.get(tabID) ?? 0;
  const modalGeneration = getModalRegistrySnapshot().generation;
  const captured = {
    ownerId: auth.user.userId,
    sessionId: auth.user.sessionId,
    workspaceId: active.workspace.id,
    activeTabId: active.tab.id,
    routeIdentity: window.location.pathname + window.location.search + window.location.hash,
    modalGeneration,
  };
  let invalidated = false;
  let disposed = false;
  const preparedSend = adapter?.send ?? null;

  const invalidateAuth = () => {
    const current = useAuthStore.getState();
    if (!current.isAuthenticated || current.user?.userId !== captured.ownerId || current.user?.sessionId !== captured.sessionId) invalidated = true;
  };
  const invalidateWorkspace = () => {
    const current = currentWorkspaceTab(tabID);
    if (!current || current.workspace.id !== captured.workspaceId || current.tab.id !== captured.activeTabId) invalidated = true;
  };
  const unsubscribeAuth = useAuthStore.subscribe(invalidateAuth);
  const unsubscribeWorkspace = useWorkspaceStore.subscribe(invalidateWorkspace);
  const onBlur = () => { invalidated = true; };
  window.addEventListener('blur', onBlur);

  const isCurrent = () => {
    if (invalidated || disposed || requestGeneration !== openingGeneration || useWorkspaceChatModalStore.getState().isOpen || !isSourceCurrent() || !canPrepareWorkspaceChatOpen(tabID)) return false;
    const currentAuth = useAuthStore.getState();
    const current = currentWorkspaceTab(tabID);
    return (
      currentAuth.user?.userId === captured.ownerId &&
      currentAuth.user?.sessionId === captured.sessionId &&
      current?.workspace.id === captured.workspaceId &&
      current.tab.id === captured.activeTabId &&
      (window.location.pathname + window.location.search + window.location.hash) === captured.routeIdentity &&
      getModalRegistrySnapshot().generation === captured.modalGeneration &&
      (adapterGenerations.get(tabID) ?? 0) === adapterGeneration &&
      getWorkspaceChatModalAdapter(tabID) === adapter &&
      (adapter ? adapter.send === preparedSend : true)
    );
  };
  const dispose = () => {
    if (disposed) return;
    disposed = true;
    invalidated = true;
    unsubscribeAuth();
    unsubscribeWorkspace();
    window.removeEventListener('blur', onBlur);
  };

  if (adapter) {
    let result: WorkspaceChatModalPrepareResult;
    try {
      result = await adapter.prepare();
    } catch (error) {
      logger.error('[workspaceChatModal] prepare() falhou:', error);
      if (isCurrent()) {
        useUIStore.getState().addToast(i18next.t('workspace.chatModal.prepareFailed'), 'error');
      }
      dispose();
      return undefined;
    }
    if (!result.ok) {
      if (result.message && isCurrent()) useUIStore.getState().addToast(result.message, 'info');
      dispose();
      return undefined;
    }
    if (!isCurrent()) {
      dispose();
      return undefined;
    }
    const prepared = result;
    return {
      isCurrent,
      dispose,
      present: (conversationID) => {
        if (!isBackendId(conversationID) || !isCurrent()) return false;
        const boundSurface = createChatSurfaceIdentity({
          conversationId: conversationID,
          surfaceId: buildWorkspaceModalChatSurfaceId(tabID),
          surfaceType: 'modal',
          tabId: tabID,
        });
        useWorkspaceChatModalStore.getState().open(prepared.contextDisplay, prepared.meta, tabID, conversationID, boundSurface, preparedSend!);
        return true;
      },
    };
  }

  return {
    isCurrent,
    dispose,
    present: (conversationID) => {
      if (conversationID !== '' || !isCurrent()) return false;
      const panel = activeWorkspacePanel(tabID);
      const input = panel?.querySelector('.chat-page .chat-input__textarea') as HTMLTextAreaElement | null;
      if (!panel || !input || !input.isConnected) return false;
      input.focus();
      return true;
    },
  };
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
    openingGeneration += 1;
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
    openingGeneration += 1;
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

  // ---------------------------------------------------------------------------
  // Fronteira de orquestração (cross-store INTENCIONAL e delimitada).
  //
  // A preparação exportada acima lê pontualmente auth, workspace, editor,
  // registro de modais e DOM para validar a origem antes de apresentar. Aqui,
  // `requestOpen` apenas entrega ao dispatcher comum; `setBoundConversation`
  // persiste a troca no workspace. Essas são as únicas fronteiras de orquestração
  // deste store; o restante permanece visual, puro e observável.
  // ---------------------------------------------------------------------------
  requestOpen: async (tabId) => {
    if (get().isOpen) {
      get().bumpFocus();
      return;
    }
    const dispatcher = workspaceChatCommandDispatcher;
    if (!dispatcher) return;
    try {
      await dispatcher(tabId);
    } catch (error) {
      logger.error('[workspaceChatModal] dispatcher falhou:', error);
    }
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
    const previous = persistConversationChainByTab.get(boundTabId) ?? Promise.resolve();
    const next = previous.then(async () => {
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
    persistConversationChainByTab.set(boundTabId, next);
    // Evita crescimento indefinido do Map quando a cadeia da aba esvazia.
    void next.finally(() => {
      if (persistConversationChainByTab.get(boundTabId) === next) {
        persistConversationChainByTab.delete(boundTabId);
        persistConversationSeqByTab.delete(boundTabId);
      }
    });
  },
}));
