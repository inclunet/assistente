import { afterEach, describe, expect, it, vi } from 'vitest';
import { prepareChatMessageEditCommand } from './commandChatMessageWails';

afterEach(() => vi.unstubAllGlobals());

describe('preparação efêmera da edição Wails', () => {
  it('transmite base e rascunho distintos sem normalizar silenciosamente o texto', async () => {
    const PrepareChatMessageEditCommand = vi.fn(async () => {});
    vi.stubGlobal('go', { app: { App: { PrepareChatMessageEditCommand } } });
    await prepareChatMessageEditCommand('ticket', 'message', 'original\n', '  editado\n');
    expect(PrepareChatMessageEditCommand).toHaveBeenCalledExactlyOnceWith('ticket', 'message', 'original\n', '  editado\n');
  });

  it('host sem contrato não usa UpdateMessage ou preparação incompleta', async () => {
    const UpdateMessage = vi.fn();
    const PrepareChatMessageCommand = vi.fn();
    vi.stubGlobal('go', { app: { App: { PrepareChatMessageCommand } }, wailsapi: { Conversations: { UpdateMessage } } });
    await expect(prepareChatMessageEditCommand('ticket', 'message', 'original', 'novo')).rejects.toThrow('unavailable');
    expect(UpdateMessage).not.toHaveBeenCalled();
    expect(PrepareChatMessageCommand).not.toHaveBeenCalled();
  });

  it('propaga recusa sem repetir a preparação', async () => {
    const error = new Error('stale original');
    const PrepareChatMessageEditCommand = vi.fn(async () => { throw error; });
    vi.stubGlobal('go', { app: { App: { PrepareChatMessageEditCommand } } });
    await expect(prepareChatMessageEditCommand('ticket', 'message', 'old', 'new')).rejects.toBe(error);
    expect(PrepareChatMessageEditCommand).toHaveBeenCalledOnce();
  });
});
