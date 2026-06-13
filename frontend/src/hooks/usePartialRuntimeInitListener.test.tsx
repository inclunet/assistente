/** @vitest-environment jsdom */
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
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

const retrySpy = vi.fn(() => Promise.resolve());
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

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

function payload(overrides: Partial<RuntimePartialInitPayload> = {}): RuntimePartialInitPayload {
  return {
    subsystems: [
      { subsystem: 'mcp', error: 'boom' },
      { subsystem: 'jobs', error: 'boom2' },
    ],
    ...overrides,
  };
}

describe('usePartialRuntimeInitListener', () => {
  beforeEach(() => {
    lastHandler = null;
    announceSpy.mockClear();
    addToastSpy.mockClear();
    addToastSpy.mockReturnValue('toast-1');
    removeToastSpy.mockClear();
    retrySpy.mockClear();
    retrySpy.mockResolvedValue(undefined);
  });

  it('anuncia (assertivo) e mostra toast de aviso com ação ao receber falhas', () => {
    renderHook(() => usePartialRuntimeInitListener());
    act(() => lastHandler?.(payload()));

    expect(announceSpy).toHaveBeenCalledWith(
      'runtimeStatus.partialInit.announce',
      'assertive',
    );
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

  it('a ação "Tentar novamente" rechama o backend e remove o toast atual', async () => {
    renderHook(() => usePartialRuntimeInitListener());
    act(() => lastHandler?.(payload()));

    const action = addToastSpy.mock.calls[0][3] as ToastAction;
    await act(async () => {
      action.onClick();
      await Promise.resolve();
    });

    expect(retrySpy).toHaveBeenCalledTimes(1);
    expect(removeToastSpy).toHaveBeenCalledWith('toast-1');
    expect(announceSpy).toHaveBeenCalledWith('runtimeStatus.partialInit.retrying', 'polite');
  });

  describe('com timers falsos', () => {
    beforeEach(() => vi.useFakeTimers());
    afterEach(() => vi.useRealTimers());

    it('anuncia sucesso quando o retry não reemite o aviso', async () => {
      renderHook(() => usePartialRuntimeInitListener());
      act(() => lastHandler?.(payload()));
      const action = addToastSpy.mock.calls[0][3] as ToastAction;

      await act(async () => {
        action.onClick();
        await vi.advanceTimersByTimeAsync(500);
      });

      expect(announceSpy).toHaveBeenCalledWith('runtimeStatus.partialInit.retrySuccess', 'polite');
      expect(addToastSpy).toHaveBeenCalledWith(
        'runtimeStatus.partialInit.retrySuccess',
        'success',
        4000,
      );
    });

    it('não anuncia sucesso quando o retry reemite o aviso (ainda falhando)', async () => {
      renderHook(() => usePartialRuntimeInitListener());
      act(() => lastHandler?.(payload()));
      const action = addToastSpy.mock.calls[0][3] as ToastAction;
      announceSpy.mockClear();

      await act(async () => {
        action.onClick();
        // Simula novo evento de falha chegando durante a janela de settle.
        lastHandler?.(payload());
        await vi.advanceTimersByTimeAsync(500);
      });

      expect(announceSpy).not.toHaveBeenCalledWith(
        'runtimeStatus.partialInit.retrySuccess',
        'polite',
      );
    });
  });
});
