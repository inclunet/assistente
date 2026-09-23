import { ReadFocusContext } from './commandContextProviders';
import { getModalRegistrySnapshot, getModalSnapshotGeneration } from './modalRegistry';
import type { CommandContextualBackendPort } from './commandContextualBackendExecution';
import type { UICommandStatus } from './commandUIExecution';

export const CHAT_MESSAGE_ACTION_IDS = ['chat.message.copy', 'chat.message.copy_markdown', 'chat.message.speak', 'chat.message.edit.open', 'chat.message.edit.save', 'chat.message.pin.toggle', 'chat.message.delete', 'chat.message.send_to_editor'] as const;
export const CHAT_MESSAGING_COMMAND_IDS = ['chat.message.send', 'chat.response.cancel', 'chat.message.retry', ...CHAT_MESSAGE_ACTION_IDS] as const;
export type ChatMessageActionID = typeof CHAT_MESSAGE_ACTION_IDS[number];
export function isChatMessageAction(id: unknown): id is ChatMessageActionID {
  return CHAT_MESSAGE_ACTION_IDS.some(command => command === id);
}
function isMessageUIEffect(id: string): boolean {
  return isChatMessageAction(id) && id !== 'chat.message.pin.toggle' && id !== 'chat.message.delete' && id !== 'chat.message.edit.open' && id !== 'chat.message.edit.save';
}
export type ChatMessagingCommandID = typeof CHAT_MESSAGING_COMMAND_IDS[number];
export const CHAT_MESSAGING_COMMAND_EVENT = 'commands:chat-messaging';
export const CHAT_MESSAGING_EVENT = CHAT_MESSAGING_COMMAND_EVENT;
export function isChatMessagingCommand(id: unknown): id is ChatMessagingCommandID {
  return CHAT_MESSAGING_COMMAND_IDS.some(command => command === id);
}
export interface ChatMessagingHandoff { readonly ticket: string; readonly handoffId: string }
/** Local guard is never serialized into ChatParams. */
export interface ChatMessagingExecution {
  readonly handoff: ChatMessagingHandoff;
  readonly isCurrent: () => boolean;
  readonly onPipelineStarted?: (revision: number) => void;
}
export class ChatMessagingStaleError extends Error {
  constructor() { super('chat-messaging-stale'); }
}
export interface PreparedChatMessagingTarget {
  readonly executionKind?: 'ui' | 'backend';
  prepareAdmission?(ticket: string): Promise<void>;
  finishAdmission?(): void;
  settled?(status: UICommandStatus): void;
  waitForAdmission?(): Promise<void>;
  succeeded?(): void;
  isCurrent(): boolean;
  canCommit(): boolean;
  execute(handoff: ChatMessagingHandoff): Promise<void>;
  dispose(): void;
}
export interface ChatMessagingTarget extends PreparedChatMessagingTarget {
  readonly queueKey?: string;
  readonly commandId: ChatMessagingCommandID;
  readonly instanceId: string;
}
export interface ChatMessagingSurface {
  readonly queueKey?: string;
  readonly root: HTMLElement;
  readonly instanceId: string;
  readonly modalId?: string;
  isCurrent(): boolean;
  canStart(commandID: ChatMessagingCommandID, keyboardTarget?: EventTarget | null): boolean;
  prepare(commandID: ChatMessagingCommandID, override?: unknown): PreparedChatMessagingTarget | undefined;
  subscribe(changed: () => void): () => void;
}
export interface ChatMessagingRequest {
  readonly commandId: ChatMessagingCommandID;
  readonly instanceId: string;
  readonly target?: ChatMessagingTarget;
}
const surfaces = new Set<ChatMessagingSurface>();
const leases = new Map<ChatMessagingSurface, Set<ChatMessagingTarget>>();
export function registerChatMessagingSurface(source: ChatMessagingSurface): () => void {
  surfaces.add(source);
  leases.set(source, new Set());
  return () => {
    surfaces.delete(source);
    leases.get(source)?.forEach(target => target.dispose());
    leases.delete(source);
  };
}
function visible(root: HTMLElement, decisionPending = false): boolean {
  if (!root.isConnected || root.closest(decisionPending ? '[hidden]' : '[hidden],[inert],[aria-hidden="true"]')) return false;
  for (let element: HTMLElement | null = root; element; element = element.parentElement) {
    const style = getComputedStyle(element);
    if (style.display === 'none' || style.visibility === 'hidden' || style.visibility === 'collapse') return false;
  }
  return true;
}
export function captureChatMessagingTarget(
  readPathname: () => string, commandID: string, expectedInstance?: string,
  keyboardTarget?: EventTarget | null, override?: unknown,
): ChatMessagingTarget | undefined {
  if (!isChatMessagingCommand(commandID) || ReadFocusContext().composition === 'active') return undefined;
  const candidates = [...surfaces].filter(source => visible(source.root) && source.isCurrent() && source.canStart(commandID, keyboardTarget));
  if (candidates.length !== 1) return undefined;
  const source = candidates[0];
  if (expectedInstance !== undefined && source.instanceId !== expectedInstance) return undefined;
  const prepared = source.prepare(commandID, override);
  if (!prepared) return undefined;
  const pathname = readPathname();
  let modalGeneration = getModalSnapshotGeneration();
  const initialModals = getModalRegistrySnapshot().ids;
  let decisionPending = false;
  let disposed = false;
  let invalid = false;
  let executed = false;
  let unsubscribe: (() => void) | undefined;
  const isCurrent = () => {
    const modals = getModalRegistrySnapshot();
    // Only deletion owns an interactive backend decision. During that admission,
    // the original chat may be inert; its immutable message/context lease remains
    // live. No other command can ignore a modal-generation change.
    const decisionStack = decisionPending && initialModals.every((id, index) => modals.ids[index] === id) &&
      (modals.ids.length === initialModals.length ||
        modals.ids.length === initialModals.length + 1 && modals.dialogCommandScope?.kind === 'decision');
    const valid = !disposed && !invalid && surfaces.has(source) && visible(source.root, decisionPending) &&
      (decisionStack || getModalSnapshotGeneration() === modalGeneration) &&
      readPathname() === pathname && source.isCurrent() && prepared.isCurrent() &&
      (!source.modalId || getModalRegistrySnapshot().ids.includes(source.modalId));
    if (!valid) invalid = true;
    return valid;
  };
  const target: ChatMessagingTarget = {
    queueKey: source.queueKey ?? source.instanceId,
    commandId: commandID, instanceId: source.instanceId, isCurrent,
    executionKind: prepared.executionKind,
    async prepareAdmission(ticket) {
      if (isChatMessageAction(commandID) && !prepared.prepareAdmission) throw new ChatMessagingStaleError();
      if (!target.canCommit()) throw new ChatMessagingStaleError();
      decisionPending = commandID === 'chat.message.delete';
      await prepared.prepareAdmission?.(ticket);
      if (!isCurrent()) throw new ChatMessagingStaleError();
    },
    finishAdmission() {
      if (decisionPending) {
        const ids = getModalRegistrySnapshot().ids;
        if (ids.length !== initialModals.length || !ids.every((id, index) => id === initialModals[index])) invalid = true;
        else modalGeneration = getModalSnapshotGeneration();
        decisionPending = false;
      }
      prepared.finishAdmission?.();
    },
    waitForAdmission: () => prepared.waitForAdmission?.() ?? Promise.resolve(),
    succeeded: () => { if (!disposed && surfaces.has(source) && source.root.isConnected && source.isCurrent()) prepared.succeeded?.(); },
    settled: status => { if (!disposed && surfaces.has(source) && source.root.isConnected && source.isCurrent()) prepared.settled?.(status); },
    canCommit: () => isCurrent() && visible(source.root) && document.hasFocus() &&
      ReadFocusContext().composition !== 'active' && prepared.canCommit(),
    async execute(handoff) {
      if (executed || !target.canCommit()) throw new ChatMessagingStaleError();
      executed = true;
      await prepared.execute(Object.freeze({ ...handoff }));
    },
    dispose() {
      if (disposed) return;
      disposed = true; unsubscribe?.(); prepared.dispose(); leases.get(source)?.delete(target);
    },
  };
  leases.get(source)?.add(target);
  unsubscribe = source.subscribe(() => { isCurrent(); });
  if (!isCurrent()) { target.dispose(); return undefined; }
  return target;
}
export function requestChatMessagingCommand(commandId: ChatMessagingCommandID, instanceId: string, target?: ChatMessagingTarget): boolean {
  const event = new CustomEvent<ChatMessagingRequest>(CHAT_MESSAGING_COMMAND_EVENT, { detail: { commandId, instanceId, target }, cancelable: true });
  window.dispatchEvent(event);
  if (!event.defaultPrevented) target?.dispose();
  return event.defaultPrevented;
}
const admissions = new Map<string, Promise<unknown>>();
export async function executeChatMessaging(port: CommandContextualBackendPort, target: ChatMessagingTarget, commandID: string = target.commandId): Promise<UICommandStatus> {
  // Opening the existing editor is presentation only. No ticket, IPC, audit,
  // or conversation submission queue is needed for this local effect.
  if (commandID === 'chat.message.edit.open') {
    try {
      if (target.commandId !== commandID || !target.canCommit()) return 'cancelled';
      await target.execute({ ticket: '', handoffId: '' });
      return 'succeeded';
    } catch (error) { return error instanceof ChatMessagingStaleError ? 'cancelled' : 'failed'; }
    finally { target.dispose(); }
  }
  if (commandID === 'chat.response.cancel') return executeCaptured(port, target, commandID);
  const key = target.queueKey ?? target.instanceId;
  const previous = admissions.get(key) ?? Promise.resolve();
  const run = previous.catch(() => undefined).then(() => executeCaptured(port, target, commandID));
  admissions.set(key, run);
  try { return await run; } finally { if (admissions.get(key) === run) admissions.delete(key); }
}
async function executeCaptured(port: CommandContextualBackendPort, target: ChatMessagingTarget, commandID: string): Promise<UICommandStatus> {
  let ticket: string | undefined;
  let handoffId: string | undefined;
  let submitted = false;
  let invocationId: string | undefined;
  let resultRequested = false;
  let takeRequested = false;
  const reconcile = async (): Promise<UICommandStatus> => {
    if (!ticket || !invocationId) return 'outcome_unknown';
    resultRequested = true;
    const result = await port.getUICommandResult(ticket);
    if (result.invocationId !== invocationId) return 'outcome_unknown';
    try { target.settled?.(result.status); } catch { /* presentation only */ }
    if (result.status === 'succeeded') {
      if (target.commandId === 'chat.response.cancel' && target.canCommit()) {
        try { await target.execute({ ticket, handoffId: handoffId! }); } catch { /* backend result authoritative */ }
      }
      try { target.succeeded?.(); } catch { /* presentation only */ }
    }
    return result.status;
  };
  try {
    if (commandID !== target.commandId || !target.isCurrent()) return 'cancelled';
    await target.waitForAdmission?.();
    if (!target.canCommit()) return 'cancelled';
    const reservation = await port.beginUICommand(target.commandId);
    ticket = reservation.ticket;
    invocationId = reservation.invocationId;
    if (!ticket || !reservation.invocationId || reservation.commandId !== target.commandId || !target.isCurrent()) return 'cancelled';
    if (isChatMessageAction(target.commandId) && !target.prepareAdmission) return 'cancelled';
    await target.prepareAdmission?.(ticket);
    if (!target.isCurrent()) return 'cancelled';
    takeRequested = true;
    const taken = await port.takeUICommand(ticket);
    handoffId = taken.handoffId;
    if (!handoffId || taken.ticket !== ticket || taken.invocationId !== reservation.invocationId || taken.commandId !== target.commandId) return 'cancelled';
    if (target.commandId === 'chat.message.delete') {
      // Take can resolve before React unmounts the accepted DecisionDialog.
      for (let i = 0; i < 10 && target.isCurrent() && !target.canCommit(); i++) {
        await new Promise(resolve => window.setTimeout(resolve, 20));
      }
    }
    target.finishAdmission?.();
    if (!target.canCommit()) return 'cancelled';
    submitted = true;
    if (target.commandId === 'chat.response.cancel') await port.commitBackendCommand(ticket, handoffId);
    else await target.execute({ ticket, handoffId });
    if (isMessageUIEffect(target.commandId)) await port.completeUICommand(ticket, handoffId, 'succeeded');
    return await reconcile();
  } catch (error) {
    if (error instanceof ChatMessagingStaleError) { submitted = false; return 'cancelled'; }
    if (submitted) {
      if (target.commandId === 'chat.message.send_to_editor' && !resultRequested) {
        // Opening the destination may already have created a tab. A lost
        // acknowledgement or partial renderer effect is not a proven failure.
        try { await port.cancelUICommand(ticket!); } catch { /* reconcile below */ }
        try { return await reconcile(); } catch { return 'outcome_unknown'; }
      }
      if (isMessageUIEffect(target.commandId) && !resultRequested) {
        // The local effect failed or its acknowledgement was lost. Do not
        // replay it and never report success from a resolved UI promise alone.
        try { await port.completeUICommand(ticket!, handoffId!, 'failed'); } catch { /* reconcile below */ }
      }
      if (resultRequested) return 'outcome_unknown';
      try { return await reconcile(); } catch { return 'outcome_unknown'; }
    }
    if (target.commandId === 'chat.message.delete' && takeRequested && ticket) {
      // A cancelled backend decision can terminate admission before a handoff.
      // Preserve that authoritative cancellation instead of announcing a failure.
      try {
        const result = await port.getUICommandResult(ticket);
        if (result.invocationId === invocationId &&
            ['cancelled', 'cancelled_stale', 'denied', 'timed_out', 'rejected_stale'].includes(result.status)) return result.status;
      } catch { /* no effect was submitted */ }
    }
    return 'failed';
  } finally {
    if (!submitted && ticket) {
      try { if (handoffId) await port.completeUICommand(ticket, handoffId, 'cancelled'); else await port.cancelUICommand(ticket); } catch { /* expiry owns cleanup */ }
    }
    target.dispose();
  }
}
