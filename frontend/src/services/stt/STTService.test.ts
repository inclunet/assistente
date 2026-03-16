/** @vitest-environment jsdom */
import { describe, it, expect, vi, beforeEach } from 'vitest';

import { STTService } from './STTService';
import { RECORDING_MODES, STT_PROVIDERS } from './types';

const mockState = vi.hoisted(() => ({
  webSpeechInstances: [] as Array<{ start: () => boolean; init: () => Promise<boolean>; options: Record<string, unknown> }>,
  whisperInstances: [] as Array<{ init: () => Promise<boolean> }>,
  audioRecorderInstances: [] as Array<{ init: () => Promise<boolean> }>,
}));

vi.mock('./providers/WebSpeechProvider', () => {
  class MockWebSpeechProvider {
    static checkSupport = vi.fn(() => true);
    init = vi.fn(async () => true);
    start = vi.fn(() => true);
    stop = vi.fn();
    abort = vi.fn();
    setLanguage = vi.fn();
    destroy = vi.fn();

    constructor(public options: Record<string, unknown>) {
      mockState.webSpeechInstances.push(this);
    }
  }

  return { WebSpeechProvider: MockWebSpeechProvider };
});

vi.mock('./providers/WhisperProvider', () => {
  class MockWhisperProvider {
    isSupported = true;
    init = vi.fn(async () => true);
    start = vi.fn(async () => true);
    stop = vi.fn();
    abort = vi.fn();
    setLanguage = vi.fn();
    destroy = vi.fn();

    constructor(public options: Record<string, unknown>) {
      mockState.whisperInstances.push(this);
    }
  }

  return { WhisperProvider: MockWhisperProvider };
});

vi.mock('./AudioRecorder', () => {
  class MockAudioRecorder {
    hasActiveStream = false;
    init = vi.fn(async () => true);
    start = vi.fn();
    stop = vi.fn();
    destroy = vi.fn();
    releaseStream = vi.fn();

    constructor(public options: Record<string, unknown>) {
      mockState.audioRecorderInstances.push(this);
    }
  }

  return { AudioRecorder: MockAudioRecorder };
});

vi.mock('../vad', () => {
  class MockVAD {
    active = false;
    init = vi.fn(async () => true);
    start = vi.fn();
    stop = vi.fn();
    destroy = vi.fn();
  }

  return { VoiceActivityDetector: MockVAD };
});

describe('STTService', () => {
  beforeEach(() => {
    mockState.webSpeechInstances.splice(0, mockState.webSpeechInstances.length);
    mockState.whisperInstances.splice(0, mockState.whisperInstances.length);
    mockState.audioRecorderInstances.splice(0, mockState.audioRecorderInstances.length);
  });

  it('inicializa providers e recorder', async () => {
    const service = new STTService({ provider: STT_PROVIDERS.WEBSPEECH });

    const ok = await service.init();

    expect(ok).toBe(true);
    expect(mockState.webSpeechInstances).toHaveLength(1);
    expect(mockState.whisperInstances).toHaveLength(1);
    expect(mockState.audioRecorderInstances).toHaveLength(1);
    expect(mockState.webSpeechInstances[0].init).toHaveBeenCalled();
  });

  it('usa WebSpeech em PTT', async () => {
    const service = new STTService({
      provider: STT_PROVIDERS.WEBSPEECH,
      mode: RECORDING_MODES.PTT,
    });

    await service.init();
    await service.startRecording();

    expect(mockState.webSpeechInstances[0].start).toHaveBeenCalled();
  });

  it('dispara callback de transcricao', async () => {
    const onTranscription = vi.fn();
    const service = new STTService({ onTranscription });

    await service.init();

    const instance = mockState.webSpeechInstances[0];
    const onEnd = instance.options.onEnd as (text: string) => void;

    onEnd('Ola mundo');

    expect(onTranscription).toHaveBeenCalledWith('Ola mundo', STT_PROVIDERS.WEBSPEECH);
  });
});
