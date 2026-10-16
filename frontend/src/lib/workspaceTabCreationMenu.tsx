import { createContext, useContext, useMemo, useRef, type ReactNode } from 'react';

export type WorkspaceTabCreationMenuOrigin = 'keyboard' | 'toolbar' | 'palette';

export type WorkspaceTabCreationCancelReason =
  | 'escape'
  | 'blur'
  | 'outside'
  | 'context'
  | 'dispose';

export interface WorkspaceTabCreationIntent {
  readonly userId: string;
  readonly sessionId: string;
  readonly workspaceId: string;
  readonly activeTabId: string | null;
  readonly routeIdentity: string;
  readonly modalGeneration: number;
}

export interface WorkspaceTabCreationItem {
  readonly commandID: string;
  readonly label: string;
  readonly icon?: ReactNode;
  readonly shortcut?: string;
  readonly disabled?: boolean;
}

export interface WorkspaceTabCreationMenuRequest {
  readonly origin: WorkspaceTabCreationMenuOrigin;
  readonly intent: WorkspaceTabCreationIntent;
  readonly items: readonly WorkspaceTabCreationItem[];
}

export interface WorkspaceTabCreationMenuHost {
  show(
    request: WorkspaceTabCreationMenuRequest,
    onSelect: (commandID: string, intent: WorkspaceTabCreationIntent) => void,
  ): boolean;
  close(options?: { restoreFocus: boolean }): void;
  focusTrigger(): void;
  isOpen(): boolean;
}

export interface WorkspaceTabCreationMenuActions {
  openFromToolbar(): void;
  cancel(reason: WorkspaceTabCreationCancelReason): void;
  /** Enfileira workspace.create no executor contextual do host Topbar. */
  requestWorkspaceCreate?(): void;
  /** Libera o dispatch depois que o menu hospedeiro terminou de fechar. */
  completeWorkspaceCreate?(restoreFocus: () => void): boolean;
}

export interface WorkspaceTabCreationMenuContextValue {
  registerHost(host: WorkspaceTabCreationMenuHost): () => void;
  registerActions(actions: WorkspaceTabCreationMenuActions): () => void;
  requestOpen(): void;
  show(
    request: WorkspaceTabCreationMenuRequest,
    onSelect: (commandID: string, intent: WorkspaceTabCreationIntent) => void,
  ): boolean;
  close(options?: { restoreFocus: boolean }): void;
  focusTrigger(): void;
  isOpen(): boolean;
  cancel(reason: WorkspaceTabCreationCancelReason): void;
  requestWorkspaceCreate(): void;
  completeWorkspaceCreate(restoreFocus: () => void): boolean;
}

const WorkspaceTabCreationMenuContext = createContext<WorkspaceTabCreationMenuContextValue | null>(null);

export function WorkspaceTabCreationMenuProvider({ children }: { children: ReactNode }) {
  const hostRef = useRef<WorkspaceTabCreationMenuHost | null>(null);
  const actionsRef = useRef<WorkspaceTabCreationMenuActions | null>(null);
  const value = useMemo<WorkspaceTabCreationMenuContextValue>(() => ({
    registerHost: (host) => {
      hostRef.current = host;
      return () => {
        if (hostRef.current === host) hostRef.current = null;
      };
    },
    registerActions: (actions) => {
      actionsRef.current = actions;
      return () => {
        if (actionsRef.current === actions) actionsRef.current = null;
      };
    },
    requestOpen: () => {
      actionsRef.current?.openFromToolbar();
    },
    show: (request, onSelect) => hostRef.current?.show(request, onSelect) ?? false,
    close: (options) => {
      hostRef.current?.close(options);
    },
    focusTrigger: () => {
      hostRef.current?.focusTrigger();
    },
    isOpen: () => hostRef.current?.isOpen() ?? false,
    cancel: (reason) => {
      actionsRef.current?.cancel(reason);
    },
    requestWorkspaceCreate: () => {
      actionsRef.current?.requestWorkspaceCreate?.();
    },
    completeWorkspaceCreate: (restoreFocus) => {
      return actionsRef.current?.completeWorkspaceCreate?.(restoreFocus) ?? false;
    },
  }), []);

  return (
    <WorkspaceTabCreationMenuContext.Provider value={value}>
      {children}
    </WorkspaceTabCreationMenuContext.Provider>
  );
}

/** Standalone Topbar/toolbar mounts have no host and remain inert. */
export function useWorkspaceTabCreationMenu(): WorkspaceTabCreationMenuContextValue | null {
  return useContext(WorkspaceTabCreationMenuContext);
}
