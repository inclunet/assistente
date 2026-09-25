import { createCommandUIEffectGuard, type CommandUIEffect, type CommandUIEffectToken } from './commandUIEffect';
import type { CommandExecutionResult } from './commandUIExecution';
import type { TrustedCommandContextSession } from './commandContextSession';
import { isCommandLayerAction } from './commandLayerActions';

export interface WorkspaceListItem {
  readonly id: string;
  readonly name: string;
  readonly profile: string;
  readonly tab_count: number;
  readonly is_active: boolean;
}

export interface WorkspaceListCommandOutput {
  readonly kind: 'workspace.list';
  readonly workspaces: readonly WorkspaceListItem[];
}

export type BackendCommandStatus = 'succeeded' | 'failed' | 'denied' | 'cancelled' | 'cancelled_stale' | 'timed_out' | 'outcome_unknown' | 'suppressed' | 'rejected_stale';

export interface BackendCommandExecutionResult extends CommandExecutionResult {
  readonly output?: WorkspaceListCommandOutput | null;
}

export type PresentationReason = 'presented' | 'missing-output' | 'context-stale' | 'backend-result-invalid' | 'backend-error' | 'backend-status' | 'disposed' | 'unsupported-command' | 'apply-error';

export interface BackendPresentationResult {
  readonly execution: BackendCommandExecutionResult;
  readonly presented: boolean;
  readonly reason: PresentationReason;
}

export interface CommandBackendExecutionPort {
  executeCommand(commandID: string, args: Record<string, unknown>): Promise<unknown>;
}

export interface CommandBackendExecution {
  execute(commandID: string, apply: (output: WorkspaceListCommandOutput) => undefined): Promise<BackendPresentationResult>;
  cancelPresentation(): void;
  dispose(): void;
}

export interface CommandBackendExecutionOptions {
  readonly trustedSession?: TrustedCommandContextSession;
  readonly surfaceID?: string;
}

const WORKSPACE_LIST_COMMAND = 'workspace.list';
const statuses = new Set<BackendCommandStatus>(['succeeded', 'failed', 'denied', 'cancelled', 'cancelled_stale', 'timed_out', 'outcome_unknown', 'suppressed', 'rejected_stale']);

function validText(value: unknown): value is string {
  return typeof value === 'string' && value.length > 0 && value.length <= 4096 && value.trim() === value;
}

function validExecution(value: unknown): value is BackendCommandExecutionResult {
  if (!value || typeof value !== 'object') return false;
  const result = value as Partial<BackendCommandExecutionResult>;
  return validText(result.invocationId) && typeof result.status === 'string' && statuses.has(result.status as BackendCommandStatus) &&
    (result.resultSummary === undefined || result.resultSummary === null || validText(result.resultSummary)) &&
    (result.errorCode === undefined || result.errorCode === null || validText(result.errorCode));
}

function validWorkspace(value: unknown): value is WorkspaceListItem {
  if (!value || typeof value !== 'object') return false;
  if (Object.keys(value).some((key) => !['id', 'name', 'profile', 'tab_count', 'is_active'].includes(key))) return false;
  const item = value as Partial<WorkspaceListItem>;
  const tabCount = item.tab_count;
  return validText(item.id) && validText(item.name) && typeof item.profile === 'string' && item.profile.length <= 4096 &&
    Number.isSafeInteger(tabCount) && typeof tabCount === 'number' && tabCount >= 0 && typeof item.is_active === 'boolean';
}

function validOutput(value: unknown): value is WorkspaceListCommandOutput {
  if (!value || typeof value !== 'object') return false;
  if (Object.keys(value).some((key) => !['kind', 'workspaces'].includes(key))) return false;
  const output = value as Partial<WorkspaceListCommandOutput>;
  if (output.kind !== 'workspace.list' || !Array.isArray(output.workspaces) || output.workspaces.length > 4096 || !output.workspaces.every(validWorkspace)) return false;
  const ids = output.workspaces.map((workspace) => workspace.id);
  return new Set(ids).size === ids.length && output.workspaces.filter((workspace) => workspace.is_active).length <= 1;
}

function fallback(): BackendCommandExecutionResult {
  return { invocationId: '', status: 'outcome_unknown', errorCode: 'malformed-backend-result' };
}

function isAsyncFunction(value: (output: WorkspaceListCommandOutput) => undefined): boolean {
  return value.constructor?.name === 'AsyncFunction';
}

function isThenable(value: unknown): boolean {
  return !!value && (typeof value === 'object' || typeof value === 'function') && typeof (value as { then?: unknown }).then === 'function';
}

export function createCommandBackendExecution(
  port: CommandBackendExecutionPort,
  options: CommandBackendExecutionOptions = {},
): CommandBackendExecution {
  if (!port || typeof port.executeCommand !== 'function') throw new TypeError('command-backend-execution-invalid-configuration');
  const guard = createCommandUIEffectGuard(options.trustedSession);
  const pending = new Map<string, Promise<BackendPresentationResult>>();
  let disposed = false;

  let presentationEpoch = 0;

  async function run(commandID: string, apply: (output: WorkspaceListCommandOutput) => undefined, token: CommandUIEffectToken, capturedEpoch: number): Promise<BackendPresentationResult> {
    let raw: unknown;
    try {
      raw = await port.executeCommand(commandID, {});
    } catch {
      return { execution: fallback(), presented: false, reason: 'backend-error' };
    }
    if (!validExecution(raw)) return { execution: fallback(), presented: false, reason: 'backend-result-invalid' };
    const execution = raw;
    if (disposed) return { execution, presented: false, reason: 'disposed' };
    if (capturedEpoch !== presentationEpoch) return { execution, presented: false, reason: 'context-stale' };
    if (execution.status !== 'succeeded') {
      return { execution, presented: false, reason: 'backend-status' };
    }
    if (isCommandLayerAction(commandID)) {
      // O efeito já aconteceu no executor; não repetir a mutação na UI nem
      // interpretar seu resultado como uma lista de workspaces.
      const presented = guard.commit(token, () => undefined);
      return { execution, presented, reason: presented ? 'presented' : 'context-stale' };
    }
    if (commandID !== WORKSPACE_LIST_COMMAND) {
      return { execution, presented: false, reason: 'unsupported-command' };
    }
    if (!validOutput(execution.output)) return { execution, presented: false, reason: 'missing-output' };
    let presented = false;
    let applyStarted = false;
    if (isAsyncFunction(apply)) return { execution, presented: false, reason: 'apply-error' };
    try {
      presented = guard.commit(token, (() => {
        applyStarted = true;
        const returned = apply(execution.output as WorkspaceListCommandOutput);
        if (isThenable(returned)) throw new Error('async-backend-presentation');
        return undefined;
      }) as CommandUIEffect);
    } catch {
      presented = false;
    }
    return { execution, presented, reason: presented ? 'presented' : (applyStarted ? 'apply-error' : 'context-stale') };
  }

  function execute(commandID: string, apply: (output: WorkspaceListCommandOutput) => undefined): Promise<BackendPresentationResult> {
    if (!validText(commandID) || (commandID !== WORKSPACE_LIST_COMMAND && !isCommandLayerAction(commandID)) || typeof apply !== 'function' || disposed) return Promise.resolve({ execution: fallback(), presented: false, reason: disposed ? 'disposed' : 'unsupported-command' });
    if (isAsyncFunction(apply)) return Promise.resolve({ execution: fallback(), presented: false, reason: 'apply-error' });
    const existing = pending.get(commandID);
    if (existing) return existing;
    const token = guard.capture(options.surfaceID);
    if (!token) return Promise.resolve({ execution: fallback(), presented: false, reason: 'context-stale' });
    const capturedEpoch = presentationEpoch;
    const promise = run(commandID, apply, token, capturedEpoch).finally(() => pending.delete(commandID));
    pending.set(commandID, promise);
    return promise;
  }

  function cancelPresentation(): void {
    presentationEpoch += 1;
  }

  function dispose(): void {
    if (disposed) return;
    disposed = true;
    presentationEpoch += 1;
    guard.dispose();
  }

  return Object.freeze({ execute, cancelPresentation, dispose });
}
