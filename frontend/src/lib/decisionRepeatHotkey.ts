import { EventsOn } from '@wailsjs/runtime/runtime';
import { acquireGlobalCommandOwnership } from './commandGlobalOwnershipWails';
import { isEditableKeyboardTarget } from './decisionMnemonic';
import { ReadFocusContext } from './commandContextProviders';
import {
  getModalRegistrySnapshot,
  subscribeModalRegistry,
} from './modalRegistry';
import { waitForWailsBridge } from './waitForWailsBridge';

const MAX_RETRIES = 3;
const HEARTBEAT_MS = 8_000;
const RETRY_DELAYS_MS = [100, 500, 1_000] as const;

export interface DecisionRepeatHotkeyAPI {
  OpenDecisionRepeatHotkeySession(): Promise<string>;
  SetDecisionRepeatHotkey(sessionID: string, revision: number, dialogID: string): Promise<void>;
  CloseDecisionRepeatHotkeySession(sessionID: string): Promise<void>;
}

interface DynamicWailsWindow extends Window {
  go?: {
    app?: {
      App?: Partial<DecisionRepeatHotkeyAPI>;
    };
  };
}

interface GlobalOwnershipLease {
  readonly isReady: () => boolean;
  readonly owns: (event: KeyboardEvent) => boolean;
  readonly dispose: () => void;
}

interface DecisionRepeatTarget {
  readonly modalId: string;
  readonly token: object;
  readonly repeat: () => void;
}

interface DecisionRepeatTargetIdentity {
  readonly modalId: string;
  readonly token: object;
}

interface NativeReservation {
  readonly modalId: string;
  readonly token: object;
  readonly revision: number;
  readonly dialogId: string;
  readonly sessionId: string;
  readonly api: DecisionRepeatHotkeyAPI;
  ready: boolean;
}

interface PendingClose {
  readonly sessionId: string;
  readonly api: DecisionRepeatHotkeyAPI;
  attempts: number;
}

export interface DecisionRepeatHotkeyCoordinatorOptions {
  readonly getAPI?: () => DecisionRepeatHotkeyAPI | undefined;
  readonly subscribeEvent?: (
    eventName: string,
    listener: (payload: unknown) => void,
  ) => (() => void) | undefined;
  readonly subscribeModal?: (listener: () => void) => (() => void) | undefined;
  readonly readModal?: typeof getModalRegistrySnapshot;
  readonly acquireOwnership?: () => GlobalOwnershipLease;
  readonly waitForBridge?: () => Promise<boolean>;
}

function createOpaqueId(prefix: string): string {
  try {
    if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
      return `${prefix}-${crypto.randomUUID()}`;
    }
  } catch {
    // Fallback abaixo é apenas um token local de ativação.
  }
  return `${prefix}-${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;
}

function defaultGetAPI(): DecisionRepeatHotkeyAPI | undefined {
  if (typeof window === 'undefined') return undefined;
  const app = (window as DynamicWailsWindow).go?.app?.App;
  if (
    typeof app?.OpenDecisionRepeatHotkeySession !== 'function' ||
    typeof app.SetDecisionRepeatHotkey !== 'function' ||
    typeof app.CloseDecisionRepeatHotkeySession !== 'function'
  ) {
    return undefined;
  }
  return app as DecisionRepeatHotkeyAPI;
}

function defaultSubscribeEvent(
  eventName: string,
  listener: (payload: unknown) => void,
): () => void {
  const unsubscribe = EventsOn(eventName, listener);
  if (typeof unsubscribe !== 'function') {
    throw new Error('EventsOn did not return an unsubscribe callback');
  }
  return unsubscribe;
}

function defaultAcquireOwnership(): GlobalOwnershipLease {
  const subscribe = (listener: (payload: unknown) => void): (() => void) => {
    const unsubscribe = EventsOn('command:global-ownership', listener);
    if (typeof unsubscribe !== 'function') {
      throw new Error('EventsOn did not return an ownership unsubscribe callback');
    }
    return unsubscribe;
  };
  return acquireGlobalCommandOwnership({
    subscribe,
  });
}

function validEvent(payload: unknown): payload is {
  sessionId: string;
  revision: number;
  dialogId: string;
} {
  if (!payload || typeof payload !== 'object' || Array.isArray(payload)) return false;
  const event = payload as Record<string, unknown>;
  return (
    Object.keys(event).every((key) => key === 'sessionId' || key === 'revision' || key === 'dialogId') &&
    typeof event.sessionId === 'string' && event.sessionId.length > 0 && event.sessionId.trim() === event.sessionId &&
    typeof event.dialogId === 'string' && event.dialogId.length > 0 && event.dialogId.trim() === event.dialogId &&
    typeof event.revision === 'number' && Number.isSafeInteger(event.revision) && event.revision > 0
  );
}

function sameTarget(
  left: DecisionRepeatTargetIdentity | undefined,
  right: DecisionRepeatTargetIdentity,
): boolean {
  return left?.modalId === right.modalId && left.token === right.token;
}

/**
 * Coordena a única reserva temporária de Ctrl+Shift+R para todos os
 * DecisionDialogs. A sessão nativa é uma otimização de alcance fora da janela;
 * o callback local continua sendo a rota segura até Set + ownership ACK.
 */
export function createDecisionRepeatHotkeyCoordinator(
  options: DecisionRepeatHotkeyCoordinatorOptions = {},
) {
  const getAPI = options.getAPI ?? defaultGetAPI;
  const subscribeEvent = options.subscribeEvent ?? defaultSubscribeEvent;
  const subscribeModal = options.subscribeModal ?? subscribeModalRegistry;
  const readModal = options.readModal ?? getModalRegistrySnapshot;
  const bridgeAbort = new AbortController();
  const waitForBridge = options.waitForBridge ?? (async () => {
    if (typeof window === 'undefined') return false;
    if (typeof (window as DynamicWailsWindow).go !== 'undefined') return true;
    try {
      await waitForWailsBridge({ timeoutMs: 1_000, signal: bridgeAbort.signal });
      return true;
    } catch {
      return false;
    }
  });

  let disposed = false;
  let subscriptionsReady = false;
  let running = false;
  const registrations = new Map<string, DecisionRepeatTarget>();
  let target: DecisionRepeatTarget | undefined;
  let reservation: NativeReservation | undefined;
  let unsupportedToken: object | undefined;
  let exhaustedToken: object | undefined;
  let retryAttempt = 0;
  let retryTimer: ReturnType<typeof setTimeout> | undefined;
  let heartbeatTimer: ReturnType<typeof setTimeout> | undefined;
  let renewRequested = false;
  let revision = 0;
  let warnedTransport = false;
  const pendingCloses = new Map<string, PendingClose>();
  let cleanupTimer: ReturnType<typeof setTimeout> | undefined;
  let composing = false;

  let ownership: GlobalOwnershipLease | undefined;
  let ownershipDisposed = false;

  const warnTransportOnce = () => {
    if (warnedTransport) return;
    warnedTransport = true;
    // Não incluir IDs, conteúdo da pergunta ou dados da sessão no diagnóstico.
    // eslint-disable-next-line no-console -- falha anômala de transporte.
    console.warn('[DecisionDialog] reserva nativa indisponível; mantendo atalho local');
  };

  const enqueue = <T,>(call: () => Promise<T>): Promise<T> => {
    const next = callQueue.then(call, call);
    callQueue = next.then(() => undefined, () => undefined);
    return next;
  };
  let callQueue = Promise.resolve();

  const isCurrentTarget = (candidate: DecisionRepeatTarget): boolean =>
    !disposed && sameTarget(target, candidate);

  const cancelHeartbeat = () => {
    if (heartbeatTimer !== undefined) {
      clearTimeout(heartbeatTimer);
      heartbeatTimer = undefined;
    }
  };

  const schedule = () => {
    if (disposed || running) return;
    running = true;
    void reconcile().catch(() => {
      // Nenhuma falha de transporte pode virar unhandled rejection de um
      // efeito React; o fallback local continua sendo a rota segura.
      warnTransportOnce();
    }).finally(() => {
      running = false;
      if (!disposed && needsWork()) schedule();
    });
  };

  const scheduleCleanup = () => {
    if (disposed && pendingCloses.size === 0) return;
    if (cleanupTimer !== undefined) return;
    cleanupTimer = setTimeout(() => {
      cleanupTimer = undefined;
      void drainPendingCloses();
    }, 1_000);
  };

  const drainPendingCloses = async () => {
    for (const [sessionId, pending] of [...pendingCloses]) {
      try {
        await enqueue(() => pending.api.CloseDecisionRepeatHotkeySession(sessionId));
        pendingCloses.delete(sessionId);
      } catch {
        pending.attempts += 1;
        if (pending.attempts >= MAX_RETRIES) {
          pendingCloses.delete(sessionId);
          warnTransportOnce();
        }
      }
    }
    if (disposed && pendingCloses.size === 0 && !ownershipDisposed) {
      ownershipDisposed = true;
      ownership?.dispose();
    }
    if (pendingCloses.size > 0) scheduleCleanup();
  };

  const closeSession = async (sessionId: string, api: DecisionRepeatHotkeyAPI) => {
    try {
      await enqueue(() => api.CloseDecisionRepeatHotkeySession(sessionId));
    } catch {
      pendingCloses.set(sessionId, { sessionId, api, attempts: 1 });
      warnTransportOnce();
      scheduleCleanup();
    }
  };

  const releaseReservation = async () => {
    cancelHeartbeat();
    renewRequested = false;
    const current = reservation;
    reservation = undefined;
    if (current) await closeSession(current.sessionId, current.api);
  };

  const scheduleRetry = (candidate: DecisionRepeatTarget) => {
    if (!isCurrentTarget(candidate) || exhaustedToken === candidate.token || retryTimer !== undefined) return;
    if (retryAttempt >= MAX_RETRIES) {
      exhaustedToken = candidate.token;
      return;
    }
    const delay = RETRY_DELAYS_MS[retryAttempt] ?? RETRY_DELAYS_MS[RETRY_DELAYS_MS.length - 1];
    retryAttempt += 1;
    retryTimer = setTimeout(() => {
      retryTimer = undefined;
      schedule();
    }, delay);
  };

  const armHeartbeat = (current: NativeReservation) => {
    cancelHeartbeat();
    heartbeatTimer = setTimeout(() => {
      heartbeatTimer = undefined;
      if (!reservation || reservation !== current || !target || !sameTarget(target, current)) return;
      renewRequested = true;
      schedule();
    }, HEARTBEAT_MS);
  };

  const isReservationCurrent = (current: NativeReservation): boolean =>
    !!target && sameTarget(target, current) &&
    readModal().topID === current.modalId && !disposed;

  async function reconcile(): Promise<void> {
    if (disposed || !subscriptionsReady) return;
    await drainPendingCloses();

    const desired = target;
    if (!desired) {
      await releaseReservation();
      return;
    }

    if ((unsupportedToken === desired.token || exhaustedToken === desired.token) && !reservation) return;

    if (reservation && !sameTarget(desired, reservation)) {
      await releaseReservation();
    }

    if (reservation) {
      if (!reservation.ready) return;
      if (!renewRequested) {
        armHeartbeat(reservation);
        return;
      }
      const current = reservation;
      renewRequested = false;
      try {
        await enqueue(() => current.api.SetDecisionRepeatHotkey(
          current.sessionId,
          current.revision,
          current.dialogId,
        ));
      } catch {
        if (reservation === current) {
          reservation = undefined;
          cancelHeartbeat();
          await closeSession(current.sessionId, current.api);
        }
        warnTransportOnce();
        scheduleRetry(desired);
        return;
      }
      if (reservation === current && isReservationCurrent(current)) armHeartbeat(current);
      return;
    }

    if (unsupportedToken === desired.token || exhaustedToken === desired.token) return;
    let api = getAPI();
    if (!api) {
      const bridgeAvailable = await waitForBridge();
      if (!isCurrentTarget(desired)) return;
      api = getAPI();
      if (!bridgeAvailable || !api) {
        unsupportedToken = desired.token;
        return;
      }
    }

    const nextRevision = ++revision;
    const dialogId = createOpaqueId('decision-repeat');
    let sessionId: string;
    try {
      sessionId = await enqueue(() => api.OpenDecisionRepeatHotkeySession());
    } catch {
      warnTransportOnce();
      scheduleRetry(desired);
      return;
    }

    if (typeof sessionId !== 'string' || sessionId.trim() !== sessionId) {
      warnTransportOnce();
      scheduleRetry(desired);
      return;
    }
    if (sessionId.length === 0) {
      unsupportedToken = desired.token;
      return;
    }

    if (!isCurrentTarget(desired)) {
      await closeSession(sessionId, api);
      return;
    }

    reservation = {
      modalId: desired.modalId,
      token: desired.token,
      revision: nextRevision,
      dialogId,
      sessionId,
      api,
      ready: false,
    };
    const current: NativeReservation = reservation;

    try {
      await enqueue(() => api.SetDecisionRepeatHotkey(sessionId, nextRevision, dialogId));
    } catch {
      if (reservation === current) {
        reservation = undefined;
        await closeSession(sessionId, api);
      }
      warnTransportOnce();
      scheduleRetry(desired);
      return;
    }

    if (!isCurrentTarget(desired) || readModal().topID !== desired.modalId) {
      if (reservation === current) {
        reservation = undefined;
        await closeSession(sessionId, api);
      }
      return;
    }

    retryAttempt = 0;
    if (reservation !== current) return;
    current.ready = true;
    armHeartbeat(current);
  }

  const handleEvent = (payload: unknown) => {
    if (!validEvent(payload) || !reservation || !target || !sameTarget(target, reservation)) return;
    if (
      payload.sessionId !== reservation.sessionId ||
      payload.revision !== reservation.revision ||
      payload.dialogId !== reservation.dialogId ||
      readModal().topID !== reservation.modalId
    ) return;

    let hasFocus = false;
    try {
      hasFocus = typeof document !== 'undefined' && document.hasFocus();
    } catch {
      hasFocus = false;
    }
    if (hasFocus) {
      if (isEditableKeyboardTarget(document.activeElement) || ReadFocusContext().composition === 'active' || composing) {
        return;
      }
    }
    target.repeat();
  };

  let unsubscribeEvent: () => void = () => undefined;
  let unsubscribeModal: () => void = () => undefined;
  let removeCompositionListeners: () => void = () => undefined;
  const refreshDesired = () => {
    const topID = readModal().topID;
    const next = topID ? registrations.get(topID) : undefined;
    if (next?.token === target?.token) return;
    target = next;
    unsupportedToken = undefined;
    exhaustedToken = undefined;
    retryAttempt = 0;
    if (retryTimer !== undefined) {
      clearTimeout(retryTimer);
      retryTimer = undefined;
    }
  };
  const onModalChange = () => {
    refreshDesired();
    schedule();
  };
  try {
    // A assinatura deve existir antes da primeira abertura de sessão.
    const eventCleanup = subscribeEvent('command:decision-repeat', handleEvent);
    if (typeof eventCleanup === 'function') {
      unsubscribeEvent = eventCleanup;
      try {
        const modalCleanup = subscribeModal(onModalChange);
        if (typeof modalCleanup === 'function') {
          unsubscribeModal = modalCleanup;
          subscriptionsReady = true;
        } else {
          unsubscribeEvent();
          unsubscribeEvent = () => undefined;
        }
      } catch {
        try { unsubscribeEvent(); } catch { /* cleanup parcial */ }
        unsubscribeEvent = () => undefined;
      }
    }
  } catch {
    // O caminho local segue funcional fora do runtime Wails.
  }
  if (subscriptionsReady) {
    try {
      ownership = options.acquireOwnership?.() ?? defaultAcquireOwnership();
      if (!ownership || typeof ownership.isReady !== 'function' || typeof ownership.owns !== 'function' || typeof ownership.dispose !== 'function') {
        throw new Error('global command ownership lease is invalid');
      }
    } catch {
      // Sem ownership global, a falha de integração nunca remove o fallback local.
      ownership = undefined;
      subscriptionsReady = false;
    }
  }
  if (typeof document !== 'undefined') {
    const start = () => { composing = true; };
    const end = () => { composing = false; };
    document.addEventListener('compositionstart', start, true);
    document.addEventListener('compositionupdate', start, true);
    document.addEventListener('compositionend', end, true);
    document.addEventListener('compositioncancel', end, true);
    document.addEventListener('blur', end, true);
    removeCompositionListeners = () => {
      document.removeEventListener('compositionstart', start, true);
      document.removeEventListener('compositionupdate', start, true);
      document.removeEventListener('compositionend', end, true);
      document.removeEventListener('compositioncancel', end, true);
      document.removeEventListener('blur', end, true);
    };
  } else {
    removeCompositionListeners = () => undefined;
  }

  function needsWork(): boolean {
    if (!subscriptionsReady) return false;
    if (pendingCloses.size > 0) return false;
    if (!target) return !!reservation;
    if (reservation) return !sameTarget(target, reservation) || renewRequested;
    return unsupportedToken !== target.token && exhaustedToken !== target.token && retryTimer === undefined;
  }

  return {
    register(modalId: string, repeat: () => void): () => void {
      if (disposed || !modalId) return () => undefined;
      const token = {};
      const next: DecisionRepeatTarget = { modalId, token, repeat };
      registrations.set(modalId, next);
      refreshDesired();
      schedule();
      let released = false;
      return () => {
        if (released) return;
        released = true;
        if (registrations.get(modalId)?.token !== token) return;
        registrations.delete(modalId);
        refreshDesired();
        schedule();
      };
    },
    ownsNative(event: KeyboardEvent): boolean {
      if (!reservation || !target || !sameTarget(target, reservation)) return false;
      if (readModal().topID !== reservation.modalId) return false;
      return !!ownership?.isReady() && !!ownership.owns(event);
    },
    dispose(): void {
      if (disposed) return;
      disposed = true;
      bridgeAbort.abort();
      registrations.clear();
      target = undefined;
      cancelHeartbeat();
      if (retryTimer !== undefined) clearTimeout(retryTimer);
      retryTimer = undefined;
      try { unsubscribeEvent(); } catch { /* cleanup defensivo de host legado */ }
      try { unsubscribeModal(); } catch { /* cleanup defensivo de host legado */ }
      removeCompositionListeners();
      const current = reservation;
      reservation = undefined;
      if (current) {
        void closeSession(current.sessionId, current.api).finally(() => { void drainPendingCloses(); });
      } else {
        void drainPendingCloses();
      }
    },
  };
}

let singleton: ReturnType<typeof createDecisionRepeatHotkeyCoordinator> | undefined;
let singletonConsumers = 0;

function getSingleton() {
  singleton ??= createDecisionRepeatHotkeyCoordinator();
  return singleton;
}

export function registerDecisionRepeatHotkey(modalId: string, repeat: () => void): () => void {
  const coordinator = getSingleton();
  singletonConsumers += 1;
  const release = coordinator.register(modalId, repeat);
  let released = false;
  return () => {
    if (released) return;
    released = true;
    release();
    singletonConsumers -= 1;
    if (singletonConsumers === 0) {
      coordinator.dispose();
      singleton = undefined;
    }
  };
}

export function ownsDecisionRepeatNatively(event: KeyboardEvent): boolean {
  return singleton?.ownsNative(event) ?? false;
}
