import { create } from 'zustand';

export type EditorMode = 'markdown' | 'rich' | 'view';

export type EditorInsertFormat = 'markdown' | 'html' | 'plain';

export type EditorInsertTarget = 'current' | 'new_document';

export interface EditorInsertRequest {
  id: string;
  target: EditorInsertTarget;
  format: EditorInsertFormat;
  content: string;
  title?: string;
  focus?: boolean;
}

export interface EditorDocument {
  id: string;
  title: string;
  markdown: string;
  mode: EditorMode;

  filePath?: string | null;
  draftId?: string | null;
  isDirty?: boolean;
}


interface EditorState {
  documents: Record<string, EditorDocument>;
  activeDocumentId: string | null;

  autoSaveEnabled: boolean;

  editorProfileSlug: string;

  createDocument: (initial?: Partial<Pick<EditorDocument, 'title' | 'markdown' | 'mode'>>) => string;
  removeDocument: (docId: string) => void;
  setActiveDocument: (docId: string) => void;
  renameDocument: (docId: string, title: string) => void;
  setDocMarkdown: (docId: string, markdown: string) => void;
  setDocMode: (docId: string, mode: EditorMode) => void;
  toggleDocMode: (docId: string) => void;

  setDocFilePath: (docId: string, filePath: string | null) => void;
  setDocDraftId: (docId: string, draftId: string | null) => void;
  setDocDirty: (docId: string, isDirty: boolean) => void;

  getDocument: (docId: string) => EditorDocument | undefined;
  getActiveDocument: () => EditorDocument | null;

  pendingInsert: EditorInsertRequest | null;
  requestInsert: (req: Omit<EditorInsertRequest, 'id'>) => string;
  consumePendingInsert: () => EditorInsertRequest | null;

  setAutoSaveEnabled: (enabled: boolean) => void;
  toggleAutoSave: () => void;

  setEditorProfileSlug: (slug: string) => void;

  hydrate: (payload: { documents: Record<string, EditorDocument>; activeDocumentId: string | null; autoSaveEnabled?: boolean; editorProfileSlug?: string }) => void;
}

function newId(): string {
  try {
    return crypto.randomUUID();
  } catch {
    return `editor-${Date.now()}-${Math.random().toString(16).slice(2)}`;
  }
}

const DEFAULT_MD = `# Novo documento\n\nComece a escrever em **Markdown**.\n\n- Ctrl+N (ou Ctrl+T): nova aba\n- Ctrl+W (ou Ctrl+F4): fechar aba\n- Ctrl+Tab: próxima aba\n- Ctrl+Shift+Tab: aba anterior\n- Ctrl+Shift+I: pedir alteração ao chat\n\n\n\n\n\n\n\n\n\n`; 

function updateDoc(documents: Record<string, EditorDocument>, docId: string, patch: Partial<EditorDocument>): Record<string, EditorDocument> {
  const existing = documents[docId];
  if (!existing) return documents;
  return { ...documents, [docId]: { ...existing, ...patch } };
}

export const useEditorStore = create<EditorState>((set, get) => ({
  documents: {},
  activeDocumentId: null,

  autoSaveEnabled: true,

  editorProfileSlug: 'editor-texto',

  pendingInsert: null,

  createDocument: (initial) => {
    const id = newId();
    const doc: EditorDocument = {
      id,
      title: initial?.title || 'Novo documento',
      markdown: initial?.markdown ?? DEFAULT_MD,
      mode: initial?.mode || 'markdown',

      filePath: null,
      draftId: id,
      isDirty: false,
    };

    set((state) => ({
      documents: { ...state.documents, [id]: doc },
      activeDocumentId: id,
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

  removeDocument: (docId) => {
    set((state) => {
      const next = { ...state.documents };
      delete next[docId];
      const nextActive = state.activeDocumentId === docId ? null : state.activeDocumentId;
      return { documents: next, activeDocumentId: nextActive };
    });
  },

  setActiveDocument: (docId) => {
    set({ activeDocumentId: docId });
  },

  renameDocument: (docId, title) => {
    set((state) => ({ documents: updateDoc(state.documents, docId, { title }) }));
  },

  setDocMarkdown: (docId, markdown) => {
    set((state) => ({ documents: updateDoc(state.documents, docId, { markdown }) }));
  },

  setDocMode: (docId, mode) => {
    const next: EditorMode = mode === 'rich' || mode === 'view' ? mode : 'markdown';
    set((state) => ({ documents: updateDoc(state.documents, docId, { mode: next }) }));
  },

  toggleDocMode: (docId) => {
    set((state) => {
      const doc = state.documents[docId];
      if (!doc) return state;
      const nextMode: EditorMode = doc.mode === 'view' ? 'markdown' : doc.mode === 'markdown' ? 'rich' : 'markdown';
      return { documents: updateDoc(state.documents, docId, { mode: nextMode }) };
    });
  },

  setDocFilePath: (docId, filePath) => {
    set((state) => ({ documents: updateDoc(state.documents, docId, { filePath }) }));
  },

  setDocDraftId: (docId, draftId) => {
    set((state) => ({ documents: updateDoc(state.documents, docId, { draftId }) }));
  },

  setDocDirty: (docId, isDirty) => {
    set((state) => ({ documents: updateDoc(state.documents, docId, { isDirty }) }));
  },

  getDocument: (docId) => get().documents[docId],
  getActiveDocument: () => {
    const { documents, activeDocumentId } = get();
    return activeDocumentId ? documents[activeDocumentId] ?? null : null;
  },

  setAutoSaveEnabled: (enabled) => set({ autoSaveEnabled: !!enabled }),
  toggleAutoSave: () => set((s) => ({ autoSaveEnabled: !s.autoSaveEnabled })),

  setEditorProfileSlug: (slug) => set({ editorProfileSlug: (slug || '').trim() || 'editor-texto' }),

  hydrate: (payload) => {
    set({
      documents: payload.documents,
      activeDocumentId: payload.activeDocumentId,
      autoSaveEnabled: typeof payload.autoSaveEnabled === 'boolean' ? payload.autoSaveEnabled : true,
      editorProfileSlug: (payload.editorProfileSlug || '').trim() || 'editor-texto',
    });
  },
}));
