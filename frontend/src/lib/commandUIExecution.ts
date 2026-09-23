import {
  createCommandUIEffectGuard,
  type CommandUIEffect,
  type CommandUIEffectToken,
} from './commandUIEffect';
import type { TrustedCommandContextSession } from './commandContextSession';

export type UICommandStatus =
  | 'succeeded'
  | 'failed'
  | 'denied'
  | 'cancelled'
  | 'cancelled_stale'
  | 'timed_out'
  | 'outcome_unknown'
  | 'suppressed'
  | 'rejected_stale';
export type UICommandCompletionStatus = 'succeeded' | 'failed' | 'cancelled';

export interface UICommandBeginResponse {
  readonly ticket: string;
  readonly invocationId: string;
  readonly commandId: string;
}

export interface UICommandTakeResponse extends UICommandBeginResponse {
  readonly handoffId: string;
}

export interface CommandExecutionResult {
  readonly invocationId: string;
  readonly status: UICommandStatus;
  readonly resultSummary?: string | null;
  readonly errorCode?: string | null;
}

export interface CommandUIExecutionPort {
  beginUICommand(commandID: string): Promise<UICommandBeginResponse>;
  takeUICommand(ticket: string): Promise<UICommandTakeResponse>;
  completeUICommand(
    ticket: string,
    handoffId: string,
    status: UICommandCompletionStatus
  ): Promise<void>;
  getUICommandResult(ticket: string): Promise<CommandExecutionResult>;
  cancelUICommand(ticket: string): Promise<void>;
}

export interface CommandUIExecution {
  execute(commandID: string): Promise<CommandExecutionResult>;
  cancel(commandID?: string): Promise<void>;
  dispose(): void;
}

export interface CommandUIExecutionOptions {
  readonly trustedSession?: TrustedCommandContextSession;
  readonly surfaceID?: string;
  /** Revalida o contexto imediatamente antes do efeito síncrono. */
  readonly canCommitEffect?: () => boolean;
  /** Captura o alvo local uma vez, antes do handoff; undefined recusa a ação. */
  readonly prepareEffect?: (commandID: string) => PreparedCommandUIEffect | undefined;
}

export interface PreparedCommandUIEffect {
  readonly effect: CommandUIEffect;
  readonly isCurrent: () => boolean;
  readonly dispose: () => void;
}

export type UICommandResult = CommandExecutionResult;

const MAX_PENDING_ATTEMPTS = 32;

function localResult(
  invocationId: string,
  status: UICommandStatus,
  errorCode: string
): UICommandResult {
  return { invocationId, status, errorCode };
}

function validText(value: unknown): value is string {
  return typeof value === 'string' && value.length > 0 && value.trim() === value;
}

function validCommandID(value: unknown): value is string {
  return validText(value) && /^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$/.test(value);
}

function validBegin(value: unknown, commandID: string): value is UICommandBeginResponse {
  if (!value || typeof value !== 'object') return false;
  const response = value as Partial<UICommandBeginResponse>;
  return (
    validText(response.ticket) &&
    validText(response.invocationId) &&
    response.commandId === commandID
  );
}

function validTake(value: unknown, begin: UICommandBeginResponse): value is UICommandTakeResponse {
  if (!value || typeof value !== 'object') return false;
  const response = value as Partial<UICommandTakeResponse>;
  return (
    response.ticket === begin.ticket &&
    response.invocationId === begin.invocationId &&
    response.commandId === begin.commandId &&
    validText(response.handoffId)
  );
}

function validResult(value: unknown, invocationId: string): value is CommandExecutionResult {
  if (!value || typeof value !== 'object') return false;
  const result = value as Partial<UICommandResult>;
  return (
    validText(result.invocationId) &&
    result.invocationId === invocationId &&
    (result.status === 'succeeded' ||
      result.status === 'failed' ||
      result.status === 'cancelled' ||
      result.status === 'denied' ||
      result.status === 'cancelled_stale' ||
      result.status === 'timed_out' ||
      result.status === 'outcome_unknown' ||
      result.status === 'suppressed' ||
      result.status === 'rejected_stale') &&
    (result.resultSummary === undefined ||
      result.resultSummary === null ||
      typeof result.resultSummary === 'string') &&
    (result.errorCode === undefined ||
      result.errorCode === null ||
      typeof result.errorCode === 'string')
  );
}

interface Attempt {
  commandID: string;
  effectStarted: boolean;
  promise: Promise<CommandExecutionResult>;
  token: CommandUIEffectToken | undefined;
  ticket: string | undefined;
  invocationId: string;
  cancelRequested: boolean;
  cancelPromise: Promise<void> | undefined;
  prepared?: PreparedCommandUIEffect;
}

function isPromiseLike(value: unknown): boolean {
  return (
    !!value &&
    (typeof value === 'object' || typeof value === 'function') &&
    typeof (value as { then?: unknown }).then === 'function'
  );
}

/**
 * Coordena uma tentativa UI síncrona com o handoff backend. O guard prova
 * apenas fatos locais; o port continua responsável por auth, ledger e alvo.
 * `handlers` é deliberadamente injetado por superfície, nunca descoberto aqui.
 */
export function createCommandUIExecution(
  port: CommandUIExecutionPort,
  handlers: ReadonlyMap<string, CommandUIEffect>,
  options?: CommandUIExecutionOptions
): CommandUIExecution {
  if (
    !port ||
    !handlers ||
    typeof handlers.get !== 'function' ||
    typeof handlers.has !== 'function' ||
    typeof port.beginUICommand !== 'function' ||
    typeof port.takeUICommand !== 'function' ||
    typeof port.completeUICommand !== 'function' ||
    typeof port.getUICommandResult !== 'function' ||
    typeof port.cancelUICommand !== 'function'
  ) {
    throw new TypeError('command-ui-execution-invalid-configuration');
  }
  const guard = createCommandUIEffectGuard(options?.trustedSession);

  const pending = new Map<string, Attempt>();
  let disposed = false;

  function cancelAttempt(attempt: Attempt, explicit = false): Promise<void> {
    // Desmontagem/blur/map refresh não podem cancelar no backend uma execução
    // cujo efeito síncrono já começou. Falhas internas ainda podem usar Cancel
    // para fechar um handoff que não conseguiu completar o efeito.
    if (explicit && attempt.effectStarted) return Promise.resolve();
    attempt.cancelRequested = true;
    if (attempt.cancelPromise || !attempt.ticket) return attempt.cancelPromise ?? Promise.resolve();
    attempt.cancelPromise = Promise.resolve()
      .then(() => port.cancelUICommand(attempt.ticket as string))
      .then(
        () => undefined,
        () => undefined
      );
    return attempt.cancelPromise;
  }

  async function readBackendResult(
    attempt: Attempt,
    fallbackCode: string
  ): Promise<UICommandResult> {
    if (!attempt.ticket) return localResult('', 'outcome_unknown', fallbackCode);
    try {
      const result = await port.getUICommandResult(attempt.ticket);
      return validResult(result, attempt.invocationId)
        ? result
        : localResult(attempt.invocationId, 'outcome_unknown', 'malformed-result');
    } catch {
      return localResult(attempt.invocationId, 'outcome_unknown', fallbackCode);
    }
  }

  async function cancelAndReadAfterTakeFailure(
    attempt: Attempt,
    fallbackCode: string
  ): Promise<UICommandResult> {
    await cancelAttempt(attempt);
    const result = await readBackendResult(attempt, fallbackCode);
    // Sem handoff confirmado, o frontend não pode tratar succeeded como efeito
    // concluído; a execução efetiva permanece desconhecida.
    return result.status === 'succeeded'
      ? localResult(attempt.invocationId, 'outcome_unknown', 'succeeded-without-handoff')
      : result;
  }

  async function completeCancelledAndRead(
    attempt: Attempt,
    ticket: string,
    handoffId: string,
    fallbackCode: string
  ): Promise<UICommandResult> {
    try {
      await port.completeUICommand(ticket, handoffId, 'cancelled');
    } catch {
      // Once Take succeeded, Cancel would deliberately become outcome_unknown.
    }
    return readBackendResult(attempt, fallbackCode);
  }

  async function run(attempt: Attempt): Promise<UICommandResult> {
    let handler = handlers.get(attempt.commandID);
    if (typeof handler !== 'function') return localResult('', 'cancelled', 'ui-handler-missing');

    if (options?.prepareEffect) {
      try {
        attempt.prepared = options.prepareEffect(attempt.commandID);
        if (!attempt.prepared || !attempt.prepared.isCurrent()) {
          return localResult('', 'cancelled', 'ui-context-stale');
        }
        handler = attempt.prepared.effect;
        if (typeof handler !== 'function') return localResult('', 'cancelled', 'ui-handler-missing');
      } catch {
        return localResult('', 'cancelled', 'ui-context-stale');
      }
    }

    attempt.token = guard.capture(options?.surfaceID, attempt.commandID);
    if (!attempt.token) return localResult('', 'cancelled', 'ui-context-stale');

    let begin: unknown;
    try {
      begin = await port.beginUICommand(attempt.commandID);
    } catch {
      return localResult('', 'outcome_unknown', 'begin-confirmation-unknown');
    }
    if (!validBegin(begin, attempt.commandID)) {
      const malformedBegin =
        begin && typeof begin === 'object' ? (begin as Partial<UICommandBeginResponse>) : undefined;
      if (malformedBegin && validText(malformedBegin.ticket)) {
        attempt.ticket = malformedBegin.ticket;
        await cancelAttempt(attempt);
      }
      return localResult('', 'outcome_unknown', 'malformed-begin');
    }
    attempt.ticket = begin.ticket;
    attempt.invocationId = begin.invocationId;
    if (disposed || attempt.cancelRequested) {
      await cancelAttempt(attempt);
      return readBackendResult(attempt, 'cancelled-confirmation-unknown');
    }

    let take: UICommandTakeResponse;
    try {
      take = await port.takeUICommand(begin.ticket);
    } catch {
      return cancelAndReadAfterTakeFailure(attempt, 'take-confirmation-unknown');
    }
    if (!validTake(take, begin)) {
      return cancelAndReadAfterTakeFailure(attempt, 'malformed-take');
    }
    if (disposed || attempt.cancelRequested) {
      return completeCancelledAndRead(
        attempt,
        begin.ticket,
        take.handoffId,
        'cancelled-confirmation-unknown'
      );
    }

    let effectStarted = false;
    let committed = false;
    let contextCurrent = false;
    try {
      contextCurrent = (!options?.canCommitEffect || options.canCommitEffect()) &&
        (!attempt.prepared || attempt.prepared.isCurrent());
    } catch {
      // Uma fonte local ausente ou com erro nunca autoriza o efeito.
    }
    if (!contextCurrent) {
      return completeCancelledAndRead(
        attempt,
        begin.ticket,
        take.handoffId,
        'ui-context-stale'
      );
    }
    try {
      committed = guard.commit(attempt.token as CommandUIEffectToken, () => {
        effectStarted = true;
        attempt.effectStarted = true;
        const result = handler();
        // A thenable means the caller violated the synchronous effect contract.
        // It may already have started work, so the outer flow must not report
        // failed or attempt a second execution.
        if (isPromiseLike(result)) throw new Error('async-ui-effect');
        return undefined;
      });
    } catch {
      committed = false;
    }
    attempt.token = undefined;
    if (!committed && !effectStarted) {
      return completeCancelledAndRead(
        attempt,
        begin.ticket,
        take.handoffId,
        'ui-effect-not-committed'
      );
    }
    if (!committed || effectStarted === false) {
      await cancelAttempt(attempt);
      return readBackendResult(attempt, 'ui-effect-not-committed');
    }

    try {
      await port.completeUICommand(begin.ticket, take.handoffId, 'succeeded');
    } catch {
      return readBackendResult(attempt, 'complete-confirmation-unknown');
    }
    return readBackendResult(attempt, 'result-confirmation-unknown');
  }

  function execute(commandID: string): Promise<UICommandResult> {
    if (!validCommandID(commandID))
      return Promise.resolve(localResult('', 'cancelled', 'invalid-command'));
    const existing = pending.get(commandID);
    if (existing) return existing.promise;
    if (disposed) return Promise.resolve(localResult('', 'cancelled', 'disposed'));
    if (pending.size >= MAX_PENDING_ATTEMPTS)
      return Promise.resolve(localResult('', 'cancelled', 'too-many-pending'));

    const attempt: Attempt = {
      commandID,
      effectStarted: false,
      promise: Promise.resolve(localResult('', 'outcome_unknown', 'not-started')),
      token: undefined,
      ticket: undefined,
      invocationId: '',
      cancelRequested: false,
      cancelPromise: undefined,
    };
    attempt.promise = run(attempt).finally(() => {
      try {
        attempt.prepared?.dispose();
      } finally {
        if (pending.get(commandID) === attempt) pending.delete(commandID);
      }
    });
    pending.set(commandID, attempt);
    return attempt.promise;
  }

  async function cancel(commandID?: string): Promise<void> {
    const attempts =
      commandID === undefined
        ? [...pending.values()]
        : pending.get(commandID)
          ? [pending.get(commandID) as Attempt]
          : [];
    await Promise.all(attempts.map((attempt) => cancelAttempt(attempt, true)));
  }

  function dispose(): void {
    if (disposed) return;
    disposed = true;
    guard.dispose();
    // A própria navegação pode desmontar a superfície. Depois do efeito
    // síncrono, run ainda deve confirmar o resultado: desmontar não desfaz
    // uma ação já executada. Falhas do handler continuam canceladas por run;
    // cancelamento explícito e validações backend não são enfraquecidos.
    for (const attempt of pending.values()) {
      if (!attempt.effectStarted) void cancelAttempt(attempt);
    }
  }

  return Object.freeze({ execute, cancel, dispose });
}
