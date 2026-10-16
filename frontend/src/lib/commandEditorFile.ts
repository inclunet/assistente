import { isModalOpen } from './modalRegistry';

/** Comandos de arquivo são duráveis; não confundir com os menus locais do editor. */
export const EDITOR_FILE_COMMAND_IDS = [
  'editor.file.save',
  'editor.file.open',
  'editor.file.save_copy',
] as const;
export type EditorFileCommandID = typeof EDITOR_FILE_COMMAND_IDS[number];

export interface EditorFilePreparation {
  readonly content?: string;
  readonly labels: Record<string, string>;
  readonly suggestedFilename: string;
  readonly confirmOverwrite: boolean;
  readonly path?: string;
}

export interface EditorFileSurfaceRegistration {
  readonly root: HTMLElement;
  readonly ownerId: string;
  readonly sessionId: string;
  readonly workspaceId: string;
  readonly tabId: string;
  readonly documentId: string;
  readonly instanceId: string;
  readonly generation: string;
  readonly isActive: () => boolean;
  readonly isCurrent: () => boolean;
  readonly canExecute: (commandID: EditorFileCommandID) => boolean;
  /** Validação final após o diálogo; pode rejeitar IME ativo e modal aberto. */
  readonly canCommit?: (commandID: EditorFileCommandID) => boolean;
  /** Dialog/flush fica aqui; a chamada é feita somente depois de Take. */
  readonly prepare: (commandID: EditorFileCommandID) => Promise<EditorFilePreparation | undefined>;
  readonly confirmOverwrite: (path: string) => Promise<boolean>;
  readonly applyCommitted?: (commandID: EditorFileCommandID, result: unknown) => void;
  readonly canApplyCommittedResult?: (commandID: EditorFileCommandID, result: unknown) => boolean;
  readonly subscribe?: (onChange: () => void) => () => void;
}

export interface EditorFileTargetLease {
  readonly ownerId: string;
  readonly sessionId: string;
  readonly workspaceId: string;
  readonly tabId: string;
  readonly documentId: string;
  readonly instanceId: string;
  readonly generation: string;
  isCurrent(): boolean;
  canExecute(commandID: string): boolean;
  canCommit(commandID: string): boolean;
  prepare(commandID: string): Promise<EditorFilePreparation | undefined>;
  confirmOverwrite(path: string): Promise<boolean>;
  applyCommitted(commandID: string, result: unknown): void;
  dispose(): void;
}

const surfaces = new Map<string, EditorFileSurfaceRegistration>();

export function isEditorFileCommand(id: string): id is EditorFileCommandID {
  return (EDITOR_FILE_COMMAND_IDS as readonly string[]).includes(id);
}

export function registerEditorFileSurface(registration: EditorFileSurfaceRegistration): () => void {
  if (!(registration.root instanceof HTMLElement) || !registration.ownerId || !registration.sessionId ||
      !registration.workspaceId || !registration.tabId || !registration.documentId || !registration.instanceId ||
      !registration.generation || typeof registration.isActive !== 'function' ||
      typeof registration.isCurrent !== 'function' || typeof registration.canExecute !== 'function' ||
      typeof registration.prepare !== 'function' || typeof registration.confirmOverwrite !== 'function') return () => undefined;
  surfaces.set(registration.instanceId, registration);
  return () => { if (surfaces.get(registration.instanceId) === registration) surfaces.delete(registration.instanceId); };
}

export function captureEditorFileTarget(): EditorFileTargetLease | undefined {
  if (typeof document === 'undefined') return undefined;
  const candidates = [...surfaces.values()].filter((surface) => {
    try { return surface.root.isConnected && !isModalOpen() && surface.isActive() && surface.isCurrent(); } catch { return false; }
  });
  if (candidates.length !== 1) return undefined;
  const captured = candidates[0];
  let disposed = false;
  let invalidated = false;
  let unsubscribe: (() => void) | undefined;
  unsubscribe = captured.subscribe?.(() => { invalidated = true; });
  const isCurrent = () => {
    try {
      const valid = !disposed && !invalidated && captured.root.isConnected && surfaces.get(captured.instanceId) === captured &&
        captured.isActive() && captured.isCurrent();
      if (!valid) invalidated = true;
      return valid;
    } catch { invalidated = true; return false; }
  };
  return {
    ownerId: captured.ownerId, sessionId: captured.sessionId, workspaceId: captured.workspaceId,
    tabId: captured.tabId, documentId: captured.documentId, instanceId: captured.instanceId,
    generation: captured.generation, isCurrent,
    canExecute(commandID) {
      return isCurrent() && isEditorFileCommand(commandID) && captured.canExecute(commandID);
    },
    canCommit(commandID) {
      return isCurrent() && !isModalOpen() && isEditorFileCommand(commandID) &&
        (captured.canCommit?.(commandID) ?? true);
    },
    prepare: async (commandID) => {
      if (!isCurrent() || !isEditorFileCommand(commandID) || !captured.canExecute(commandID)) return undefined;
      const request = await captured.prepare(commandID);
      return isCurrent() ? request : undefined;
    },
    confirmOverwrite: async (path) => {
      if (!isCurrent()) return false;
      const confirmed = await captured.confirmOverwrite(path);
      return isCurrent() && confirmed;
    },
    applyCommitted: (commandID, result) => {
      if (!disposed && isEditorFileCommand(commandID) &&
          (isCurrent() || captured.canApplyCommittedResult?.(commandID, result) === true)) {
        captured.applyCommitted?.(commandID, result);
      }
    },
    dispose() { disposed = true; unsubscribe?.(); unsubscribe = undefined; },
  };
}
