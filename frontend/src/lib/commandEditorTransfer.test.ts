import { afterEach, expect, it, vi } from 'vitest';
import { registerEditorTransferSurface, transferEditorContent, type EditorTransferTarget } from './commandEditorTransfer';

const disposers: Array<() => void> = [];
afterEach(() => { disposers.splice(0).forEach(off => off()); vi.useRealTimers(); });
function target(): EditorTransferTarget {
  return { documentId: 'doc', isCurrent: () => true, apply: vi.fn(), dispose: vi.fn() };
}
const request = () => ({ targetDocumentId: 'doc', content: 'hello', format: 'plain' as const, isCurrent: () => true });
it('aguarda readiness mesmo com superfície registrada e aplica uma vez', async () => {
  vi.useFakeTimers();
  const dest = target(); let ready = false;
  disposers.push(registerEditorTransferSurface({ documentId: 'doc', capture: () => ready ? dest : undefined }));
  const done = transferEditorContent(request());
  await vi.advanceTimersByTimeAsync(100);
  expect(dest.apply).not.toHaveBeenCalled(); ready = true;
  await vi.advanceTimersByTimeAsync(20); await done;
  expect(dest.apply).toHaveBeenCalledTimes(1);
});
it('captura antes beforeApply e recusa alteração durante await', async () => {
  const dest = target(); let current = true;
  dest.isCurrent = () => current;
  await expect(transferEditorContent({ ...request(), destination: dest, beforeApply: async () => { current = false; } })).rejects.toMatchObject({ code: 'stale' });
  expect(dest.apply).not.toHaveBeenCalled();
});
it('timeout durante validação impede efeito tardio', async () => {
  vi.useFakeTimers(); const dest = target(); let release!: () => void;
  const done = transferEditorContent({ ...request(), destination: dest, timeoutMs: 40, beforeApply: () => new Promise<void>(resolve => { release = resolve; }) });
  const rejected = expect(done).rejects.toMatchObject({ code: 'timeout' });
  await vi.advanceTimersByTimeAsync(50); await rejected;
  release(); await Promise.resolve(); expect(dest.apply).not.toHaveBeenCalled();
});
it('efeito que lança não é repetido e retorna outcome_unknown', async () => {
  const dest = target(); dest.apply = vi.fn(() => { throw new Error('after mutation'); });
  await expect(transferEditorContent({ ...request(), destination: dest })).rejects.toMatchObject({ code: 'outcome_unknown' });
  expect(dest.apply).toHaveBeenCalledTimes(1);
});
it('rejeita ambiguidade sem escolher superfície', async () => {
  const dest = target();
  for (let i = 0; i < 2; i++) disposers.push(registerEditorTransferSurface({ documentId: 'doc', capture: () => dest }));
  await expect(transferEditorContent(request())).rejects.toMatchObject({ code: 'ambiguous' });
  expect(dest.apply).not.toHaveBeenCalled();
});
it('copia payload antes de validação assíncrona', async () => {
  const dest = target(); const input = { ...request(), destination: dest, beforeApply: async () => { input.content = 'mutated'; } };
  await transferEditorContent(input);
  expect(dest.apply).toHaveBeenCalledWith(expect.objectContaining({ content: 'hello' }));
});
it('esperas concorrentes não sobrescrevem pedidos', async () => {
  const first = transferEditorContent(request()); const second = transferEditorContent({ ...request(), content: 'second' });
  const applied: string[] = [];
  disposers.push(registerEditorTransferSurface({ documentId: 'doc', capture: () => ({ ...target(), apply: payload => { applied.push(payload.content); } }) }));
  await Promise.all([first, second]); expect(applied).toEqual(['hello', 'second']);
});
