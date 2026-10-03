import { act, renderHook } from '@testing-library/react';
import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest';
import { createInstance } from 'i18next';
import en from '../locales/en';
import { useUIStore } from '../store/uiStore';
import { useLegacyImportSummaryListener } from './useLegacyImportSummaryListener';

const state = vi.hoisted(() => ({ isAuthenticated: false, isLoading: true, user: null as { userId: string } | null }));
const announce = vi.hoisted(() => vi.fn());
let handler: (data: unknown) => void;
const unsubscribe = vi.fn();
vi.mock('@wailsjs/runtime/runtime', () => ({ EventsOn: (_name: string, listener: typeof handler) => { handler = listener; return unsubscribe; } }));
vi.mock('../store/authStore', () => ({ useAuthStore: Object.assign((selector: (value: typeof state) => unknown) => selector(state), { getState: () => state }) }));
vi.mock('../services/voiceAccessibility/announcerBroker', () => ({ announceWithOrigin: announce }));
const i18n = createInstance();
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: i18n.t.bind(i18n) }) }));

const payload = {
  userId: 'alice', imported: 1, warningCount: 1,
  entries: [{ warningMessages: [{ code: 'mcpServer.oauthRecoveryRequired', params: { slug: 'historical' }, message: 'fallback português' }] }],
};

describe('resumo de recuperação OAuth após login', () => {
  beforeEach(async () => {
    vi.useFakeTimers();
    vi.clearAllMocks();
    state.isAuthenticated = false;
    state.isLoading = true;
    state.user = null;
    useUIStore.setState({ toasts: [] });
    await i18n.init({ lng: 'en', resources: { en }, initImmediate: false });
  });
  afterEach(() => { vi.clearAllTimers(); vi.useRealTimers(); });

  it('retém o evento durante login e mostra/anuncia instruções traduzidas uma vez', () => {
    const { rerender, unmount } = renderHook(() => useLegacyImportSummaryListener());
    act(() => handler(payload));
    expect(useUIStore.getState().toasts).toHaveLength(0);
    state.isAuthenticated = true;
    state.isLoading = false;
    state.user = { userId: 'alice' };
    rerender();
    const toasts = useUIStore.getState().toasts;
    expect(toasts).toHaveLength(1);
    expect(toasts[0].message).toContain(i18n.t('portability.messages.mcpServer.oauthRecoveryRequired', { slug: 'historical' }));
    expect(toasts[0].message).not.toContain('fallback português');
    expect(announce).toHaveBeenCalledTimes(1);
    expect(announce).toHaveBeenCalledWith(expect.objectContaining({ message: toasts[0].message, announcePriority: 'assertive' }));
    rerender();
    expect(useUIStore.getState().toasts).toHaveLength(1);
    unmount();
    expect(unsubscribe).toHaveBeenCalled();
  });

  it('não exibe recuperação de outro usuário após login', () => {
    const { rerender } = renderHook(() => useLegacyImportSummaryListener());
    act(() => handler(payload));
    state.isAuthenticated = true;
    state.isLoading = false;
    state.user = { userId: 'bob' };
    rerender();
    act(() => handler(payload));
    expect(useUIStore.getState().toasts).toHaveLength(0);
    expect(announce).not.toHaveBeenCalled();
  });

  it('descarta evento pendente se login falhar', () => {
    const { rerender } = renderHook(() => useLegacyImportSummaryListener());
    act(() => handler(payload));
    state.isLoading = false;
    rerender();
    state.isAuthenticated = true;
    state.user = { userId: 'alice' };
    rerender();
    expect(useUIStore.getState().toasts).toHaveLength(0);
  });
});
