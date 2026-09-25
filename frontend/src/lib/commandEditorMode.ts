import { getModalSnapshotGeneration, isModalOpen } from './modalRegistry';
import type { EditorMode } from '../store/editorStore';
export type { EditorMode } from '../store/editorStore';

export const EDITOR_MODE_COMMAND_IDS = [
  'editor.mode.markdown',
  'editor.mode.rich',
  'editor.mode.view',
] as const;
export const EDITOR_MODE_COMMAND_EVENT = 'commands:editor-mode' as const;

export type EditorModeCommandID = typeof EDITOR_MODE_COMMAND_IDS[number];

const modeByCommand: Record<EditorModeCommandID, EditorMode> = {
  'editor.mode.markdown': 'markdown',
  'editor.mode.rich': 'rich',
  'editor.mode.view': 'view',
};

export interface EditorModeSurfaceRegistration {
  readonly root: HTMLElement;
  readonly ownerId: string;
  readonly sessionId: string;
  readonly workspaceId: string;
  readonly tabId: string;
  readonly documentId: string;
  readonly instanceId: string;
  readonly generation: string;
  readonly mode: EditorMode;
  readonly readOnly: boolean;
  readonly isAsking: () => boolean;
  readonly isActive: () => boolean;
  readonly isCurrent: () => boolean;
  readonly isBlocked?: () => boolean;
  /** Exceção apenas para o refoco local do documento a partir de landmarks do workspace. */
  readonly canFocusViewFromWorkspaceLandmark?: () => boolean;
  readonly flushRichMarkdownNow?: () => void;
  readonly focusCurrentView?: () => boolean;
  readonly applyCommitted: (mode: EditorMode, restoreFocus?: boolean) => boolean;
  readonly subscribe?: (onChange: () => void) => () => void;
}

export interface EditorModeTargetLease {
  readonly ownerId: string;
  readonly sessionId: string;
  readonly workspaceId: string;
  readonly tabId: string;
  readonly documentId: string;
  readonly instanceId: string;
  readonly generation: string;
  readonly commandID: string;
  isCurrent(): boolean;
  prepare(): boolean;
  applyCommitted(): boolean;
  dispose(): void;
}

export interface EditorViewFocusLease {
  readonly ownerId: string;
  readonly sessionId: string;
  readonly workspaceId: string;
  readonly tabId: string;
  readonly documentId: string;
  readonly instanceId: string;
  readonly generation: string;
  isCurrent(): boolean;
  focus(): boolean;
  dispose(): void;
}

const surfaces = new Map<string, EditorModeSurfaceRegistration>();

export function isEditorModeCommand(id: string): id is EditorModeCommandID {
  return (EDITOR_MODE_COMMAND_IDS as readonly string[]).includes(id);
}

export function editorModeForCommand(commandID: EditorModeCommandID): EditorMode {
  return modeByCommand[commandID];
}

export function editorModeCommandForMode(mode: EditorMode): EditorModeCommandID {
  switch (mode) {
    case 'rich': return 'editor.mode.rich';
    case 'view': return 'editor.mode.view';
    default: return 'editor.mode.markdown';
  }
}

export function requestEditorModeCommand(commandID: EditorModeCommandID): boolean {
  if (!isEditorModeCommand(commandID) || typeof window === 'undefined') return false;
  const event = new CustomEvent<{ commandID: EditorModeCommandID }>(EDITOR_MODE_COMMAND_EVENT, {
    detail: { commandID },
    cancelable: true,
  });
  window.dispatchEvent(event);
  return event.defaultPrevented;
}

function visibleRoot(root: HTMLElement): boolean {
  if (!root.isConnected) return false;
  for (let current: HTMLElement | null = root; current; current = current.parentElement) {
    if (current.hidden || current.hasAttribute('inert') || current.getAttribute('aria-hidden') === 'true') return false;
    const style = window.getComputedStyle(current);
    if (style.display === 'none' || style.visibility === 'hidden' || style.visibility === 'collapse') return false;
  }
  return root.getClientRects().length > 0 || window.getComputedStyle(root).display !== 'none';
}

function currentSurface(surface: EditorModeSurfaceRegistration): boolean {
  try {
    return surfaces.get(surface.instanceId) === surface &&
      visibleRoot(surface.root) &&
      !isModalOpen() &&
      surface.isActive() &&
      surface.isCurrent();
  } catch {
    return false;
  }
}

function isEligibleForCommand(
  surface: EditorModeSurfaceRegistration,
  targetMode: EditorMode,
): boolean {
  try {
    return currentSurface(surface) &&
      surface.mode !== targetMode &&
      (!surface.readOnly || targetMode === 'view') &&
      !surface.isAsking();
  } catch {
    return false;
  }
}

export function registerEditorModeSurface(
  registration: EditorModeSurfaceRegistration,
): () => void {
  if (!(registration.root instanceof HTMLElement) ||
      !registration.ownerId || !registration.sessionId || !registration.workspaceId ||
      !registration.tabId || !registration.documentId || !registration.instanceId ||
      !registration.generation ||
      !(['markdown', 'rich', 'view'] as readonly EditorMode[]).includes(registration.mode) ||
      typeof registration.readOnly !== 'boolean' ||
      typeof registration.isAsking !== 'function' ||
      typeof registration.isActive !== 'function' || typeof registration.isCurrent !== 'function' ||
      typeof registration.applyCommitted !== 'function') return () => undefined;

  surfaces.set(registration.instanceId, registration);
  return () => {
    if (surfaces.get(registration.instanceId) === registration) surfaces.delete(registration.instanceId);
  };
}

export function captureEditorModeTarget(
  readPathname: () => string,
  commandID: string,
): EditorModeTargetLease | undefined {
  if (!isEditorModeCommand(commandID)) return undefined;
  try {
    const capturedPathname = readPathname();
    if (capturedPathname !== '/' && capturedPathname !== '') return undefined;
    const targetMode = editorModeForCommand(commandID);
    const candidates = Array.from(surfaces.values()).filter((surface) =>
      isEligibleForCommand(surface, targetMode));
    if (candidates.length !== 1) return undefined;
    const captured = candidates[0];
    const capturedCommand = commandID;
    const modalGeneration = getModalSnapshotGeneration();
    let disposed = false;
    let invalidated = false;
    let prepared = false;
    let committed = false;
    let preparedFocus: Element | null = null;
    let unsubscribe: (() => void) | undefined;
    unsubscribe = captured.subscribe?.(() => { invalidated = true; });

    const isCurrent = () => {
      try {
        const valid = !disposed && !invalidated && !committed &&
          surfaces.get(captured.instanceId) === captured &&
          getModalSnapshotGeneration() === modalGeneration &&
          readPathname() === capturedPathname &&
          isEligibleForCommand(captured, targetMode);
        if (!valid) invalidated = true;
        return valid;
      } catch {
        invalidated = true;
        return false;
      }
    };

    const finish = () => {
      disposed = true;
      unsubscribe?.();
      unsubscribe = undefined;
    };

    return {
      ownerId: captured.ownerId,
      sessionId: captured.sessionId,
      workspaceId: captured.workspaceId,
      tabId: captured.tabId,
      documentId: captured.documentId,
      instanceId: captured.instanceId,
      generation: captured.generation,
      commandID: capturedCommand,
      isCurrent,
      prepare() {
        if (!isCurrent() || captured.isBlocked?.() === true || prepared) return false;
        try {
          if (!isCurrent()) return false;
          if (captured.mode === 'rich' && targetMode !== 'rich') captured.flushRichMarkdownNow?.();
          if (!isCurrent()) return false;
          preparedFocus = document.activeElement;
          prepared = true;
          return true;
        } catch {
          invalidated = true;
          return false;
        }
      },
      applyCommitted() {
        if (!prepared || !isCurrent()) return false;
        try {
          const restoreFocus = typeof document.hasFocus === 'function' && document.hasFocus() &&
            document.activeElement === preparedFocus && captured.isBlocked?.() !== true;
          committed = true;
          const accepted = captured.applyCommitted(targetMode, restoreFocus) === true;
          if (!accepted) { invalidated = true; }
          finish();
          return accepted;
        } catch {
          invalidated = true;
          finish();
          return false;
        }
      },
      dispose() { finish(); },
    };
  } catch {
    return undefined;
  }
}

/**
 * Captures the already-admitted editor view surface for the keyboard command's
 * focus-only same-mode action. This deliberately does not create a mode target
 * and cannot enter the Begin/Take/Commit path.
 */
export function captureEditorViewFocusTarget(readPathname: () => string): EditorViewFocusLease | undefined {
  try {
    const capturedPathname = readPathname();
    if ((capturedPathname !== '/' && capturedPathname !== '') || isModalOpen()) return undefined;
    const viewFocusAllowed = (surface: EditorModeSurfaceRegistration) => {
      if (surface.isBlocked?.() !== true) return true;
      try { return surface.canFocusViewFromWorkspaceLandmark?.() === true; }
      catch { return false; }
    };
    const candidates = Array.from(surfaces.values()).filter((surface) => {
      try {
        return surface.mode === 'view' && typeof surface.focusCurrentView === 'function' &&
          currentSurface(surface) && !surface.isAsking() && viewFocusAllowed(surface);
      } catch {
        return false;
      }
    });
    if (candidates.length !== 1) return undefined;

    const captured = candidates[0];
    const modalGeneration = getModalSnapshotGeneration();
    let disposed = false;
    let invalidated = false;
    let focused = false;
    const unsubscribe = captured.subscribe?.(() => { invalidated = true; });
    const isCurrent = () => {
      try {
        const valid = !disposed && !invalidated && !focused &&
          surfaces.get(captured.instanceId) === captured &&
          getModalSnapshotGeneration() === modalGeneration &&
          readPathname() === capturedPathname && !isModalOpen() &&
          captured.mode === 'view' && currentSurface(captured) &&
          !captured.isAsking() && viewFocusAllowed(captured);
        if (!valid) invalidated = true;
        return valid;
      } catch {
        invalidated = true;
        return false;
      }
    };
    const finish = () => { disposed = true; unsubscribe?.(); };

    return {
      ownerId: captured.ownerId,
      sessionId: captured.sessionId,
      workspaceId: captured.workspaceId,
      tabId: captured.tabId,
      documentId: captured.documentId,
      instanceId: captured.instanceId,
      generation: captured.generation,
      isCurrent,
      focus() {
        if (!isCurrent()) return false;
        try {
          focused = true;
          const accepted = captured.focusCurrentView?.() === true;
          finish();
          return accepted;
        } catch {
          invalidated = true;
          finish();
          return false;
        }
      },
      dispose: finish,
    };
  } catch {
    return undefined;
  }
}
