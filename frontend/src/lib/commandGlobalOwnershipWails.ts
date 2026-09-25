import { createCommandGlobalOwnership } from './commandGlobalOwnership';
import { waitForWailsBridge } from './waitForWailsBridge';

interface OwnershipAPI {
  GetGlobalCommandOwnership(): Promise<unknown>;
  AckGlobalCommandOwnership(instanceId: string, revision: number): Promise<boolean>;
}
type OwnershipWindow = Window & { go?: { app?: { App?: Partial<OwnershipAPI> } } };

export interface GlobalCommandOwnershipConnectionOptions {
  subscribe: (listener: (frame: unknown) => void) => () => void;
  target?: OwnershipWindow;
  readSnapshot?: () => Promise<unknown>;
  acknowledge?: (instanceId: string, revision: number) => Promise<boolean>;
  onChange?: () => void;
}

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
function record(raw: unknown): Record<string, unknown> | undefined {
  return raw !== null && typeof raw === 'object' && !Array.isArray(raw)
    ? raw as Record<string, unknown> : undefined;
}

/** Subscribe before bootstrap; only the getter may establish the backend instance. */
export function connectGlobalCommandOwnership(options: GlobalCommandOwnershipConnectionOptions) {
  let disposed = false;
  let initialized = false;
  let expectedInstance: string | undefined;
  let retry: ReturnType<typeof setTimeout> | undefined;
  const buffered: unknown[] = [];
  const abort = new AbortController();
  const appFor = async (): Promise<OwnershipAPI> => {
    const target = options.target ?? window as OwnershipWindow;
    await waitForWailsBridge({ target, signal: abort.signal });
    const app = target.go?.app?.App;
    if (typeof app?.GetGlobalCommandOwnership !== 'function' ||
        typeof app.AckGlobalCommandOwnership !== 'function') {
      throw new Error('Global command ownership API unavailable');
    }
    return app as OwnershipAPI;
  };
  const readSnapshot = options.readSnapshot ?? (async () => {
    const app = await appFor();
    return disposed ? undefined : app.GetGlobalCommandOwnership();
  });
  const acknowledge = options.acknowledge ?? (async (instanceId, revision) => {
    const app = await appFor();
    return disposed ? false : app.AckGlobalCommandOwnership(instanceId, revision);
  });
  const state = createCommandGlobalOwnership({
    onChange: options.onChange,
    ack: ({ instanceId, revision }) => {
      if (!disposed) void acknowledge(instanceId, revision).catch(() => {
        // Keep reservations installed on transport failure. Never unblock by guessing.
      });
    },
  });
  const apply = (raw: unknown): boolean => {
    if (disposed || record(raw)?.instanceId !== expectedInstance) return false;
    return state.applyFrame(raw);
  };
  const unsubscribe = options.subscribe((raw) => {
    if (disposed) return;
    if (!initialized) {
      buffered.push(raw);
      return;
    }
    apply(raw);
  });
  const bootstrap = async (): Promise<void> => {
    try {
      const raw = await readSnapshot();
      if (disposed) return;
      const frame = record(raw);
      if (!frame || frame.version !== 1 || !Array.isArray(frame.combinations)) return;
      if (frame.platform === 'unsupported' && frame.combinations.length === 0) {
        initialized = true;
        buffered.length = 0;
        return;
      }
      if (frame.platform !== 'windows' || typeof frame.instanceId !== 'string' || !UUID.test(frame.instanceId)) return;
      expectedInstance = frame.instanceId;
      if (!(frame.revision === 0 && frame.combinations.length === 0) && !apply(raw)) return;
      initialized = true;
      for (const pending of buffered) apply(pending);
      buffered.length = 0;
    } catch {
      // Local command dispatch stays blocked until ownership can be established.
    } finally {
      if (!disposed && !initialized) {
        retry = setTimeout(() => { void bootstrap(); }, 1000);
      }
    }
  };
  const ready = bootstrap();
  return {
    // Completion of the first attempt; isReady remains authoritative after retries.
    ready,
    isReady: () => !disposed && initialized,
    owns: (event: KeyboardEvent) => !disposed && state.owns(event),
    dispose: () => {
      if (disposed) return;
      disposed = true;
      if (retry !== undefined) clearTimeout(retry);
      abort.abort();
      unsubscribe();
      buffered.length = 0;
      state.dispose();
    },
  };
}

// App keeps the bridge alive outside AuthGate; Topbar shares precisely that
// state. Separate subscribers must never ACK a frame before the keyboard's
// exclusion map has been updated.
let shared: ReturnType<typeof connectGlobalCommandOwnership> | undefined;
const changeListeners = new Set<() => void>();
let consumers = 0;
export function acquireGlobalCommandOwnership(options: GlobalCommandOwnershipConnectionOptions) {
  // Each lease owns a distinct listener, even when callers reuse one callback.
  const change = options.onChange ? () => options.onChange!() : undefined;
  if (change) changeListeners.add(change);
  if (!shared) {
    shared = connectGlobalCommandOwnership({
      ...options,
      onChange: () => {
        for (const listener of changeListeners) {
          try { listener(); } catch { /* One consumer must not prevent others from invalidating. */ }
        }
      },
    });
  }
  consumers++;
  const connection = shared;
  let released = false;
  return {
    ready: connection.ready,
    owns: connection.owns,
    isReady: connection.isReady,
    dispose: () => {
      if (released) return;
      released = true;
      if (change) changeListeners.delete(change);
      if (--consumers === 0) {
        connection.dispose();
        shared = undefined;
      }
    },
  };
}
