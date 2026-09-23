import { afterEach, describe, expect, it, vi } from 'vitest';
import { prepareChatEditorCommand, openChatEditorCommand, validateChatEditorCommand } from './commandChatEditorWails';

afterEach(() => vi.unstubAllGlobals());
function host() {
  const api = {
    PrepareChatEditorCommand: vi.fn(async () => ({ tabId: 'editor', draftId: 'draft' })),
    OpenChatEditorCommand: vi.fn(async () => ({ tabId: 'editor', draftId: 'draft', workspace: { id: 'not-a-store-update' } })),
    ValidateChatEditorCommand: vi.fn(async () => {}),
  };
  vi.stubGlobal('go', { app: { App: api } });
  return api;
}
describe('transporte de transferência chat → editor', () => {
  it('envia identidade e conteúdo original na preparação; aplica apenas plano imutável', async () => {
    const api = host();
    const prepared = await prepareChatEditorCommand('ticket', 'message', 'original', 'editor');
    const opened = await openChatEditorCommand('ticket', 'handoff', 'Title');
    await validateChatEditorCommand('ticket', 'handoff');
    expect(api.PrepareChatEditorCommand).toHaveBeenCalledExactlyOnceWith('ticket', 'message', 'original', 'editor');
    expect(api.OpenChatEditorCommand).toHaveBeenCalledExactlyOnceWith('ticket', 'handoff', 'Title');
    expect(api.ValidateChatEditorCommand).toHaveBeenCalledExactlyOnceWith('ticket', 'handoff');
    expect(Object.isFrozen(prepared)).toBe(true);
    expect(Object.isFrozen(opened)).toBe(true);
    expect(opened).not.toHaveProperty('workspace');
  });
  it('host incompleto não aciona criação legada', async () => {
    const AddTab = vi.fn();
    vi.stubGlobal('go', { app: { App: { AddTab } } });
    await expect(prepareChatEditorCommand('t', 'm', 'original', '')).rejects.toThrow('unavailable');
    await expect(openChatEditorCommand('t', 'h', '')).rejects.toThrow('unavailable');
    await expect(validateChatEditorCommand('t', 'h')).rejects.toThrow('unavailable');
    expect(AddTab).not.toHaveBeenCalled();
  });
  it.each(['', '   '])('rejeita destino vazio %j sem repetir chamada', async tabId => {
    const api = host();
    api.PrepareChatEditorCommand.mockResolvedValue({ tabId, draftId: 'draft' });
    await expect(prepareChatEditorCommand('t', 'm', 'original', '')).rejects.toThrow('Invalid');
    expect(api.PrepareChatEditorCommand).toHaveBeenCalledOnce();
  });
  it('erro de transporte não faz replay', async () => {
    const api = host(); api.OpenChatEditorCommand.mockRejectedValue(new Error('lost response'));
    await expect(openChatEditorCommand('t', 'h', '')).rejects.toThrow('lost response');
    expect(api.OpenChatEditorCommand).toHaveBeenCalledOnce();
  });
});
