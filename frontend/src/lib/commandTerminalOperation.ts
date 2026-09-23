import { useAuthStore } from '../store/authStore';
import {
  useTerminalStore,
  type HistoryEntry,
  type SessionInfo,
} from '../store/terminalStore';
import {
  useWorkspaceStore,
  type WorkspaceData,
  type WorkspaceTab,
} from '../store/workspaceStore';
import { ReadFocusContext } from './commandContextProviders';
import { getModalRegistrySnapshot, isModalOpen } from './modalRegistry';
import type { CommandContextualBackendPort } from './commandContextualBackendExecution';
import type {
  CommandExecutionResult,
  UICommandStatus,
} from './commandUIExecution';

export const TERMINAL_INTERRUPT_COMMAND = 'terminal.command.interrupt';
export const TERMINAL_SESSION_CREATE_COMMAND = 'terminal.session.create';
export const TERMINAL_SESSION_CLOSE_COMMAND = 'terminal.session.close';
export const TERMINAL_OPERATION_EVENT = 'commands:terminal-operation';

export type TerminalSessionOperationCommand =
  | typeof TERMINAL_SESSION_CREATE_COMMAND
  | typeof TERMINAL_SESSION_CLOSE_COMMAND;

export type TerminalOperationCommand =
  | typeof TERMINAL_INTERRUPT_COMMAND
  | TerminalSessionOperationCommand;

export interface TerminalOperationRequest {
  readonly instanceId?: string;
  readonly commandId?: TerminalOperationCommand;
}

export interface TerminalOperationSurface {
  readonly root: HTMLElement;
  readonly instanceId: string;
  readonly tabId?: string;
  readonly modalId?: string;
  isCurrent(): boolean;
  canStart(commandId?: TerminalOperationCommand): boolean;
  subscribe(changed: () => void): () => void;
}

export interface TerminalOperationBinding {
  readonly workspaceId: string;
  readonly tabId: string;
  readonly sessionId: string;
}

export interface TerminalOperationTarget {
  readonly instanceId: string;
  readonly ownerId: string;
  readonly sessionId: string;
  readonly workspaceId: string;
  readonly tabId: string;
  readonly binding: TerminalOperationBinding;
  readonly workspace: WorkspaceData;
  readonly tab: WorkspaceTab;
  readonly session: SessionInfo | null;
  readonly activeEntryId: string | null;
  readonly currentCommandId: string | null;
  readonly currentCommand: HistoryEntry | null;
  readonly commandId: TerminalOperationCommand;
  isCurrent(): boolean;
  canCommit(): boolean;
  startDecision(): void;
  finishDecision(): boolean;
  dispose(): void;
}

export interface TerminalOperationPort extends CommandContextualBackendPort {
  prepareTerminalInterruptCommand(
    ticket: string,
    workspaceId: string,
    tabId: string,
    sessionId: string,
    commandId: string,
  ): Promise<void>;
  prepareTerminalSessionCommand(
    ticket: string,
    workspaceId: string,
    tabId: string,
    sessionId: string,
  ): Promise<void>;
}

const surfaces = new Set<TerminalOperationSurface>();
const leases = new Map<TerminalOperationSurface, Set<TerminalOperationTarget>>();

export function registerTerminalOperationSurface(
  surface: TerminalOperationSurface,
): () => void {
  surfaces.add(surface);
  leases.set(surface, new Set());
  return () => {
    surfaces.delete(surface);
    leases.get(surface)?.forEach(target => target.dispose());
    leases.delete(surface);
  };
}

export function requestTerminalOperation(instanceId?: string): boolean {
  if (typeof window === 'undefined') return false;
  const detail: TerminalOperationRequest = instanceId ? { instanceId } : {};
  const event = new CustomEvent<TerminalOperationRequest>(TERMINAL_OPERATION_EVENT, {
    detail,
    cancelable: true,
  });
  window.dispatchEvent(event);
  return event.defaultPrevented;
}

export function requestTerminalSessionOperation(
  commandId: TerminalSessionOperationCommand,
  instanceId?: string,
): boolean {
  if (typeof window === 'undefined') return false;
  const event = new CustomEvent<TerminalOperationRequest>(TERMINAL_OPERATION_EVENT, {
    detail: { instanceId, commandId },
    cancelable: true,
  });
  window.dispatchEvent(event);
  return event.defaultPrevented;
}

export function isTerminalSessionOperationCommand(
  commandId: string,
): commandId is TerminalSessionOperationCommand {
  return commandId === TERMINAL_SESSION_CREATE_COMMAND || commandId === TERMINAL_SESSION_CLOSE_COMMAND;
}

function currentTabFor(surface: TerminalOperationSurface): {
  workspace: WorkspaceData;
  tab: WorkspaceTab;
} | undefined {
  const workspace = useWorkspaceStore.getState().workspace;
  if (!workspace || !workspace.activeTabId) return undefined;
  const tab = workspace.tabs.find(candidate => candidate.id === workspace.activeTabId);
  if (!tab || tab.type !== 'terminal' || (surface.tabId && tab.id !== surface.tabId)) return undefined;
  return { workspace, tab };
}

function currentSessionFor(tab: WorkspaceTab): {
  sessionId: string;
  session: SessionInfo;
  activeEntryId: string | null;
  currentCommand: HistoryEntry | null;
} | undefined {
  const sessionId = typeof tab.state?.sessionId === 'string' ? tab.state.sessionId : '';
  if (!sessionId) return undefined;
  const terminal = useTerminalStore.getState();
  const session = terminal.sessions.find(candidate => candidate.id === sessionId);
  if (!session) return undefined;
  const activeEntryId = terminal.activeEntryBySession[sessionId] ?? null;
  const history = terminal.historyBySession[sessionId] ?? [];
  const currentCommand = activeEntryId
    ? history.find(entry => entry.id === activeEntryId) ?? null
    : null;
  return { sessionId, session, activeEntryId, currentCommand };
}

function sameCommandExecution(
  left: HistoryEntry | null,
  right: HistoryEntry | null,
): boolean {
  if (left === right) return true;
  if (!left || !right) return false;
  // O objeto é recriado a cada chunk de output. A execução, porém, continua
  // sendo a mesma enquanto sua identidade de origem não mudar.
  return left.id === right.id && left.command === right.command &&
    left.startedAt === right.startedAt && left.source === right.source;
}

export function captureTerminalOperationTarget(
  readPathname: () => string,
  expectedInstance?: string,
  commandId: TerminalOperationCommand = TERMINAL_INTERRUPT_COMMAND,
): TerminalOperationTarget | undefined {
  if (typeof document === 'undefined') return undefined;

  let pathname: string;
  try {
    if (!document.hasFocus() || isModalOpen() || ReadFocusContext().composition === 'active') return undefined;
    pathname = readPathname();
  } catch {
    return undefined;
  }

  const auth = useAuthStore.getState();
  if (!auth.isAuthenticated || !auth.user) return undefined;

  const candidates = [...surfaces].filter(surface => {
    if (!surface.root.isConnected || surface.instanceId !== (expectedInstance ?? surface.instanceId)) return false;
    try {
      return surface.isCurrent() && surface.canStart(commandId);
    } catch {
      return false;
    }
  });
  if (candidates.length !== 1) return undefined;

  const source = candidates[0];
  if (expectedInstance !== undefined && source.instanceId !== expectedInstance) return undefined;
  const current = currentTabFor(source);
  const terminal = current && currentSessionFor(current.tab);
  if (!current) return undefined;
  if (commandId === TERMINAL_INTERRUPT_COMMAND && (!terminal?.activeEntryId || !terminal.currentCommand)) return undefined;
  if (commandId === TERMINAL_SESSION_CLOSE_COMMAND && !terminal) return undefined;
  const bindingSessionId = typeof current.tab.state?.sessionId === 'string'
    ? current.tab.state.sessionId
    : '';

  const initialModal = getModalRegistrySnapshot();
  const captured = Object.freeze({
    ownerId: auth.user.userId,
    sessionId: auth.user.sessionId,
    workspaceId: current.workspace.id,
    tabId: current.tab.id,
    bindingSessionId,
    workspace: current.workspace,
    tab: current.tab,
    session: terminal?.session ?? null,
    activeEntryId: terminal?.activeEntryId ?? null,
    currentCommand: terminal?.currentCommand ?? null,
    modalGeneration: initialModal.generationNumber,
    modalIds: initialModal.ids,
  });

  let disposed = false;
  let invalidated = false;
  let decisionPending = false;
  let generation = captured.modalGeneration;
  const invalidate = () => { invalidated = true; };

  const isCurrent = (): boolean => {
    try {
      const authNow = useAuthStore.getState();
      const active = currentTabFor(source);
      const terminalNow = active ? currentSessionFor(active.tab) : undefined;
      const activeBindingSessionId = active && typeof active.tab.state?.sessionId === 'string'
        ? active.tab.state.sessionId
        : '';
      const modal = getModalRegistrySnapshot();
      const decisionStack = commandId === TERMINAL_SESSION_CLOSE_COMMAND && decisionPending &&
        captured.modalIds.every((id, index) => modal.ids[index] === id) &&
        (modal.ids.length === captured.modalIds.length ||
          (modal.ids.length === captured.modalIds.length + 1 && modal.dialogCommandScope?.kind === 'decision'));
      const valid = !disposed && !invalidated && document.hasFocus() && (decisionStack || !isModalOpen()) &&
        ReadFocusContext().composition !== 'active' &&
        readPathname() === pathname &&
        (decisionStack || modal.generationNumber === generation) &&
        surfaces.has(source) && source.root.isConnected && source.isCurrent() &&
        (decisionStack || source.canStart(commandId)) &&
        authNow.isAuthenticated && authNow.user?.userId === captured.ownerId &&
        authNow.user.sessionId === captured.sessionId &&
        active?.workspace.id === captured.workspaceId &&
        active.workspace.activeTabId === captured.tabId &&
        active.tab.id === captured.tabId && active.tab.type === 'terminal' &&
        activeBindingSessionId === captured.bindingSessionId &&
        (terminalNow?.session ?? null) === captured.session &&
        (commandId !== TERMINAL_SESSION_CLOSE_COMMAND || terminalNow !== undefined) &&
        (commandId !== TERMINAL_INTERRUPT_COMMAND && commandId !== TERMINAL_SESSION_CLOSE_COMMAND || (
          terminalNow?.activeEntryId === captured.activeEntryId &&
          sameCommandExecution(terminalNow.currentCommand, captured.currentCommand)
        ));
      if (!valid) invalidated = true;
      return valid;
    } catch {
      invalidated = true;
      return false;
    }
  };

  const unsubscribeSource = source.subscribe(() => { isCurrent(); });
  const unsubscribeAuth = useAuthStore.subscribe(() => { isCurrent(); });
  const unsubscribeWorkspace = useWorkspaceStore.subscribe(() => { isCurrent(); });
  const unsubscribeTerminal = useTerminalStore.subscribe(() => { isCurrent(); });
  window.addEventListener('blur', invalidate);

  const target: TerminalOperationTarget = {
    instanceId: source.instanceId,
    ownerId: captured.ownerId,
    sessionId: captured.sessionId,
    workspaceId: captured.workspaceId,
    tabId: captured.tabId,
    binding: Object.freeze({
      workspaceId: captured.workspaceId,
      tabId: captured.tabId,
      sessionId: captured.bindingSessionId,
    }),
    workspace: captured.workspace,
    tab: captured.tab,
    session: captured.session,
    activeEntryId: captured.activeEntryId,
    currentCommandId: captured.activeEntryId,
    currentCommand: captured.currentCommand,
    commandId,
    isCurrent,
    canCommit: () => isCurrent() && document.hasFocus() && !isModalOpen() &&
      ReadFocusContext().composition !== 'active',
    startDecision: () => {
      decisionPending = commandId === TERMINAL_SESSION_CLOSE_COMMAND;
    },
    finishDecision: () => {
      if (commandId !== TERMINAL_SESSION_CLOSE_COMMAND) return isCurrent();
      const modal = getModalRegistrySnapshot();
      if (modal.ids.length !== captured.modalIds.length ||
          !captured.modalIds.every((id, index) => modal.ids[index] === id)) {
        invalidated = true;
      }
      generation = modal.generationNumber;
      decisionPending = false;
      return isCurrent();
    },
    dispose: () => {
      if (disposed) return;
      disposed = true;
      unsubscribeSource();
      unsubscribeAuth();
      unsubscribeWorkspace();
      unsubscribeTerminal();
      window.removeEventListener('blur', invalidate);
      leases.get(source)?.delete(target);
    },
  };
  leases.get(source)?.add(target);
  if (!target.isCurrent()) {
    target.dispose();
    return undefined;
  }
  return target;
}

function validBegin(value: unknown): value is { ticket: string; invocationId: string; commandId: string } {
  if (!value || typeof value !== 'object') return false;
  const response = value as Partial<{ ticket: string; invocationId: string; commandId: string }>;
  return Boolean(response.ticket && response.invocationId && response.commandId);
}

function validTake(
  value: unknown,
  reservation: { ticket: string; invocationId: string; commandId: string },
): value is { ticket: string; invocationId: string; commandId: string; handoffId: string } {
  if (!value || typeof value !== 'object') return false;
  const response = value as Partial<{ ticket: string; invocationId: string; commandId: string; handoffId: string }>;
  return response.ticket === reservation.ticket && response.invocationId === reservation.invocationId &&
    response.commandId === reservation.commandId && Boolean(response.handoffId);
}

function validResult(value: unknown, invocationId: string): value is CommandExecutionResult {
  if (!value || typeof value !== 'object') return false;
  const result = value as Partial<CommandExecutionResult>;
  return result.invocationId === invocationId && typeof result.status === 'string';
}

export async function executeTerminalInterrupt(
  port: TerminalOperationPort,
  target: TerminalOperationTarget,
): Promise<UICommandStatus> {
  let ticket: string | undefined;
  let handoff: string | undefined;
  let commitStarted = false;

  try {
    if (typeof port.prepareTerminalInterruptCommand !== 'function') return 'failed';
    if (!target.isCurrent() || !target.canCommit()) return 'cancelled';
    const reservation = await port.beginUICommand(TERMINAL_INTERRUPT_COMMAND);
    if (validBegin(reservation)) {
      ticket = reservation.ticket;
    }
    if (!validBegin(reservation) || reservation.commandId !== TERMINAL_INTERRUPT_COMMAND || !target.isCurrent()) {
      return 'cancelled';
    }
    const admittedInvocationId = reservation.invocationId;

    const admittedTicket = reservation.ticket;
    if (!target.canCommit()) return 'cancelled';
    await port.prepareTerminalInterruptCommand(
      admittedTicket,
      target.binding.workspaceId,
      target.binding.tabId,
      target.binding.sessionId,
      target.currentCommandId ?? '',
    );
    if (!target.isCurrent() || !target.canCommit()) return 'cancelled';

    const taken = await port.takeUICommand(admittedTicket);
    if (typeof taken === 'object' && taken !== null && 'handoffId' in taken) {
      handoff = String((taken as { handoffId?: unknown }).handoffId || '');
    }
    if (!validTake(taken, reservation) || !target.canCommit()) return 'cancelled';

    commitStarted = true;
    const admittedHandoff = taken.handoffId;
    await port.commitBackendCommand(admittedTicket, admittedHandoff);
    const result = await port.getUICommandResult(admittedTicket);
    if (!validResult(result, admittedInvocationId)) return 'outcome_unknown';
    return result.status;
  } catch {
    // Depois do commit, a resposta pode ter se perdido após o efeito. Nunca
    // repita a interrupção nem transforme essa incerteza em sucesso/falha.
    return commitStarted ? 'outcome_unknown' : 'failed';
  } finally {
    if (!commitStarted && ticket) {
      try {
        if (handoff) await port.completeUICommand(ticket, handoff, 'cancelled');
        else await port.cancelUICommand(ticket);
      } catch {
        // A expiração/recusa do lease no backend continua sendo a autoridade.
      }
    }
    target.dispose();
  }
}

export async function executeTerminalSessionOperation(
  port: TerminalOperationPort,
  target: TerminalOperationTarget,
): Promise<UICommandStatus> {
  let ticket: string | undefined;
  let handoff: string | undefined;
  let commitStarted = false;
  let taking = false;
  let invocationId = '';

  try {
    if (!isTerminalSessionOperationCommand(target.commandId) ||
        typeof port.prepareTerminalSessionCommand !== 'function') return 'failed';
    if (!target.isCurrent() || !target.canCommit()) return 'cancelled';
    if (target.commandId === TERMINAL_SESSION_CLOSE_COMMAND) target.startDecision();
    const reservation = await port.beginUICommand(target.commandId);
    if (validBegin(reservation)) ticket = reservation.ticket;
    if (!validBegin(reservation) || reservation.commandId !== target.commandId ||
        !target.isCurrent()) return 'cancelled';
    invocationId = reservation.invocationId;
    await port.prepareTerminalSessionCommand(
      reservation.ticket,
      target.binding.workspaceId,
      target.binding.tabId,
      target.binding.sessionId,
    );
    if (!target.isCurrent()) return 'cancelled';

    taking = true;
    const taken = await port.takeUICommand(reservation.ticket);
    if (typeof taken === 'object' && taken !== null && 'handoffId' in taken) {
      handoff = String((taken as { handoffId?: unknown }).handoffId || '');
    }
    if (target.commandId === TERMINAL_SESSION_CLOSE_COMMAND && !target.finishDecision()) return 'cancelled';
    if (!validTake(taken, reservation) || !target.canCommit()) return 'cancelled';

    commitStarted = true;
    await port.commitBackendCommand(reservation.ticket, taken.handoffId);
    const result = await port.getUICommandResult(reservation.ticket);
    if (!validResult(result, invocationId)) return 'outcome_unknown';
    return result.status;
  } catch {
    if (commitStarted) return 'outcome_unknown';
    if (taking && ticket) {
      try {
        const result = await port.getUICommandResult(ticket);
        if (validResult(result, invocationId) &&
            ['cancelled', 'denied', 'cancelled_stale', 'rejected_stale', 'timed_out'].includes(result.status)) {
          return result.status;
        }
      } catch {
        // O broker continua sendo a autoridade quando a decisão não é legível.
      }
    }
    return 'failed';
  } finally {
    if (!commitStarted && ticket) {
      try {
        if (handoff) await port.completeUICommand(ticket, handoff, 'cancelled');
        else await port.cancelUICommand(ticket);
      } catch {
        // O lease backend continua sendo a autoridade quando a UI perde a corrida.
      }
    }
    target.dispose();
  }
}
