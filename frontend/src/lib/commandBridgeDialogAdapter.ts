import {
  CommandBridgeError,
  type CommandBridge,
  type CommandBridgeOwner,
  type DialogCommandScope,
  type CommandLifecycleEvent,
} from './commandBridge';
import type { QuestionnairePayload } from '../components/ui/QuestionnaireDialog';
import {
  createDecisionQuestionnaireScope,
  isValidDecisionQuestionnairePayload,
} from './decisionQuestionnaireScope';
import {
  useQuestionnaireUIStore,
  type QuestionnaireUIResult,
} from '../store/questionnaireUIStore';

/** Contexto de ownership que o host deve fornecer; o adapter não o infere. */
export interface CommandDialogRequest {
  readonly owner: CommandBridgeOwner;
  readonly sessionId: string;
  readonly generation: string;
  readonly payload: QuestionnairePayload;
}

/** Porta mínima da stack de diálogos existente. */
export interface CommandDialogUI {
  request(payload: QuestionnairePayload, scope?: DialogCommandScope): Promise<QuestionnaireUIResult>;
  /** Cancela somente o payload indicado, ativo ou enfileirado nessa stack. */
  cancelById(payloadId: string): boolean;
}

/**
 * Adapter concreto da stack já montada por DecisionQuestionnaireHost.
 *
 * A store remove por id exato tanto o item ativo quanto o enfileirado; nunca
 * usa o modal atualmente visível como fallback de ownership.
 */
export const questionnaireCommandDialogUI: CommandDialogUI = {
  request: (payload, scope) => useQuestionnaireUIStore.getState().request(payload, scope),
  cancelById: (payloadId) => useQuestionnaireUIStore.getState().cancelById(payloadId),
};

export interface CommandBridgeDialogAdapter {
  present(request: CommandDialogRequest): Promise<QuestionnaireUIResult>;
  lifecycle(event: CommandLifecycleEvent): Promise<void>;
  shutdown(): Promise<void>;
}

interface PendingDialog {
  readonly request: CommandDialogRequest;
  readonly payload: QuestionnairePayload;
  readonly resolve: (result: QuestionnaireUIResult) => void;
  readonly reject: (error: unknown) => void;
  settled: boolean;
}

function validText(value: unknown): value is string {
  return typeof value === 'string' && value.length > 0 && value.trim() === value;
}

function validWorkspaceId(value: unknown): value is string {
  // A string vazia representa workspace_id nulo no escopo global; espaços não
  // são uma representação válida de nenhum escopo.
  return typeof value === 'string' && value.trim() === value;
}

function validGeneration(value: unknown): value is string {
  return typeof value === 'string' && /^[1-9]\d*$/.test(value);
}

function validOwner(owner: unknown): owner is CommandBridgeOwner {
  if (!owner || typeof owner !== 'object') return false;
  const value = owner as Partial<CommandBridgeOwner>;
  return validText(value.userId) && validText(value.sessionId) && validWorkspaceId(value.workspaceId);
}

function validRequest(request: CommandDialogRequest): boolean {
  if (!request || typeof request !== 'object') return false;
  if (!validText(request.sessionId) || !validGeneration(request.generation)) return false;
  if (!validOwner(request.owner) || request.owner.sessionId !== request.sessionId) return false;
  if (!request.payload || typeof request.payload !== 'object') return false;
  return isValidDecisionQuestionnairePayload(request.payload, request.payload.id);
}

function cloneAndFreeze<T>(value: T, seen = new WeakMap<object, unknown>()): T {
  if (!value || typeof value !== 'object') return value;
  const objectValue = value as object;
  const existing = seen.get(objectValue);
  if (existing) return existing as T;

  const clone = (Array.isArray(value) ? [] : Object.create(null)) as Record<string, unknown> | unknown[];
  seen.set(objectValue, clone);
  for (const key of Object.keys(value as Record<string, unknown>)) {
    Object.defineProperty(clone, key, {
      configurable: true,
      enumerable: true,
      value: cloneAndFreeze((value as Record<string, unknown>)[key], seen),
      writable: true,
    });
  }
  return Object.freeze(clone) as T;
}

function cancelledResult(): QuestionnaireUIResult {
  return { answers: {}, cancelled: true };
}

function shouldCancelForLifecycle(
  pending: PendingDialog,
  event: CommandLifecycleEvent,
): boolean {
  if (event.sessionId !== pending.request.sessionId) return false;
  switch (event.kind) {
    case 'lock':
    case 'logout':
      return true;
    case 'generation':
      return event.generation !== undefined && event.generation !== pending.request.generation;
    case 'repeat':
    case 'release':
    case 'blur':
      return false;
  }
}

/**
 * Liga decisões de comando ao lifecycle do bridge sem criar um segundo host
 * visual. O bridge continua sendo a autoridade de sessão e transporte.
 */
export function createCommandBridgeDialogAdapter(
  bridge: Pick<CommandBridge, 'lifecycle' | 'shutdown'>,
  ui: CommandDialogUI = questionnaireCommandDialogUI,
): CommandBridgeDialogAdapter {
  const pending = new Map<string, PendingDialog>();
  let closed = false;
  let shutdownPromise: Promise<void> | undefined;

  const cancelPending = (entry: PendingDialog) => {
    if (entry.settled) return;
    entry.settled = true;
    pending.delete(entry.payload.id);
    // O resultado local é encerrado mesmo quando o pedido ainda está atrás de
    // outro diálogo. A store remove o item pelo id, sem cancelar terceiros.
    entry.resolve(cancelledResult());
    try {
      ui.cancelById(entry.payload.id);
    } catch {
      // O resultado local já foi fechado; um erro de cancelamento de uma porta
      // não pode impedir a drenagem dos demais pedidos do lifecycle.
    }
  };

  const present = (request: CommandDialogRequest): Promise<QuestionnaireUIResult> => {
    if (closed) return Promise.reject(new CommandBridgeError('bridge-closed'));
    if (!validRequest(request)) return Promise.reject(new CommandBridgeError('invalid-request'));
    if (pending.has(request.payload.id)) {
      return Promise.reject(new CommandBridgeError('invalid-request'));
    }

    const stableRequest = cloneAndFreeze(request);
    const payload = stableRequest.payload;
    const scope = createDecisionQuestionnaireScope(payload, payload.id);
    if (!scope) return Promise.reject(new CommandBridgeError('invalid-request'));
    return new Promise<QuestionnaireUIResult>((resolve, reject) => {
      const entry: PendingDialog = {
        request: stableRequest,
        payload,
        resolve,
        reject,
        settled: false,
      };
      pending.set(payload.id, entry);

      let uiResult: Promise<QuestionnaireUIResult>;
      try {
        uiResult = ui.request(payload, scope);
      } catch (error) {
        pending.delete(payload.id);
        entry.settled = true;
        reject(error);
        return;
      }

      void uiResult.then(
        (result) => {
          if (entry.settled) return;
          entry.settled = true;
          pending.delete(payload.id);
          resolve(result);
        },
        (error) => {
          if (entry.settled) return;
          entry.settled = true;
          pending.delete(payload.id);
          reject(error);
        },
      );
    });
  };

  const lifecycle = async (event: CommandLifecycleEvent): Promise<void> => {
    // Invalida a UI imediatamente; o callback do bridge pode aguardar uma
    // porta de adapter e não deve manter um diálogo revogado visível.
    for (const entry of [...pending.values()]) {
      if (shouldCancelForLifecycle(entry, event)) cancelPending(entry);
    }

    let bridgeError: unknown;
    try {
      await bridge.lifecycle(event);
    } catch (error) {
      bridgeError = error;
    }

    if (bridgeError !== undefined) throw bridgeError;
  };

  const shutdown = (): Promise<void> => {
    if (shutdownPromise) return shutdownPromise;
    closed = true;
    for (const entry of [...pending.values()]) cancelPending(entry);
    shutdownPromise = bridge.shutdown();
    return shutdownPromise;
  };

  return { present, lifecycle, shutdown };
}
