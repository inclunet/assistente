/** @vitest-environment jsdom */
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { usePartialRuntimeInitListener } from './usePartialRuntimeInitListener';
import type { RuntimePartialInitPayload } from '../types/runtime';
import type { ToastAction } from '../store/uiStore';

let lastHandler: ((data: unknown) => void) | null = null;
vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: (_event: string, handler: (data: unknown) => void) => {
    lastHandler = handler;
    return vi.fn();
  },
}));

const retrySpy = vi.fn<() => Promise<RuntimePartialInitPayload>>(() =>
  Promise.resolve({ subsystems: [] }),
);
vi.mock('@wailsjs/go/app/App', () => ({
  RetryUserRuntimeInit: () => retrySpy(),
}));

const announceSpy = vi.fn();
vi.mock('./useAnnouncer', () => ({
  useAnnouncer: () => ({ announce: announceSpy }),
}));

const addToastSpy = vi.fn(
  (_message: string, _type: string, _duration?: number, _action?: ToastAction): string => 'toast-1',
);
const removeToastSpy = vi.fn((_id: string) => {});
vi.mock('../store/uiStore', () => ({
  useUIStore: (
    selector: (s: { addToast: typeof addToastSpy; removeToast: typeof removeToastSpy }) => unknown,
  ) => selector({ addToast: addToastSpy, removeToast: removeToastSpy }),
}));

const authState = vi.hoisted(() => ({ isAuthenticated: true }));
vi.mock('../store/authStore', () => ({
  useAuthStore: (selector: (s: { isAuthenticated: boolean }) => unknown) =>
    selector({ isAuthenticated: authState.isAuthenticated }),
}));

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

function payload(overrides: Partial<RuntimePartialInitPayload> = {}): RuntimePartialInitPayload {
  return {
    subsystems: [{ subsystem: 'mcp' }, { subsystem: 'jobs' }],
    ...overrides,
  };
}

/** Dispara o evento e devolve a ação ("Tentar novamente") do último toast. */
function emitWarning(): ToastAction {
  act(() => lastHandler?.(payload()));
  const calls = addToastSpy.mock.calls;
  return calls[calls.length - 1][3] as ToastAction;
}

describe('usePartialRuntimeInitListener', () => {
  beforeEach(() => {
    lastHandler = null;
    announceSpy.mockClear();
    addToastSpy.mockClear();
    addToastSpy.mockReturnValue('toast-1');
    removeToastSpy.mockClear();
    retrySpy.mockClear();
    retrySpy.mockResolvedValue({ subsystems: [] });
    authState.isAuthenticated = true;
  });

  it('anuncia (assertivo) e mostra toast de aviso persistente com ação ao receber falhas', () => {
    renderHook(() => usePartialRuntimeInitListener());
    act(() => lastHandler?.(payload()));

    expect(announceSpy).toHaveBeenCalledWith('runtimeStatus.partialInit.announce', 'assertive');
    expect(addToastSpy).toHaveBeenCalledTimes(1);
    const [message, variant, duration, action] = addToastSpy.mock.calls[0];
    expect(message).toBe('runtimeStatus.partialInit.message');
    expect(variant).toBe('warning');
    expect(duration).toBe(0);
    expect((action as ToastAction).label).toBe('runtimeStatus.partialInit.retry');
  });

  it('ignora payload vazio ou inválido', () => {
    renderHook(() => usePartialRuntimeInitListener());
    act(() => lastHandler?.(payload({ subsystems: [] })));
    act(() => lastHandler?.(null));
    act(() => lastHandler?.({}));

    expect(announceSpy).not.toHaveBeenCalled();
    expect(addToastSpy).not.toHaveBeenCalled();
  });

  it('retry com sucesso remove o aviso só após a RPC e anuncia sucesso', async () => {
    renderHook(() => usePartialRuntimeInitListener());
    const action = emitWarning();
    removeToastSpy.mockClear();

    await act(async () => {
      action.onClick();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(retrySpy).toHaveBeenCalledTimes(1);
    expect(announceSpy).toHaveBeenCalledWith('runtimeStatus.partialInit.retrying', 'polite');
    expect(removeToastSpy).toHaveBeenCalledWith('toast-1');
    expect(announceSpy).toHaveBeenCalledWith('runtimeStatus.partialInit.retrySuccess', 'polite');
    expect(addToastSpy).toHaveBeenCalledWith(
      'runtimeStatus.partialInit.retrySuccess',
      'success',
      4000,
      undefined,
      { suppressAnnounce: true },
    );
  });

  it('retry que ainda falha reexibe o aviso e NÃO anuncia sucesso', async () => {
    renderHook(() => usePartialRuntimeInitListener());
    const action = emitWarning();
    retrySpy.mockResolvedValue({ subsystems: [{ subsystem: 'mcp' }] });
    addToastSpy.mockClear();

    await act(async () => {
      action.onClick();
      await Promise.resolve();
      await Promise.resolve();
    });

    // Reexibe o aviso persistente (warning, duration 0) com a lista atual.
    expect(addToastSpy).toHaveBeenCalledWith(
      'runtimeStatus.partialInit.message',
      'warning',
      0,
      expect.objectContaining({ label: 'runtimeStatus.partialInit.retry' }),
      { suppressAnnounce: true },
    );
    expect(announceSpy).not.toHaveBeenCalledWith(
      'runtimeStatus.partialInit.retrySuccess',
      'polite',
    );
  });

  it('falha da RPC mantém o aviso persistente (não remove) e mostra erro', async () => {
    renderHook(() => usePartialRuntimeInitListener());
    const action = emitWarning();
    retrySpy.mockRejectedValue(new Error('rpc down'));
    removeToastSpy.mockClear();

    await act(async () => {
      action.onClick();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(removeToastSpy).not.toHaveBeenCalled();
    expect(announceSpy).toHaveBeenCalledWith('runtimeStatus.partialInit.retryError', 'assertive');
    expect(addToastSpy).toHaveBeenCalledWith(
      'runtimeStatus.partialInit.retryError',
      'error',
      undefined,
      undefined,
      { suppressAnnounce: true },
    );
  });

  it('remove o aviso persistente ao detectar logout (isAuthenticated=false)', () => {
    const { rerender } = renderHook(() => usePartialRuntimeInitListener());
    act(() => lastHandler?.(payload()));
    removeToastSpy.mockClear();

    authState.isAuthenticated = false;
    rerender();

    expect(removeToastSpy).toHaveBeenCalledWith('toast-1');
  });
});
