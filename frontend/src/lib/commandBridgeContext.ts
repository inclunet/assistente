import {
  CommandBridgeError,
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

/**
 * Composição entre uma sessão entregue pela borda confiável e os leitores
 * síncronos de contexto da UI. O frame é evidência de UI, nunca autoridade de
 * autorização backend; o bridge/host continua decidindo o dispatch.
 */
export interface AuthenticatedCommandBridge {
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

  const invoke = async (
    invocation: CommandInvocation,
    surfaceID?: string,
  ): Promise<CommandInvocationAck> => {
    if (disposed) throw closedError();
    const owned = requireContext(surfaceID);
    if (
      invocation.sessionId !== session.id ||
      invocation.generation !== session.generation ||
      owned.owner.sessionId !== session.id
    ) {
      throw new CommandBridgeError('stale-generation');
    }
    return bridge.invoke(invocation, owned.owner);
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
    readContext,
    invoke,
    cancel,
    lifecycle,
    subscribeResult,
    dispose,
  });
}
