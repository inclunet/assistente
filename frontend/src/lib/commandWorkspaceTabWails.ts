import { createCommandUIExecutionWailsPort, type CommandUIExecutionWailsOptions } from './commandUIExecutionWails';
import type { CommandContextualBackendPort } from './commandContextualBackendExecution';
import type { UICommandBeginResponse } from './commandUIExecution';
import { waitForWailsBridge } from './waitForWailsBridge';
import { isContextualPagePaletteCommand, isContextualPagePaletteSurface } from './commandContextualPalette';

interface ContextualVisualCommandLease {
  readonly generation: string;
  readonly commandId: string;
  readonly observed: { readonly surfaceType: string; readonly surfaceId: string; readonly appPage?: import('./commandAppPage').AppPage; readonly profile?: string };
  isCurrent(): boolean;
  /** Only file commands may pin an authorized continuation before native UI. */
  prepareNativeFileContinuation?(): (() => boolean) | undefined;
}

export type ContextualPaletteCommandLease = ContextualVisualCommandLease & (
  { readonly source?: 'palette' } |
  { readonly source: 'deck'; beginUICommand(): Promise<UICommandBeginResponse> }
);

export interface WorkspaceTabCommandWailsOptions extends CommandUIExecutionWailsOptions {
  readonly contextualPalette?: ContextualPaletteCommandLease;
}

interface WorkspaceTabCommandWindow extends Window {
  go?: { app?: { App?: {
    CommitWorkspaceTabCommand?: (ticket: string, handoffID: string) => Promise<void>;
    BeginContextualPaletteUICommand?: (generation: string, commandID: string, observed: ContextualPaletteCommandLease['observed']) => Promise<UICommandBeginResponse>;
    BeginContextualPagePaletteUICommand?: (generation: string, commandID: string, surfaceType: string, profile: string) => Promise<UICommandBeginResponse>;
  } } };
}

/** A API nova é uma fachada tipada, não uma edição dos bindings gerados. */
export function createCommandWorkspaceTabWailsPort(
  options: WorkspaceTabCommandWailsOptions = {},
): CommandContextualBackendPort {
  const lease = options.contextualPalette;
  const generation = lease?.generation;
  const commandId = lease?.commandId;
  const observed = lease ? Object.freeze({ surfaceType: lease.observed.surfaceType, surfaceId: lease.observed.surfaceId,
    ...(lease.observed.appPage !== undefined ? { appPage: lease.observed.appPage } : {}),
    ...(lease.observed.profile !== undefined ? { profile: lease.observed.profile } : {}) }) : undefined;
  return {
    ...createCommandUIExecutionWailsPort(options),
    ...(lease ? { beginUICommand: async (commandID: string) => {
      if (commandID !== commandId || !lease.isCurrent()) throw new Error('contextual-palette-stale');
      if (lease.source === 'deck') return lease.beginUICommand();
      await waitForWailsBridge({ target: options.target ?? window, signal: options.signal, timeoutMs: options.timeoutMs });
      const app = ((options.target ?? window) as WorkspaceTabCommandWindow).go?.app?.App;
      if (isContextualPagePaletteCommand(commandID)) {
        if (!isContextualPagePaletteSurface(commandID, observed!.surfaceType)) throw new Error('contextual-page-palette-surface');
        if (typeof app?.BeginContextualPagePaletteUICommand !== 'function') throw new Error('Contextual page palette API is unavailable');
        if (!lease.isCurrent()) throw new Error('contextual-palette-stale');
        return app.BeginContextualPagePaletteUICommand(generation!, commandID, observed!.surfaceType, observed!.profile ?? '');
      }
      if (typeof app?.BeginContextualPaletteUICommand !== 'function') throw new Error('Contextual palette API is unavailable');
      if (!lease.isCurrent()) throw new Error('contextual-palette-stale');
      return app.BeginContextualPaletteUICommand(generation!, commandID, observed!);
    } } : {}),
    commitBackendCommand: (ticket, handoffID) => {
      // Begin/Take já esperaram a bridge. Aqui não pode haver await antes da
      // submissão, pois o chamador acabou de revalidar o contexto visual.
      const target = (options.target ?? window) as WorkspaceTabCommandWindow;
      const app = target.go?.app?.App;
      if (typeof app?.CommitWorkspaceTabCommand !== 'function') {
        throw new Error('Workspace tab command API is unavailable');
      }
      if (lease && !lease.isCurrent()) throw new Error('contextual-palette-stale');
      return app.CommitWorkspaceTabCommand(ticket, handoffID);
    },
  };
}
