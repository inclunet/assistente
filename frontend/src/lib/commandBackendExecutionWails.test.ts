import { describe, expect, it, vi } from 'vitest';
import { createCommandBackendExecutionWailsPort } from './commandBackendExecutionWails';
vi.mock('./waitForWailsBridge', () => ({ waitForWailsBridge: vi.fn(async () => undefined) }));
describe('commandBackendExecutionWails', () => {
  it('adapta ExecutePaletteCommand e envia args vazios', async () => {
    const app = { ExecutePaletteCommand: vi.fn(async (commandID: string) => ({ invocationId: 'i', status: 'succeeded', commandID })) }; const target = { go: { app: { App: app } } } as unknown as Window;
    const port = createCommandBackendExecutionWailsPort({ target }); await port.executeCommand('workspace.list', {}); expect(app.ExecutePaletteCommand).toHaveBeenCalledWith('workspace.list', {});
  });
  it('falha fechado sem API', async () => {
    const port = createCommandBackendExecutionWailsPort({ target: { go: { app: { App: {} } } } as unknown as Window }); await expect(port.executeCommand('workspace.list', {})).rejects.toThrow('not available');
  });
});
