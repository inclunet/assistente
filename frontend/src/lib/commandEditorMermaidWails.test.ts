import { afterEach, describe, expect, it, vi } from 'vitest';
import { beginEditorMermaidKey, captureEditorMermaidKeyboard } from './commandEditorMermaidWails';
import type { CommandShortcut } from './commandShortcut';
const key: CommandShortcut = { version: 1, code: 'KeyS', modifiers: ['Control'] };
const reservation = { ticket: 'ticket', invocationId: 'invocation', commandId: 'editor.mermaid.apply' };
const cleanup: Array<() => void> = [];
function host() {
  const api = { BeginEditorMermaidUIKey: vi.fn(async () => reservation), DispatchLocalCommandKey: vi.fn(async () => null), BeginUICommand: vi.fn() };
  vi.stubGlobal('go', { app: { App: api } }); return api;
}
afterEach(() => { cleanup.splice(0).forEach(dispose => dispose()); vi.unstubAllGlobals(); });
describe('atalho modal Mermaid mantém origem e libera ocorrência', () => {
  it.each([key, { ...key, modifiers: ['Meta'] }, { ...key, code: 'Enter' }] as CommandShortcut[])('encaminha somente combinação permitida %j', async shortcut => {
    const api = host(); expect(await beginEditorMermaidKey('generation', shortcut)).toEqual(reservation);
    expect(api.BeginEditorMermaidUIKey).toHaveBeenCalledExactlyOnceWith('generation', shortcut, false);
    expect(api.BeginUICommand).not.toHaveBeenCalled();
  });
  it('não usa origem paleta se endpoint modal estiver ausente', async () => {
    vi.stubGlobal('go', { app: { App: { BeginUICommand: vi.fn() } } });
    await expect(beginEditorMermaidKey('g', key)).rejects.toThrow('unavailable');
  });
  it('combinação inválida é recusada antes do transporte', async () => {
    const api = host();
    await expect(beginEditorMermaidKey('g', { ...key, modifiers: ['Alt'] })).rejects.toThrow('Invalid');
    expect(api.BeginEditorMermaidUIKey).not.toHaveBeenCalled();
  });
  it('keyup anterior ao Begin aguarda reserva, sem perder release nem duplicar Begin', async () => {
    const api = host(); const lifetime = captureEditorMermaidKeyboard('g', key); cleanup.push(lifetime.dispose);
    window.dispatchEvent(new KeyboardEvent('keyup', { code: 'KeyS' }));
    expect(api.DispatchLocalCommandKey).not.toHaveBeenCalled();
    await lifetime.begin(); await lifetime.begin(); await vi.waitFor(() => expect(api.DispatchLocalCommandKey).toHaveBeenCalledOnce());
    expect(api.DispatchLocalCommandKey).toHaveBeenCalledWith('g', key, 'up', false);
    expect(api.BeginEditorMermaidUIKey).toHaveBeenCalledOnce();
  });
  it('cleanup sem admissão não envia evento solto ao backend', async () => {
    const api = host(); const lifetime = captureEditorMermaidKeyboard('g', key); lifetime.dispose();
    expect(api.DispatchLocalCommandKey).not.toHaveBeenCalled(); expect(api.BeginEditorMermaidUIKey).not.toHaveBeenCalled();
  });
  it('resposta perdida também libera a tecla sem repetir efeito', async () => {
    const api = host(); api.BeginEditorMermaidUIKey.mockRejectedValue(new Error('lost'));
    const lifetime = captureEditorMermaidKeyboard('g', key); cleanup.push(lifetime.dispose);
    await expect(lifetime.begin()).rejects.toThrow('lost'); lifetime.dispose();
    await vi.waitFor(() => expect(api.DispatchLocalCommandKey).toHaveBeenCalledOnce());
    expect(api.BeginEditorMermaidUIKey).toHaveBeenCalledOnce();
  });
});
