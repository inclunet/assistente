import {
  CommandBridgeError,
  DECISION_REPEAT_TRIGGER,
  DECISION_RESPOND_COMMAND_ID,
  type DialogCommandScope,
  type CommandBridge,
  type CommandBridgeOwner,
  type CommandCancelAck,
  type CommandCancelRequest,
  type CommandInvocation,
  type CommandInvocationAck,
  type CommandLifecycleEvent,
  type CommandResult,
  type CommandResultListener,
  type CommandSession,
} from './commandBridge';
import {
  type OwnedCommandContextFrame,
  type TrustedCommandContextSession,
} from './commandContextSession';
import { isEditableKeyboardTarget } from './decisionMnemonic';

export type CommandKeyboardDispatchResult =
  // Todos os resultados exceto dispatched encerram esta tentativa. ignored
  // preserva o evento nativo, mas nunca autoriza fallback para outro binding.
  | { readonly kind: 'ignored' }
  | { readonly kind: 'dialog-reserved'; readonly scope: DialogCommandScope }
  | { readonly kind: 'blocked' }
  | { readonly kind: 'dispatched'; readonly ack: CommandInvocationAck };

/**
 * Composição entre uma sessão entregue pela borda confiável e os leitores
 * síncronos de contexto da UI. O frame é evidência de UI, nunca autoridade de
 * autorização backend; o bridge/host continua decidindo o dispatch.
 */
export interface AuthenticatedCommandBridge {
  /** Entrada interna síncrona de resolução; não instala observadores de teclado. */
  dispatchLocalKeyboard(
    event: KeyboardEvent,
    resolve: (context: OwnedCommandContextFrame) => CommandInvocation | undefined,
    surfaceID?: string,
  ): Promise<CommandKeyboardDispatchResult>;
  readContext(surfaceID?: string): OwnedCommandContextFrame | undefined;
  invoke(
    invocation: CommandInvocation,
    surfaceID?: string,
  ): Promise<CommandInvocationAck>;
  cancel(request: CommandCancelRequest): Promise<CommandCancelAck>;
  lifecycle(event: CommandLifecycleEvent): Promise<void>;
  subscribeResult(listener: CommandResultListener): () => void;
  dispose(): Promise<void>;
}

export interface AuthenticatedCommandBridgeConfig {
  /** Instância já composta pelo host; não é criada nem inferida pela UI. */
  readonly bridge: CommandBridge;
  /** Leitor vinculado às fontes autenticadas atuais da sessão frontend. */
  readonly context: TrustedCommandContextSession;
  /** Sessão e owner recebidos da borda confiável. */
  readonly session: CommandSession;
  /** O dispose encerra o bridge inteiro; compartilhamento não é permitido. */
  readonly ownership: 'exclusive';
}

function validText(value: unknown): value is string {
  return typeof value === 'string' && value.length > 0 && value.trim() === value;
}

function validGeneration(value: unknown): value is string {
  return typeof value === 'string' && /^[1-9]\d*$/.test(value);
}

function validOwner(owner: CommandBridgeOwner): boolean {
  return (
    validText(owner?.userId) &&
    validText(owner?.sessionId) &&
    validText(owner?.workspaceId)
  );
}

function validSession(session: CommandSession): boolean {
  return (
    validText(session?.id) &&
    validGeneration(session?.generation) &&
    validOwner(session.owner) &&
    session.owner.sessionId === session.id
  );
}

function cloneSession(session: CommandSession): CommandSession {
  return Object.freeze({
    id: session.id,
    generation: session.generation,
    owner: Object.freeze({ ...session.owner }),
  });
}

function sameOwner(left: CommandBridgeOwner, right: CommandBridgeOwner): boolean {
  return (
    left.userId === right.userId &&
    left.sessionId === right.sessionId &&
    left.workspaceId === right.workspaceId
  );
}

function isNewerGeneration(next: string, current: string): boolean {
  return BigInt(next) > BigInt(current);
}

function closedError(): CommandBridgeError {
  return new CommandBridgeError('bridge-closed');
}

function hasMatchingDialogProof(invocation: CommandInvocation, scope: DialogCommandScope | null): boolean {
  const proof = invocation.dialogProof;
  return !!scope &&
    !!proof &&
    invocation.commandId === DECISION_RESPOND_COMMAND_ID &&
    invocation.ownership === 'local' &&
    invocation.source === 'keyboard.local' &&
    proof.dialogId === scope.dialogId &&
    proof.kind === scope.kind &&
    proof.scopeGeneration === scope.generation &&
    proof.commandId === DECISION_RESPOND_COMMAND_ID &&
    proof.triggerSpec === DECISION_REPEAT_TRIGGER &&
    scope.allowedCommandIds.includes(DECISION_RESPOND_COMMAND_ID) &&
    scope.allowedTriggerSpecs.includes(DECISION_REPEAT_TRIGGER);
}

// Providers devolvem JSON destacado/congelado. Compare também o conteúdo:
// snapshotVersion sozinho não detecta um provider que reutilize sua versão.
function sameContextValue(left: unknown, right: unknown): boolean {
  if (left === right) return true;
  if (left === null || right === null || typeof left !== 'object' || typeof right !== 'object') return false;
  if (Array.isArray(left) !== Array.isArray(right)) return false;
  const a = left as Record<string, unknown>;
  const b = right as Record<string, unknown>;
  const keys = Object.keys(a);
  return keys.length === Object.keys(b).length && keys.every((key) =>
    Object.prototype.hasOwnProperty.call(b, key) && sameContextValue(a[key], b[key]));
}

/**
 * Binda uma sessão autenticada do host aos providers reais da UI.
 *
 * A sessão não é lida de Zustand nem de payload de contexto: chega como
 * argumento do host. Cada leitura reconsulta owner, Modal/foco e surface;
 * logout/troca de usuário, surface ausente/stale ou geração antiga falham
 * fechado antes de alcançar a porta de dispatch.
 */
export function createAuthenticatedCommandBridge(
  config: AuthenticatedCommandBridgeConfig,
): AuthenticatedCommandBridge {
  if (
    !config?.bridge ||
    typeof config.bridge.invoke !== 'function' ||
    typeof config.bridge.cancel !== 'function' ||
    typeof config.bridge.lifecycle !== 'function' ||
    typeof config.bridge.shutdown !== 'function' ||
    !config.context ||
    typeof config.context.readOwnedCommandContextFrame !== 'function' ||
    typeof config.context.dispose !== 'function' ||
    config.ownership !== 'exclusive' ||
    !validSession(config.session)
  ) {
    throw new CommandBridgeError('invalid-configuration');
  }

  // Capture as referências na construção. O chamador não pode trocar a porta
  // ou o reader depois de publicar a composição e redirecionar uma chamada.
  const bridge = config.bridge;
  const context = config.context;
  let session = cloneSession(config.session);
  let disposed = false;
  let disposePromise: Promise<void> | undefined;

  const readContext = (surfaceID?: string): OwnedCommandContextFrame | undefined => {
    if (disposed) return undefined;
    const owned = context.readOwnedCommandContextFrame(surfaceID);
    if (!owned || !sameOwner(session.owner, owned.owner)) return undefined;
    if (surfaceID !== undefined && owned.frame.surface === null) return undefined;
    return owned;
  };

  const requireContext = (surfaceID?: string): OwnedCommandContextFrame => {
    const owned = readContext(surfaceID);
    if (!owned) throw new CommandBridgeError('session-unavailable');
    return owned;
  };

  const invokeInContext = async (
    invocation: CommandInvocation,
    owned: OwnedCommandContextFrame,
  ): Promise<CommandInvocationAck> => {
    if (disposed) throw closedError();
    if (
      invocation.sessionId !== session.id ||
      invocation.generation !== session.generation ||
      owned.owner.sessionId !== session.id
    ) {
      throw new CommandBridgeError('stale-generation');
    }
    if (owned.frame.modal.topID !== null) {
      if (!hasMatchingDialogProof(invocation, owned.frame.modal.dialogCommandScope)) {
        return { invocationId: invocation.invocationId, accepted: false, reason: 'dialog-blocked' };
      }
    }
    return bridge.invoke(invocation, owned.owner);
  };

  const invoke = async (
    invocation: CommandInvocation,
    surfaceID?: string,
  ): Promise<CommandInvocationAck> => {
    if (disposed) throw closedError();
    return invokeInContext(invocation, requireContext(surfaceID));
  };

  const dispatchLocalKeyboard = async (
    event: KeyboardEvent,
    resolve: (context: OwnedCommandContextFrame) => CommandInvocation | undefined,
    surfaceID?: string,
  ): Promise<CommandKeyboardDispatchResult> => {
    if (disposed) throw closedError();
    const owned = requireContext(surfaceID);
    const guarded = () => event.type !== 'keydown' || event.defaultPrevented ||
      event.repeat || event.isComposing || event.keyCode === 229 ||
      isEditableKeyboardTarget(event.target) ||
      isEditableKeyboardTarget(document.activeElement);
    if (guarded()) return { kind: 'ignored' };
    const modal = owned.frame.modal;
    if (modal.topID !== null) {
      const scope = modal.dialogCommandScope;
      if (scope && event.ctrlKey && event.shiftKey && !event.altKey &&
        !event.metaKey && event.key.toLowerCase() === 'r') {
        // Reserva antes de qualquer binding/ownership global. Não consome o
        // evento: o handler existente do DecisionDialog faz o mesmo anúncio.
        return { kind: 'dialog-reserved', scope };
      }
      return { kind: 'blocked' };
    }
    const candidate = resolve(owned);
    if (!candidate) return { kind: 'ignored' };
    const current = requireContext(surfaceID);
    if (disposed) throw closedError();
    // capturedAt do frame muda a cada leitura e não é uma versão. O foco não
    // tem generation própria: compare todos os fatos, inclusive capacidades
    // do mesmo elemento. A surface inclui snapshotVersion e freshness.
    if (guarded() || current.frame.version !== owned.frame.version ||
      current.frame.modal.generation !== modal.generation ||
      !sameContextValue(current.frame.focus, owned.frame.focus) ||
      !sameContextValue(current.frame.surface, owned.frame.surface)) {
      return { kind: 'blocked' };
    }
    if (candidate.source !== 'keyboard.local' || candidate.ownership !== 'local') {
      throw new CommandBridgeError('invalid-request');
    }
    // Sem terceira leitura de getters após comparar: o handoff usa exatamente
    // o frame revalidado e passa pelas mesmas checagens de invoke.
    return { kind: 'dispatched', ack: await invokeInContext(candidate, current) };
  };

  const lifecycle = async (event: CommandLifecycleEvent): Promise<void> => {
    if (disposed) throw closedError();
    if (event.sessionId !== session.id) throw new CommandBridgeError('invalid-request');
    await bridge.lifecycle(event);
    if (
      !disposed &&
      event.kind === 'generation' &&
      event.generation !== undefined &&
      isNewerGeneration(event.generation, session.generation)
    ) {
      session = Object.freeze({ ...session, generation: event.generation });
    }
  };

  const cancel = async (request: CommandCancelRequest): Promise<CommandCancelAck> => {
    if (disposed) return Promise.reject(closedError());
    const owned = requireContext();
    if (
      request.sessionId !== session.id ||
      request.generation !== session.generation ||
      !sameOwner(request.owner, owned.owner)
    ) {
      return Promise.reject(new CommandBridgeError('stale-generation'));
    }
    return bridge.cancel(request);
  };

  const subscribeResult = (listener: CommandResultListener): (() => void) => {
    if (disposed) throw closedError();
    const filter = (result: Readonly<CommandResult>) => {
      if (disposed) return;
      const owned = readContext();
      if (
        !owned ||
        result.sessionId !== session.id ||
        result.generation !== session.generation ||
        !sameOwner(result.owner, session.owner) ||
        !sameOwner(result.owner, owned.owner)
      ) {
        return;
      }
      listener(result);
    };
    return bridge.subscribeResult(filter);
  };

  const dispose = (): Promise<void> => {
    if (disposePromise) return disposePromise;
    disposed = true;
    context.dispose();
    disposePromise = bridge.shutdown();
    return disposePromise;
  };

  return Object.freeze({
    dispatchLocalKeyboard,
    readContext,
    invoke,
    cancel,
    lifecycle,
    subscribeResult,
    dispose,
  });
}
