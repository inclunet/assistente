import { afterEach, expect, it, vi } from 'vitest';
import { createPageMutationWailsPort, readProfileCommandTarget, readTaskListCommandTarget } from './commandPageMutationWails';

afterEach(() => vi.unstubAllGlobals());
it('calls the real App methods without rewriting generated bindings', async () => {
  const request = { targetId: 'list', expectedFingerprint: 'revision', title: 'Name', description: '' };
  const prepare = vi.fn(async () => undefined);
  const result = vi.fn(async () => ({ id: 'list', title: 'Name' }));
  const target = { go: { app: { App: { PreparePageMutationCommand: prepare, GetPageMutationCommandResult: result } } } } as unknown as Window;
  const port = createPageMutationWailsPort({ target });
  await port.preparePageMutationCommand('ticket', request);
  expect(prepare).toHaveBeenCalledWith('ticket', request);
  expect(await port.getPageMutationCommandResult('ticket')).toEqual({ id: 'list', title: 'Name' });
});
it('fails closed when the running backend does not provide the mutation API', async () => {
  const port = createPageMutationWailsPort({ target: {} as Window });
  await expect(port.preparePageMutationCommand('ticket', { targetId: '', expectedFingerprint: '', title: 'Name', description: '' })).rejects.toThrow('unavailable');
  await expect(port.getPageMutationCommandResult('ticket')).rejects.toThrow('unavailable');
});
it('reads payload and fingerprint in one backend request', async () => {
  const value = { taskList: { id: 'list', title: 'Name' }, fingerprint: 'version' };
  const read = vi.fn(async () => value);
  vi.stubGlobal('window', { go: { app: { App: { ReadTaskListCommandTarget: read } } } });
  expect(await readTaskListCommandTarget('list')).toBe(value);
  expect(read).toHaveBeenCalledExactlyOnceWith('list');
});
it('reads a profile target through the dynamic App facade, including the empty create slug', async () => {
  const value = { profile: undefined, fingerprint: 'profiles-snapshot' };
  const read = vi.fn(async () => value);
  vi.stubGlobal('window', { go: { app: { App: { ReadProfileCommandTarget: read } } } });
  expect(await readProfileCommandTarget('')).toBe(value);
  expect(read).toHaveBeenCalledExactlyOnceWith('');
});
