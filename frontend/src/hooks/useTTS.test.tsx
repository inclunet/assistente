/** @vitest-environment jsdom */
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';

import { useTTS } from './useTTS';

const listeners = new Map<string, Set<(payload?: unknown) => void>>();

const ttsServiceMock = vi.hoisted(() => ({
  getConfig: vi.fn(() => ({
    enabled: false,
    autoRead: false,
    enabledForUser: false,
    provider: 'webspeech',
    rate: 1,
    pitch: 1,
    volume: 1,
  })),
  getVoices: vi.fn(async () => [{ id: 'v1', name: 'Voice 1', language: 'pt-BR', provider: 'webspeech' }]),
  stop: vi.fn(),
  pause: vi.fn(),
  resume: vi.fn(),
  speakWithOverride: vi.fn(async () => {}),
  setEnabled: vi.fn(),
  setAutoRead: vi.fn(),
  setRate: vi.fn(async () => {}),
  setPitch: vi.fn(),
  setVolume: vi.fn(async () => {}),
  setVoice: vi.fn(async () => {}),
  isSupported: vi.fn(() => true),
  hasVoiceConfig: vi.fn(() => true),
  on: vi.fn((event: string, handler: (payload?: unknown) => void) => {
    if (!listeners.has(event)) listeners.set(event, new Set());
    listeners.get(event)!.add(handler);
  }),
  off: vi.fn((event: string, handler: (payload?: unknown) => void) => {
    listeners.get(event)?.delete(handler);
  }),
}));

vi.mock('../services/tts', () => ({
  ttsService: ttsServiceMock,
}));

describe('useTTS', () => {
  beforeEach(() => {
    listeners.clear();
    vi.clearAllMocks();
  });

  it('carrega vozes e expõe funcoes do serviço', async () => {
    const { result } = renderHook(() => useTTS());

    await waitFor(() => {
      expect(result.current.voices.length).toBe(1);
    });

    await act(async () => {
      await result.current.speakWithOverride('Oi', { voiceName: 'test' });
    });

    expect(ttsServiceMock.speakWithOverride).toHaveBeenCalledWith('Oi', { voiceName: 'test' });
  });

  it('atualiza config quando recebe evento', async () => {
    const { result } = renderHook(() => useTTS());

    act(() => {
      listeners.get('configChanged')?.forEach((handler) =>
        handler({
          enabled: true,
          autoRead: true,
          enabledForUser: false,
          provider: 'webspeech',
          rate: 1,
          pitch: 1,
          volume: 1,
        })
      );
    });

    await waitFor(() => {
      expect(result.current.isEnabled).toBe(true);
      expect(result.current.isAutoReadEnabled).toBe(true);
    });
  });
});
