import { describe, expect, it, vi } from 'vitest';
import { createEditorFileCommandWailsPort } from './commandEditorFileWails';

describe('contrato Wails de arquivos do editor', () => {
  it('recusa runtime antigo sem fallback de escrita direta', () => {
    expect(() => createEditorFileCommandWailsPort({} as Window)).toThrow('unavailable');
  });

  it('transporta conteúdo apenas na preparação e receipt no commit', async () => {
    const prepare = vi.fn().mockResolvedValue({ token: 'receipt', path: 'file.md', cancelled: false, requiresOverwrite: true });
    const commit = vi.fn().mockResolvedValue({ tabId: 'editor-a', path: 'file.md', written: true });
    const target = { go: { wailsapi: { Editor: { EditorPrepareCommand: prepare, EditorCommitCommand: commit } } } } as unknown as Window;
    const port = createEditorFileCommandWailsPort(target);
    const take = { ticket: 'ticket', handoffId: 'handoff', invocationId: 'invocation', commandId: 'editor.file.save' };
    await port.prepare(take, 'editor.file.save', { content: 'private document', labels: { title: 'Salvar' }, suggestedFilename: 'file.md', confirmOverwrite: false });
    expect(prepare).toHaveBeenCalledExactlyOnceWith({ ticket: 'ticket', handoffId: 'handoff', content: 'private document', labels: { title: 'Salvar' }, suggestedFilename: 'file.md' });
    await port.commit(take, 'receipt', true);
    expect(commit).toHaveBeenCalledExactlyOnceWith({ ticket: 'ticket', handoffId: 'handoff', token: 'receipt', confirmOverwrite: true });
    expect(JSON.stringify(commit.mock.calls)).not.toContain('private document');
  });
});
