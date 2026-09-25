import type { BackendCommandExecutionResult, CommandBackendExecutionPort } from './commandBackendExecution';
import type { ContextualPaletteCommandLease } from './commandWorkspaceTabWails';
import { isCommandLayerAction } from './commandLayerActions';
import { waitForWailsBridge } from './waitForWailsBridge';

type LocalCommandKeyboardContext = ContextualPaletteCommandLease['observed'];

type DeckLayerWindow = Window & { go?: { app?: { App?: {
  ExecuteContextualDeckLayerCommand?: (offerID: string, generation: string, observed: LocalCommandKeyboardContext) => Promise<BackendCommandExecutionResult>;
} } } };

/** The selected ID is a local guard only. Host-owned offer determines command/args. */
export function createContextualDeckLayerWailsPort(options: {
  readonly offerId: string;
  readonly generation: string;
  readonly commandId: string;
  readonly observed: LocalCommandKeyboardContext;
  isCurrent(): boolean;
  readonly target?: DeckLayerWindow;
}): CommandBackendExecutionPort {
  const { offerId, generation, commandId, isCurrent } = options;
  const observed = Object.freeze({ surfaceType: options.observed.surfaceType, surfaceId: options.observed.surfaceId,
    ...(options.observed.profile !== undefined ? { profile: options.observed.profile } : {}) });
  const expires = Date.now() + 10000;
  let consumed = false;
  return Object.freeze({ executeCommand: async (id: string) => {
    if (consumed || id !== commandId || !isCommandLayerAction(id) || !offerId || !generation ||
        !observed.surfaceId || !['chat', 'editor', 'terminal', 'tasklist'].includes(observed.surfaceType) || !isCurrent()) {
      throw new Error('contextual-deck-layer-stale');
    }
    consumed = true;
    const target: DeckLayerWindow = options.target ?? window;
    await waitForWailsBridge({ target });
    const app = target.go?.app?.App;
    const execute = app?.ExecuteContextualDeckLayerCommand;
    if (typeof execute !== 'function') throw new Error('Contextual Deck layer API unavailable');
    // Resolve the bridge before the final guard; no await until submission.
    if (Date.now() >= expires || !isCurrent()) throw new Error('contextual-deck-layer-stale');
    return execute.call(app, offerId, generation, observed);
  } });
}
