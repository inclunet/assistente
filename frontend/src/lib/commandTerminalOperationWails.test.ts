import { describe, expect, it, vi } from 'vitest';
import { createCommandTerminalOperationWailsPort } from './commandTerminalOperationWails';

describe('terminal interrupt bridge', () => {
  it('passes target assertions only to preparation and ticket/handoff to commit', async () => {
    const prepare = vi.fn(async () => undefined);
    const commit = vi.fn(async () => undefined);
    const target = { go: { app: { App: { PrepareTerminalInterruptCommand: prepare, CommitWorkspaceTabCommand: commit } } } } as unknown as Window;
    const port = createCommandTerminalOperationWailsPort({ target });
    await port.prepareTerminalInterruptCommand('ticket', 'workspace', 'tab', 'session', 'command');
    expect(prepare).toHaveBeenCalledExactlyOnceWith('ticket', 'workspace', 'tab', 'session', 'command');
    expect(commit).not.toHaveBeenCalled();
    const pending = port.commitBackendCommand('ticket', 'handoff');
    expect(commit).toHaveBeenCalledExactlyOnceWith('ticket', 'handoff');
    await pending;
  });
  it('rejects missing preparation instead of invoking the legacy interrupt API', async () => {
    const legacy = vi.fn();
    const target = { go: { app: { App: { InterruptTerminalCommand: legacy } } } } as unknown as Window;
    const port = createCommandTerminalOperationWailsPort({ target });
    await expect(port.prepareTerminalInterruptCommand('ticket', 'workspace', 'tab', 'session', 'command')).rejects.toThrow('unavailable');
    expect(legacy).not.toHaveBeenCalled();
  });
  it('propagates preparation refusal without submitting a commit', async () => {
    const prepare = vi.fn(async () => { throw new Error('stale'); });
    const commit = vi.fn();
    const target = { go: { app: { App: { PrepareTerminalInterruptCommand: prepare, CommitWorkspaceTabCommand: commit } } } } as unknown as Window;
    await expect(createCommandTerminalOperationWailsPort({ target }).prepareTerminalInterruptCommand('t', 'w', 'tab', 's', 'c')).rejects.toThrow('stale');
    expect(commit).not.toHaveBeenCalled();
  });
  it('encaminha a preparação de criação/fechamento para a API de sessão', async () => {
    const prepare = vi.fn(async () => undefined);
    const target = { go: { app: { App: { PrepareTerminalSessionCommand: prepare } } } } as unknown as Window;
    const port = createCommandTerminalOperationWailsPort({ target });

    await port.prepareTerminalSessionCommand('ticket', 'workspace', 'tab', 'old-session');

    expect(prepare).toHaveBeenCalledExactlyOnceWith('ticket', 'workspace', 'tab', 'old-session');
  });
});
