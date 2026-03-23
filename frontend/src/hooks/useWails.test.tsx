/** @vitest-environment jsdom */
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook } from '@testing-library/react';

import { useWailsEvent, useWailsAPI } from './useWails';

let lastHandler: ((data: unknown) => void) | null = null;
const unsubMock = vi.fn();
const eventsOnMock = vi.fn((_eventName: string, handler: (data: unknown) => void) => {
  lastHandler = handler;
  return unsubMock;
});

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: (eventName: string, handler: (data: unknown) => void) => eventsOnMock(eventName, handler),
}));

describe('useWailsEvent', () => {
  beforeEach(() => {
    lastHandler = null;
    eventsOnMock.mockClear();
    unsubMock.mockClear();
  });

  it('registra e limpa o listener', () => {
    const callback = vi.fn();
    const { unmount } = renderHook(() => useWailsEvent('app:event', callback));

    expect(eventsOnMock).toHaveBeenCalledWith('app:event', expect.any(Function));

    lastHandler?.({ value: 123 });
    expect(callback).toHaveBeenCalledWith({ value: 123 });

    unmount();
    expect(unsubMock).toHaveBeenCalled();
  });
});

describe('useWailsAPI', () => {
  it('retorna resultado quando a chamada resolve', async () => {
    const apiFn = vi.fn().mockResolvedValue('ok');
    const { result } = renderHook(() => useWailsAPI(apiFn));

    await expect(result.current('arg')).resolves.toBe('ok');
    expect(apiFn).toHaveBeenCalledWith('arg');
  });

  it('propaga erro e loga no console quando falha', async () => {
    const apiFn = vi.fn().mockRejectedValue(new Error('falha'));
    const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
    const { result } = renderHook(() => useWailsAPI(apiFn));

    await expect(result.current()).rejects.toThrow('falha');
    expect(consoleSpy).toHaveBeenCalled();

    consoleSpy.mockRestore();
  });
});
