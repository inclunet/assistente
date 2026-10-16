import { describe, expect, it, vi } from 'vitest';
import { createCommandUIExecutionWailsPort } from './commandUIExecutionWails';

vi.mock('./waitForWailsBridge', () => ({ waitForWailsBridge: vi.fn(async () => undefined) }));

function target() {
  const app = {
    BeginUICommand: vi.fn(async (commandId: string) => ({ ticket: 't', invocationId: 'i', commandId })),
    TakeUICommand: vi.fn(async (ticket: string) => ({ ticket, invocationId: 'i', commandId: 'help.shortcuts.show', handoffId: 'h' })),
    CompleteUICommand: vi.fn(async () => undefined),
    GetUICommandResult: vi.fn(async () => ({ invocationId: 'i', status: 'succeeded' as const })),
    CancelUICommand: vi.fn(async () => undefined),
  };
  const windowTarget = { go: { app: { App: app } } } as unknown as Window;
  return { app, windowTarget };
}

describe('commandUIExecutionWails', () => {
  it('adapta os cinco métodos do App sem tocar bindings gerados', async () => {
    const { app, windowTarget } = target();
    const port = createCommandUIExecutionWailsPort({ target: windowTarget });
    await expect(port.beginUICommand('help.shortcuts.show')).resolves.toMatchObject({ ticket: 't' });
    await expect(port.takeUICommand('t')).resolves.toMatchObject({ handoffId: 'h' });
    await expect(port.completeUICommand('t', 'h', 'succeeded')).resolves.toBeUndefined();
    await expect(port.getUICommandResult('t')).resolves.toMatchObject({ invocationId: 'i' });
    await expect(port.cancelUICommand('t')).resolves.toBeUndefined();
    expect(app.BeginUICommand).toHaveBeenCalledWith('help.shortcuts.show');
    expect(app.CompleteUICommand).toHaveBeenCalledWith('t', 'h', 'succeeded');
    expect(app.GetUICommandResult).toHaveBeenCalledWith('t');
    expect(app.CancelUICommand).toHaveBeenCalledWith('t');
  });

  it('falha fechado quando a API ainda não está publicada', async () => {
    const port = createCommandUIExecutionWailsPort({ target: { go: { app: { App: {} } } } as unknown as Window });
    await expect(port.beginUICommand('help.shortcuts.show')).rejects.toThrow('Command UI execution Wails API is not available');
  });
});
