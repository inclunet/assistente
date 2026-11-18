import { useCallback, useEffect, useRef, useState } from 'react';
import { useAuthStore } from '../../store/authStore';
import { useWorkspaceStore } from '../../store/workspaceStore';
import { isModalOpen } from '../../lib/modalRegistry';
import { scheduleDefaultFocusRestore } from '../../lib/deferredDefaultFocus';
import { logger } from '../../utils/logger';

interface WorkspaceListOrigin {
  commandID: string;
  userId: string;
  sessionId: string;
  workspaceId: string;
  activeTabId: string | null;
  routeIdentity: string;
}

interface Options {
  readRouteIdentity(): string;
  isOriginCurrent(origin: WorkspaceListOrigin): boolean;
  canActivate(): boolean;
  onDismiss(): void;
}

interface PendingSelection {
  ready: Promise<void>;
  isCurrent(): boolean;
}

/** Owns the list lifetime; command admission remains with the caller. */
export function useWorkspaceList(options: Options) {
  const optionsRef = useRef(options);
  optionsRef.current = options;
  const rootRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLElement | null>(null);
  const mountedRef = useRef(true);
  const [origin, setOrigin] = useState<WorkspaceListOrigin | null>(null);
  // Revokes old callbacks immediately, including before the closing render.
  const activeOriginRef = useRef<WorkspaceListOrigin | null>(null);
  const pendingSelectionRef = useRef<PendingSelection | null>(null);
  const afterCloseRef = useRef<(() => void) | null>(null);
  const isOpen = origin !== null;

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      activeOriginRef.current = null;
      pendingSelectionRef.current = null;
      afterCloseRef.current = null;
    };
  }, []);

  // Restore only after the dialog has left the modal registry.
  useEffect(() => {
    if (isOpen) return;
    const afterClose = afterCloseRef.current;
    afterCloseRef.current = null;
    afterClose?.();
  }, [isOpen]);

  const open = useCallback((trigger: HTMLElement) => {
    afterCloseRef.current = null;
    pendingSelectionRef.current = null;
    const auth = useAuthStore.getState();
    const workspace = useWorkspaceStore.getState().workspace;
    if (!auth.isAuthenticated || !auth.user || !workspace) return;
    triggerRef.current = trigger;
    const nextOrigin: WorkspaceListOrigin = {
      commandID: 'workspace.list',
      userId: auth.user.userId,
      sessionId: auth.user.sessionId,
      workspaceId: workspace.id,
      activeTabId: workspace.activeTabId,
      routeIdentity: optionsRef.current.readRouteIdentity(),
    };
    activeOriginRef.current = nextOrigin;
    setOrigin(nextOrigin);
  }, []);

  const close = useCallback(() => {
    optionsRef.current.onDismiss();
    activeOriginRef.current = null;
    setOrigin(null);
    const trigger = triggerRef.current;
    triggerRef.current = null;
    afterCloseRef.current = () => {
      if (!isModalOpen() && trigger?.isConnected) trigger.focus();
    };
  }, []);

  const activate = useCallback((workspaceId: string) => {
    if (!origin || activeOriginRef.current !== origin ||
      !optionsRef.current.isOriginCurrent(origin) || !optionsRef.current.canActivate()) return;
    const state = useWorkspaceStore.getState();
    if (!state.workspaces.some(workspace => workspace.id === workspaceId)) return;

    optionsRef.current.onDismiss();
    pendingSelectionRef.current = null;
    if (state.workspace?.id !== workspaceId) {
      // Switching expires the opening origin; completion needs its own guard.
      const user = useAuthStore.getState().user;
      const routeIdentity = optionsRef.current.readRouteIdentity();
      const originalWorkspaceId = state.workspace?.id;
      let focusWorkspaceId: string | undefined = workspaceId;
      const selection: PendingSelection = {
        ready: state.switchWorkspace(workspaceId).catch(error => {
          focusWorkspaceId = originalWorkspaceId;
          logger.error('[Workspace] Falha ao trocar workspace', error);
        }),
        isCurrent: () => {
          const auth = useAuthStore.getState();
          return pendingSelectionRef.current === selection && mountedRef.current &&
            auth.isAuthenticated && auth.user?.userId === user?.userId &&
            auth.user?.sessionId === user?.sessionId &&
            optionsRef.current.readRouteIdentity() === routeIdentity && Boolean(focusWorkspaceId) &&
            useWorkspaceStore.getState().workspace?.id === focusWorkspaceId;
        },
      };
      pendingSelectionRef.current = selection;
    }
    activeOriginRef.current = null;
    setOrigin(null);
    const selection = pendingSelectionRef.current;
    afterCloseRef.current = () => scheduleDefaultFocusRestore({
      after: selection?.ready,
      isCurrent: selection?.isCurrent,
    });
  }, [origin]);

  return { isOpen, rootRef, open, close, activate };
}
