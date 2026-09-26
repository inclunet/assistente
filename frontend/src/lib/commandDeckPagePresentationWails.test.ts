import { describe, expect, it, vi } from 'vitest';
import { createCommandDeckPagePresentationWailsPort } from './commandDeckPagePresentationWails';

vi.mock('./waitForWailsBridge', () => ({ waitForWailsBridge: vi.fn(async () => undefined) }));

describe('commandDeckPagePresentationWails', () => {
  it('publica e limpa somente página, geração e revisão monotônica', async () => {
    const app = {
      PublishCommandDeckPagePresentation: vi.fn(async (_appPage: string, _generation: string, _revision: number) => undefined),
      ClearCommandDeckPagePresentation: vi.fn(async (_generation: string, _revision: number) => undefined),
    };
    const port = createCommandDeckPagePresentationWailsPort({ target: { go: { app: { App: app } } } as unknown as Window });

    await port.publish('profiles', '019b4e53-5f14-7c23-8b55-442200000001');
    await port.clear('019b4e53-5f14-7c23-8b55-442200000001');

    const [, , publishRevision] = app.PublishCommandDeckPagePresentation.mock.calls[0];
    const [, clearRevision] = app.ClearCommandDeckPagePresentation.mock.calls[0];
    expect(app.PublishCommandDeckPagePresentation).toHaveBeenCalledExactlyOnceWith('profiles', '019b4e53-5f14-7c23-8b55-442200000001', publishRevision);
    expect(app.ClearCommandDeckPagePresentation).toHaveBeenCalledExactlyOnceWith('019b4e53-5f14-7c23-8b55-442200000001', clearRevision);
    expect(clearRevision).toBeGreaterThan(publishRevision);
  });

  it('falha fechado se os métodos Wails não estiverem disponíveis', async () => {
    const port = createCommandDeckPagePresentationWailsPort({ target: { go: { app: { App: {} } } } as unknown as Window });
    await expect(port.publish('settings', '019b4e53-5f14-7c23-8b55-442200000001')).rejects.toThrow('not available');
  });
});
