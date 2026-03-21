export interface EditorMergeSessionPayload {
  originalPath?: string;
  mineDraftId?: string;
  diskDraftId?: string;
  conflictDraftId?: string;
  createdAt?: number;
}

export interface EditorSessionDocPayload {
  id?: string;
  title?: string;
  mode?: string;
  filePath?: string;
  draftId?: string;
}

export interface EditorSessionPayload {
  version?: number;
  autoSaveEnabled?: boolean;
  activeDocumentId?: string;
  profileSlug?: string;
  documents?: EditorSessionDocPayload[];

  fileModeByPath?: Record<string, string>;
  externalConflictLockedByDocId?: Record<string, boolean>;
  mergeSessionsByDocId?: Record<string, EditorMergeSessionPayload>;
}

export function toEditorSessionPayload(input: unknown): EditorSessionPayload {
  if (!input || typeof input !== 'object') return {};
  return input as EditorSessionPayload;
}
