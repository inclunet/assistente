/** @vitest-environment jsdom */
import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  captureVoiceInputTarget,
  executeGlobalVoiceReservation,
  parseVoiceInputReservation,
  VOICE_INPUT_COMMAND,
  type VoiceInputCommandPort,
  type VoiceInputHandoff,
} from './commandVoiceInput';
import { registerVoiceHotkeyRecipient } from '../services/voiceHotkeyRouting';
import { unregisterOpenModal } from './modalRegistry';

vi.mock('@wailsjs/runtime/runtime', () => ({ EventsOn: vi.fn(() => () => undefined) }));

const cleanups: Array<() => void> = [];
const reservation = {
  ticket: 'ticket-1',
  invocationId: 'invocation-1',
  commandId: VOICE_INPUT_COMMAND as typeof VOICE_INPUT_COMMAND,
  profile_slug: 'perfil-voz',
  trigger_type: 'hotkey',
  bring_to_front: true,
} as const;

function handoff(overrides: Partial<VoiceInputHandoff> = {}): VoiceInputHandoff {
  return {
    ticket: reservation.ticket,
    invocationId: reservation.invocationId,
    commandId: VOICE_INPUT_COMMAND,
    handoffId: 'handoff-1',
    profile_slug: reservation.profile_slug,
    trigger_type: reservation.trigger_type,
    bring_to_front: reservation.bring_to_front,
    ...overrides,
  };
}

function port(overrides: Partial<VoiceInputCommandPort> = {}): VoiceInputCommandPort {
  return {
    takeGlobalVoiceCommand: vi.fn(async () => handoff()),
    completeUICommand: vi.fn(async () => undefined),
    getUICommandResult: vi.fn(async () => ({ invocationId: reservation.invocationId, status: 'succeeded' as const })),
    cancelUICommand: vi.fn(async () => undefined),
    ...overrides,
  };
}

function register(eligible: () => boolean = () => true, deliver = vi.fn(), getProfileGeneration: () => number = () => 0) {
  cleanups.push(registerVoiceHotkeyRecipient({
    getProfileSlug: () => reservation.profile_slug,
    getProfileGeneration,
    isEligible: eligible,
    deliver,
  }));
  return deliver;
}

afterEach(() => {
  while (cleanups.length) cleanups.pop()?.();
  document.querySelectorAll('.modal-overlay').forEach(node => node.remove());
  unregisterOpenModal('voice-input-test-modal');
});

describe('commandVoiceInput', () => {
  it('valida a reserva e prepara somente os três metadados de voz', () => {
    expect(parseVoiceInputReservation(reservation)).toEqual(reservation);
    expect(parseVoiceInputReservation({ ...reservation, extra: true })).toBeNull();
    expect(parseVoiceInputReservation({ ...reservation, commandId: 'chat.message.send' })).toBeNull();
    expect(parseVoiceInputReservation({ ...reservation, bring_to_front: 'true' })).toBeNull();

    register();
    const target = captureVoiceInputTarget(reservation)!;
    expect(target.commandId).toBe(VOICE_INPUT_COMMAND);
    expect(target.payload).toEqual({
      profile_slug: 'perfil-voz', trigger_type: 'hotkey', bring_to_front: true,
    });
    target.dispose();
  });

  it('captura antes do Take e executa uma vez quando o handoff confirma o payload', async () => {
    const deliver = register();
    const p = port();
    const result = await executeGlobalVoiceReservation(reservation, p);

    expect(result).toMatchObject({ invocationId: 'invocation-1', status: 'succeeded' });
    expect(p.takeGlobalVoiceCommand).toHaveBeenCalledExactlyOnceWith('ticket-1');
    expect(p.completeUICommand).toHaveBeenCalledExactlyOnceWith('ticket-1', 'handoff-1', 'succeeded');
    expect(p.getUICommandResult).toHaveBeenCalledExactlyOnceWith('ticket-1');
    expect(p.cancelUICommand).not.toHaveBeenCalled();
    expect(deliver).toHaveBeenCalledExactlyOnceWith({ triggerType: 'hotkey', bringToFront: true });
  });

  it('não mente cancelamento quando o efeito síncrono lança depois de iniciar', async () => {
    const deliver = register(() => true, vi.fn(() => { throw new Error('stt-started-then-failed'); }));
    const p = port({
      getUICommandResult: vi.fn(async () => ({ invocationId: reservation.invocationId, status: 'outcome_unknown' as const })),
    });
    const result = await executeGlobalVoiceReservation(reservation, p);

    expect(result.status).toBe('outcome_unknown');
    expect(deliver).toHaveBeenCalledOnce();
    expect(p.cancelUICommand).toHaveBeenCalledExactlyOnceWith('ticket-1');
    expect(p.completeUICommand).not.toHaveBeenCalled();
  });

  it.each([
    ['profile_slug', { profile_slug: 'outro-perfil' }],
    ['trigger_type', { trigger_type: 'outro-trigger' }],
    ['bring_to_front', { bring_to_front: false }],
  ] as const)('recusa handoff com %s divergente antes do efeito', async (_field, mismatch) => {
    const deliver = register();
    const p = port({ takeGlobalVoiceCommand: vi.fn(async () => handoff(mismatch)) });
    const result = await executeGlobalVoiceReservation(reservation, p);

    expect(result.status).toBe('outcome_unknown');
    expect(deliver).not.toHaveBeenCalled();
    expect(p.completeUICommand).not.toHaveBeenCalled();
    expect(p.cancelUICommand).toHaveBeenCalledExactlyOnceWith('ticket-1');
  });

  it('revalida o alvo depois do Take e conclui cancelamento sem retarget', async () => {
    let eligible = true;
    const deliver = register(() => eligible);
    const p = port({
      takeGlobalVoiceCommand: vi.fn(async () => { eligible = false; return handoff(); }),
      getUICommandResult: vi.fn(async () => ({ invocationId: reservation.invocationId, status: 'cancelled' as const })),
    });
    const result = await executeGlobalVoiceReservation(reservation, p);

    expect(result.status).toBe('cancelled');
    expect(deliver).not.toHaveBeenCalled();
    expect(p.completeUICommand).toHaveBeenCalledExactlyOnceWith('ticket-1', 'handoff-1', 'cancelled');
    expect(p.cancelUICommand).not.toHaveBeenCalled();
  });

  it('invalida o mesmo receptor quando o perfil recarrega durante o Take', async () => {
    let generation = 1;
    const deliver = register(() => true, vi.fn(), () => generation);
    const p = port({
      takeGlobalVoiceCommand: vi.fn(async () => {
        generation += 1;
        return handoff();
      }),
      getUICommandResult: vi.fn(async () => ({ invocationId: reservation.invocationId, status: 'cancelled' as const })),
    });
    const result = await executeGlobalVoiceReservation(reservation, p);

    expect(result.status).toBe('cancelled');
    expect(deliver).not.toHaveBeenCalled();
    expect(p.completeUICommand).toHaveBeenCalledExactlyOnceWith('ticket-1', 'handoff-1', 'cancelled');
    expect(p.cancelUICommand).not.toHaveBeenCalled();
  });

  it('invalida o alvo quando um segundo receptor do mesmo perfil surge durante o Take', async () => {
    const first = register();
    const p = port({
      takeGlobalVoiceCommand: vi.fn(async () => {
        cleanups.push(register());
        return handoff();
      }),
      getUICommandResult: vi.fn(async () => ({ invocationId: reservation.invocationId, status: 'cancelled' as const })),
    });

    const result = await executeGlobalVoiceReservation(reservation, p);

    expect(result.status).toBe('cancelled');
    expect(first).not.toHaveBeenCalled();
    expect(p.completeUICommand).toHaveBeenCalledExactlyOnceWith('ticket-1', 'handoff-1', 'cancelled');
    expect(p.cancelUICommand).not.toHaveBeenCalled();
  });

  it('cancela reserva sem alvo ou com payload malformado', async () => {
    const p = port();
    expect(await executeGlobalVoiceReservation({ ...reservation, profile_slug: 'ausente' }, p)).toMatchObject({ status: 'cancelled' });
    expect(p.cancelUICommand).toHaveBeenCalledExactlyOnceWith('ticket-1');

    const malformed = { ...reservation, invocationId: '' };
    expect(await executeGlobalVoiceReservation(malformed, p)).toMatchObject({ status: 'cancelled', errorCode: 'malformed-reservation' });
    expect(p.takeGlobalVoiceCommand).not.toHaveBeenCalled();
  });
});
