/** @vitest-environment jsdom */
import { describe, it, expect, vi, beforeEach } from 'vitest';

let playSound: typeof import('./audioFeedback').playSound;
let resumeAudioContext: typeof import('./audioFeedback').resumeAudioContext;
let SOUND_TYPES: typeof import('./audioFeedback').SOUND_TYPES;

let oscillatorStarts = 0;
let lastContext: MockAudioContext | null = null;

class MockOscillator {
  frequency = { setValueAtTime: vi.fn() };
  type = 'sine';
  connect = vi.fn();
  start = vi.fn(() => { oscillatorStarts += 1; });
  stop = vi.fn();
}

class MockGain {
  gain = {
    value: 0,
    setValueAtTime: vi.fn(),
    linearRampToValueAtTime: vi.fn(),
    exponentialRampToValueAtTime: vi.fn(),
  };
  connect = vi.fn();
}

class MockAudioContext {
  currentTime = 0;
  state = 'running';
  destination = {};
  createdOscillators: MockOscillator[] = [];

  constructor() {
    lastContext = this;
  }

  createOscillator() {
    const osc = new MockOscillator();
    this.createdOscillators.push(osc);
    return osc;
  }

  createGain() {
    return new MockGain();
  }

  resume = vi.fn(async () => {});
}

describe('audioFeedback', () => {
  beforeEach(async () => {
    oscillatorStarts = 0;
    (window as unknown as { AudioContext?: unknown }).AudioContext = MockAudioContext as never;
    lastContext = null;

    vi.resetModules();
    const mod = await import('./audioFeedback');
    playSound = mod.playSound;
    resumeAudioContext = mod.resumeAudioContext;
    SOUND_TYPES = mod.SOUND_TYPES;
  });

  it('reproduz sons sem erro', () => {
    expect(() => playSound(SOUND_TYPES.SEND)).not.toThrow();
    expect(oscillatorStarts).toBeGreaterThan(0);
  });

  it('toca o som de erro "tun tum" reusando a nota grave (330Hz) duas vezes', () => {
    expect(() => playSound(SOUND_TYPES.ERROR)).not.toThrow();
    expect(oscillatorStarts).toBe(2);

    const freqs = (lastContext?.createdOscillators ?? []).flatMap((osc) =>
      osc.frequency.setValueAtTime.mock.calls.map((call) => call[0] as number)
    );
    expect(freqs).toEqual([330, 330]);
  });

  it('toca o som de alerta agudo→médio (880Hz, 660Hz)', () => {
    expect(() => playSound(SOUND_TYPES.ALERT)).not.toThrow();
    expect(oscillatorStarts).toBe(2);

    const freqs = (lastContext?.createdOscillators ?? []).flatMap((osc) =>
      osc.frequency.setValueAtTime.mock.calls.map((call) => call[0] as number)
    );
    expect(freqs).toEqual([880, 660]);
  });

  it('avisa em tipos desconhecidos', () => {
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {});

    playSound('unknown' as never);

    expect(warnSpy).toHaveBeenCalled();
    warnSpy.mockRestore();
  });

  it('resume AudioContext quando suspenso', async () => {
    class SuspendedAudioContext extends MockAudioContext {
      state = 'suspended';
    }

    (window as unknown as { AudioContext?: unknown }).AudioContext = SuspendedAudioContext as never;

    await resumeAudioContext();
    expect(lastContext?.resume).toHaveBeenCalledTimes(1);
  });
});
