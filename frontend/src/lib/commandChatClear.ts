import { getModalRegistrySnapshot } from './modalRegistry';
import { ReadFocusContext } from './commandContextProviders';
import type { CommandContextualBackendPort } from './commandContextualBackendExecution';

export const CHAT_CLEAR_COMMAND = 'chat.conversation.clear';
export const CHAT_CLEAR_EVENT = 'commands:chat-clear';
interface Surface {
  root: HTMLElement;
  instanceId: string;
  modalId?: string;
  isCurrent(): boolean;
  canStart(keyboardTarget?: EventTarget | null): boolean;
  subscribe(changed: () => void): () => void;
  succeeded(): void;
}
export interface ChatClearTarget {
  isCurrent(): boolean;
  canCommit(): boolean;
  succeeded(): void;
  dispose(): void;
}
const surfaces = new Set<Surface>();
export function registerChatClearSurface(surface: Surface): () => void {
  surfaces.add(surface);
  return () => { surfaces.delete(surface); };
}
export function requestChatClear(instanceId: string) {
  window.dispatchEvent(new CustomEvent(CHAT_CLEAR_EVENT, { detail: { instanceId }, cancelable: true }));
}
export function captureChatClearTarget(readPathname: () => string, expectedInstance?: string, keyboardTarget?: EventTarget | null): ChatClearTarget | undefined {
  if (ReadFocusContext().composition === 'active') return undefined;
  const candidates = [...surfaces].filter(surface => surface.root.isConnected && surface.isCurrent() && surface.canStart(keyboardTarget) &&
    (expectedInstance === undefined || surface.instanceId === expectedInstance));
  if (candidates.length !== 1) return undefined;
  const source = candidates[0];
  const pathname = readPathname();
  let invalid = false;
  let disposed = false;
  const isCurrent = () => {
    const valid = !disposed && !invalid && surfaces.has(source) && source.root.isConnected && source.isCurrent() &&
      pathname === readPathname() && (!source.modalId || getModalRegistrySnapshot().ids.includes(source.modalId));
    if (!valid) invalid = true;
    return valid;
  };
  const unsubscribe = source.subscribe(() => { isCurrent(); });
  return {
    isCurrent,
    canCommit: () => isCurrent() && document.hasFocus() && ReadFocusContext().composition !== 'active' && source.canStart(),
    succeeded: () => { if (isCurrent()) source.succeeded(); },
    dispose() { if (disposed) return; disposed = true; unsubscribe(); },
  };
}

/** The backend owns the destructive decision and single-use receipt. Do not
 * capture a focus guard before that dialog: its own focus transition is expected.
 * The immutable conversation lease remains pinned throughout admission/decision. */
export async function executeChatClear(port: CommandContextualBackendPort, target: ChatClearTarget) {
  let ticket: string | undefined;
  let handoff: string | undefined;
  let commitStarted = false;
  try {
    if (!target.isCurrent()) return 'cancelled';
    const reservation = await port.beginUICommand(CHAT_CLEAR_COMMAND);
    ticket = reservation.ticket;
    if (!ticket || !reservation.invocationId || reservation.commandId !== CHAT_CLEAR_COMMAND || !target.isCurrent()) return 'cancelled';
    const taken = await port.takeUICommand(ticket);
    handoff = taken.handoffId;
    if (!handoff || taken.ticket !== ticket || taken.commandId !== CHAT_CLEAR_COMMAND || taken.invocationId !== reservation.invocationId || !target.isCurrent()) return 'cancelled';
    // Receipt acceptance resolves before React removes DecisionDialog. Allow
    // that cleanup only; never continue through a different blocking modal.
    for (let i = 0; i < 10 && target.isCurrent() && !target.canCommit(); i++) {
      await new Promise(resolve => window.setTimeout(resolve, 20));
    }
    if (!target.canCommit()) return 'cancelled';
    commitStarted = true;
    await port.commitBackendCommand(ticket, handoff);
    const result = await port.getUICommandResult(ticket);
    if (result.invocationId !== reservation.invocationId) return 'outcome_unknown';
    if (result.status === 'succeeded') {
      // A presentation failure must not turn a confirmed deletion into an
      // uncertain backend outcome or invite another destructive attempt.
      try { target.succeeded(); } catch { /* The committed result is authoritative. */ }
    }
    return result.status;
  } catch {
    // Never retry a destructive commit after an uncertain transport response.
    return commitStarted ? 'outcome_unknown' : 'cancelled';
  } finally {
    if (!commitStarted && ticket) {
      try {
        if (handoff) await port.completeUICommand(ticket, handoff, 'cancelled');
        else await port.cancelUICommand(ticket);
      } catch { /* Backend lease expiry still prevents reuse. */ }
    }
    target.dispose();
  }
}
