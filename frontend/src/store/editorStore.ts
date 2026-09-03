import { logger } from '../utils/logger';
import { create } from 'zustand';

export type EditorMode = 'markdown' | 'rich' | 'view';

export function normalizeEditorMode(
  value: unknown,
  fallback: EditorMode = 'markdown',
): EditorMode {
  return value === 'markdown' || value === 'rich' || value === 'view'
    ? value
    : fallback;
}

export function resolveEditorDisplayMode(
  persistedMode: unknown,
  legacyMode: EditorMode,
  readOnly: boolean,
): EditorMode {
  return readOnly ? 'view' : normalizeEditorMode(persistedMode, legacyMode);
}

export function preferLiveEditorDocument(
  loaded: EditorDocument,
  existing: EditorDocument | undefined,
): EditorDocument {
  return existing?.sessionHydrated === false ? loaded : existing ?? loaded;
}

export type EditorInsertFormat = 'markdown' | 'html' | 'plain';

export type EditorInsertTarget = 'document' | 'new_document';

export interface EditorDocumentProjection {
  format: string;
  pages?: number;
  warnings: string[];
  warningCode?: string;
}

interface EditorInsertRequestBase {
  format: EditorInsertFormat;
  content: string;
  title?: string;
  focus?: boolean;
}

export type EditorInsertRequest =
  | ({
      id: string;
      target: 'document';
      targetDocumentId: string;
    } & EditorInsertRequestBase)
  | ({
      id: string;
      target: 'new_document';
      targetDocumentId?: never;
    } & EditorInsertRequestBase);

export interface EditorDocument {
  id: string;
  title: string;
  markdown: string;
  mode: EditorMode;

  filePath?: string | null;
  draftId?: string | null;
  isDirty?: boolean;
  readOnly?: boolean;
  projection?: EditorDocumentProjection | null;
  loadError?: boolean;
  /** Distingue o documento provisório do controller do snapshot de sessão. */
  sessionHydrated?: boolean;
}


interface EditorState {
  documents: Record<string, EditorDocument>;

  createDocument: (initial?: Partial<Pick<EditorDocument, 'id' | 'title' | 'markdown' | 'mode' | 'filePath' | 'draftId' | 'readOnly' | 'projection' | 'loadError' | 'sessionHydrated'>>) => string;
  removeDocument: (docId: string) => void;
  renameDocument: (docId: string, title: string) => void;
  setDocMarkdown: (docId: string, markdown: string) => void;
  setDocMode: (docId: string, mode: EditorMode) => void;
  toggleDocMode: (docId: string) => void;

  setDocFilePath: (docId: string, filePath: string | null) => void;
  setDocDraftId: (docId: string, draftId: string | null) => void;
  setDocDirty: (docId: string, isDirty: boolean) => void;
  setDocProjection: (docId: string, projection: EditorDocumentProjection | null) => void;

  getDocument: (docId: string) => EditorDocument | undefined;

  pendingInsert: EditorInsertRequest | null;
  requestInsert: (req: Omit<EditorInsertRequest, 'id'>) => string | null;
  consumePendingInsert: () => EditorInsertRequest | null;

  hydrate: (payload: { documents: Record<string, EditorDocument> }) => void;
}

function newId(): string {
  try {
    return crypto.randomUUID();
  } catch {
    return `editor-${Date.now()}-${Math.random().toString(16).slice(2)}`;
  }
}

export const DEFAULT_MD = `# Novo documento\n\nComece a escrever em **Markdown**.\n\n- Ctrl+N (ou Ctrl+T): nova aba\n- Ctrl+W (ou Ctrl+F4): fechar aba\n- Ctrl+Tab: próxima aba\n- Ctrl+Shift+Tab: aba anterior\n- Ctrl+Shift+I: pedir alteração ao chat\n\n\n\n\n\n\n\n\n\n`;

function updateDoc(documents: Record<string, EditorDocument>, docId: string, patch: Partial<EditorDocument>): Record<string, EditorDocument> {
  const existing = documents[docId];
  if (!existing) return documents;
  return { ...documents, [docId]: { ...existing, ...patch } };
}

export const useEditorStore = create<EditorState>((set, get) => ({
  documents: {},

  pendingInsert: null,

  createDocument: (initial) => {
    const id = initial?.id || newId();
    const hasFilePath = !!initial?.filePath;
    const doc: EditorDocument = {
      id,
      title: initial?.title || 'Novo documento',
      markdown: initial?.markdown ?? DEFAULT_MD,
      mode: initial?.mode || 'markdown',

      filePath: initial?.filePath || null,
      draftId: initial?.draftId !== undefined ? initial.draftId : (hasFilePath ? null : id),
      isDirty: false,
      readOnly: initial?.readOnly ?? false,
      projection: initial?.projection ?? null,
      loadError: initial?.loadError ?? false,
      sessionHydrated: initial?.sessionHydrated ?? true,
    };

    set((state) => ({
      documents: { ...state.documents, [id]: doc },
    }));

    return id;
  },

  requestInsert: (req) => {
    const base = {
      id: newId(),
      format: req.format,
      content: String(req.content ?? ''),
      title: req.title,
      focus: req.focus,
    } satisfies EditorInsertRequestBase & { id: string };

    const normalized: EditorInsertRequest | null =
      req.target === 'document'
        ? (() => {
            const targetDocumentId = String(req.targetDocumentId ?? '').trim();
            if (!targetDocumentId) {
              logger.error('[EditorStore] requestInsert rejected: document target requires targetDocumentId');
              return null;
            }
            return {
              ...base,
              target: 'document',
              targetDocumentId,
            };
          })()
        : {
            ...base,
            target: 'new_document',
          };

    if (!normalized) return null;
    set({ pendingInsert: normalized });
    return normalized.id;
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
      return { documents: next };
    });
  },

  renameDocument: (docId, title) => {
    set((state) => ({ documents: updateDoc(state.documents, docId, { title }) }));
  },

  setDocMarkdown: (docId, markdown) => {
    set((state) => ({ documents: updateDoc(state.documents, docId, { markdown }) }));
  },

  setDocMode: (docId, mode) => {
    set((state) => {
      const doc = state.documents[docId];
      if (!doc) return state;
      const next: EditorMode = doc.readOnly ? 'view' : mode === 'rich' || mode === 'view' ? mode : 'markdown';
      return { documents: updateDoc(state.documents, docId, { mode: next }) };
    });
  },

  toggleDocMode: (docId) => {
    set((state) => {
      const doc = state.documents[docId];
      if (!doc) return state;
      if (doc.readOnly) return state;
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

  setDocProjection: (docId, projection) => {
    set((state) => {
      const doc = state.documents[docId];
      if (!doc) return state;
      const wasProtected =
        !!doc.loadError || (doc.projection !== null && doc.projection !== undefined);
      return {
        documents: updateDoc(state.documents, docId, {
          projection,
          readOnly: projection !== null,
          mode: projection !== null ? 'view' : wasProtected ? 'markdown' : doc.mode,
          loadError: false,
        }),
      };
    });
  },

  getDocument: (docId) => get().documents[docId],

  hydrate: (payload) => {
    set({
      documents: payload.documents,
    });
  },
}));
