import { beforeEach, describe, expect, it, vi } from 'vitest';
import { GetToolInvocationDetails } from '@wailsjs/go/wailsapi/Conversations';
import {
  clearToolInvocationDetailsCache,
  invalidateToolInvocationDetails,
  loadToolInvocationDetails,
  toolInvocationDetailsCacheTestApi,
} from './toolInvocationDetailsCache';

vi.mock('@wailsjs/go/wailsapi/Conversations', () => ({
  GetToolInvocationDetails: vi.fn(),
}));

const getDetailsMock = vi.mocked(GetToolInvocationDetails);

const detail = (invocationId: string, output = '{"content":"ok"}') => ({
  invocationId,
  callId: `call-${invocationId}`,
  name: 'search',
  status: 'succeeded',
  attempt: 1,
  dryRun: false,
  input: '{"query":"x"}',
  output,
  resultAvailability: 'available',
  retryable: false,
  retryabilityKnown: true,
  queuedAt: '2026-09-13T00:00:00Z',
});

describe('toolInvocationDetailsCache', () => {
  beforeEach(() => {
    clearToolInvocationDetailsCache();
    getDetailsMock.mockReset();
    getDetailsMock.mockImplementation(async (ids) => ids.map((id) => detail(id)) as never);
  });

  it('coalesce requests e reutiliza a entrada user-scoped', async () => {
    const [first, second] = await Promise.all([
      loadToolInvocationDetails('user-a', ['inv-1']),
      loadToolInvocationDetails('user-a', ['inv-1']),
    ]);
    expect(getDetailsMock).toHaveBeenCalledTimes(1);
    expect(first.get('inv-1')?.output).toContain('ok');
    expect(second.get('inv-1')?.output).toContain('ok');

    await loadToolInvocationDetails('user-a', ['inv-1']);
    expect(getDetailsMock).toHaveBeenCalledTimes(1);
  });

  it('limpa detalhes ao trocar usuário e permite invalidação explícita', async () => {
    await loadToolInvocationDetails('user-a', ['inv-1']);
    await loadToolInvocationDetails('user-b', ['inv-1']);
    expect(getDetailsMock).toHaveBeenCalledTimes(2);
    expect(toolInvocationDetailsCacheTestApi.size).toBe(1);

    invalidateToolInvocationDetails(['inv-1']);
    expect(toolInvocationDetailsCacheTestApi.size).toBe(0);
  });

  it('preserva IDs opacos na consulta, no cache e na invalidação independente', async () => {
    const ids = ['inv', ' inv '];
    const result = await loadToolInvocationDetails('user-a', [...ids, '', ' ', 'inv']);
    expect(getDetailsMock).toHaveBeenCalledWith(ids);
    expect([...result.keys()]).toEqual(ids);
    expect(toolInvocationDetailsCacheTestApi.size).toBe(2);

    for (const id of ids) {
      getDetailsMock.mockClear();
      invalidateToolInvocationDetails([id]);
      expect(toolInvocationDetailsCacheTestApi.size).toBe(1);
      const reloaded = await loadToolInvocationDetails('user-a', ids);
      expect(getDetailsMock).toHaveBeenCalledTimes(1);
      expect(getDetailsMock).toHaveBeenCalledWith([id]);
      expect(reloaded.get(id)?.invocationId).toBe(id);
      expect(toolInvocationDetailsCacheTestApi.size).toBe(2);
    }
  });

  it('não retém uma entrada maior que o limite de memória', async () => {
    getDetailsMock.mockResolvedValueOnce([detail('inv-large', 'x'.repeat(4 * 1024 * 1024 + 1))] as never);
    await loadToolInvocationDetails('user-a', ['inv-large']);
    expect(toolInvocationDetailsCacheTestApi.size).toBe(0);
    expect(toolInvocationDetailsCacheTestApi.bytes).toBe(0);
  });

  it('descarta resposta que termina depois da troca de usuário', async () => {
    let finishRequest: ((value: ReturnType<typeof detail>[]) => void) | undefined;
    getDetailsMock.mockImplementationOnce(() => new Promise((resolve) => {
      finishRequest = resolve as typeof finishRequest;
    }) as never);
    const staleRequest = loadToolInvocationDetails('user-a', ['inv-stale']);
    await loadToolInvocationDetails('user-b', ['inv-b']);
    finishRequest?.([detail('inv-stale')]);
    expect((await staleRequest).size).toBe(0);
    expect(toolInvocationDetailsCacheTestApi.size).toBe(1);
  });
});
