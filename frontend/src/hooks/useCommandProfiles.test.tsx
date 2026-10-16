import { act, renderHook, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const { getProfiles, eventsOn } = vi.hoisted(() => ({ getProfiles: vi.fn(), eventsOn: vi.fn() }));
vi.mock('@wailsjs/go/wailsapi/Profiles', () => ({ GetProfiles: getProfiles }));
vi.mock('@wailsjs/runtime/runtime', () => ({ EventsOn: eventsOn }));

import { useCommandProfiles } from './useCommandProfiles';

describe('useCommandProfiles', () => {
  beforeEach(() => {
    vi.resetAllMocks();
    eventsOn.mockReturnValue(() => undefined);
  });

  it('ignora resposta tardia de outra identidade', async () => {
    let resolveOld: (value: unknown[]) => void = () => undefined;
    let resolveNew: (value: unknown[]) => void = () => undefined;
    getProfiles
      .mockImplementationOnce(() => new Promise((resolve) => { resolveOld = resolve; }))
      .mockImplementationOnce(() => new Promise((resolve) => { resolveNew = resolve; }));
    const { result, rerender } = renderHook(({ identity }) => useCommandProfiles(identity), {
      initialProps: { identity: 'user-a:session-a:scope-a' },
    });

    rerender({ identity: 'user-b:session-b:scope-b' });
    expect(result.current.profiles).toEqual([]);
    await act(async () => resolveOld([{ name: 'Antigo', slug: 'antigo' }]));
    expect(result.current.profiles).toEqual([]);
    await act(async () => resolveNew([{ name: 'Novo', slug: 'novo' }]));
    await waitFor(() => expect(result.current.profiles).toEqual([{ name: 'Novo', slug: 'novo' }]));
  });

  it('expõe erro e permite retry', async () => {
    getProfiles.mockRejectedValueOnce(new Error('indisponível')).mockResolvedValueOnce([{ name: 'Padrão', slug: 'padrao' }]);
    const { result } = renderHook(() => useCommandProfiles('identity'));
    await waitFor(() => expect(result.current.error).toBe(true));
    await act(async () => result.current.reload());
    expect(result.current.profiles).toEqual([{ name: 'Padrão', slug: 'padrao' }]);
    expect(result.current.error).toBe(false);
  });
});
