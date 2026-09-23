import {
  getModalSnapshotGeneration,
  getTopmostChatPresentationCommandIds,
  getTopmostModalID,
  isModalOpen,
  registerChatPresentationModalScope,
} from './modalRegistry';

export const CHAT_PICKER_COMMAND_IDS = [
  'chat.model.open',
  'chat.history.open',
  'chat.profile.open',
  'chat.pinned.open',
  'chat.tokens.open',
] as const;

export type ChatPickerCommandID = typeof CHAT_PICKER_COMMAND_IDS[number];

export const CHAT_PRESENTATION_COMMAND_EVENT = 'commands:chat-presentation';

export interface ChatPresentationCommandRequest {
  readonly commandID: ChatPickerCommandID;
  readonly instanceId: string;
}

export function requestChatPresentationCommand(commandID: ChatPickerCommandID, instanceId: string): boolean {
  if (!isChatPickerCommand(commandID) || !instanceId?.trim()) return false;
  const event = new CustomEvent<ChatPresentationCommandRequest>(CHAT_PRESENTATION_COMMAND_EVENT, {
    detail: { commandID, instanceId },
    cancelable: true,
  });
  window.dispatchEvent(event);
  return event.defaultPrevented;
}

export interface ChatPickerSurfaceRegistration {
  readonly root: HTMLElement;
  readonly workspaceId: string;
  readonly ownerId: string;
  readonly sessionId: string;
  readonly tabId: string;
  readonly conversationId: string | null;
  readonly instanceId: string;
  readonly generation: string;
  readonly modalId?: string;
  readonly isActive: () => boolean;
  readonly isCurrent: () => boolean;
  readonly isRouteCurrent?: (pathname: string) => boolean;
  readonly allowedCommandIds: readonly ChatPickerCommandID[];
  readonly subscribe?: (onChange: () => void) => () => void;
  readonly canOpen: (commandID: ChatPickerCommandID, target: EventTarget | null) => boolean;
  readonly open: (commandID: ChatPickerCommandID) => boolean;
}

export interface ChatPickerTargetLease {
  readonly workspaceId: string;
  readonly tabId: string;
  readonly conversationId: string | null;
  readonly instanceId: string;
  readonly generation: string;
  readonly modalId?: string;
  isCurrent(): boolean;
  canOpen(commandID: string, target?: EventTarget | null): boolean;
  open(commandID: string): boolean;
  dispose(): void;
}

const surfaces = new Map<string, ChatPickerSurfaceRegistration>();

export function isChatPickerCommand(id: string): id is ChatPickerCommandID {
  return (CHAT_PICKER_COMMAND_IDS as readonly string[]).includes(id);
}

function isVisibleRoot(root: HTMLElement): boolean {
  if (!root.isConnected) return false;
  for (let current: HTMLElement | null = root; current; current = current.parentElement) {
    if (current.hidden || current.getAttribute('aria-hidden') === 'true' || current.hasAttribute('inert')) return false;
    const style = window.getComputedStyle(current);
    if (style.display === 'none' || style.visibility === 'hidden' || style.visibility === 'collapse') return false;
  }
  return root.getClientRects().length > 0 || window.getComputedStyle(root).display !== 'none';
}

function modalOwnsInput(surface: ChatPickerSurfaceRegistration): boolean {
  if (surface.modalId) {
    const owningOverlay = surface.root.closest('.modal-overlay');
    return getTopmostModalID() === surface.modalId &&
      owningOverlay?.getAttribute('data-modal-id') === surface.modalId &&
      (getTopmostChatPresentationCommandIds()?.length ?? 0) > 0;
  }
  return !isModalOpen();
}

function currentSurface(surface: ChatPickerSurfaceRegistration, pathname?: string): boolean {
  try {
    return surfaces.get(surface.instanceId) === surface &&
      isVisibleRoot(surface.root) &&
      modalOwnsInput(surface) &&
      surface.isActive() &&
      surface.isCurrent() &&
      (pathname === undefined || surface.isRouteCurrent?.(pathname) !== false);
  } catch {
    return false;
  }
}

export function registerChatPickerSurface(
  registration: ChatPickerSurfaceRegistration,
): () => void {
  const allowed = registration.allowedCommandIds;
  if (!(registration.root instanceof HTMLElement) ||
      !registration.instanceId || !registration.tabId || !registration.workspaceId ||
      !registration.ownerId || !registration.sessionId || !registration.generation ||
      !allowed.length || new Set(allowed).size !== allowed.length ||
      allowed.some((commandID) => !isChatPickerCommand(commandID)) ||
      typeof registration.isActive !== 'function' || typeof registration.isCurrent !== 'function' ||
      typeof registration.canOpen !== 'function' || typeof registration.open !== 'function' ||
      (registration.modalId !== undefined && !registration.modalId)) {
    return () => undefined;
  }

  const previous = surfaces.get(registration.instanceId);
  if (previous && previous !== registration) surfaces.delete(registration.instanceId);
  surfaces.set(registration.instanceId, registration);
  const disposeModalScope = registration.modalId
    ? registerChatPresentationModalScope(registration.modalId, registration.allowedCommandIds)
    : undefined;
  if (registration.modalId && !disposeModalScope) {
    surfaces.delete(registration.instanceId);
    return () => undefined;
  }
  return () => {
    if (surfaces.get(registration.instanceId) === registration) {
      surfaces.delete(registration.instanceId);
    }
    disposeModalScope?.();
  };
}

export function captureChatPickerTarget(
  readPathname: () => string,
  expectedInstanceId?: string,
): ChatPickerTargetLease | undefined {
  try {
    const capturedPathname = readPathname();
    const candidates = Array.from(surfaces.values()).filter((candidate) => currentSurface(candidate, capturedPathname));
    if (candidates.length !== 1) return undefined;
    const surface = candidates[0];
    if (expectedInstanceId !== undefined && surface.instanceId !== expectedInstanceId) return undefined;
    const capturedRegistration = surface;
    const capturedModalGeneration = getModalSnapshotGeneration();
    let disposed = false;
    let invalidated = false;
    let opened = false;
    let unsubscribe: (() => void) | undefined;
    try {
      unsubscribe = capturedRegistration.subscribe?.(() => { invalidated = true; });
    } catch {
      return undefined;
    }

    const isCurrent = () => {
      try {
        if (invalidated || disposed || opened) return false;
        const current =
          !opened &&
          surfaces.get(capturedRegistration.instanceId) === capturedRegistration &&
          getModalSnapshotGeneration() === capturedModalGeneration &&
          currentSurface(capturedRegistration, readPathname()) &&
          readPathname() === capturedPathname;
        if (!current) invalidated = true;
        return current;
      } catch {
        invalidated = true;
        return false;
      }
    };

    return {
      workspaceId: capturedRegistration.workspaceId,
      tabId: capturedRegistration.tabId,
      conversationId: capturedRegistration.conversationId,
      instanceId: capturedRegistration.instanceId,
      generation: capturedRegistration.generation,
      modalId: capturedRegistration.modalId,
      isCurrent,
      canOpen(commandID, target = typeof document === 'undefined' ? null : document.activeElement) {
        try {
          if (!isCurrent() || !isChatPickerCommand(commandID)) return false;
          if (!capturedRegistration.allowedCommandIds.includes(commandID)) return false;
          if (capturedRegistration.modalId && !getTopmostChatPresentationCommandIds()?.includes(commandID)) return false;
          return capturedRegistration.canOpen(commandID, target ?? null) === true;
        } catch {
          return false;
        }
      },
      open(commandID) {
        if (!isCurrent() || !isChatPickerCommand(commandID)) return false;
        if (!capturedRegistration.allowedCommandIds.includes(commandID)) return false;
        if (capturedRegistration.modalId && !getTopmostChatPresentationCommandIds()?.includes(commandID)) return false;
        try {
          const activeElement = typeof document === 'undefined' ? null : document.activeElement;
          if (capturedRegistration.canOpen(commandID, activeElement) !== true) return false;
          opened = true;
          disposed = true;
          unsubscribe?.();
          unsubscribe = undefined;
          const accepted = capturedRegistration.open(commandID) === true;
          return accepted;
        } catch {
          return false;
        }
      },
      dispose() {
        disposed = true;
        unsubscribe?.();
        unsubscribe = undefined;
      },
    };
  } catch {
    return undefined;
  }
}
