import type { SurfaceContext } from '../lib/chatSurface';

export const EXTERNAL_UI_CONNECTION_STATUS_EVENT = 'external:ui-connection:status';
export const EXTERNAL_COMMAND_READY_EVENT = 'external:command:ready';

export interface ExternalUIOwnerProof {
  readonly userId: string;
  readonly sessionId: string;
  readonly workspaceId: string;
}

export interface ExternalUIInvitation {
  readonly invitation: string;
  readonly expiresAt: string;
}

export interface ExternalUIDestination {
  readonly workspaceId: string;
  readonly tabId?: string;
  readonly surface: SurfaceContext;
}

export interface ExternalUIConnectionStatus {
  readonly state: 'disconnected' | 'waiting_claim' | 'connected';
  readonly connectionId?: string;
  readonly generation?: string;
  readonly owner: ExternalUIOwnerProof;
  /** O DTO Go mantém Target como struct zero quando desconectado; normalizamos para null. */
  readonly target: ExternalUIDestination | null;
  readonly targetSnapshotId?: string;
  readonly contextVersion?: string;
  readonly expiresAt?: string;
}

export interface ExternalUIContextPublication {
  readonly owner: ExternalUIOwnerProof;
  readonly connectionId: string;
  readonly generation: string;
  readonly expectedTargetSnapshotId: string;
  readonly expectedContextVersion: string;
  readonly target: ExternalUIDestination;
}

export interface ExternalUILease {
  readonly owner: ExternalUIOwnerProof;
  readonly connectionId: string;
  readonly generation: string;
}

export interface ExternalUICommandReadyEvent {
  readonly connectionId: string;
  readonly generation: string;
  readonly invocationId: string;
  readonly targetSnapshotId: string;
  readonly contextVersion: string;
  readonly commandId: string;
}

export interface TakeExternalUICommandRequest {
  readonly owner: ExternalUIOwnerProof;
  readonly connectionId: string;
  readonly generation: string;
  readonly invocationId: string;
  readonly targetSnapshotId: string;
  readonly contextVersion: string;
}

export interface TakeExternalUICommandResult {
  readonly invocationId: string;
  readonly commandId: string;
  readonly arguments: Readonly<Record<string, unknown>>;
  readonly receiptId: string;
  readonly targetSnapshotId: string;
  readonly contextVersion: string;
  readonly target: ExternalUIDestination;
}

export type ExternalUICommandOutcome = 'succeeded' | 'failed' | 'cancelled' | 'unknown';

export interface CompleteExternalUICommandRequest {
  readonly owner: ExternalUIOwnerProof;
  readonly connectionId: string;
  readonly generation: string;
  readonly invocationId: string;
  readonly receiptId: string;
  readonly targetSnapshotId: string;
  readonly contextVersion: string;
  readonly outcome: ExternalUICommandOutcome;
}

export interface CompleteExternalUICommandResult {
  readonly accepted: boolean;
}

/** Contrato Wails manual para injeção/testes; a integração aguarda bindings gerados. */
export interface ExternalUIConnectionPort {
  ReadExternalUIConnection(): Promise<unknown>;
  BeginExternalUIConnection(target: ExternalUIDestination): Promise<unknown>;
  PublishExternalUIContext(request: ExternalUIContextPublication): Promise<unknown>;
  HeartbeatExternalUIConnection(lease: ExternalUILease): Promise<unknown>;
  DisconnectExternalUIConnection(lease: ExternalUILease): Promise<void>;
  TakeExternalUICommand(request: TakeExternalUICommandRequest): Promise<unknown>;
  CompleteExternalUICommand(request: CompleteExternalUICommandRequest): Promise<unknown>;
}

export interface ExternalUIConnectionService {
  refresh(): Promise<ExternalUIConnectionStatus | null>;
  begin(target: ExternalUIDestination): Promise<ExternalUIInvitation>;
  publishContext(target: ExternalUIDestination): Promise<ExternalUIConnectionStatus>;
  heartbeat(): Promise<ExternalUIConnectionStatus>;
  disconnect(): Promise<void>;
  take(event: ExternalUICommandReadyEvent): Promise<TakeExternalUICommandResult>;
  complete(event: ExternalUICommandReadyEvent, take: TakeExternalUICommandResult, outcome: ExternalUICommandOutcome): Promise<boolean>;
  getSnapshot(): ExternalUIConnectionStatus | null;
  subscribe(listener: () => void): () => void;
  subscribeReady(listener: (event: ExternalUICommandReadyEvent) => void): () => void;
  dispose(): void;
}

export type ExternalUIEventsOn = (eventName: string, listener: (payload: unknown) => void) => () => void;

function validText(value: unknown, maxLength = 4096): value is string {
  return typeof value === 'string' && value.length > 0 && value.length <= maxLength && value.trim() === value;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false;
  const prototype = Object.getPrototypeOf(value);
  return prototype === Object.prototype || prototype === null;
}

function validOwner(value: unknown): value is ExternalUIOwnerProof {
  return isRecord(value) && validText(value.userId) && validText(value.sessionId) && validText(value.workspaceId);
}

function validSurface(value: unknown): value is SurfaceContext {
  return isRecord(value) && validText(value.surfaceType) && validText(value.surfaceId) && validText(value.snapshotVersion);
}

function validDestination(value: unknown): value is ExternalUIDestination {
  return isRecord(value) && validText(value.workspaceId) &&
    (value.tabId === undefined || validText(value.tabId)) && validSurface(value.surface);
}

function isZeroDestination(value: unknown): boolean {
  if (!isRecord(value) || value.workspaceId !== '' || value.tabId !== undefined || !isRecord(value.surface)) return false;
  return value.surface.surfaceType === '' && value.surface.surfaceId === '' && value.surface.snapshotVersion === '' &&
    Object.keys(value.surface).every(key => ['surfaceType', 'surfaceId', 'snapshotVersion'].includes(key));
}

function validTimestamp(value: unknown): value is string {
  return validText(value, 128) && Number.isFinite(Date.parse(value));
}

function exactKeys(record: Record<string, unknown>, expected: readonly string[]): boolean {
  const keys = Object.keys(record).sort();
  const sortedExpected = [...expected].sort();
  return keys.length === sortedExpected.length && keys.every((key, index) => key === sortedExpected[index]);
}

/** Valida o DTO manual recebido por evento/read sem confiar no type assertion do runtime. */
export function parseExternalUIConnectionStatus(value: unknown): ExternalUIConnectionStatus | undefined {
  if (!isRecord(value)) return undefined;
  const allowed = ['state', 'connectionId', 'generation', 'owner', 'target', 'targetSnapshotId', 'contextVersion', 'expiresAt'];
  if (Object.keys(value).some(key => !allowed.includes(key))) return undefined;
  if (!['disconnected', 'waiting_claim', 'connected'].includes(value.state as string) ||
      !validOwner(value.owner) || !isRecord(value.target)) return undefined;
  if (value.connectionId !== undefined && !validText(value.connectionId)) return undefined;
  if (value.generation !== undefined && !validText(value.generation)) return undefined;
  if (value.targetSnapshotId !== undefined && !validText(value.targetSnapshotId)) return undefined;
  if (value.contextVersion !== undefined && !validText(value.contextVersion)) return undefined;
  if (value.expiresAt !== undefined && !validTimestamp(value.expiresAt)) return undefined;
  const disconnected = value.state === 'disconnected';
  if (disconnected ? !isZeroDestination(value.target) : !validDestination(value.target)) return undefined;
  if (value.state === 'connected' && (!validText(value.connectionId) || !validText(value.generation) ||
      !validText(value.targetSnapshotId) || !validText(value.contextVersion) || !validTimestamp(value.expiresAt))) return undefined;
  if (value.state === 'waiting_claim' && (!validTimestamp(value.expiresAt) || value.connectionId !== undefined || value.generation !== undefined)) return undefined;
  const rawTarget = value.target as Record<string, unknown>;
  const rawSurface = rawTarget.surface as Record<string, unknown>;
  const target: ExternalUIDestination | null = disconnected ? null : Object.freeze({
    workspaceId: rawTarget.workspaceId as string,
    ...(rawTarget.tabId !== undefined ? { tabId: rawTarget.tabId as string } : {}),
    surface: Object.freeze({ ...rawSurface }) as SurfaceContext,
  });
  return Object.freeze({
    state: value.state as ExternalUIConnectionStatus['state'],
    ...(value.connectionId !== undefined ? { connectionId: value.connectionId as string } : {}),
    ...(value.generation !== undefined ? { generation: value.generation as string } : {}),
    owner: Object.freeze({ userId: value.owner.userId, sessionId: value.owner.sessionId, workspaceId: value.owner.workspaceId }),
    target,
    ...(value.targetSnapshotId !== undefined ? { targetSnapshotId: value.targetSnapshotId as string } : {}),
    ...(value.contextVersion !== undefined ? { contextVersion: value.contextVersion as string } : {}),
    ...(value.expiresAt !== undefined ? { expiresAt: value.expiresAt as string } : {}),
  });
}

export function parseExternalUIInvitation(value: unknown): ExternalUIInvitation | undefined {
  if (!isRecord(value) || !exactKeys(value, ['invitation', 'expiresAt']) ||
      !validText(value.invitation) || !validTimestamp(value.expiresAt)) return undefined;
  return Object.freeze({ invitation: value.invitation, expiresAt: value.expiresAt });
}

/** O ready event deliberadamente rejeita qualquer campo extra, inclusive arguments/rawArgs. */
export function parseExternalUICommandReadyEvent(value: unknown): ExternalUICommandReadyEvent | undefined {
  if (!isRecord(value) || !exactKeys(value, [
    'connectionId', 'generation', 'invocationId', 'targetSnapshotId', 'contextVersion', 'commandId',
  ])) return undefined;
  if (!validText(value.connectionId) || !validText(value.generation) || !validText(value.invocationId) ||
      !/^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(value.invocationId) ||
      !validText(value.targetSnapshotId) || !validText(value.contextVersion) || !validText(value.commandId) ||
      !/^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$/.test(value.commandId)) return undefined;
  return Object.freeze({
    connectionId: value.connectionId,
    generation: value.generation,
    invocationId: value.invocationId,
    targetSnapshotId: value.targetSnapshotId,
    contextVersion: value.contextVersion,
    commandId: value.commandId,
  });
}

export function subscribeExternalUIConnectionStatus(
  eventsOn: ExternalUIEventsOn,
  listener: (status: ExternalUIConnectionStatus) => void,
): () => void {
  return eventsOn(EXTERNAL_UI_CONNECTION_STATUS_EVENT, raw => {
    const status = parseExternalUIConnectionStatus(raw);
    if (status) listener(status);
  });
}

export function subscribeExternalUICommandReady(
  eventsOn: ExternalUIEventsOn,
  connection: { readonly connectionId: string; readonly generation: string },
  onReady: (event: ExternalUICommandReadyEvent) => void,
): () => void {
  if (!validText(connection.connectionId) || !validText(connection.generation) || typeof onReady !== 'function') {
    throw new TypeError('external-ui-command-subscription-invalid');
  }
  return eventsOn(EXTERNAL_COMMAND_READY_EVENT, raw => {
    const event = parseExternalUICommandReadyEvent(raw);
    if (!event || event.connectionId !== connection.connectionId || event.generation !== connection.generation) return;
    onReady(event);
  });
}

export interface ExternalUIEvents {
  readonly on: ExternalUIEventsOn;
}

function sameOwner(left: ExternalUIOwnerProof, right: ExternalUIOwnerProof | null): boolean {
  return !!right && left.userId === right.userId && left.sessionId === right.sessionId && left.workspaceId === right.workspaceId;
}

function validTakeResult(value: unknown, event: ExternalUICommandReadyEvent): value is TakeExternalUICommandResult {
  if (!isRecord(value) || !exactKeys(value, [
    'invocationId', 'commandId', 'arguments', 'receiptId', 'targetSnapshotId', 'contextVersion', 'target',
  ])) return false;
  return value.invocationId === event.invocationId && value.commandId === event.commandId &&
    validText(value.receiptId) && value.targetSnapshotId === event.targetSnapshotId &&
    value.contextVersion === event.contextVersion && validDestination(value.target) && isRecord(value.arguments);
}

/**
 * Client puro em torno da porta tipada. A produção injeta o adapter Wails
 * somente após a geração oficial; owner/context/eventos permanecem explícitos.
 */
export function createExternalUIConnectionService(
  port: ExternalUIConnectionPort,
  ownerReader: () => ExternalUIOwnerProof | null,
  events: ExternalUIEvents,
): ExternalUIConnectionService {
  let snapshot: ExternalUIConnectionStatus | null = null;
  let disposed = false;
  let statusOperations = Promise.resolve();
  const listeners = new Set<() => void>();
  const readyListeners = new Set<(event: ExternalUICommandReadyEvent) => void>();
  const serializeStatusOperation = <T,>(operation: () => Promise<T>): Promise<T> => {
    const result = statusOperations.then(operation);
    statusOperations = result.then(() => undefined, () => undefined);
    return result;
  };
  const setSnapshot = (next: ExternalUIConnectionStatus | null) => {
    if (disposed) return;
    snapshot = next;
    for (const listener of listeners) {
      try { listener(); } catch { /* Um consumidor não pode interromper a publicação aos demais. */ }
    }
  };
  const currentOwner = (): ExternalUIOwnerProof => {
    const owner = ownerReader();
    if (!owner || !validOwner(owner)) throw new Error('external-ui-owner-unavailable');
    return owner;
  };
  const currentConnection = (owner = currentOwner()): ExternalUIConnectionStatus => {
    const current = snapshot;
    if (!current || current.state !== 'connected' || !current.connectionId || !current.generation || !sameOwner(current.owner, owner)) {
      throw new Error('external-ui-connection-not-current');
    }
    return current;
  };
  const commitStatus = (raw: unknown, owner = currentOwner()): ExternalUIConnectionStatus => {
    if (!sameOwner(owner, ownerReader())) throw new Error('external-ui-owner-changed');
    const parsed = parseExternalUIConnectionStatus(raw);
    if (!parsed || !sameOwner(parsed.owner, owner)) throw new Error('external-ui-status-invalid-or-owner-mismatch');
    setSnapshot(parsed);
    return parsed;
  };
  const readAuthoritativeStatus = async (owner: ExternalUIOwnerProof): Promise<ExternalUIConnectionStatus> =>
    commitStatus(await port.ReadExternalUIConnection(), owner);
  let refreshStatus: (() => Promise<ExternalUIConnectionStatus | null>) | undefined;

  const unsubscribeStatus = events.on(EXTERNAL_UI_CONNECTION_STATUS_EVENT, raw => {
    if (disposed) return;
    const parsed = parseExternalUIConnectionStatus(raw);
    const owner = ownerReader();
    if (!parsed || !sameOwner(parsed.owner, owner)) return;
    // Eventos não carregam uma revisão monotônica. Use-os apenas como aviso
    // para reler o estado autoritativo; um evento atrasado nunca regride o cache.
    void refreshStatus?.().catch(() => undefined);
  });
  const unsubscribeReady = events.on(EXTERNAL_COMMAND_READY_EVENT, raw => {
    if (disposed) return;
    const event = parseExternalUICommandReadyEvent(raw);
    const current = snapshot;
    if (!event || !current || current.state !== 'connected' ||
        event.connectionId !== current.connectionId || event.generation !== current.generation ||
        event.targetSnapshotId !== current.targetSnapshotId || event.contextVersion !== current.contextVersion ||
        !sameOwner(current.owner, ownerReader())) return;
    for (const listener of readyListeners) listener(event);
  });

  const service: ExternalUIConnectionService = {
    refresh() {
      return serializeStatusOperation(async () => {
        if (disposed) return null;
        return readAuthoritativeStatus(currentOwner());
      });
    },
    begin(target) {
      return serializeStatusOperation(async () => {
        if (disposed || !validDestination(target)) throw new Error('external-ui-target-unavailable');
        const owner = currentOwner(); // Wails deriva/revalida a sessão; nenhum JWT é passado.
        const invitation = parseExternalUIInvitation(await port.BeginExternalUIConnection(target));
        if (!invitation || Date.parse(invitation.expiresAt) <= Date.now()) throw new Error('external-ui-invitation-invalid');
        // Begin só devolve o convite; reler é o que inicia o polling de waiting_claim.
        // Se a leitura falhar, o convite ainda é mostrado e a UI tenta reler enquanto ele existe.
        try { await readAuthoritativeStatus(owner); } catch { /* leitura será repetida enquanto o convite estiver visível */ }
        if (!sameOwner(owner, ownerReader())) throw new Error('external-ui-owner-changed');
        return invitation;
      });
    },
    publishContext(target) {
      return serializeStatusOperation(async () => {
        if (disposed || !validDestination(target)) throw new Error('external-ui-target-unavailable');
        const owner = currentOwner();
        const current = currentConnection(owner);
        const next = await port.PublishExternalUIContext({
          owner,
          connectionId: current.connectionId!,
          generation: current.generation!,
          expectedTargetSnapshotId: current.targetSnapshotId!,
          expectedContextVersion: current.contextVersion!,
          target,
        });
        return commitStatus(next, owner);
      });
    },
    heartbeat() {
      return serializeStatusOperation(async () => {
        if (disposed) throw new Error('external-ui-service-disposed');
        const owner = currentOwner();
        const current = currentConnection(owner);
        const next = await port.HeartbeatExternalUIConnection({ owner, connectionId: current.connectionId!, generation: current.generation! });
        return commitStatus(next, owner);
      });
    },
    disconnect() {
      return serializeStatusOperation(async () => {
        if (disposed) return;
        const owner = currentOwner();
        const current = currentConnection(owner);
        await port.DisconnectExternalUIConnection({ owner, connectionId: current.connectionId!, generation: current.generation! });
        const latest = snapshot;
        if (sameOwner(owner, ownerReader()) && latest?.connectionId === current.connectionId &&
            latest?.generation === current.generation) {
          setSnapshot(Object.freeze({ state: 'disconnected', owner, target: null }));
        }
      });
    },
    async take(event) {
      if (disposed) throw new Error('external-ui-service-disposed');
      const owner = currentOwner();
      const current = currentConnection(owner);
      if (event.connectionId !== current.connectionId || event.generation !== current.generation ||
          event.targetSnapshotId !== current.targetSnapshotId || event.contextVersion !== current.contextVersion) {
        throw new Error('external-ui-command-stale');
      }
      const result = await port.TakeExternalUICommand({
        owner,
        connectionId: current.connectionId!,
        generation: current.generation!,
        invocationId: event.invocationId,
        targetSnapshotId: event.targetSnapshotId,
        contextVersion: event.contextVersion,
      });
      if (!validTakeResult(result, event)) throw new Error('external-ui-take-invalid');
      const after = currentConnection(currentOwner());
      if (!sameOwner(owner, ownerReader()) || !sameOwner(after.owner, owner) ||
          after.connectionId !== current.connectionId || after.generation !== current.generation ||
          after.targetSnapshotId !== current.targetSnapshotId || after.contextVersion !== current.contextVersion) {
        throw new Error('external-ui-command-stale');
      }
      return result;
    },
    async complete(event, take, outcome) {
      if (disposed) return false;
      const owner = currentOwner();
      if (take.invocationId !== event.invocationId || take.receiptId.length === 0 ||
          take.targetSnapshotId !== event.targetSnapshotId || take.contextVersion !== event.contextVersion) return false;
      const result = await port.CompleteExternalUICommand({
        owner,
        connectionId: event.connectionId,
        generation: event.generation,
        invocationId: event.invocationId,
        receiptId: take.receiptId,
        targetSnapshotId: event.targetSnapshotId,
        contextVersion: event.contextVersion,
        outcome,
      });
      return isRecord(result) && result.accepted === true;
    },
    getSnapshot: () => snapshot,
    subscribe(listener) {
      if (disposed) return () => undefined;
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    subscribeReady(listener) {
      if (disposed) return () => undefined;
      readyListeners.add(listener);
      return () => readyListeners.delete(listener);
    },
    dispose() {
      if (disposed) return;
      disposed = true;
      snapshot = null;
      listeners.clear();
      readyListeners.clear();
      unsubscribeStatus();
      unsubscribeReady();
    },
  };
  refreshStatus = service.refresh;
  return Object.freeze(service);
}
