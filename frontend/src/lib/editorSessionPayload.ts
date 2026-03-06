export interface EditorMergeSessionPayload {
  originalPath?: string;
  mineDraftId?: string;
  diskDraftId?: string;
  conflictDraftId?: string;
  createdAt?: number;
}

export interface EditorSessionTabPayload {
  id?: string;
  title?: string;
  mode?: string;
  filePath?: string;
  draftId?: string;
}

export interface EditorSessionPayload {
  version?: number;
  autoSaveEnabled?: boolean;
  activeTabId?: string;
  profileSlug?: string;
  tabs?: EditorSessionTabPayload[];

  fileModeByPath?: Record<string, string>;
  externalConflictLockedByTabId?: Record<string, boolean>;
  mergeSessionsByTabId?: Record<string, EditorMergeSessionPayload>;
}

export function toEditorSessionPayload(input: unknown): EditorSessionPayload {
  if (!input || typeof input !== 'object') return {};
  return input as EditorSessionPayload;
}
