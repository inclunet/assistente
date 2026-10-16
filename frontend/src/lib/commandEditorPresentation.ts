import { getModalSnapshotGeneration, isModalOpen } from './modalRegistry';

export const EDITOR_PRESENTATION_COMMAND_IDS = [
  'editor.menu.file.open',
  'editor.menu.format.open',
  'editor.menu.insert.open',
  'editor.menu.mode.open',
  'editor.slides.open',
  'editor.presentation.fullscreen',
  'editor.table.cell.next',
  'editor.table.cell.previous',
] as const;

export type EditorPresentationCommandID = typeof EDITOR_PRESENTATION_COMMAND_IDS[number];

export interface EditorPresentationSurfaceRegistration {
  readonly root: HTMLElement;
  readonly ownerId: string;
  readonly sessionId: string;
  readonly workspaceId: string;
  readonly tabId: string;
  readonly documentId: string;
  readonly instanceId: string;
  readonly generation: string;
  readonly isRouteCurrent?: (pathname: string) => boolean;
  readonly allowedCommandIds: readonly EditorPresentationCommandID[];
  readonly isActive: () => boolean;
  readonly isCurrent: () => boolean;
  readonly canOpen: (commandID: EditorPresentationCommandID, target: EventTarget | null) => boolean;
  readonly open: (commandID: EditorPresentationCommandID) => boolean;
  readonly captureTarget?: () => unknown;
  readonly isCapturedTargetCurrent?: (capturedTarget: unknown, commandID: EditorPresentationCommandID) => boolean;
  readonly subscribeCapturedTarget?: (
    capturedTarget: unknown,
    onChange: () => void,
  ) => () => void;
  readonly subscribe?: (onChange: () => void) => () => void;
}

export interface EditorPresentationTargetLease {
  readonly ownerId: string;
  readonly sessionId: string;
  readonly workspaceId: string;
  readonly tabId: string;
  readonly documentId: string;
  readonly instanceId: string;
  readonly generation: string;
  isCurrent(): boolean;
  canOpen(commandID: string, target?: EventTarget | null): boolean;
  open(commandID: string): boolean;
  dispose(): void;
}

const surfaces = new Map<string, EditorPresentationSurfaceRegistration>();

export function isEditorPresentationCommand(id: string): id is EditorPresentationCommandID {
  return (EDITOR_PRESENTATION_COMMAND_IDS as readonly string[]).includes(id);
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

function currentSurface(surface: EditorPresentationSurfaceRegistration): boolean {
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

export function registerEditorPresentationSurface(
  registration: EditorPresentationSurfaceRegistration,
): () => void {
  if (!(registration.root instanceof HTMLElement) ||
      !registration.ownerId || !registration.sessionId || !registration.workspaceId ||
      !registration.tabId || !registration.documentId || !registration.instanceId ||
      !registration.generation || !Array.isArray(registration.allowedCommandIds) ||
      registration.allowedCommandIds.length === 0 ||
      new Set(registration.allowedCommandIds).size !== registration.allowedCommandIds.length ||
      registration.allowedCommandIds.some((id) => !isEditorPresentationCommand(id)) ||
      typeof registration.isActive !== 'function' ||
      typeof registration.isCurrent !== 'function' || typeof registration.canOpen !== 'function' ||
      typeof registration.open !== 'function') return () => undefined;

  surfaces.set(registration.instanceId, registration);
  return () => {
    if (surfaces.get(registration.instanceId) === registration) surfaces.delete(registration.instanceId);
  };
}

export function captureEditorPresentationTarget(
  readPathname: () => string,
): EditorPresentationTargetLease | undefined {
  try {
    const capturedPathname = readPathname();
    if (capturedPathname !== '/' && capturedPathname !== '') return undefined;
    const candidates = Array.from(surfaces.values()).filter((surface) => currentSurface(surface));
    if (candidates.length !== 1) return undefined;
    const captured = candidates[0];
    const capturedTarget = captured.captureTarget?.();
    const modalGeneration = getModalSnapshotGeneration();
    let disposed = false;
    let invalidated = false;
    let opened = false;
    let unsubscribe: (() => void) | undefined;
    let unsubscribeCapturedTarget: (() => void) | undefined;
    unsubscribe = captured.subscribe?.(() => { invalidated = true; });
    unsubscribeCapturedTarget = captured.subscribeCapturedTarget?.(capturedTarget, () => { invalidated = true; });
    const cleanupSubscriptions = () => {
      unsubscribe?.();
      unsubscribe = undefined;
      unsubscribeCapturedTarget?.();
      unsubscribeCapturedTarget = undefined;
    };

    const isCurrent = () => {
      try {
        if (disposed || invalidated || opened) return false;
      const valid = surfaces.get(captured.instanceId) === captured &&
          getModalSnapshotGeneration() === modalGeneration &&
          readPathname() === capturedPathname &&
          captured.isRouteCurrent?.(capturedPathname) !== false &&
          currentSurface(captured);
        if (!valid) invalidated = true;
        return valid;
      } catch {
        invalidated = true;
        return false;
      }
    };

    return {
      ownerId: captured.ownerId,
      sessionId: captured.sessionId,
      workspaceId: captured.workspaceId,
      tabId: captured.tabId,
      documentId: captured.documentId,
      instanceId: captured.instanceId,
      generation: captured.generation,
      isCurrent,
      canOpen(commandID, target = typeof document === 'undefined' ? null : document.activeElement) {
        try {
          if (!isCurrent() || !isEditorPresentationCommand(commandID) ||
              !captured.allowedCommandIds.includes(commandID)) return false;
          return isCurrent() &&
            captured.isCapturedTargetCurrent?.(capturedTarget, commandID) !== false &&
            captured.canOpen(commandID, target ?? null) === true;
        } catch {
          return false;
        }
      },
      open(commandID) {
        if (!isCurrent() || !isEditorPresentationCommand(commandID) ||
            !captured.allowedCommandIds.includes(commandID)) return false;
        try {
          if (captured.isCapturedTargetCurrent?.(capturedTarget, commandID) === false ||
              captured.canOpen(commandID, typeof document === 'undefined' ? null : document.activeElement) !== true) return false;
          opened = true;
          disposed = true;
          cleanupSubscriptions();
          return captured.open(commandID) === true;
        } catch {
          opened = true;
          disposed = true;
          cleanupSubscriptions();
          return false;
        }
      },
      dispose() {
        disposed = true;
        cleanupSubscriptions();
      },
    };
  } catch {
    return undefined;
  }
}
