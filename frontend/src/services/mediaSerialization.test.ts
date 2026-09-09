import { describe, expect, it, vi } from 'vitest';
import { MediaCategory, type MediaFile } from './mediaService';
import {
  arrayBufferToBase64,
  isMediaSerializationError,
  serializeFilesToJson,
  serializeMediaFiles,
  type MediaSerializationWorkerResponse,
} from './mediaSerialization';

const mediaFile = (file: File): MediaFile => ({
  id: file.name,
  file,
  category: MediaCategory.DOCUMENT,
  mimeType: file.type,
  extension: '.txt',
  fileName: file.name,
  fileSize: file.size,
  fileSizeFormatted: `${file.size} B`,
  icon: '',
});

describe('mediaSerialization Worker', () => {
  it('converte buffers grandes sem corromper fronteiras de base64', () => {
    const bytes = Uint8Array.from({ length: 50_003 }, (_, index) => index % 251);
    const expected = btoa(Array.from(bytes, (value) => String.fromCharCode(value)).join(''));
    expect(arrayBufferToBase64(bytes.buffer)).toBe(expected);
  });

  it('prepara múltiplos anexos e serializa o JSON uma única vez', async () => {
    const files = [
      new File(['primeiro'], 'a.txt', { type: 'text/plain' }),
      new File(['segundo'], 'b.txt', { type: 'text/plain' }),
      new File(['terceiro'], 'c.txt', { type: 'text/plain' }),
    ];

    const result = JSON.parse(await serializeFilesToJson(files)) as Array<Record<string, unknown>>;

    expect(result).toHaveLength(3);
    expect(result.map((item) => item.name)).toEqual(['a.txt', 'b.txt', 'c.txt']);
    expect(result.map((item) => atob(String(item.data)))).toEqual(['primeiro', 'segundo', 'terceiro']);
  });

  it('cancela preparação em andamento e termina o Worker', async () => {
    let onmessage: ((event: MessageEvent<MediaSerializationWorkerResponse>) => void) | null = null;
    const terminate = vi.fn();
    const postMessage = vi.fn();
    const workerFactory = () => ({
      postMessage,
      terminate,
      get onmessage() { return onmessage; },
      set onmessage(value) { onmessage = value; },
      onerror: null,
    });
    const controller = new AbortController();
    const promise = serializeMediaFiles(
      [mediaFile(new File(['dados'], 'cancel.txt', { type: 'text/plain' }))],
      controller.signal,
      workerFactory,
    );

    controller.abort();

    await expect(promise).rejects.toMatchObject({ name: 'AbortError' });
    expect(postMessage).toHaveBeenCalledWith({ type: 'cancel' });
    expect(terminate).toHaveBeenCalledOnce();
  });

  it('converte falha técnica do Worker em erro codificado para tradução na UI', async () => {
    let onmessage: ((event: MessageEvent<MediaSerializationWorkerResponse>) => void) | null = null;
    const workerFactory = () => ({
      postMessage: vi.fn(() => {
        queueMicrotask(() => onmessage?.(
          new MessageEvent<MediaSerializationWorkerResponse>('message', { data: { type: 'error' } }),
        ));
      }),
      terminate: vi.fn(),
      get onmessage() { return onmessage; },
      set onmessage(value) { onmessage = value; },
      onerror: null,
    });

    const promise = serializeMediaFiles(
      [mediaFile(new File(['dados'], 'falha.txt', { type: 'text/plain' }))],
      undefined,
      workerFactory,
    );

    await expect(promise).rejects.toSatisfy(isMediaSerializationError);
  });
});
