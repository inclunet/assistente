/** @vitest-environment jsdom */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { act, renderHook } from '@testing-library/react';

import { useWakewordDetection } from './useWakewordDetection';

type RecognitionEvent = {
  resultIndex: number;
  results: Array<{ 0: { transcript: string; confidence: number }; length: number }>;
};

let lastRecognition: MockRecognition | null = null;

class MockRecognition {
  continuous = false;
  interimResults = false;
  lang = 'pt-BR';
  maxAlternatives = 1;
  onresult: ((event: RecognitionEvent) => void) | null = null;
  onerror: ((event: { error: string }) => void) | null = null;
  onend: ((event: Event) => void) | null = null;

  constructor() {
    lastRecognition = this;
  }

  start = vi.fn();
  stop = vi.fn();

  emitResult(text: string) {
    const event: RecognitionEvent = {
      resultIndex: 0,
      results: [{ 0: { transcript: text, confidence: 0.9 }, length: 1 }],
    };
    this.onresult?.(event);
  }
}

describe('useWakewordDetection', () => {
  const originalSpeechRecognition = (window as unknown as { SpeechRecognition?: unknown }).SpeechRecognition;

  beforeEach(() => {
    (window as unknown as { SpeechRecognition?: unknown }).SpeechRecognition = MockRecognition as never;
    lastRecognition = null;
  });

  afterEach(() => {
    (window as unknown as { SpeechRecognition?: unknown }).SpeechRecognition = originalSpeechRecognition;
  });

  it('inicia escuta e detecta keyword', () => {
    const onDetected = vi.fn();
    const { result, unmount } = renderHook(() =>
      useWakewordDetection({ keyword: 'assistente', onDetected })
    );

    act(() => {
      result.current.startListening();
    });

    expect(result.current.isListening).toBe(true);

    act(() => {
      lastRecognition?.emitResult('Oi assistente');
    });

    expect(onDetected).toHaveBeenCalledWith('assistente', 'oi assistente');

    act(() => {
      result.current.stopListening();
    });

    unmount();
  });

  it('sinaliza erro quando nao suportado', () => {
    (window as unknown as { SpeechRecognition?: unknown }).SpeechRecognition = undefined;

    const onError = vi.fn();
    const { result } = renderHook(() =>
      useWakewordDetection({ keyword: 'teste', onError })
    );

    act(() => {
      result.current.startListening();
    });

    expect(onError).toHaveBeenCalled();
  });
});
