/** @vitest-environment jsdom */
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { act, renderHook } from '@testing-library/react';

import { useVAD } from './useVAD';

type VADCallbacks = {
  onActivityStart?: () => void;
  onActivityEnd?: () => void;
  onVolumeChange?: (vol: number) => void;
};

type VADInstanceLike = {
  init: () => Promise<boolean>;
  start: () => void;
  stop: () => void;
  destroy: () => void;
  updateConfig: (config: Record<string, unknown>) => void;
  callbacks: VADCallbacks;
};

const mockState = vi.hoisted(() => ({
  lastInstance: null as VADInstanceLike | null,
}));

const MockVAD = vi.hoisted(() => {
  return class MockVAD {
    init = vi.fn(async () => true);
    start = vi.fn();
    stop = vi.fn();
    destroy = vi.fn();
    updateConfig = vi.fn();
    callbacks: VADCallbacks;

    constructor(options: VADCallbacks = {}) {
      this.callbacks = options;
      mockState.lastInstance = this;
    }
  };
});

vi.mock('../services/vad', () => ({
  VoiceActivityDetector: MockVAD,
}));

describe('useVAD', () => {
  beforeEach(() => {
    mockState.lastInstance = null;
  });

  it('inicializa e controla start/stop', async () => {
    const { result } = renderHook(() => useVAD({ autoInit: false }));

    await act(async () => {
      await result.current.init();
    });

    expect(mockState.lastInstance?.init).toHaveBeenCalled();
    expect(result.current.isInitialized).toBe(true);

    act(() => {
      result.current.start();
    });
    expect(mockState.lastInstance?.start).toHaveBeenCalled();

    act(() => {
      result.current.stop();
    });
    expect(mockState.lastInstance?.stop).toHaveBeenCalled();
  });

  it('propaga callbacks de atividade', async () => {
    const { result } = renderHook(() => useVAD({ autoInit: false }));

    await act(async () => {
      await result.current.init();
    });

    act(() => {
      (mockState.lastInstance?.callbacks.onActivityStart as () => void)?.();
    });

    expect(result.current.isSpeaking).toBe(true);

    act(() => {
      (mockState.lastInstance?.callbacks.onActivityEnd as () => void)?.();
    });

    expect(result.current.isSpeaking).toBe(false);
  });
});
