import { beforeEach, describe, expect, it, vi } from 'vitest';
import { getRuntimeCommandToolGuidance, runtimeToolIDFromCommandID } from './commandToolGuidance';

const catalogAPI = vi.hoisted(() => ({ getRuntimeToolCatalog: vi.fn() }));

vi.mock('@wailsjs/go/wailsapi/Tools', () => ({
  GetRuntimeToolCatalog: (...args: unknown[]) => catalogAPI.getRuntimeToolCatalog(...args),
}));

const compactID = '018f123456787abc8def012345678901';
const dashedID = '018f1234-5678-7abc-8def-012345678901';
const commandID = `tool.execute.t_${compactID}`;
const query = (offset: number) => ({ availabilityStatus: 'available', limit: 50, offset });

beforeEach(() => {
  vi.resetAllMocks();
});

describe('runtime command tool guidance', () => {
  it('derives canonical runtime UUID only from compact UUIDv7 command IDs', () => {
    expect(runtimeToolIDFromCommandID(commandID)).toBe(dashedID);
    expect(runtimeToolIDFromCommandID('tool.execute.t_018f123456787abccdef012345678901')).toBeNull();
    expect(runtimeToolIDFromCommandID('tool.execute.t_not-a-uuid')).toBeNull();
  });

  it.each(['object', 'string', 'bytes'] as const)('loads available metadata with schema encoded as %s', async (encoding) => {
    const schema = { type: 'object', properties: { query: { type: 'string' } } };
    // json.RawMessage is serialized by Wails as JSON, despite the generated number[] type.
    const wireSchema = encoding === 'object' ? schema : encoding === 'string'
      ? JSON.stringify(schema) : Array.from(new TextEncoder().encode(JSON.stringify(schema)));
    catalogAPI.getRuntimeToolCatalog.mockResolvedValueOnce([{
      id: dashedID.toUpperCase(), name: 'search', displayName: 'Pesquisa', description: 'Busca autorizada.',
      schema: wireSchema,
    }]);

    await expect(getRuntimeCommandToolGuidance(commandID)).resolves.toEqual({
      available: true, displayName: 'Pesquisa', description: 'Busca autorizada.', schema,
    });
    expect(catalogAPI.getRuntimeToolCatalog).toHaveBeenCalledExactlyOnceWith(query(0));
  });

  it('para após a página final curta quando a tool não está presente', async () => {
    catalogAPI.getRuntimeToolCatalog.mockResolvedValueOnce(Array.from({ length: 3 }, (_, index) => ({
      id: `different-${index}`, name: 'other', displayName: 'Outra ferramenta',
    })));

    await expect(getRuntimeCommandToolGuidance(commandID)).resolves.toEqual({ available: false, displayName: commandID });
    expect(catalogAPI.getRuntimeToolCatalog).toHaveBeenCalledExactlyOnceWith(query(0));
  });

  it('limita a busca a 82 páginas, suficientes para o catálogo máximo de 4096 entradas', async () => {
    catalogAPI.getRuntimeToolCatalog.mockImplementation(async () =>
      Array.from({ length: 50 }, (_, index) => ({ id: `different-${index}`, name: 'other', displayName: 'Outra' })));

    await expect(getRuntimeCommandToolGuidance(commandID)).resolves.toEqual({ available: false, displayName: commandID });
    expect(catalogAPI.getRuntimeToolCatalog).toHaveBeenCalledTimes(82);
    expect(catalogAPI.getRuntimeToolCatalog).toHaveBeenNthCalledWith(1, query(0));
    expect(catalogAPI.getRuntimeToolCatalog).toHaveBeenNthCalledWith(82, query(4050));
  });

  it('interrompe a paginação quando o contexto deixa de ser atual', async () => {
    let current = true;
    catalogAPI.getRuntimeToolCatalog.mockImplementation(async () => {
      current = false;
      return Array.from({ length: 50 }, (_, index) => ({ id: `different-${index}`, name: 'other', displayName: 'Outra' }));
    });

    await expect(getRuntimeCommandToolGuidance(commandID, () => current)).resolves.toEqual({ available: false, displayName: commandID });
    expect(catalogAPI.getRuntimeToolCatalog).toHaveBeenCalledExactlyOnceWith(query(0));
  });
});
