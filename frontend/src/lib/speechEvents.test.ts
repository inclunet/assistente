import { describe, it, expect } from 'vitest';
import {
  TTS_STREAM_START,
  TTS_STREAM_CHUNK,
  TTS_STREAM_DONE,
  TTS_STREAM_ERROR,
} from './speechEvents';

describe('speechEvents', () => {
  it('exporta todas as constantes de eventos TTS', () => {
    expect(TTS_STREAM_START).toBe('tts:stream:start');
    expect(TTS_STREAM_CHUNK).toBe('tts:stream:chunk');
    expect(TTS_STREAM_DONE).toBe('tts:stream:done');
    expect(TTS_STREAM_ERROR).toBe('tts:stream:error');
  });

  it('constantes seguem o padrão tts:stream:*', () => {
    const events = [TTS_STREAM_START, TTS_STREAM_CHUNK, TTS_STREAM_DONE, TTS_STREAM_ERROR];
    for (const event of events) {
      expect(event).toMatch(/^tts:stream:[a-z]+$/);
    }
  });
});
