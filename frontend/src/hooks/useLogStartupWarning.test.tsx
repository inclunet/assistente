import { act, renderHook, waitFor } from '@testing-library/react';
import { beforeEach, expect, it, vi } from 'vitest';

const { getSettings, addToast, translate, auth } = vi.hoisted(() => ({
  getSettings: vi.fn(), addToast: vi.fn(), translate: (key: string) => key,
  auth: { user: { sessionId: 'session-1' }, isAuthenticated: true },
}));
vi.mock('@wailsjs/go/wailsapi/Settings', () => ({ GetLogSettings: getSettings }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: translate }) }));
vi.mock('../store/authStore', () => ({
  useAuthStore: (select: (state: unknown) => unknown) => select(auth),
}));
vi.mock('../store/uiStore', () => ({ useUIStore: (select: (state: unknown) => unknown) => select({ addToast }) }));

import { useLogStartupWarning } from './useLogStartupWarning';

beforeEach(() => { vi.clearAllMocks(); auth.user.sessionId = 'session-1'; });

it('avisa sem bloquear quando o destino configurado não iniciou', async () => {
  getSettings.mockResolvedValue({ error: 'destino indisponível' });
  renderHook(() => useLogStartupWarning());
  await waitFor(() => expect(addToast).toHaveBeenCalledWith('logSettings.startupToast', 'warning', 15000));
});

it('não anuncia falha quando os logs estão disponíveis', async () => {
  getSettings.mockResolvedValue({ error: '' });
  renderHook(() => useLogStartupWarning());
  await waitFor(() => expect(getSettings).toHaveBeenCalled());
  expect(addToast).not.toHaveBeenCalled();
});

it('avisa se uma falha ocorrer depois da inicialização', async () => {
  vi.useFakeTimers();
  try {
    getSettings.mockResolvedValueOnce({ error: '' }).mockResolvedValue({ error: 'falha de escrita' });
    renderHook(() => useLogStartupWarning());
    await act(async () => { await Promise.resolve(); });
    expect(addToast).not.toHaveBeenCalled();
    await act(async () => { await vi.advanceTimersByTimeAsync(30000); });
    expect(addToast).toHaveBeenCalledTimes(1);
    await act(async () => { await vi.advanceTimersByTimeAsync(30000); });
    expect(addToast).toHaveBeenCalledTimes(1);
  } finally {
    vi.useRealTimers();
  }
});

it('avisa a nova sessão mesmo em troca direta de usuário', async () => {
  getSettings.mockResolvedValue({ error: 'falha de escrita' });
  const hook = renderHook(() => useLogStartupWarning());
  await waitFor(() => expect(addToast).toHaveBeenCalledTimes(1));
  auth.user.sessionId = 'session-2';
  hook.rerender();
  await waitFor(() => expect(addToast).toHaveBeenCalledTimes(2));
});

it('não interrompe a interface se a ponte lançar erro síncrono', async () => {
  getSettings.mockImplementation(() => { throw new Error('ponte indisponível'); });
  renderHook(() => useLogStartupWarning());
  await waitFor(() => expect(getSettings).toHaveBeenCalled());
  expect(addToast).not.toHaveBeenCalled();
});
