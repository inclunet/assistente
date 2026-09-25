import {
  createCommandWorkspaceTabWailsPort,
  type WorkspaceTabCommandWailsOptions,
} from './commandWorkspaceTabWails';
import type {
  TerminalOperationPort,
} from './commandTerminalOperation';

interface TerminalOperationAppAPI {
  PrepareTerminalInterruptCommand(
    ticket: string,
    workspaceId: string,
    tabId: string,
    sessionId: string,
    commandId: string,
  ): Promise<void>;
  PrepareTerminalSessionCommand(
    ticket: string,
    workspaceId: string,
    tabId: string,
    sessionId: string,
  ): Promise<void>;
}

type TerminalOperationWindow = Window & {
  go?: { app?: { App?: Partial<TerminalOperationAppAPI> } };
};

/** Fachada da API nova; os bindings Wails continuam gerados e intocados. */
export function createCommandTerminalOperationWailsPort(
  options: WorkspaceTabCommandWailsOptions = {},
): TerminalOperationPort {
  const common = createCommandWorkspaceTabWailsPort(options);
  return Object.freeze({
    ...common,
    prepareTerminalInterruptCommand: async (
      ticket: string,
      workspaceId: string,
      tabId: string,
      sessionId: string,
      commandId: string,
    ) => {
      const target = (options.target ?? window) as TerminalOperationWindow;
      const app = target.go?.app?.App;
      if (typeof app?.PrepareTerminalInterruptCommand !== 'function') {
        throw new Error('Terminal interrupt preparation API is unavailable');
      }
      if (options.contextualPalette && !options.contextualPalette.isCurrent()) throw new Error('contextual-command-stale');
      await app.PrepareTerminalInterruptCommand(ticket, workspaceId, tabId, sessionId, commandId);
    },
    prepareTerminalSessionCommand: async (ticket: string, workspaceId: string, tabId: string, sessionId: string) => {
      const target = (options.target ?? window) as TerminalOperationWindow;
      const app = target.go?.app?.App;
      if (typeof app?.PrepareTerminalSessionCommand !== 'function') {
        throw new Error('Terminal session preparation API is unavailable');
      }
      if (options.contextualPalette && !options.contextualPalette.isCurrent()) throw new Error('contextual-command-stale');
      await app.PrepareTerminalSessionCommand(ticket, workspaceId, tabId, sessionId);
    },
  });
}
