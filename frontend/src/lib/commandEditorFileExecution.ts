import { isEditorFileCommand, type EditorFileCommandID, type EditorFilePreparation, type EditorFileTargetLease } from './commandEditorFile';

export interface EditorFileBegin {
  readonly ticket: string;
  readonly invocationId: string;
  readonly commandId: string;
}
export interface EditorFileTake extends EditorFileBegin { readonly handoffId: string; }
export interface EditorFileResult {
  readonly invocationId: string;
  readonly status: string;
  readonly errorCode?: string | null;
  readonly resultSummary?: string | null;
  readonly tabId?: string;
  readonly path?: string;
  readonly opened?: unknown;
  readonly written?: boolean;
}

export interface EditorFileExecutionPort {
  begin(commandID: EditorFileCommandID): Promise<EditorFileBegin>;
  take(ticket: string): Promise<EditorFileTake>;
  prepare(take: EditorFileTake, commandID: EditorFileCommandID, request: EditorFilePreparation): Promise<{
    token: string;
    path: string;
    requiresOverwrite: boolean;
    cancelled: boolean;
  } | undefined>;
  commit(take: EditorFileTake, token: string, confirmOverwrite: boolean): Promise<unknown>;
  completeCancelled(ticket: string, handoffId: string): Promise<void>;
  getResult(ticket: string): Promise<EditorFileResult>;
  cancel(ticket: string): Promise<void>;
}

export interface EditorFileExecutionOptions {
  readonly target: EditorFileTargetLease;
  /** Consome a autorização/guard inicial antes de abrir diálogo nativo. */
  readonly authorizeInitial: () => boolean;
  /** Consome a continuação contextual imediatamente antes do diálogo nativo. */
  readonly authorizePreparation?: () => boolean;
  /** Revalidação final; deve rejeitar modal aberto e IME ativo. */
  readonly authorizeCommit: () => boolean;
}

export async function executeEditorFileCommand(
  port: EditorFileExecutionPort,
  commandID: string,
  options: EditorFileExecutionOptions,
): Promise<EditorFileResult> {
  if (!isEditorFileCommand(commandID) || !options.target.isCurrent() || !options.authorizeInitial()) {
    return { invocationId: '', status: 'cancelled', errorCode: 'ui-context-stale' };
  }
  let begin: EditorFileBegin | undefined;
  let take: EditorFileTake | undefined;
  let commitResponse: unknown;
  let commitResolved = false;
  let commitSubmitted = false;
  const cancelled = (invocationId: string, errorCode: string): EditorFileResult => ({ invocationId, status: 'cancelled', errorCode });
  const nonempty = (value: unknown): value is string => typeof value === 'string' && value.trim() !== '';
  const validBegin = (value: EditorFileBegin): boolean => !!value && nonempty(value.ticket) && nonempty(value.invocationId) && value.commandId === commandID;
  const validTake = (value: EditorFileTake): boolean => validBegin(value) && nonempty(value.handoffId) && value.ticket === begin?.ticket && value.invocationId === begin?.invocationId;
  const readResult = async (ticket: string, invocationId: string): Promise<EditorFileResult> => {
    const result = await port.getResult(ticket);
    const statuses = new Set(['succeeded', 'failed', 'denied', 'cancelled', 'cancelled_stale', 'timed_out', 'outcome_unknown', 'suppressed', 'rejected_stale', 'conflict', 'stale']);
    if (!result || result.invocationId !== invocationId || !statuses.has(result.status)) return { invocationId, status: 'outcome_unknown', errorCode: 'malformed-result' };
    return result;
  };
  try {
    begin = await port.begin(commandID);
    if (!validBegin(begin)) return { invocationId: '', status: 'outcome_unknown', errorCode: 'malformed-begin' };
    if (!options.target.isCurrent()) { await port.cancel(begin.ticket).catch(() => undefined); return cancelled(begin.invocationId, 'ui-context-stale'); }
    take = await port.take(begin.ticket);
    if (!validTake(take)) {
      await port.cancel(begin.ticket).catch(() => undefined);
      return cancelled(begin.invocationId, 'malformed-take');
    }
    if (!options.target.isCurrent()) {
      await port.completeCancelled(begin.ticket, take.handoffId).catch(() => undefined);
      return cancelled(begin.invocationId, 'ui-context-stale');
    }
    const localPreparation = await options.target.prepare(commandID);
    if (!localPreparation || !options.target.isCurrent()) {
      await port.completeCancelled(begin.ticket, take.handoffId).catch(() => undefined);
      return cancelled(begin.invocationId, 'ui-context-stale');
    }
    if (options.authorizePreparation && !options.authorizePreparation()) {
      await port.completeCancelled(begin.ticket, take.handoffId).catch(() => undefined);
      return cancelled(begin.invocationId, 'ui-context-stale');
    }
    const prepared = await port.prepare(take, commandID, localPreparation);
    if (!prepared || prepared.cancelled) {
      await port.completeCancelled(begin.ticket, take.handoffId).catch(() => undefined);
      return cancelled(begin.invocationId, 'ui-preparation-cancelled');
    }
    if (!nonempty(prepared.token) || !nonempty(prepared.path) || typeof prepared.requiresOverwrite !== 'boolean') {
      await port.completeCancelled(begin.ticket, take.handoffId).catch(() => undefined);
      return { invocationId: begin.invocationId, status: 'outcome_unknown', errorCode: 'malformed-preparation' };
    }
    let confirmOverwrite = localPreparation.confirmOverwrite;
    if (prepared.requiresOverwrite) {
      confirmOverwrite = await options.target.confirmOverwrite(prepared.path);
    }
    if (!options.target.isCurrent() || !options.authorizeCommit() || !options.target.canCommit(commandID) ||
        (prepared.requiresOverwrite && !confirmOverwrite)) {
      await port.completeCancelled(begin.ticket, take.handoffId).catch(() => undefined);
      return cancelled(begin.invocationId, 'ui-context-stale');
    }
    // Não há await entre a última revalidação e a submissão. O resultado do
    // commit não é autoridade; GetResult continua sendo a confirmação.
    commitSubmitted = true;
    const commitPromise = port.commit(take, prepared.token, confirmOverwrite);
    void commitPromise.catch(() => undefined);
    try { commitResponse = await commitPromise; commitResolved = true; } catch { /* GetResult remains authoritative; no local apply without response. */ }
    const result = await readResult(begin.ticket, begin.invocationId);
    if (result.status === 'succeeded' && commitResolved) options.target.applyCommitted(commandID, commitResponse);
    return result;
  } catch {
    if (!commitSubmitted && begin && take) await port.completeCancelled(begin.ticket, take.handoffId).catch(() => undefined);
    else if (!commitSubmitted && begin) await port.cancel(begin.ticket).catch(() => undefined);
    if (begin) return await readResult(begin.ticket, begin.invocationId).catch(() => ({ invocationId: begin!.invocationId, status: 'outcome_unknown', errorCode: 'file-command-outcome-unknown' }));
    return { invocationId: '', status: 'outcome_unknown', errorCode: 'file-command-outcome-unknown' };
  }
}
