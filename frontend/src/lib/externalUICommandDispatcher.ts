import type {
  ExternalUICommandOutcome,
  ExternalUICommandReadyEvent,
  ExternalUIConnectionService,
  ExternalUIDestination,
  ExternalUIOwnerProof,
  TakeExternalUICommandResult,
} from '../services/externalUIConnection';

export interface ExternalUICommandFrame {
  readonly owner: ExternalUIOwnerProof;
  readonly destination: ExternalUIDestination;
  /** Non-null only while the local command context/keyboard owner is current. */
  readonly localGeneration: string | null;
  readonly hasFocus: boolean;
}

type DispatcherService = Pick<ExternalUIConnectionService,
  'getSnapshot' | 'subscribeReady' | 'take' | 'complete'>;

export interface ExternalUICommandDispatcherOptions {
  readonly service: DispatcherService;
  readonly readCurrentFrame: () => ExternalUICommandFrame | null;
  readonly isSupported: (commandId: string, arguments_: Readonly<Record<string, unknown>>) => boolean;
  /** Synchronous UI command dispatcher; true means accepted for dispatch, not that a later render/persist completed. */
  readonly execute: (commandId: string, event: ExternalUICommandReadyEvent) => boolean;
  /** Pauses context publication while an admitted command is between Take and Complete. */
  readonly onCommandPendingChange?: (pending: boolean, event: ExternalUICommandReadyEvent) => void;
  /** Runs only after Complete, so a command's own navigation is not published ahead of its receipt. */
  readonly publishCurrentContext: (event: ExternalUICommandReadyEvent) => Promise<void>;
}

function sameOwner(left: ExternalUIOwnerProof, right: ExternalUIOwnerProof): boolean {
  return left.userId === right.userId && left.sessionId === right.sessionId && left.workspaceId === right.workspaceId;
}

function sameDestination(left: ExternalUIDestination | null, right: ExternalUIDestination | null): boolean {
  return !!left && !!right && left.workspaceId === right.workspaceId && (left.tabId ?? '') === (right.tabId ?? '') &&
    left.surface.surfaceId === right.surface.surfaceId && left.surface.surfaceType === right.surface.surfaceType &&
    left.surface.snapshotVersion === right.surface.snapshotVersion;
}

/**
 * Bridges ready IDs to existing UI command handlers. The receipt is taken once,
 * and Complete precedes publication of any context changed by that handler.
 */
export function createExternalUICommandDispatcher(options: ExternalUICommandDispatcherOptions): {
  dispose(): void;
} {
  let disposed = false;
  let processing = Promise.resolve();
  const seen = new Set<string>();
  const seenOrder: string[] = [];
  const remember = (invocationId: string) => {
    if (seen.has(invocationId)) return false;
    seen.add(invocationId);
    seenOrder.push(invocationId);
    if (seenOrder.length > 4096) seen.delete(seenOrder.shift()!);
    return true;
  };

  const frameMatches = (event: ExternalUICommandReadyEvent, frame: ExternalUICommandFrame | null): boolean => {
    const status = options.service.getSnapshot();
    return !!frame && frame.hasFocus && !!frame.localGeneration && !!status && status.state === 'connected' &&
      status.connectionId === event.connectionId && status.generation === event.generation &&
      status.targetSnapshotId === event.targetSnapshotId && status.contextVersion === event.contextVersion &&
      sameOwner(status.owner, frame.owner) && sameDestination(status.target, frame.destination);
  };

  const processReady = async (event: ExternalUICommandReadyEvent) => {
    if (disposed) return;
    let current: ExternalUICommandFrame | null = null;
    try { current = options.readCurrentFrame(); } catch { /* Fail closed below. */ }
    let currentMatches = false;
    try { currentMatches = frameMatches(event, current); } catch { /* Fail closed. */ }
    if (!currentMatches) {
      void options.publishCurrentContext(event).catch(() => undefined);
      return;
    }
    const admittedLocalGeneration = current!.localGeneration;
    try { options.onCommandPendingChange?.(true, event); } catch { /* Accounting callbacks are non-authoritative. */ }
    let take: TakeExternalUICommandResult;
    try {
      take = await options.service.take(event);
    } catch {
      // Take is single-use at the backend. Do not retry on an uncertain transport result.
      try { options.onCommandPendingChange?.(false, event); } catch { /* Non-authoritative callback. */ }
      try { await options.publishCurrentContext(event); } catch { /* Best-effort context reconciliation. */ }
      return;
    }

    let outcome: ExternalUICommandOutcome = 'unknown';
    try {
      current = options.readCurrentFrame();
    } catch { current = null; }
    let matches = false;
    try {
      matches = !disposed && frameMatches(event, current) &&
        current?.localGeneration === admittedLocalGeneration &&
        sameDestination(take.target, options.service.getSnapshot()?.target ?? null);
    } catch { /* A torn-down or inconsistent UI frame cannot authorize an effect. */ }
    if (!matches || take.commandId !== event.commandId || take.invocationId !== event.invocationId) {
      outcome = 'cancelled';
    } else {
      let supported = false;
      try { supported = Object.keys(take.arguments).length === 0 && options.isSupported(take.commandId, take.arguments); }
      catch { supported = false; }
      if (!supported) outcome = 'failed';
      else {
        try {
          outcome = options.execute(take.commandId, event) ? 'succeeded' : 'cancelled';
        } catch {
          outcome = 'unknown';
        }
      }
    }

    try {
      await options.service.complete(event, take, outcome);
    } catch {
      // Never replay a command after a completion transport failure.
    }
    try { options.onCommandPendingChange?.(false, event); } catch { /* Non-authoritative callback. */ }
    try {
      await options.publishCurrentContext(event);
    } catch {
      // Context publication is CAS/readiness maintenance, never a command retry.
    }
  };

  const onReady = (event: ExternalUICommandReadyEvent) => {
    if (disposed || !remember(event.invocationId)) return;
    processing = processing.then(() => processReady(event)).catch(() => undefined);
  };

  const unsubscribe = options.service.subscribeReady(onReady);
  return {
    dispose() {
      if (disposed) return;
      disposed = true;
      unsubscribe();
    },
  };
}
