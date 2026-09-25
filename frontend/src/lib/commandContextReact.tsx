import { createContext, useContext, useLayoutEffect, useState, type ReactNode, type RefObject } from 'react';
import { useLocation } from 'react-router-dom';
import { useAuthStore } from '../store/authStore';
import { useWorkspaceStore } from '../store/workspaceStore';
import { createTrustedCommandContextSession, type TrustedCommandContextSession, type OwnedCommandContextFrame } from './commandContextSession';
import { getModalRegistrySnapshot } from './modalRegistry';
import type { SurfaceContextGetter } from './commandContextProviders';

export interface CommandContextScope {
  readonly session: TrustedCommandContextSession;
  registerSurface(id: string, root: RefObject<HTMLElement>, getter: SurfaceContextGetter): () => void;
  surfaceForElement(element: Element | null): string | undefined;
  /** Metadata only: callers must prove ownership of the current modal first.
   * Never use this read to authorize an effect behind a modal. */
  readOwnedCommandOriginMetadata(surfaceId: string, modalId: string): OwnedCommandContextFrame | undefined;
  dispose(): void;
}

function visibleRoot(root: HTMLElement | null): root is HTMLElement {
  return !!root?.isConnected && !root.closest('[hidden], [inert], [aria-hidden="true"]');
}

/** One mounted UI instance owns this scope; no process-wide surface singleton. */
export function createCommandContextScope(onOwnerChange?: () => void): CommandContextScope {
  const session = createTrustedCommandContextSession();
  const roots = new Map<string, { root: RefObject<HTMLElement> }>();
  let originRead: { entry: { root: RefObject<HTMLElement> }; modalId: string } | undefined;
  const visibleModalOrigin = (root: HTMLElement | null, modalId: string) => {
    if (!root?.isConnected || getModalRegistrySnapshot().topID !== modalId) return false;
    const appRoot = document.getElementById('root');
    if (!appRoot?.contains(root) || !appRoot.hasAttribute('inert') || appRoot.getAttribute('aria-hidden') !== 'true') return false;
    for (let element: HTMLElement | null = root; element; element = element.parentElement) {
      if (element.hasAttribute('hidden') || element !== appRoot && (element.hasAttribute('inert') || element.getAttribute('aria-hidden') === 'true')) return false;
    }
    return true;
  };
  let disposed = false;
  const readOwner = () => {
    const auth = useAuthStore.getState();
    return JSON.stringify([auth.isAuthenticated, auth.user?.userId, auth.user?.sessionId,
      useWorkspaceStore.getState().workspace?.id]);
  };
  const owner = readOwner();
  let unsubscribeAuth = () => undefined as void;
  let unsubscribeWorkspace = () => undefined as void;
  const dispose = () => {
    if (disposed) return;
    disposed = true;
    unsubscribeAuth();
    unsubscribeWorkspace();
    roots.clear();
    session.dispose();
  };
  const current = () => {
    if (disposed) return false;
    if (readOwner() !== owner) {
      dispose();
      onOwnerChange?.();
    }
    return !disposed;
  };
  unsubscribeAuth = useAuthStore.subscribe(current);
  unsubscribeWorkspace = useWorkspaceStore.subscribe(current);
  return {
    session,
    registerSurface(id, root, getter) {
      if (!current()) return () => undefined;
      const entry = { root };
      const mountedRoot = root.current;
      const unregister = session.registerSurfaceContext(id, () => {
        // Consume the exception before invoking product code: reentrant normal
        // reads cannot inherit this metadata-only visibility allowance.
        const origin = originRead?.entry === entry ? originRead : undefined;
        if (origin) originRead = undefined;
        if (!current() || root.current !== mountedRoot || !(origin ? visibleModalOrigin(mountedRoot, origin.modalId) : visibleRoot(mountedRoot))) return null;
        return getter();
      });
      roots.set(id, entry);
      return () => {
        unregister();
        if (roots.get(id) === entry) roots.delete(id);
      };
    },
    surfaceForElement(element) {
      if (!current() || !element?.isConnected) return undefined;
      let match: { id: string; root: HTMLElement } | undefined;
      for (const [id, entry] of roots) {
        const root = entry.root.current;
        if (root?.isConnected && root.contains(element) && (!match || match.root.contains(root))) {
          match = { id, root };
        }
      }
      // Return the explicit registration even when its data provider is absent:
      // capture must reject it, not fall back to a different surface.
      return match?.id;
    },
    readOwnedCommandOriginMetadata(surfaceId, modalId) {
      if (!current() || !modalId || originRead || getModalRegistrySnapshot().topID !== modalId) return;
      const entry = roots.get(surfaceId);
      if (!entry) return;
      const modalGeneration = getModalRegistrySnapshot().generationNumber;
      originRead = { entry, modalId };
      try {
        const owned = session.readOwnedCommandContextFrame(surfaceId);
        return current() && roots.get(surfaceId) === entry && getModalRegistrySnapshot().topID === modalId &&
          getModalRegistrySnapshot().generationNumber === modalGeneration && owned?.frame.surface ? owned : undefined;
      } finally { originRead = undefined; }
    },
    dispose,
  };
}

const CommandContext = createContext<CommandContextScope | null>(null);
export const useCommandContextScope = () => useContext(CommandContext);

export function CommandContextProvider({ children }: { children: ReactNode }) {
  const owner = useAuthStore((s) => s.isAuthenticated && s.user
    ? JSON.stringify([s.user.userId, s.user.sessionId]) : null);
  const workspaceId = useWorkspaceStore((s) => s.workspace?.id);
  const location = useLocation();
  const route = JSON.stringify([location.pathname, location.search, location.hash, location.key]);
  const [scope, setScope] = useState<CommandContextScope | null>(null);
  const [ownerRevision, setOwnerRevision] = useState(0);
  useLayoutEffect(() => {
    const mounted = createCommandContextScope(() => setOwnerRevision((revision) => revision + 1));
    setScope(mounted);
    return () => mounted.dispose();
  }, [owner, workspaceId, route, ownerRevision]);
  return <CommandContext.Provider value={scope}>{children}</CommandContext.Provider>;
}
