/**
 * Ponte tipada entre a UI e um host confiável de comandos.
 *
 * O módulo não importa Wails e não instala listeners do SO. O host injeta
 * CommandBridgePort; isso deixa o contrato montável por Godel sem transformar
 * uma porta fake em integração de produto.
 */

export type CommandOwnership = 'local' | 'global';
export type CommandSource =
  | 'keyboard.local'
  | 'keyboard.global'
  | 'streamdeck.key'
  | 'palette'
  | 'ui.action'
  | 'chat'
  | 'cli'
  | 'event'
  | 'system';

export const DECISION_RESPOND_COMMAND_ID = 'decision.respond';
export const DECISION_REPEAT_TRIGGER = 'keyboard.local:Ctrl+Shift+R';

/** Scope do dispatcher enquanto uma decisão está no topo do stack. */
export interface DialogCommandScope {
  readonly dialogId: string;
  readonly kind: 'decision';
  /** Geração monotônica do scope, distinta da geração da sessão. */
  readonly generation: string;
  readonly allowedCommandIds: readonly ['decision.respond'];
  readonly allowedTriggerSpecs: readonly ['keyboard.local:Ctrl+Shift+R'];
}

export interface CommandBridgeOwner {
  readonly userId: string;
  readonly sessionId: string;
  readonly workspaceId: string;
}

export interface CommandCapability {
  readonly id: string;
  readonly commandId: string;
  readonly generation: string;
  readonly owner: CommandBridgeOwner;
}

export interface CommandSession {
  readonly id: string;
  readonly generation: string;
  readonly owner: CommandBridgeOwner;
}

export interface CommandInvocation {
  readonly sessionId: string;
  readonly invocationId: string;
  readonly commandId: string;
  readonly generation: string;
  readonly capabilityId: string;
  readonly ownership: CommandOwnership;
  readonly source: CommandSource;
  readonly occurrenceId?: string;
  readonly eventId?: string;
}

export interface CommandInvocationAck {
  readonly invocationId: string;
  readonly accepted: boolean;
  readonly reason?: string;
}

export type CommandResultStatus = 'succeeded' | 'failed' | 'cancelled';

export interface CommandResult {
  readonly sessionId: string;
  readonly invocationId: string;
  readonly commandId: string;
  readonly generation: string;
  readonly capabilityId: string;
  readonly ownership: CommandOwnership;
  readonly occurrenceId?: string;
  readonly eventId?: string;
  readonly owner: CommandBridgeOwner;
  readonly status: CommandResultStatus;
  readonly payload?: unknown;
}

export interface CommandResultAck {
  readonly invocationId: string;
  readonly accepted: boolean;
  readonly reason?: string;
}

export interface CommandCancelRequest {
  readonly sessionId: string;
  readonly invocationId: string;
  readonly generation: string;
  readonly capabilityId: string;
  readonly owner: CommandBridgeOwner;
}

export interface CommandCancelAck {
  readonly invocationId: string;
  readonly accepted: boolean;
  readonly reason?: string;
}

export interface CommandBridgePort {
  dispatch(invocation: CommandInvocation): Promise<CommandInvocationAck>;
  cancel(request: CommandCancelRequest): Promise<void>;
  /** Libera recursos do adapter, quando a porta os possui. */
  shutdown?: () => Promise<void>;
}

export type CommandInputKind = 'down' | 'up';

export interface CommandInput {
  readonly sessionId: string;
  readonly source: string;
  readonly key: string;
  readonly generation: string;
  readonly kind: CommandInputKind;
  readonly repeat?: boolean;
  readonly invocation: CommandInvocation;
  readonly owner: CommandBridgeOwner;
}

export type CommandLifecycleKind = 'generation' | 'repeat' | 'release' | 'blur' | 'lock' | 'logout';

export interface CommandLifecycleEvent {
  readonly kind: CommandLifecycleKind;
  readonly sessionId: string;
  readonly generation?: string;
  readonly input?: CommandInput;
}

export type CommandResultListener = (result: Readonly<CommandResult>) => void;

export class CommandBridgeError extends Error {
  readonly code:
    | 'invalid-configuration'
    | 'invalid-request'
    | 'unknown-session'
    | 'stale-generation'
    | 'session-unavailable'
    | 'capability-denied'
    | 'ownership-conflict'
    | 'invocation-replay'
    | 'unknown-invocation'
    | 'bridge-closed';

  constructor(code: CommandBridgeError['code'], message = code) {
    super(message);
    this.name = 'CommandBridgeError';
    this.code = code;
  }
}

export interface CommandBridge {
  openSession(session: CommandSession): void;
  replaceCapabilities(capabilities: readonly CommandCapability[]): void;
  advanceGeneration(sessionId: string, generation: string): Promise<void>;
  invoke(invocation: CommandInvocation, owner: CommandBridgeOwner): Promise<CommandInvocationAck>;
  acceptResult(result: CommandResult): CommandResultAck;
  cancel(request: CommandCancelRequest): Promise<CommandCancelAck>;
  input(input: CommandInput): Promise<CommandInvocationAck>;
  lifecycle(event: CommandLifecycleEvent): Promise<void>;
  shutdown(): Promise<void>;
  subscribeResult(listener: CommandResultListener): () => void;
}

interface SessionState {
  session: CommandSession;
  locked: boolean;
  pressed: Set<string>;
}

interface PendingInvocation {
  invocation: CommandInvocation;
  owner: CommandBridgeOwner;
  claimKey: string;
}

const ownerships: readonly CommandOwnership[] = ['local', 'global'];

function validText(value: unknown): value is string {
  return typeof value === 'string' && value.length > 0 && value.trim() === value;
}

function validOwner(owner: CommandBridgeOwner): boolean {
  return validText(owner?.userId) && validText(owner?.sessionId) && validText(owner?.workspaceId);
}

function validGeneration(value: unknown): value is string {
  return typeof value === 'string' && /^[1-9]\d*$/.test(value);
}

function validUUID7(value: unknown): value is string {
  return typeof value === 'string' && /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(value);
}

function generationAtMost(left: string, right: string): boolean {
  return BigInt(left) <= BigInt(right);
}

function sameOwner(left: CommandBridgeOwner, right: CommandBridgeOwner): boolean {
  return left.userId === right.userId && left.sessionId === right.sessionId && left.workspaceId === right.workspaceId;
}

function validOwnership(value: CommandOwnership): boolean {
  return ownerships.includes(value);
}

function validSession(session: CommandSession): boolean {
  return validText(session?.id) && validGeneration(session?.generation) && validOwner(session.owner) && session.owner.sessionId === session.id;
}

function validInvocation(invocation: CommandInvocation): boolean {
  if (!validText(invocation?.sessionId) || !validUUID7(invocation?.invocationId) || !validText(invocation?.commandId) || !validGeneration(invocation?.generation) || !validText(invocation?.capabilityId) || !validOwnership(invocation?.ownership) || !validSource(invocation?.source) || (invocation.occurrenceId !== undefined && !validText(invocation.occurrenceId))) return false;
  if (invocation.source === 'event') return validUUID7(invocation.eventId);
  return invocation.eventId === undefined;
}

function validCapability(capability: CommandCapability): boolean {
  return validText(capability?.id) && validText(capability?.commandId) && validGeneration(capability?.generation) && validOwner(capability.owner);
}

function validResult(result: CommandResult): boolean {
  return validInvocation({ ...result, source: 'ui.action', eventId: undefined }) && (result.eventId === undefined || validUUID7(result.eventId)) && validOwner(result.owner) && ['succeeded', 'failed', 'cancelled'].includes(result.status);
}

function cloneOwner(owner: CommandBridgeOwner): CommandBridgeOwner {
  return Object.freeze({ ...owner });
}

function cloneSession(session: CommandSession): CommandSession {
  return Object.freeze({ ...session, owner: cloneOwner(session.owner) });
}

function cloneCapability(capability: CommandCapability): CommandCapability {
  return Object.freeze({ ...capability, owner: cloneOwner(capability.owner) });
}

function bridgeError(code: CommandBridgeError['code']): CommandBridgeError {
  return new CommandBridgeError(code);
}

function normalizeOccurrence(source: string, key: string): string {
  const bytes = (value: string) => new TextEncoder().encode(value).length;
  return `${bytes(source)}:${source}${bytes(key)}:${key}`;
}

function ownershipClaimKey(invocation: CommandInvocation, owner: CommandBridgeOwner): string {
  // O commandId e o tipo local/global ficam fora: ownership é do adapter/
  // trigger, enquanto invocações diretas (sem ocorrência) não adquirem claim.
  return [invocation.sessionId, owner.userId, owner.sessionId, owner.workspaceId, invocation.generation, invocation.occurrenceId].join('\u0000');
}

function validSource(value: unknown): value is CommandSource {
  return ['keyboard.local', 'keyboard.global', 'streamdeck.key', 'palette', 'ui.action', 'chat', 'cli', 'event', 'system'].includes(value as string);
}

export function createCommandBridge(config: { readonly port: CommandBridgePort; readonly capabilities: readonly CommandCapability[] }): CommandBridge {
  if (!config?.port || typeof config.port.dispatch !== 'function' || typeof config.port.cancel !== 'function' || config.capabilities.length === 0) {
    throw bridgeError('invalid-configuration');
  }

  const buildCapabilities = (values: readonly CommandCapability[]): Map<string, CommandCapability> => {
    if (values.length === 0) throw bridgeError('invalid-configuration');
    const next = new Map<string, CommandCapability>();
    for (const capability of values) {
      if (!validCapability(capability) || next.has(capability.id)) throw bridgeError('invalid-configuration');
      next.set(capability.id, cloneCapability(capability));
    }
    return next;
  };
  let capabilities = buildCapabilities(config.capabilities);

  const sessions = new Map<string, SessionState>();
  const pending = new Map<string, PendingInvocation>();
  const claims = new Set<string>();
  const listeners = new Set<CommandResultListener>();
  let closed = false;
  let shutdownPromise: Promise<void> | undefined;

  const activeCancellationBatches = new Set<Promise<void>>();

  const cancelRequests = (requests: readonly CommandCancelRequest[]): Promise<void> => {
    const batch = (async () => {
      let firstError: unknown;
      for (const request of requests) {
        try {
          await config.port.cancel({ ...request, owner: cloneOwner(request.owner) });
        } catch (error) {
          firstError ??= error;
        }
      }
      if (firstError !== undefined) throw firstError;
    })();
    activeCancellationBatches.add(batch);
    void batch.then(
      () => activeCancellationBatches.delete(batch),
      () => activeCancellationBatches.delete(batch),
    );
    return batch;
  };

  const removePending = (invocationId: string): void => {
    const current = pending.get(invocationId);
    if (!current) return;
    pending.delete(invocationId);
    if (current.claimKey !== '') claims.delete(current.claimKey);
  };

  const invalidateSession = (sessionId: string): CommandCancelRequest[] => {
    const requests: CommandCancelRequest[] = [];
    for (const [invocationId, current] of pending) {
      if (current.invocation.sessionId !== sessionId) continue;
      requests.push({ sessionId, invocationId, generation: current.invocation.generation, capabilityId: current.invocation.capabilityId, owner: current.owner });
      removePending(invocationId);
    }
    return requests;
  };

  const invalidateAll = (): CommandCancelRequest[] => {
    const requests: CommandCancelRequest[] = [];
    for (const [invocationId, current] of pending) {
      requests.push({ sessionId: current.invocation.sessionId, invocationId, generation: current.invocation.generation, capabilityId: current.invocation.capabilityId, owner: current.owner });
      removePending(invocationId);
    }
    for (const state of sessions.values()) {
      state.locked = true;
      state.pressed.clear();
    }
    return requests;
  };

  const bridge: CommandBridge = {
    openSession(session) {
      if (closed) throw bridgeError('bridge-closed');
      if (!validSession(session) || sessions.has(session.id)) throw bridgeError('invalid-request');
      sessions.set(session.id, { session: cloneSession(session), locked: false, pressed: new Set() });
    },

    replaceCapabilities(values) {
      if (closed) throw bridgeError('bridge-closed');
      capabilities = buildCapabilities(values);
    },

    async advanceGeneration(sessionId, generation) {
      if (closed) throw bridgeError('bridge-closed');
      if (!validText(sessionId) || !validGeneration(generation)) throw bridgeError('invalid-request');
      const state = sessions.get(sessionId);
      if (!state) throw bridgeError('unknown-session');
      if (state.locked) throw bridgeError('session-unavailable');
      if (generationAtMost(generation, state.session.generation)) throw bridgeError('stale-generation');
      state.session = Object.freeze({ ...state.session, generation });
      state.pressed.clear();
      await cancelRequests(invalidateSession(sessionId));
    },

    async invoke(invocation, owner) {
      if (closed) throw bridgeError('bridge-closed');
      if (!validInvocation(invocation) || !validOwner(owner)) throw bridgeError('invalid-request');
      const state = sessions.get(invocation.sessionId);
      if (!state) throw bridgeError('unknown-session');
      if (state.locked) throw bridgeError('session-unavailable');
      if (invocation.generation !== state.session.generation || !sameOwner(owner, state.session.owner)) throw bridgeError('stale-generation');
      const capability = capabilities.get(invocation.capabilityId);
      if (!capability || capability.commandId !== invocation.commandId || capability.generation !== invocation.generation || !sameOwner(capability.owner, owner)) throw bridgeError('capability-denied');
      if (pending.has(invocation.invocationId)) throw bridgeError('invocation-replay');
      const stableOwner = cloneOwner(owner);
      const claimKey = invocation.occurrenceId === undefined ? '' : ownershipClaimKey(invocation, stableOwner);
      if (claimKey !== '' && claims.has(claimKey)) throw bridgeError('ownership-conflict');
      const stableInvocation = Object.freeze({ ...invocation });
      pending.set(invocation.invocationId, { invocation: stableInvocation, owner: stableOwner, claimKey });
      if (claimKey !== '') claims.add(claimKey);
      try {
        const response = await config.port.dispatch(stableInvocation);
        if (response.invocationId && response.invocationId !== invocation.invocationId) throw bridgeError('invalid-request');
        if (!response.accepted) throw bridgeError('invalid-request');
        return Object.freeze({ ...response, invocationId: invocation.invocationId, accepted: true });
      } catch (error) {
        removePending(invocation.invocationId);
        throw error;
      }
    },

    acceptResult(result) {
      if (closed) throw bridgeError('bridge-closed');
      if (!validResult(result)) throw bridgeError('invalid-request');
      const current = pending.get(result.invocationId);
      if (!current) throw bridgeError('unknown-invocation');
      const invocation = current.invocation;
      if (result.sessionId !== invocation.sessionId || result.commandId !== invocation.commandId || result.generation !== invocation.generation || result.capabilityId !== invocation.capabilityId || result.ownership !== invocation.ownership || result.occurrenceId !== invocation.occurrenceId || result.eventId !== invocation.eventId || !sameOwner(result.owner, current.owner)) throw bridgeError('invalid-request');
      removePending(result.invocationId);
      const stableResult = Object.freeze({ ...result, owner: cloneOwner(result.owner) });
      for (const listener of listeners) listener(stableResult);
      return Object.freeze({ invocationId: result.invocationId, accepted: true });
    },

    async cancel(request) {
      if (closed) throw bridgeError('bridge-closed');
      if (!validText(request?.sessionId) || !validText(request?.invocationId) || !validGeneration(request?.generation) || !validText(request?.capabilityId) || !validOwner(request.owner)) throw bridgeError('invalid-request');
      const current = pending.get(request.invocationId);
      if (!current) throw bridgeError('unknown-invocation');
      if (current.invocation.sessionId !== request.sessionId || current.invocation.generation !== request.generation || current.invocation.capabilityId !== request.capabilityId || !sameOwner(current.owner, request.owner)) throw bridgeError('invalid-request');
      await cancelRequests([request]);
      if (pending.get(request.invocationId) !== current) throw bridgeError('unknown-invocation');
      removePending(request.invocationId);
      return Object.freeze({ invocationId: request.invocationId, accepted: true });
    },

    async input(input) {
      if (closed) throw bridgeError('bridge-closed');
      if (!validText(input?.sessionId) || !validText(input?.source) || !validText(input?.key) || !validGeneration(input?.generation) || !validOwner(input.owner)) throw bridgeError('invalid-request');
      const state = sessions.get(input.sessionId);
      if (!state) throw bridgeError('unknown-session');
      if (state.locked) throw bridgeError('session-unavailable');
      if (input.generation !== state.session.generation) throw bridgeError('stale-generation');
      const key = `${input.source}\u0000${input.key}`;
      if (input.kind === 'up') {
        state.pressed.delete(key);
        return Object.freeze({ invocationId: input.invocation.invocationId, accepted: false, reason: 'not-a-dispatch-edge' });
      }
      if (input.kind !== 'down' || input.repeat || state.pressed.has(key)) return Object.freeze({ invocationId: input.invocation.invocationId, accepted: false, reason: 'not-a-dispatch-edge' });
      state.pressed.add(key);
      if (input.invocation.sessionId !== input.sessionId || input.invocation.generation !== input.generation) throw bridgeError('invalid-request');
      const occurrenceId = normalizeOccurrence(input.source, input.key);
      const invocation = input.invocation.occurrenceId === undefined ? { ...input.invocation, occurrenceId } : input.invocation;
      if (invocation.occurrenceId !== occurrenceId) throw bridgeError('invalid-request');
      return bridge.invoke(invocation, input.owner);
    },

    async lifecycle(event) {
      if (closed) throw bridgeError('bridge-closed');
      if (!validText(event?.sessionId)) throw bridgeError('invalid-request');
      if (event.kind === 'generation') {
        if (event.generation === undefined) throw bridgeError('invalid-request');
        await bridge.advanceGeneration(event.sessionId, event.generation);
        return;
      }
      const state = sessions.get(event.sessionId);
      if (!state) throw bridgeError('unknown-session');
      if (event.kind === 'blur') {
        if (event.generation !== state.session.generation) throw bridgeError('stale-generation');
        state.pressed.clear();
        return;
      }
      if (event.kind === 'repeat' || event.kind === 'release') {
        if (!event.input) throw bridgeError('invalid-request');
        await bridge.input({ ...event.input, sessionId: event.sessionId, kind: event.kind === 'repeat' ? 'down' : 'up', repeat: event.kind === 'repeat' });
        return;
      }
      if (event.kind !== 'lock' && event.kind !== 'logout') throw bridgeError('invalid-request');
      if (event.generation !== undefined && event.generation !== state.session.generation) throw bridgeError('stale-generation');
      state.locked = true;
      await cancelRequests(invalidateSession(event.sessionId));
    },

    shutdown() {
      if (shutdownPromise) return shutdownPromise;
      closed = true;
      const requests = invalidateAll();
      shutdownPromise = (async () => {
        let firstError: unknown;
        for (;;) {
          const activeResults = await Promise.allSettled([...activeCancellationBatches]);
          for (const result of activeResults) {
            if (result.status === 'rejected') {
              firstError ??= result.reason;
            }
          }
          if (activeCancellationBatches.size === 0) {
            break;
          }
        }
        try {
          await cancelRequests(requests);
        } catch (error) {
          firstError = error;
        }
        listeners.clear();
        sessions.clear();
        capabilities.clear();
        pending.clear();
        claims.clear();
        if (config.port.shutdown) {
          try {
            await config.port.shutdown();
          } catch (error) {
            firstError ??= error;
          }
        }
        if (firstError !== undefined) throw firstError;
      })();
      return shutdownPromise;
    },

    subscribeResult(listener) {
      if (closed) throw bridgeError('bridge-closed');
      if (typeof listener !== 'function') throw bridgeError('invalid-request');
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
  };

  return bridge;
}
