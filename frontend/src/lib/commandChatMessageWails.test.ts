import { afterEach, describe, expect, it, vi } from 'vitest';
import { commitChatMessageCommand, prepareChatMessageCommand } from './commandChatMessageWails';

afterEach(() => vi.unstubAllGlobals());
describe('porta Wails de ações sobre mensagens', () => {
  it('transmite só ticket e ID, sem conteúdo ou autoridade declarada pelo chamador', async () => {
    const PrepareChatMessageCommand = vi.fn(async () => {});
    const CommitChatMessageCommand = vi.fn(async () => {});
    vi.stubGlobal('go', { app: { App: { PrepareChatMessageCommand, CommitChatMessageCommand } } });
    await prepareChatMessageCommand('ticket', 'message');
    await commitChatMessageCommand('ticket', 'handoff');
    expect(PrepareChatMessageCommand).toHaveBeenCalledExactlyOnceWith('ticket', 'message');
    expect(CommitChatMessageCommand).toHaveBeenCalledExactlyOnceWith('ticket', 'handoff');
  });
  it('não usa o endpoint legado quando o host não suporta o contrato', async () => {
    const DeleteMessage = vi.fn();
    vi.stubGlobal('go', { wailsapi: { Conversations: { DeleteMessage } } });
    await expect(prepareChatMessageCommand('t', 'm')).rejects.toThrow('unavailable');
    await expect(commitChatMessageCommand('t', 'h')).rejects.toThrow('unavailable');
    expect(DeleteMessage).not.toHaveBeenCalled();
  });
});
