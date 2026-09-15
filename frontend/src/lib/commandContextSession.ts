import { useAuthStore } from '../store/authStore';
import { useWorkspaceStore } from '../store/workspaceStore';
import {
  ReadFocusContext,
  ReadSurfaceContextFromGetterDetailed,
  type CommandContextFrame,
  type FocusSnapshot,
  type SurfaceContext,
  type SurfaceContextCleanup,
  type SurfaceContextGetter,
  type SurfaceContextRead,
} from './commandContextProviders';
import { getModalRegistrySnapshot, type ModalRegistrySnapshot } from './modalRegistry';

export interface TrustedOwner {
  readonly userId: string;
  readonly sessionId: string;
  readonly workspaceId: string;
}

interface ScopedSurfaceEntry {
  readonly owner: TrustedOwner;
  readonly getter: SurfaceContextGetter;
}

export interface TrustedCommandContextSession {
  registerSurfaceContext(surfaceID: string, getter: SurfaceContextGetter): SurfaceContextCleanup;
  readSurfaceContext(surfaceID: string): Readonly<SurfaceContext> | undefined;
  readSurfaceContextDetailed(surfaceID: string): SurfaceContextRead;
  readCommandContextFrame(surfaceID?: string): CommandContextFrame;
  readOwnedCommandContextFrame(surfaceID?: string): OwnedCommandContextFrame | undefined;
  /** Encerra a sessão e remove todas as leases de surface que ela possui. */
  dispose(): void;
}

export interface OwnedCommandContextFrame {
  readonly owner: TrustedOwner;
  readonly frame: CommandContextFrame;
}

const OWNER_ID_PATTERN = /^(?!\s)(?:[\s\S]*\S)$/;

function hasLoneSurrogate(value: string): boolean {
  for (let index = 0; index < value.length; index += 1) {
    const code = value.charCodeAt(index);
    if (code >= 0xdc00 && code <= 0xdfff) return true;
    if (code >= 0xd800 && code <= 0xdbff) {
      if (index + 1 >= value.length) return true;
      const next = value.charCodeAt(index + 1);
      if (next < 0xdc00 || next > 0xdfff) return true;
      index += 1;
    }
  }
  return false;
}

function isValidOwnerPart(value: unknown): value is string {
  return (
    typeof value === 'string' &&
    OWNER_ID_PATTERN.test(value) &&
    value.trim() === value &&
    !hasLoneSurrogate(value)
  );
}

function readTrustedOwner(): TrustedOwner | null {
  const auth = useAuthStore.getState();
  const workspace = useWorkspaceStore.getState().workspace;
  const user = auth.user;
  if (
    !auth.isAuthenticated ||
    !user ||
    !isValidOwnerPart(user.userId) ||
    !isValidOwnerPart(user.sessionId) ||
    !workspace ||
    !isValidOwnerPart(workspace.id)
  ) {
    return null;
  }
  return {
    userId: user.userId,
    sessionId: user.sessionId,
    workspaceId: workspace.id,
  };
}

function sameOwner(left: TrustedOwner, right: TrustedOwner | null): boolean {
  return (
    right !== null &&
    left.userId === right.userId &&
    left.sessionId === right.sessionId &&
    left.workspaceId === right.workspaceId
  );
}

function missingSurface(): SurfaceContextRead {
  return { status: 'missing', context: null };
}

/**
 * Creates a read-only command-context view bound to the actual auth/workspace
 * stores. Caller-supplied owner fields do not exist in this API. The neutral
 * provider remains useful for UI prototyping, but this session is the scoped
 * read source intended for the backend adapter; the frontend still does not
 * authenticate the principal.
 */
export function createTrustedCommandContextSession(): TrustedCommandContextSession {
  const scopedSurfaces = new Map<string, ScopedSurfaceEntry>();
  let disposed = false;

  function registerSurfaceContext(
    surfaceID: string,
    getter: SurfaceContextGetter,
  ): SurfaceContextCleanup {
    if (disposed) return () => undefined;
    if (!isValidOwnerPart(surfaceID) || typeof getter !== 'function') {
      throw new TypeError('surface-context-registration-invalid');
    }
    const owner = readTrustedOwner();
    if (!owner) return () => undefined;

    const entry = { owner, getter };
    scopedSurfaces.set(surfaceID, entry);
    return () => {
      if (scopedSurfaces.get(surfaceID) === entry) scopedSurfaces.delete(surfaceID);
    };
  }

  function readSurfaceContextDetailed(surfaceID: string): SurfaceContextRead {
    if (disposed) return missingSurface();
    if (!isValidOwnerPart(surfaceID)) return missingSurface();
    const entry = scopedSurfaces.get(surfaceID);
    const ownerBeforeGetter = readTrustedOwner();
    if (!entry || !sameOwner(entry.owner, ownerBeforeGetter)) return missingSurface();

    const result = ReadSurfaceContextFromGetterDetailed(surfaceID, entry.getter);
    // A synchronous getter cannot race a store update unless it itself causes
    // one, but checking again also closes that re-entrant logout/user-switch path.
    if (!sameOwner(entry.owner, readTrustedOwner())) return missingSurface();
    return result;
  }

  function readSurfaceContext(surfaceID: string): Readonly<SurfaceContext> | undefined {
    const result = readSurfaceContextDetailed(surfaceID);
    return result.status === 'available' ? result.context ?? undefined : undefined;
  }

  function readCommandContextFrame(surfaceID?: string): CommandContextFrame {
    const owner = readTrustedOwner();
    const surface = owner && surfaceID !== undefined ? readSurfaceContext(surfaceID) ?? null : null;
    const modal: ModalRegistrySnapshot = getModalRegistrySnapshot();
    const focus: FocusSnapshot = ReadFocusContext();
    return Object.freeze({
      version: 1 as const,
      capturedAt: new Date().toISOString(),
      modal,
      focus,
      surface,
    });
  }

  function readOwnedCommandContextFrame(
    surfaceID?: string,
  ): OwnedCommandContextFrame | undefined {
    if (disposed) return undefined;
    const ownerBeforeCapture = readTrustedOwner();
    if (!ownerBeforeCapture) return undefined;

    const surface = surfaceID === undefined ? null : readSurfaceContext(surfaceID) ?? null;
    const frame = Object.freeze({
      version: 1 as const,
      capturedAt: new Date().toISOString(),
      modal: getModalRegistrySnapshot(),
      focus: ReadFocusContext(),
      surface,
    });
    const ownerAfterCapture = readTrustedOwner();
    if (!sameOwner(ownerBeforeCapture, ownerAfterCapture)) return undefined;

    return Object.freeze({
      owner: Object.freeze({ ...ownerBeforeCapture }),
      frame,
    });
  }

  function dispose(): void {
    if (disposed) return;
    disposed = true;
    scopedSurfaces.clear();
  }

  return {
    registerSurfaceContext,
    readSurfaceContext,
    readSurfaceContextDetailed,
    readCommandContextFrame,
    readOwnedCommandContextFrame,
    dispose,
  };
}

export const createCommandContextSession = createTrustedCommandContextSession;
