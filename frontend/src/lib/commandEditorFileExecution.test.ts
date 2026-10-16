import { describe, expect, it, vi } from 'vitest';
import { executeEditorFileCommand, type EditorFileExecutionPort } from './commandEditorFileExecution';
import type { EditorFileTargetLease } from './commandEditorFile';

function fixture() {
  const target: EditorFileTargetLease = { isCurrent: vi.fn(() => true), canExecute: vi.fn(() => true), canCommit: vi.fn(() => true), applyCommitted: vi.fn(), prepare: vi.fn(async () => ({ content: 'x', labels: {}, suggestedFilename: 'x.md', confirmOverwrite: false })), confirmOverwrite: vi.fn(async () => true), dispose: vi.fn(), ownerId: 'o', sessionId: 's', workspaceId: 'w', tabId: 't', documentId: 'd', instanceId: 'i', generation: '1' };
  const port: EditorFileExecutionPort = { begin: vi.fn(async () => ({ ticket: 'ticket', invocationId: 'inv', commandId: 'editor.file.save' })), take: vi.fn(async () => ({ ticket: 'ticket', invocationId: 'inv', commandId: 'editor.file.save', handoffId: 'handoff' })), prepare: vi.fn(async () => ({ token: 'token', path: 'x.md', requiresOverwrite: false, cancelled: false })), commit: vi.fn(async () => ({ tabId: 't', path: 'x.md', written: true })), completeCancelled: vi.fn(async () => undefined), getResult: vi.fn(async () => ({ invocationId: 'inv', status: 'succeeded' })), cancel: vi.fn(async () => undefined) };
  return { target, port };
}
const options = (target: EditorFileTargetLease) => ({ target, authorizeInitial: () => true, authorizeCommit: () => true });

describe('commandEditorFileExecution', () => {
  it('recusa fonte alterada durante preparação local antes de abrir diálogo nativo', async () => {
    const f = fixture();
    let current = true;
    const prepare = f.target.prepare;
    f.target.prepare = async id => { const value = await prepare(id); current = false; return value; };
    const result = await executeEditorFileCommand(f.port, 'editor.file.save', { ...options(f.target), authorizePreparation: () => current });
    expect(result.status).toBe('cancelled');
    expect(f.port.prepare).not.toHaveBeenCalled();
    expect(f.port.commit).not.toHaveBeenCalled();
    expect(f.port.completeCancelled).toHaveBeenCalledExactlyOnceWith('ticket', 'handoff');
  });
  it('consome continuação imediatamente antes do prepare nativo e revalida antes do commit', async () => {
    const f = fixture(); const order: string[] = [];
    const prepare = f.port.prepare;
    f.port.prepare = async (...args) => { order.push('native'); return prepare(...args); };
    await executeEditorFileCommand(f.port, 'editor.file.save', { ...options(f.target),
      authorizePreparation: () => { order.push('admit'); queueMicrotask(() => order.push('microtask')); return true; },
      authorizeCommit: () => { order.push('commit-guard'); return false; },
    });
    expect(order).toEqual(['admit', 'native', 'microtask', 'commit-guard']);
    expect(f.port.commit).not.toHaveBeenCalled();
  });
  it('recusar overwrite cancela sem submeter gravação', async () => {
    const f = fixture();
    f.port.prepare = vi.fn(async () => ({ token: 'receipt', path: 'target.md', requiresOverwrite: true, cancelled: false }));
    f.target.confirmOverwrite = vi.fn(async () => false);
    expect((await executeEditorFileCommand(f.port, 'editor.file.save', options(f.target))).status).toBe('cancelled');
    expect(f.target.confirmOverwrite).toHaveBeenCalledWith('target.md');
    expect(f.port.completeCancelled).toHaveBeenCalledOnce();
    expect(f.port.commit).not.toHaveBeenCalled();
  });
  it('receipt incompleto não pode virar escrita', async () => {
    const f = fixture();
    f.port.prepare = vi.fn(async () => ({ token: '', path: 'x.md', requiresOverwrite: false, cancelled: false }));
    expect((await executeEditorFileCommand(f.port, 'editor.file.save', options(f.target))).status).toBe('outcome_unknown');
    expect(f.port.commit).not.toHaveBeenCalled();
  });
  it('revalida o alvo depois da confirmação de overwrite', async () => {
    const f = fixture();
    f.port.prepare = vi.fn(async () => ({ token: 'receipt', path: 'target.md', requiresOverwrite: true, cancelled: false }));
    f.target.confirmOverwrite = vi.fn(async () => { f.target.isCurrent = () => false; return true; });
    await executeEditorFileCommand(f.port, 'editor.file.save', options(f.target));
    expect(f.port.commit).not.toHaveBeenCalled();
  });
  it('não cancela nem repete commit quando a consulta do resultado falha', async () => {
    const f = fixture();
    f.port.getResult = vi.fn(async () => { throw new Error('transport'); });
    expect((await executeEditorFileCommand(f.port, 'editor.file.save', options(f.target))).status).toBe('outcome_unknown');
    expect(f.port.commit).toHaveBeenCalledOnce();
    expect(f.port.completeCancelled).not.toHaveBeenCalled();
    expect(f.port.cancel).not.toHaveBeenCalled();
    expect(f.target.applyCommitted).not.toHaveBeenCalled();
  });
  it('cancela depois de Begin stale sem vazar ticket', async () => { const f = fixture(); f.target.isCurrent = vi.fn().mockReturnValueOnce(true).mockReturnValue(false); await expect(executeEditorFileCommand(f.port, 'editor.file.save', options(f.target))).resolves.toMatchObject({ status: 'cancelled' }); expect(f.port.cancel).toHaveBeenCalledWith('ticket'); expect(f.port.take).not.toHaveBeenCalled(); });
  it('usa Complete cancelled após Take quando prepare/target falha', async () => { const f = fixture(); f.target.canCommit = vi.fn(() => false); await expect(executeEditorFileCommand(f.port, 'editor.file.save', options(f.target))).resolves.toMatchObject({ status: 'cancelled' }); expect(f.port.completeCancelled).toHaveBeenCalledWith('ticket', 'handoff'); expect(f.port.commit).not.toHaveBeenCalled(); });
  it('rejeita Take cruzado sem commit', async () => { const f = fixture(); f.port.take = vi.fn(async () => ({ ticket: 'other', invocationId: 'other', commandId: 'editor.file.open', handoffId: 'h' })); await executeEditorFileCommand(f.port, 'editor.file.save', options(f.target)); expect(f.port.cancel).toHaveBeenCalledWith('ticket'); expect(f.port.commit).not.toHaveBeenCalled(); });
  it('aplica resposta efêmera somente após GetResult succeeded', async () => { const f = fixture(); const result = await executeEditorFileCommand(f.port, 'editor.file.save', options(f.target)); expect(result.status).toBe('succeeded'); expect(f.target.applyCommitted).toHaveBeenCalledWith('editor.file.save', { tabId: 't', path: 'x.md', written: true }); });
  it('não reaplica quando a resposta de commit é perdida', async () => { const f = fixture(); f.port.commit = vi.fn(async () => { throw new Error('lost'); }); await executeEditorFileCommand(f.port, 'editor.file.save', options(f.target)); expect(f.target.applyCommitted).not.toHaveBeenCalled(); });
  it('não transforma resultado inválido ou desconhecido em sucesso', async () => { const f = fixture(); f.port.getResult = vi.fn(async () => ({ invocationId: 'wrong', status: 'succeeded' })); await expect(executeEditorFileCommand(f.port, 'editor.file.save', options(f.target))).resolves.toMatchObject({ status: 'outcome_unknown' }); });
});
