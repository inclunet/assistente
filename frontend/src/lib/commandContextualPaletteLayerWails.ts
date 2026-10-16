import type { ContextualPaletteCommandLease } from './commandWorkspaceTabWails';
import type { BackendCommandExecutionResult, CommandBackendExecutionPort } from './commandBackendExecution';
import { isCommandLayerAction } from './commandLayerActions';
import { waitForWailsBridge } from './waitForWailsBridge';

type LayerWindow = Window & { go?: { app?: { App?: {
  ExecuteContextualPaletteLayerCommand?: (generation: string, commandID: string,
    observed: ContextualPaletteCommandLease['observed']) => Promise<BackendCommandExecutionResult>;
} } } };

/** One submission, no UI reservation and no legacy fallback. */
export function createContextualPaletteLayerWailsPort(
  lease: ContextualPaletteCommandLease,
  authorize: () => boolean,
  target: LayerWindow = window,
): CommandBackendExecutionPort {
  const { generation, commandId } = lease;
  const observed = Object.freeze({ ...lease.observed });
  return Object.freeze({ executeCommand: async (id: string) => {
    if (id !== commandId || !isCommandLayerAction(id) ||
        !['chat', 'editor', 'terminal', 'tasklist'].includes(observed.surfaceType) ||
        !observed.surfaceId || !lease.isCurrent() || !authorize()) throw new Error('contextual-layer-stale');
    await waitForWailsBridge({ target });
    const app = target.go?.app?.App;
    const execute = app?.ExecuteContextualPaletteLayerCommand;
    if (typeof execute !== 'function') throw new Error('Contextual palette layer API is unavailable');
    // Resolve the bridge first. No await (or late bridge lookup) after guards.
    if (!lease.isCurrent() || !authorize()) throw new Error('contextual-layer-stale');
    return execute.call(app, generation, id, observed);
  } });
}
