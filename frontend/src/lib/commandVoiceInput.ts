import {
  captureVoiceHotkeyTarget,
  type VoiceHotkeyEvent,
  type VoiceHotkeyTarget,
} from '../services/voiceHotkeyRouting';
import type {
  CommandUIExecutionPort,
  UICommandResult,
  UICommandStatus,
} from './commandUIExecution';

export const VOICE_INPUT_COMMAND = 'voice.input.activate';

export interface VoiceInputReservationPayload {
  readonly ticket: string;
  readonly invocationId: string;
  readonly commandId: typeof VOICE_INPUT_COMMAND;
  readonly profile_slug: string;
  readonly trigger_type: string;
  readonly bring_to_front: boolean;
}

export interface VoiceInputPreparedPayload {
  readonly profile_slug: string;
  readonly trigger_type: string;
  readonly bring_to_front: boolean;
}

export interface VoiceInputHandoff {
  readonly ticket: string;
  readonly invocationId: string;
  readonly commandId: typeof VOICE_INPUT_COMMAND;
  readonly handoffId: string;
  readonly profile_slug: string;
  readonly trigger_type: string;
  readonly bring_to_front: boolean;
}

export interface VoiceInputCommandPort extends Pick<CommandUIExecutionPort, 'completeUICommand' | 'getUICommandResult' | 'cancelUICommand'> {
  takeGlobalVoiceCommand(ticket: string): Promise<VoiceInputHandoff>;
}

export interface VoiceInputTarget extends VoiceHotkeyTarget {
  readonly commandId: typeof VOICE_INPUT_COMMAND;
  readonly payload: VoiceInputPreparedPayload;
}

const RESERVATION_KEYS = ['ticket', 'invocationId', 'commandId', 'profile_slug', 'trigger_type', 'bring_to_front'] as const;
const RESULT_STATUSES: readonly UICommandStatus[] = [
  'succeeded', 'failed', 'denied', 'cancelled', 'cancelled_stale', 'timed_out',
  'outcome_unknown', 'suppressed', 'rejected_stale',
];

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function hasOnlyKeys(value: Record<string, unknown>): boolean {
  return Object.keys(value).every(key => RESERVATION_KEYS.includes(key as typeof RESERVATION_KEYS[number]));
}

function validText(value: unknown): value is string {
  return typeof value === 'string' && value.length > 0 && value.length <= 4096 && value.trim() === value;
}

function validReservationResult(value: unknown, invocationId: string): value is UICommandResult {
  if (!isRecord(value) || value.invocationId !== invocationId || !validText(value.invocationId) ||
      typeof value.status !== 'string' || !RESULT_STATUSES.includes(value.status as UICommandStatus)) return false;
  return (value.resultSummary === undefined || value.resultSummary === null || typeof value.resultSummary === 'string') &&
    (value.errorCode === undefined || value.errorCode === null || typeof value.errorCode === 'string');
}

function validTake(value: unknown, reservation: VoiceInputReservationPayload): value is VoiceInputHandoff {
  return isRecord(value) && value.ticket === reservation.ticket &&
    value.invocationId === reservation.invocationId && value.commandId === VOICE_INPUT_COMMAND &&
    validText(value.handoffId) && value.profile_slug === reservation.profile_slug &&
    value.trigger_type === reservation.trigger_type && value.bring_to_front === reservation.bring_to_front;
}

function localResult(invocationId: string, status: UICommandStatus, errorCode: string): UICommandResult {
  return { invocationId, status, errorCode };
}

export function parseVoiceInputReservation(data: unknown): VoiceInputReservationPayload | null {
  if (!isRecord(data) || !hasOnlyKeys(data) ||
      !validText(data.ticket) || !validText(data.invocationId) || data.commandId !== VOICE_INPUT_COMMAND ||
      !validText(data.profile_slug) || !validText(data.trigger_type) || typeof data.bring_to_front !== 'boolean') {
    return null;
  }

  return Object.freeze({
    ticket: data.ticket,
    invocationId: data.invocationId,
    commandId: VOICE_INPUT_COMMAND,
    profile_slug: data.profile_slug,
    trigger_type: data.trigger_type,
    bring_to_front: data.bring_to_front,
  });
}

export function captureVoiceInputTarget(data: unknown): VoiceInputTarget | undefined {
  const reservation = parseVoiceInputReservation(data);
  if (!reservation) return undefined;

  const event: VoiceHotkeyEvent = {
    triggerType: reservation.trigger_type,
    bringToFront: reservation.bring_to_front,
  };
  const captured = captureVoiceHotkeyTarget(reservation.profile_slug, event);
  if (!captured) return undefined;

  return {
    commandId: VOICE_INPUT_COMMAND,
    payload: Object.freeze({
      profile_slug: reservation.profile_slug,
      trigger_type: reservation.trigger_type,
      bring_to_front: reservation.bring_to_front,
    }),
    isCurrent: captured.isCurrent,
    execute: captured.execute,
    dispose: captured.dispose,
  };
}

/**
 * Consome uma reserva global emitida pelo backend. O alvo é capturado antes de
 * qualquer await e permanece fixo durante Take/Complete; o payload do evento
 * apenas restringe qual receptor de voz pode ser usado.
 */
export async function executeGlobalVoiceReservation(
  data: unknown,
  port: VoiceInputCommandPort,
): Promise<UICommandResult> {
  const reservation = parseVoiceInputReservation(data);
  if (!reservation) return localResult('', 'cancelled', 'malformed-reservation');

  let target: VoiceInputTarget | undefined;
  try {
    target = captureVoiceInputTarget(reservation);
  } catch {
    target = undefined;
  }
  if (!target) {
    try { await port.cancelUICommand(reservation.ticket); } catch { /* expiry/reconciliation owns the ticket */ }
    return localResult(reservation.invocationId, 'cancelled', 'voice-target-unavailable');
  }

  const readResult = async (fallbackCode: string): Promise<UICommandResult> => {
    try {
      const result = await port.getUICommandResult(reservation.ticket);
      return validReservationResult(result, reservation.invocationId)
        ? result
        : localResult(reservation.invocationId, 'outcome_unknown', fallbackCode);
    } catch {
      return localResult(reservation.invocationId, 'outcome_unknown', fallbackCode);
    }
  };

  let handoffId: string | undefined;
  let taken = false;
  try {
    if (!target.isCurrent()) {
      try { await port.cancelUICommand(reservation.ticket); } catch { /* best effort */ }
      return localResult(reservation.invocationId, 'cancelled', 'voice-target-stale');
    }

    let handoff: VoiceInputHandoff;
    try {
      handoff = await port.takeGlobalVoiceCommand(reservation.ticket);
    } catch {
      try { await port.cancelUICommand(reservation.ticket); } catch { /* expiry/reconciliation owns the ticket */ }
      return localResult(reservation.invocationId, 'outcome_unknown', 'take-confirmation-unknown');
    }
    if (!validTake(handoff, reservation)) {
      try { await port.cancelUICommand(reservation.ticket); } catch { /* expiry/reconciliation owns the ticket */ }
      return localResult(reservation.invocationId, 'outcome_unknown', 'malformed-take');
    }
    handoffId = handoff.handoffId;
    taken = true;

    if (!target.isCurrent()) {
      await port.completeUICommand(reservation.ticket, handoffId, 'cancelled').catch(() => undefined);
      return readResult('voice-target-stale');
    }

    let executed: boolean;
    try {
      executed = target.execute();
    } catch {
      // The synchronous effect may have started before throwing. Do not claim
      // cancellation or replay it; let the broker reconcile the outcome.
      try { await port.cancelUICommand(reservation.ticket); } catch { /* outcome query below is authoritative */ }
      return readResult('voice-input-effect-unknown');
    }
    if (!executed) {
      await port.completeUICommand(reservation.ticket, handoffId, 'cancelled').catch(() => undefined);
      return readResult('voice-target-stale');
    }

    try {
      await port.completeUICommand(reservation.ticket, handoffId, 'succeeded');
    } catch {
      // The synchronous effect has already run; reconcile instead of replaying it.
    }
    return readResult('complete-confirmation-unknown');
  } catch {
    if (taken && handoffId) {
      try { await port.cancelUICommand(reservation.ticket); } catch { /* outcome query below is authoritative */ }
      return readResult('voice-input-outcome-unknown');
    }
    try { await port.cancelUICommand(reservation.ticket); } catch { /* expiry owns cleanup */ }
    return localResult(reservation.invocationId, 'outcome_unknown', 'voice-input-outcome-unknown');
  } finally {
    target.dispose();
  }
}
