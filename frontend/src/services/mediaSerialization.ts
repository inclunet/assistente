import type { MediaFile } from './mediaService';

export interface SerializedMediaData {
  name: string;
  type: string;
  data: string;
  size: number;
}

export type MediaSerializationWorkerRequest =
  | { type: 'serialize'; files: File[] }
  | { type: 'cancel' };

export type MediaSerializationWorkerResponse =
  | { type: 'success'; mediaJson: string }
  | { type: 'error'; message: string }
  | { type: 'cancelled' };

type WorkerLike = Pick<Worker, 'postMessage' | 'terminate' | 'onmessage' | 'onerror'>;
type WorkerFactory = () => WorkerLike;

const activeSerializations = new Map<string, AbortController>();

const createWorker = (): WorkerLike => new Worker(
  new URL('./mediaSerialization.worker.ts', import.meta.url),
  { type: 'module', name: 'media-serialization' },
);

export function arrayBufferToBase64(buffer: ArrayBuffer): string {
  const bytes = new Uint8Array(buffer);
  const chunkSize = 3 * 8192;
  const chunks: string[] = [];
  for (let offset = 0; offset < bytes.length; offset += chunkSize) {
    const chunk = bytes.subarray(offset, Math.min(offset + chunkSize, bytes.length));
    let binary = '';
    for (let index = 0; index < chunk.length; index += 1) {
      binary += String.fromCharCode(chunk[index]);
    }
    chunks.push(btoa(binary));
  }
  return chunks.join('');
}

function readFileBuffer(file: File): Promise<ArrayBuffer> {
  if (typeof file.arrayBuffer === 'function') return file.arrayBuffer();
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result as ArrayBuffer);
    reader.onerror = () => reject(reader.error ?? new Error('Unable to read media file'));
    reader.readAsArrayBuffer(file);
  });
}

export async function serializeFilesToJson(files: File[]): Promise<string> {
  const data = await Promise.all(files.map(async (file): Promise<SerializedMediaData> => ({
    name: file.name,
    type: file.type,
    data: arrayBufferToBase64(await readFileBuffer(file)),
    size: file.size,
  })));
  return JSON.stringify(data);
}

export function serializeMediaFiles(
  mediaFiles: MediaFile[],
  signal?: AbortSignal,
  workerFactory: WorkerFactory = createWorker,
): Promise<string> {
  if (mediaFiles.length === 0) return Promise.resolve('');

  // Vitest/jsdom não fornece Worker. Produção Vite/Wails sempre usa o módulo
  // acima; o fallback mantém apenas os testes unitários independentes do browser.
  if (typeof Worker === 'undefined' && import.meta.env.MODE === 'test' && workerFactory === createWorker) {
    return serializeFilesToJson(mediaFiles.map((item) => item.file));
  }

  return new Promise((resolve, reject) => {
    const worker = workerFactory();
    let settled = false;
    const finish = (callback: () => void) => {
      if (settled) return;
      settled = true;
      signal?.removeEventListener('abort', abort);
      worker.terminate();
      callback();
    };
    const abort = () => {
      worker.postMessage({ type: 'cancel' } satisfies MediaSerializationWorkerRequest);
      finish(() => reject(new DOMException('Media serialization cancelled', 'AbortError')));
    };

    worker.onmessage = (event: MessageEvent<MediaSerializationWorkerResponse>) => {
      const response = event.data;
      if (response.type === 'success') {
        finish(() => resolve(response.mediaJson));
      } else if (response.type === 'error') {
        finish(() => reject(new Error(response.message)));
      } else {
        finish(() => reject(new DOMException('Media serialization cancelled', 'AbortError')));
      }
    };
    worker.onerror = (event) => {
      finish(() => reject(new Error(event.message || 'Media serialization worker failed')));
    };

    if (signal?.aborted) {
      abort();
      return;
    }
    signal?.addEventListener('abort', abort, { once: true });
    worker.postMessage({
      type: 'serialize',
      files: mediaFiles.map((item) => item.file),
    } satisfies MediaSerializationWorkerRequest);
  });
}

export async function serializeMediaForConversation(
  conversationId: string,
  mediaFiles: MediaFile[],
): Promise<string> {
  const previous = activeSerializations.get(conversationId);
  previous?.abort();
  const controller = new AbortController();
  activeSerializations.set(conversationId, controller);
  try {
    return await serializeMediaFiles(mediaFiles, controller.signal);
  } finally {
    if (activeSerializations.get(conversationId) === controller) {
      activeSerializations.delete(conversationId);
    }
  }
}

export function cancelMediaSerialization(conversationId: string): boolean {
  const controller = activeSerializations.get(conversationId);
  if (!controller) return false;
  controller.abort();
  activeSerializations.delete(conversationId);
  return true;
}
