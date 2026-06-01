/** @vitest-environment jsdom */
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { useConnectionStatusListener } from './useConnectionStatusListener';
import { useConnectionStore } from '../store/connectionStore';
import type { ConnectionStatusPayload } from '../types/connection';

let lastHandler: ((data: unknown) => void) | null = null;
vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: (_event: string, handler: (data: unknown) => void) => {
    lastHandler = handler;
    return vi.fn();
  },
}));

const announceSpy = vi.fn();
vi.mock('./useAnnouncer', () => ({
  useAnnouncer: () => ({ announce: announceSpy }),
}));

const addToastSpy = vi.fn();
vi.mock('../store/uiStore', () => ({
  useUIStore: (selector: (s: { addToast: typeof addToastSpy }) => unknown) => selector({ addToast: addToastSpy }),
}));

const authState = vi.hoisted(() => ({ isAuthenticated: true }));
vi.mock('../store/authStore', () => ({
  useAuthStore: (selector: (s: { isAuthenticated: boolean }) => unknown) =>
    selector({ isAuthenticated: authState.isAuthenticated }),
}));

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

function payload(overrides: Partial<ConnectionStatusPayload>): ConnectionStatusPayload {
  return {
    state: 'online',
    providerId: 'p1',
    providerName: 'Prov',
    model: '',
    latencyMs: 10,
    avgLatencyMs: 10,
    error: '',
    errorType: '',
    timestamp: 0,
    ...overrides,
  };
}

describe('useConnectionStatusListener', () => {
  beforeEach(() => {
    lastHandler = null;
    announceSpy.mockClear();
    addToastSpy.mockClear();
    authState.isAuthenticated = true;
    useConnectionStore.getState().reset();
  });

  it('atualiza a store a cada evento recebido', () => {
    renderHook(() => useConnectionStatusListener());
    act(() => lastHandler?.(payload({ state: 'online', avgLatencyMs: 42 })));

    const status = useConnectionStore.getState().status;
    expect(status?.state).toBe('online');
    expect(status?.avgLatencyMs).toBe(42);
  });

  it('não anuncia no primeiro estado estável', () => {
    renderHook(() => useConnectionStatusListener());
    act(() => lastHandler?.(payload({ state: 'online' })));
    expect(announceSpy).not.toHaveBeenCalled();
  });

  it('anuncia (assertivo) e mostra toast de erro quando a conexão cai', () => {
    renderHook(() => useConnectionStatusListener());
    act(() => lastHandler?.(payload({ state: 'online' })));
    act(() => lastHandler?.(payload({ state: 'offline' })));

    expect(announceSpy).toHaveBeenCalledWith('connectionStatus.announce.lost', 'assertive');
    expect(addToastSpy).toHaveBeenCalledWith('connectionStatus.announce.lost', 'error');
  });

  it('ignora o estado intermediário "checking" para não gerar anúncios espúrios', () => {
    renderHook(() => useConnectionStatusListener());
    act(() => lastHandler?.(payload({ state: 'online' })));
    act(() => lastHandler?.(payload({ state: 'offline' })));
    announceSpy.mockClear();

    act(() => lastHandler?.(payload({ state: 'checking' })));
    expect(announceSpy).not.toHaveBeenCalled();
  });

  it('anuncia restauração quando a conexão volta', () => {
    renderHook(() => useConnectionStatusListener());
    act(() => lastHandler?.(payload({ state: 'online' })));
    act(() => lastHandler?.(payload({ state: 'offline' })));
    announceSpy.mockClear();
    addToastSpy.mockClear();

    act(() => lastHandler?.(payload({ state: 'checking' })));
    act(() => lastHandler?.(payload({ state: 'online' })));

    expect(announceSpy).toHaveBeenCalledWith('connectionStatus.announce.restored', 'polite');
    expect(addToastSpy).toHaveBeenCalledWith('connectionStatus.announce.restored', 'success', 4000);
  });

  it('reseta a store e o tracking ao perder a sessão, sem anúncio espúrio ao relogar', () => {
    const { rerender } = renderHook(() => useConnectionStatusListener());
    act(() => lastHandler?.(payload({ state: 'online' })));
    expect(useConnectionStore.getState().state).toBe('online');

    // Logout: isAuthenticated -> false deve limpar a store e o tracking.
    authState.isAuthenticated = false;
    rerender();
    expect(useConnectionStore.getState().state).toBe('unknown');
    expect(useConnectionStore.getState().status).toBeNull();

    // Relogin: primeiro estado estável (offline) não deve anunciar, pois não
    // há estado "herdado" (lastStableRef foi resetado).
    authState.isAuthenticated = true;
    rerender();
    announceSpy.mockClear();
    addToastSpy.mockClear();
    act(() => lastHandler?.(payload({ state: 'offline' })));
    expect(announceSpy).not.toHaveBeenCalled();
    expect(addToastSpy).not.toHaveBeenCalled();
  });
});
