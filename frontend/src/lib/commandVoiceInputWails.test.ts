import { describe, expect, it, vi } from 'vitest';
import { createCommandVoiceInputWailsPort } from './commandVoiceInputWails';
import { VOICE_INPUT_COMMAND } from './commandVoiceInput';

vi.mock('./waitForWailsBridge', () => ({ waitForWailsBridge: vi.fn(async () => undefined) }));

function target(overrides: Record<string, unknown> = {}) {
  const app = {
    TakeGlobalVoiceCommand: vi.fn(async (ticket: string) => ({
      ticket, invocationId: 'invocation-1', commandId: VOICE_INPUT_COMMAND,
      handoffId: 'handoff-1', profile_slug: 'perfil-voz', trigger_type: 'hotkey', bring_to_front: true,
    })),
    CompleteUICommand: vi.fn(async () => undefined),
    GetUICommandResult: vi.fn(async () => ({ invocationId: 'invocation-1', status: 'succeeded' as const })),
    CancelUICommand: vi.fn(async () => undefined),
    AdmitGlobalCommandOccurrence: vi.fn(async () => true),
    ...overrides,
  };
  const windowTarget = { go: { app: { App: app } } } as unknown as Window;
  return { app, windowTarget };
}

describe('commandVoiceInputWails', () => {
  it('adapta o handoff autoritativo e a admissão de ocorrência', async () => {
    const { app, windowTarget } = target();
    const port = createCommandVoiceInputWailsPort({ target: windowTarget });
    await expect(port.takeGlobalVoiceCommand('ticket-1')).resolves.toMatchObject({ handoffId: 'handoff-1' });
    await port.completeUICommand('ticket-1', 'handoff-1', 'succeeded');
    await port.getUICommandResult('ticket-1');
    await port.cancelUICommand('ticket-1');
    await expect(port.admitGlobalCommandOccurrence('invocation-1', true, 'modal-generation-1')).resolves.toBe(true);

    expect(app.TakeGlobalVoiceCommand).toHaveBeenCalledWith('ticket-1');
    expect(app.CompleteUICommand).toHaveBeenCalledWith('ticket-1', 'handoff-1', 'succeeded');
    expect(app.GetUICommandResult).toHaveBeenCalledWith('ticket-1');
    expect(app.CancelUICommand).toHaveBeenCalledWith('ticket-1');
    expect(app.AdmitGlobalCommandOccurrence).toHaveBeenCalledWith('invocation-1', true);
  });

  it('revalida a admissão depois da espera do bridge e falha fechado', async () => {
    const { app, windowTarget } = target();
    const port = createCommandVoiceInputWailsPort({
      target: windowTarget,
      isGlobalJobAdmissionCurrent: () => false,
    });

    await expect(port.admitGlobalCommandOccurrence('invocation-1', true, 'modal-generation-1')).resolves.toBe(true);
    expect(app.AdmitGlobalCommandOccurrence).toHaveBeenCalledWith('invocation-1', false);
  });

  it('falha fechado quando a API global não está disponível', async () => {
    const port = createCommandVoiceInputWailsPort({ target: { go: { app: { App: {} } } } as unknown as Window });
    await expect(port.takeGlobalVoiceCommand('ticket-1')).rejects.toThrow('Global voice command Wails API is not available');
  });
});
