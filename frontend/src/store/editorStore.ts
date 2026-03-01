import { create } from 'zustand';

export type EditorMode = 'markdown' | 'rich';

export interface EditorTab {
  id: string;
  title: string;
  markdown: string;
  mode: EditorMode;

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
  toggleTabMode: (tabId: string) => void;

  setTabFilePath: (tabId: string, filePath: string | null) => void;
  setTabDraftId: (tabId: string, draftId: string | null) => void;
  setTabDirty: (tabId: string, isDirty: boolean) => void;

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

  createTab: (initial) => {
    const id = newId();
    const tab: EditorTab = {
      id,
      title: initial?.title || 'Novo documento',
      markdown: initial?.markdown ?? DEFAULT_MD,
      mode: initial?.mode || 'markdown',

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

  toggleTabMode: (tabId) => {
    set((state) => ({
      tabs: state.tabs.map((t) =>
        t.id === tabId ? { ...t, mode: t.mode === 'markdown' ? 'rich' : 'markdown' } : t
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
