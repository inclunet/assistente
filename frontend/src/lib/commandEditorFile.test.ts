import { beforeEach, describe, expect, it, vi } from 'vitest';
import { captureEditorFileTarget, registerEditorFileSurface } from './commandEditorFile';

function surface(overrides: Partial<Parameters<typeof registerEditorFileSurface>[0]> = {}) {
  const root = document.createElement('div'); document.body.append(root);
  return { root, ownerId: 'owner', sessionId: 'session', workspaceId: 'workspace', tabId: 'tab', documentId: 'doc', instanceId: crypto.randomUUID(), generation: '1', isActive: () => true, isCurrent: () => true, canExecute: () => true, canCommit: () => true, prepare: vi.fn(async () => ({ content: 'body', labels: {}, suggestedFilename: 'doc.md', confirmOverwrite: false })), confirmOverwrite: vi.fn(async () => true), applyCommitted: vi.fn(), ...overrides };
}
beforeEach(() => { document.body.replaceChildren(); });

describe('registry de arquivos', () => {
  it('não aplica depois de invalidar a sessão mesmo mantendo registro montado', () => {
    let invalidate = () => {};
    const registration = surface({ subscribe: (callback) => { invalidate = callback; return () => {}; } });
    const unregister = registerEditorFileSurface(registration);
    const target = captureEditorFileTarget()!;
    invalidate();
    target.applyCommitted('editor.file.save', { tabId: 'tab', written: true });
    expect(registration.applyCommitted).not.toHaveBeenCalled();
    target.dispose(); unregister();
  });
  it('recusa uma raiz removida mesmo antes do cleanup React', () => {
    const registration = surface(); const unregister = registerEditorFileSurface(registration);
    const target = captureEditorFileTarget()!;
    registration.root.remove();
    expect(target.isCurrent()).toBe(false);
    target.dispose(); unregister();
  });
  it('captura somente um surface ativo e preserva o ID save_copy', () => {
    const registration = surface(); const unregister = registerEditorFileSurface(registration); const target = captureEditorFileTarget();
    expect(target?.canExecute('editor.file.save_copy')).toBe(true); expect(target?.canExecute('editor.file.save_as')).toBe(false); unregister();
  });
  it('mantém lease durante preparação assíncrona e exige canCommit após o diálogo', async () => {
    const pending = Promise.resolve({ content: 'x', labels: {}, suggestedFilename: 'x.md', confirmOverwrite: false }); const registration = surface({ prepare: vi.fn(() => pending) }); const unregister = registerEditorFileSurface(registration); const target = captureEditorFileTarget()!;
    expect(await target.prepare('editor.file.save')).toMatchObject({ content: 'x' }); expect(target.canCommit('editor.file.save')).toBe(true); unregister();
  });
  it('não aplica resultado após dispose sem autorização explícita de resultado', () => {
    const registration = surface(); const unregister = registerEditorFileSurface(registration); const target = captureEditorFileTarget()!; target.dispose(); target.applyCommitted('editor.file.open', { tabId: 'new-tab' }); expect(registration.applyCommitted).not.toHaveBeenCalled(); unregister();
  });
  it('permite aplicar nova aba somente por predicado explícito', () => {
    const registration = surface({ canApplyCommittedResult: (_id, value) => (value as { tabId?: string }).tabId === 'new-tab' }); const unregister = registerEditorFileSurface(registration); const target = captureEditorFileTarget()!; unregister();
    target.applyCommitted('editor.file.open', { tabId: 'new-tab' }); expect(registration.applyCommitted).toHaveBeenCalledOnce(); target.applyCommitted('editor.file.open', { tabId: 'other' }); expect(registration.applyCommitted).toHaveBeenCalledOnce();
  });
});
