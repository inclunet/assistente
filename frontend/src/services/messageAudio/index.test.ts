/** @vitest-environment jsdom */
import { describe, it, expect, vi, beforeEach } from 'vitest';

import { messageAudioService } from './index';

const getMessageAudioMock = vi.fn();
const generateAndSaveMessageAudioMock = vi.fn();
const saveMessageAudioMock = vi.fn();

vi.mock('@wailsjs/go/main/App', () => ({
  GetMessageAudio: (...args: unknown[]) => getMessageAudioMock(...args),
  GenerateAndSaveMessageAudio: (...args: unknown[]) => generateAndSaveMessageAudioMock(...args),
  SaveMessageAudio: (...args: unknown[]) => saveMessageAudioMock(...args),
}));

class MockAudio {
  paused = false;
  volume = 1;
  onended: (() => void) | null = null;
  onerror: (() => void) | null = null;

  play = vi.fn(async () => {
    this.onended?.();
  });
  pause = vi.fn();

  constructor(public src?: string) {}
}

class MockFileReader {
  onload: (() => void) | null = null;
  onerror: (() => void) | null = null;
  result: string | null = 'data:audio/mpeg;base64,AAA';

  readAsDataURL() {
    this.onload?.();
  }
}

describe('messageAudioService', () => {
  beforeEach(() => {
    getMessageAudioMock.mockReset();
    generateAndSaveMessageAudioMock.mockReset();
    saveMessageAudioMock.mockReset();

    globalThis.URL.createObjectURL = vi.fn(() => 'blob:url');
    globalThis.URL.revokeObjectURL = vi.fn();
    (globalThis as unknown as { Audio?: unknown }).Audio = MockAudio as never;
    (globalThis as unknown as { FileReader?: unknown }).FileReader = MockFileReader as never;
  });

  it('retorna audio do DB quando existe', async () => {
    getMessageAudioMock.mockResolvedValue({ audio: 'base64', mimeType: 'audio/mpeg' });

    const result = await messageAudioService.getAudioFromDB(1);

    expect(result).toEqual({ audio: 'base64', mimeType: 'audio/mpeg' });
  });

  it('reproduz audio base64', async () => {
    await messageAudioService.playAudioBase64('base64', 'audio/mpeg', 0.5);

    expect(globalThis.URL.createObjectURL).toHaveBeenCalled();
  });

  it('salva audio no DB', async () => {
    const blob = new Blob(['data'], { type: 'audio/mpeg' });

    await messageAudioService.saveAudioToDB(10, blob);

    expect(saveMessageAudioMock).toHaveBeenCalledWith(10, 'AAA', 'audio/mpeg');
  });

  it('converte base64 em blob', () => {
    const blob = messageAudioService.base64ToBlob('QQ==', 'audio/mpeg');

    expect(blob.type).toBe('audio/mpeg');
  });
});
