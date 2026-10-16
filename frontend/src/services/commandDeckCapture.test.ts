import { describe, expect, it, vi } from 'vitest';
import { beginCommandDeckCapture, cancelCommandDeckCapture } from './commandDeckCapture';

vi.mock('../lib/waitForWailsBridge', () => ({ waitForWailsBridge: vi.fn(async () => undefined) }));

describe('commandDeckCapture', () => {
  it('adapta a fachada Wails de captura sem depender de bindings gerados', async () => {
    const app = {
      BeginCommandDeckCapture: vi.fn(async () => undefined),
      CancelCommandDeckCapture: vi.fn(async () => undefined),
    };
    const target = { go: { app: { App: app } } } as unknown as Window;

    await beginCommandDeckCapture('request-1', target as never);
    await cancelCommandDeckCapture('request-1', target as never);

    expect(app.BeginCommandDeckCapture).toHaveBeenCalledWith('request-1');
    expect(app.CancelCommandDeckCapture).toHaveBeenCalledWith('request-1');
  });

  it('falha fechado quando a fachada não está disponível', async () => {
    const target = { go: { app: { App: {} } } } as unknown as Window;
    await expect(beginCommandDeckCapture('request-2', target as never)).rejects.toThrow(
      'Command Deck capture Wails API is not available'
    );
  });
});
