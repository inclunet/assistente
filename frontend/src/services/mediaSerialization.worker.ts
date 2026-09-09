/// <reference lib="webworker" />

import {
  serializeFilesToJson,
  type MediaSerializationWorkerRequest,
  type MediaSerializationWorkerResponse,
} from './mediaSerialization';

const workerScope: DedicatedWorkerGlobalScope = self as unknown as DedicatedWorkerGlobalScope;
let cancelled = false;

workerScope.onmessage = async (event: MessageEvent<MediaSerializationWorkerRequest>) => {
  if (event.data.type === 'cancel') {
    cancelled = true;
    workerScope.postMessage({ type: 'cancelled' } satisfies MediaSerializationWorkerResponse);
    return;
  }

  cancelled = false;
  try {
    const mediaJson = await serializeFilesToJson(event.data.files);
    if (cancelled) {
      workerScope.postMessage({ type: 'cancelled' } satisfies MediaSerializationWorkerResponse);
      return;
    }
    workerScope.postMessage({ type: 'success', mediaJson } satisfies MediaSerializationWorkerResponse);
  } catch {
    workerScope.postMessage({ type: 'error' } satisfies MediaSerializationWorkerResponse);
  }
};

export {};
