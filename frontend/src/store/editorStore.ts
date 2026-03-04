import { create } from 'zustand';

export type EditorMode = 'markdown' | 'rich' | 'view';

export type EditorInsertFormat = 'markdown' | 'html' | 'plain';

export type EditorInsertTarget = 'current' | 'new_tab';

export interface EditorInsertRequest {
  id: string;
  target: EditorInsertTarget;
  format: EditorInsertFormat;
  content: string;
  title?: string;
  /** Quando true, o editor deve focar após inserir. */
  focus?: boolean;
  /** Vínculo com a conversa que originou o envio (para contexto do mini-chat). */
  source?: {
    chatTabId?: string | null;
    conversationId?: number | null;
    messageId?: string | null;
  };
}

export interface EditorTab {
  id: string;
  title: string;
  markdown: string;
  mode: EditorMode;

  /** Aba de chat vinculada a este documento (para o mini-chat do editor ter contexto). */
  linkedChatTabId?: string | null;
  linkedConversationId?: number | null;

  /** Path real no disco (quando já foi salvo/aberto) */
  filePath?: string | null;
  /** ID do draft em ~/.assistente/editor/drafts quando ainda não tem destino */
  draftId?: string | null;
  /** Indica mudanças não persistidas no destino (quando autosave está off) */
  isDirty?: boolean;
}

interface EditorState {
  tabs: EditorTab[];
  activeTabId: string | null;

  autoSaveEnabled: boolean;

  /** Perfil local usado para mensagens do mini-chat do editor (profileSlug em SendMessage) */
  editorProfileSlug: string;

  createTab: (initial?: Partial<Pick<EditorTab, 'title' | 'markdown' | 'mode'>>) => string;
  closeTab: (tabId: string) => void;
  setActiveTab: (tabId: string) => void;
  renameTab: (tabId: string, title: string) => void;
  setTabMarkdown: (tabId: string, markdown: string) => void;
  setTabMode: (tabId: string, mode: EditorMode) => void;
  toggleTabMode: (tabId: string) => void;

  setTabFilePath: (tabId: string, filePath: string | null) => void;
  setTabDraftId: (tabId: string, draftId: string | null) => void;
  setTabDirty: (tabId: string, isDirty: boolean) => void;

  setTabLinkedChat: (tabId: string, link: { chatTabId?: string | null; conversationId?: number | null }) => void;

  /** Requisição pendente para inserir conteúdo no editor ao abrir/ativar a página. */
  pendingInsert: EditorInsertRequest | null;
  requestInsert: (req: Omit<EditorInsertRequest, 'id'>) => string;
  consumePendingInsert: () => EditorInsertRequest | null;

  setAutoSaveEnabled: (enabled: boolean) => void;
  toggleAutoSave: () => void;

  setEditorProfileSlug: (slug: string) => void;

  hydrate: (payload: { tabs: EditorTab[]; activeTabId: string | null; autoSaveEnabled?: boolean; editorProfileSlug?: string }) => void;
}

function newId(): string {
  try {
    return crypto.randomUUID();
  } catch {
    return `editor-${Date.now()}-${Math.random().toString(16).slice(2)}`;
  }
}

const DEFAULT_MD = `# Novo documento\n\nComece a escrever em **Markdown**.\n\n- Ctrl+N (ou Ctrl+T): nova aba\n- Ctrl+W (ou Ctrl+F4): fechar aba\n- Ctrl+Tab: próxima aba\n- Ctrl+Shift+Tab: aba anterior\n- Ctrl+Shift+I: pedir alteração ao chat\n\n\n\n\n\n\n\n\n\n`; 

export const useEditorStore = create<EditorState>((set, get) => ({
  tabs: [],
  activeTabId: null,

  autoSaveEnabled: true,

  editorProfileSlug: 'editor-texto',

  pendingInsert: null,

  createTab: (initial) => {
    const id = newId();
    const tab: EditorTab = {
      id,
      title: initial?.title || 'Novo documento',
      markdown: initial?.markdown ?? DEFAULT_MD,
      mode: initial?.mode || 'markdown',

      linkedChatTabId: null,
      linkedConversationId: null,

      filePath: null,
      draftId: id,
      isDirty: false,
    };

    set((state) => ({
      tabs: [...state.tabs, tab],
      activeTabId: id,
    }));

    return id;
  },

  requestInsert: (req) => {
    const id = newId();
    const normalized: EditorInsertRequest = {
      id,
      target: req.target,
      format: req.format,
      content: String(req.content ?? ''),
      title: req.title,
      focus: req.focus,
      source: req.source,
    };
    set({ pendingInsert: normalized });
    return id;
  },

  consumePendingInsert: () => {
    const cur = get().pendingInsert;
    if (!cur) return null;
    set({ pendingInsert: null });
    return cur;
  },

  closeTab: (tabId) => {
    const { tabs, activeTabId } = get();
    const idx = tabs.findIndex((t) => t.id === tabId);
    if (idx === -1) return;

    const nextTabs = tabs.filter((t) => t.id !== tabId);

    let nextActive: string | null = activeTabId;
    if (activeTabId === tabId) {
      const next = nextTabs[idx] || nextTabs[idx - 1] || null;
      nextActive = next?.id ?? null;
    }

    set({ tabs: nextTabs, activeTabId: nextActive });
  },

  setActiveTab: (tabId) => {
    set({ activeTabId: tabId });
  },

  renameTab: (tabId, title) => {
    set((state) => ({
      tabs: state.tabs.map((t) => (t.id === tabId ? { ...t, title } : t)),
    }));
  },

  setTabMarkdown: (tabId, markdown) => {
    set((state) => ({
      tabs: state.tabs.map((t) => (t.id === tabId ? { ...t, markdown } : t)),
    }));
  },

  setTabMode: (tabId, mode) => {
    const next: EditorMode = mode === 'rich' || mode === 'view' ? mode : 'markdown';
    set((state) => ({
      tabs: state.tabs.map((t) => (t.id === tabId ? { ...t, mode: next } : t)),
    }));
  },

  toggleTabMode: (tabId) => {
    set((state) => ({
      tabs: state.tabs.map((t) =>
        t.id === tabId
          ? { ...t, mode: t.mode === 'view' ? 'markdown' : t.mode === 'markdown' ? 'rich' : 'markdown' }
          : t
      ),
    }));
  },

  setTabFilePath: (tabId, filePath) => {
    set((state) => ({
      tabs: state.tabs.map((t) => (t.id === tabId ? { ...t, filePath } : t)),
    }));
  },

  setTabDraftId: (tabId, draftId) => {
    set((state) => ({
      tabs: state.tabs.map((t) => (t.id === tabId ? { ...t, draftId } : t)),
    }));
  },

  setTabDirty: (tabId, isDirty) => {
    set((state) => ({
      tabs: state.tabs.map((t) => (t.id === tabId ? { ...t, isDirty } : t)),
    }));
  },

  setTabLinkedChat: (tabId, link) => {
    set((state) => ({
      tabs: state.tabs.map((t) =>
        t.id === tabId
          ? {
              ...t,
              linkedChatTabId: typeof link?.chatTabId === 'string' ? link.chatTabId : null,
              linkedConversationId: typeof link?.conversationId === 'number' ? link.conversationId : null,
            }
          : t
      ),
    }));
  },

  setAutoSaveEnabled: (enabled) => set({ autoSaveEnabled: !!enabled }),
  toggleAutoSave: () => set((s) => ({ autoSaveEnabled: !s.autoSaveEnabled })),

  setEditorProfileSlug: (slug) => set({ editorProfileSlug: (slug || '').trim() || 'editor-texto' }),

  hydrate: (payload) => {
    set({
      tabs: payload.tabs,
      activeTabId: payload.activeTabId,
      autoSaveEnabled: typeof payload.autoSaveEnabled === 'boolean' ? payload.autoSaveEnabled : true,
      editorProfileSlug: (payload.editorProfileSlug || '').trim() || 'editor-texto',
    });
  },
}));
