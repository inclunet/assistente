import { EventsOn } from '@wailsjs/runtime/runtime';

export const COMMAND_DECK_FEEDBACK_EVENT = 'command:deck-feedback';
export const COMMAND_DECK_STATE_EVENT = 'command:deck-state';
export const COMMAND_DECK_FEEDBACK_MAX_FUTURE_MS = 10_000;
// The backend normalizes titles to at most 256 Unicode code points. UTF-16
// strings can use two code units per code point, so leave room for that full
// contract at the frontend boundary.
export const COMMAND_DECK_FEEDBACK_MAX_TITLE_LENGTH = 512;
export const COMMAND_DECK_FEEDBACK_MAX_INVOCATIONS = 64;

export const COMMAND_DECK_FEEDBACK_STATES = [
  'on',
  'off',
  'waiting',
  'running',
  'succeeded',
  'failed',
  'denied',
  'cancelled',
  'timed_out',
  'outcome_unknown',
] as const;

export type CommandDeckFeedbackState = typeof COMMAND_DECK_FEEDBACK_STATES[number];

export interface CommandDeckFeedbackEvent {
  readonly invocationId: string;
  readonly state: CommandDeckFeedbackState;
  readonly title: string;
  readonly userId: string;
  readonly sessionId: string;
  readonly workspaceId: string;
  readonly generation: string;
  readonly expiresAt: number;
}

export interface CommandDeckFeedbackOwner {
  readonly userId: string;
  readonly sessionId: string;
  readonly workspaceId: string;
}

export interface CommandDeckStateEvent extends Omit<CommandDeckFeedbackEvent, 'invocationId' | 'state'> {
  readonly eventId: string;
  readonly state: 'on' | 'off';
}

export interface CommandDeckFeedbackContext {
  readonly authenticated: boolean;
  readonly owner: CommandDeckFeedbackOwner | null;
  readonly workspaceId: string | null;
  readonly generation: string | null;
  readonly paletteDeadline: number;
}

export interface SubscribeCommandDeckFeedbackOptions {
  readonly announce: (message: string) => void;
  readonly getContext: () => CommandDeckFeedbackContext;
  readonly translate: (state: CommandDeckFeedbackState, title: string) => string;
  readonly now?: () => number;
  readonly requireFocus?: boolean;
  readonly hasFocus?: () => boolean;
}

type Progress = { readonly rank: number; readonly terminal: boolean };

const TERMINAL_STATES = new Set<CommandDeckFeedbackState>([
  'on',
  'off',
  'succeeded',
  'failed',
  'denied',
  'cancelled',
  'timed_out',
  'outcome_unknown',
]);

const STATE_RANK: Record<CommandDeckFeedbackState, number> = {
  on: 3,
  off: 3,
  waiting: 1,
  running: 2,
  succeeded: 3,
  failed: 3,
  denied: 3,
  cancelled: 3,
  timed_out: 3,
  outcome_unknown: 3,
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}

function isBoundedNonEmptyString(value: unknown, maxLength: number): value is string {
  return typeof value === 'string' && value.length > 0 && value.length <= maxLength &&
    value.trim() === value && !/[\u0000-\u001f\u007f]/u.test(value);
}

function parseEvent(raw: unknown): CommandDeckFeedbackEvent | null {
  if (!isRecord(raw) ||
      !isBoundedNonEmptyString(raw.invocationId, 256) ||
      !isBoundedNonEmptyString(raw.title, COMMAND_DECK_FEEDBACK_MAX_TITLE_LENGTH) ||
      !isBoundedNonEmptyString(raw.userId, 256) ||
      !isBoundedNonEmptyString(raw.sessionId, 256) ||
      !isBoundedNonEmptyString(raw.workspaceId, 256) ||
      !isBoundedNonEmptyString(raw.generation, 256) ||
      typeof raw.expiresAt !== 'number' || !Number.isSafeInteger(raw.expiresAt) ||
      typeof raw.state !== 'string' ||
      !COMMAND_DECK_FEEDBACK_STATES.includes(raw.state as CommandDeckFeedbackState)) return null;

  return {
    invocationId: raw.invocationId,
    state: raw.state as CommandDeckFeedbackState,
    title: raw.title,
    userId: raw.userId,
    sessionId: raw.sessionId,
    workspaceId: raw.workspaceId,
    generation: raw.generation,
    expiresAt: raw.expiresAt,
  };
}

function matchesContext(event: CommandDeckFeedbackEvent, context: CommandDeckFeedbackContext, now: number): boolean {
  const owner = context.owner;
  return context.authenticated && owner !== null && context.workspaceId !== null &&
    owner.userId === event.userId && owner.sessionId === event.sessionId &&
    owner.workspaceId === event.workspaceId && context.workspaceId === event.workspaceId &&
    context.generation === event.generation &&
    (context.paletteDeadline === 0 || (Number.isSafeInteger(context.paletteDeadline) && now < context.paletteDeadline));
}

/**
 * Consumes backend Stream Deck lifecycle feedback through the shared announcer.
 * Context and focus are read for every event so stale frames cannot speak after
 * logout, workspace/map changes, or a background transition.
 */
export function subscribeCommandDeckFeedback(options: SubscribeCommandDeckFeedbackOptions): () => void {
  let disposed = false;
  const progressByInvocation = new Map<string, Progress>();
  const now = options.now ?? Date.now;
  const requireFocus = options.requireFocus === true;
  const hasFocus = options.hasFocus ?? (() => document.hasFocus());

  const onEvent = (raw: unknown, persistent = false) => {
    if (disposed) return;
    const event = parseEvent(raw);
    if (!event) return;
    if ((event.state === 'on' || event.state === 'off') !== persistent) return;
    const timestamp = now();
    if (!Number.isSafeInteger(timestamp) || event.expiresAt <= timestamp ||
        event.expiresAt > timestamp + COMMAND_DECK_FEEDBACK_MAX_FUTURE_MS ||
        !matchesContext(event, options.getContext(), timestamp) ||
        requireFocus && !hasFocus()) return;

    const previous = progressByInvocation.get(event.invocationId);
    const rank = STATE_RANK[event.state];
    if (previous && (previous.terminal || rank <= previous.rank)) return;

    const terminal = TERMINAL_STATES.has(event.state);
    if (progressByInvocation.size >= COMMAND_DECK_FEEDBACK_MAX_INVOCATIONS && !previous) {
      const oldest = progressByInvocation.keys().next().value;
      if (oldest !== undefined) progressByInvocation.delete(oldest);
    }
    progressByInvocation.set(event.invocationId, { rank, terminal });

    try {
      options.announce(options.translate(event.state, event.title));
    } catch {
      // Announcement is best-effort; malformed presentation must not break the
      // Wails event dispatcher or revive an already consumed lifecycle state.
    }
  };

  const unsubscribe = EventsOn(COMMAND_DECK_FEEDBACK_EVENT, (raw: unknown) => onEvent(raw));
  const unsubscribeState = EventsOn(COMMAND_DECK_STATE_EVENT, (raw: unknown) => {
    if (!isRecord(raw) || !isBoundedNonEmptyString(raw.eventId, 256)) return;
    // Persistent state changes have their own event identity, never a fabricated
    // command invocation or an execution-success claim.
    onEvent({ ...raw, invocationId: `presentation:${raw.eventId}` }, true);
  });
  return () => {
    if (disposed) return;
    disposed = true;
    unsubscribe();
    unsubscribeState();
    progressByInvocation.clear();
  };
}
