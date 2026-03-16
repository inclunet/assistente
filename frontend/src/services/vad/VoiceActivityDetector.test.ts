/** @vitest-environment jsdom */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

import { VoiceActivityDetector } from './VoiceActivityDetector';

let currentValue = 0;

const analyser = {
  fftSize: 0,
  smoothingTimeConstant: 0,
  frequencyBinCount: 4,
  getByteFrequencyData: (arr: Uint8Array) => {
    arr.fill(currentValue);
  },
};

class MockAudioContext {
  state = 'running';
  createAnalyser() {
    return analyser;
  }
  createMediaStreamSource() {
    return { connect: vi.fn(), disconnect: vi.fn() };
  }
  resume = vi.fn(async () => {});
  close = vi.fn(async () => {});
}

describe('VoiceActivityDetector', () => {
  const originalAudioContext = (window as unknown as { AudioContext?: unknown }).AudioContext;
  const originalMediaDevices = navigator.mediaDevices;

  beforeEach(() => {
    (window as unknown as { AudioContext?: unknown }).AudioContext = MockAudioContext as never;
    currentValue = 0;
    vi.useFakeTimers();
    vi.setSystemTime(0);
    Object.defineProperty(navigator, 'mediaDevices', {
      value: {
        getUserMedia: vi.fn(async () => ({
          getTracks: () => [{ stop: vi.fn() }],
        } as unknown as MediaStream)),
      },
      configurable: true,
    });
  });

  afterEach(() => {
    (window as unknown as { AudioContext?: unknown }).AudioContext = originalAudioContext;
    Object.defineProperty(navigator, 'mediaDevices', {
      value: originalMediaDevices,
      configurable: true,
    });
    vi.useRealTimers();
  });

  it('detecta atividade e silencio', async () => {
    const onActivityStart = vi.fn();
    const onActivityEnd = vi.fn();
    const onSilenceStart = vi.fn();

    const vad = new VoiceActivityDetector({
      activityThreshold: 0.1,
      activityDuration: 100,
      silenceThreshold: 0.1,
      silenceDuration: 100,
      checkInterval: 50,
      onActivityStart,
      onActivityEnd,
      onSilenceStart,
    });

    await vad.init();

    currentValue = 255;
    vad.start();
    vi.advanceTimersByTime(200);

    expect(onActivityStart).toHaveBeenCalled();

    currentValue = 0;
    vi.advanceTimersByTime(50);
    expect(onSilenceStart).toHaveBeenCalled();

    vi.advanceTimersByTime(150);
    expect(onActivityEnd).toHaveBeenCalled();

    vad.destroy();
  });
});
