/** @vitest-environment jsdom */
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { act, renderHook } from '@testing-library/react';

import { useSTT } from './useSTT';

type STTServiceLike = {
  isWebSpeechSupported: boolean;
  isWhisperSupported: boolean;
  init: () => Promise<boolean>;
  startRecording: () => Promise<boolean>;
  stopRecording: () => void;
  cancelRecording: () => void;
  toggleRecording: () => void;
  setProvider: (provider: string) => void;
  setMode: (mode: string) => void;
  setLanguage: (language: string) => void;
  updateConfig: (config: Record<string, unknown>) => void;
  destroy: () => void;
};

const mockState = vi.hoisted(() => ({
  lastInstance: null as STTServiceLike | null,
}));

const MockSTTService = vi.hoisted(() => {
  return class MockSTTService {
    isWebSpeechSupported = true;
    isWhisperSupported = false;
    init = vi.fn(async () => true);
    startRecording = vi.fn(async () => true);
    stopRecording = vi.fn();
    cancelRecording = vi.fn();
    toggleRecording = vi.fn();
    setProvider = vi.fn();
    setMode = vi.fn();
    setLanguage = vi.fn();
    updateConfig = vi.fn();
    destroy = vi.fn();

    constructor(public options: Record<string, unknown> = {}) {
      mockState.lastInstance = this;
    }
  };
});

vi.mock('../services/stt', () => ({
  STTService: MockSTTService,
  STT_STATES: {
    IDLE: 'idle',
    LISTENING: 'listening',
    RECORDING: 'recording',
    PROCESSING: 'processing',
    ERROR: 'error',
  },
}));

describe('useSTT', () => {
  beforeEach(() => {
    mockState.lastInstance = null;
  });

  it('inicializa e expõe flags de suporte', async () => {
    const { result } = renderHook(() => useSTT({ autoInit: false }));

    await act(async () => {
      await result.current.init();
    });

    expect(mockState.lastInstance?.init).toHaveBeenCalled();
    expect(result.current.isInitialized).toBe(true);
    expect(result.current.isWebSpeechSupported).toBe(true);
    expect(result.current.isWhisperSupported).toBe(false);
  });

  it('encaminha start/stop para o serviço', async () => {
    const { result } = renderHook(() => useSTT({ autoInit: false }));

    await act(async () => {
      await result.current.init();
      await result.current.startRecording();
      result.current.stopRecording();
    });

    expect(mockState.lastInstance?.startRecording).toHaveBeenCalled();
    expect(mockState.lastInstance?.stopRecording).toHaveBeenCalled();
  });

  it('atualiza provider no state e no serviço', async () => {
    const { result } = renderHook(() => useSTT({ autoInit: false }));

    await act(async () => {
      await result.current.init();
    });

    act(() => {
      result.current.setProvider('whisper_api');
    });

    expect(result.current.provider).toBe('whisper_api');
    expect(mockState.lastInstance?.setProvider).toHaveBeenCalledWith('whisper_api');
  });
});
