import { getChatTimelineClient } from './chatTimelineWails';
import type { ChatSurfaceOrigin } from './chatSessionRegistry';
import type { TimelineItem } from '../types/chatTimelineEvents';
import { logger } from '../utils/logger';

export interface ChatCompletion {
  conversationId: string;
  itemId?: string;
  turnId?: string;
  assistantMessageId?: string;
  reason?: string;
  errorMessage?: string;
  surfaceOrigin?: ChatSurfaceOrigin;
}

export function canonicalConclusion(item: TimelineItem): string {
  return item.segments.filter(segment => segment.conclusion).map(segment => {
    if (segment.kind !== 'text') throw new Error('Canonical conclusion unavailable');
    return segment.text ?? '';
  }).join('');
}

/** Consulta a resposta exata sem deslocar a janela que o usuário está lendo. */
export async function readChatCompletion(completion: ChatCompletion, signal?: AbortSignal): Promise<string> {
  if (completion.reason !== 'completed' || completion.errorMessage || !completion.itemId
    || !completion.turnId || !completion.assistantMessageId) throw new Error('Chat completion unavailable');
  signal?.throwIfAborted();
  const client = getChatTimelineClient();
  const surface = `completion:${crypto.randomUUID()}`;
  let closing: Promise<void> | undefined;
  const close = () => closing ??= client.close(completion.conversationId, surface);
  let abort!: () => void;
  const cancelled = new Promise<never>((_resolve, reject) => {
    abort = () => {
      // Close invalida a superfície antes de Open retornar sua capability.
      // O cliente fecha a capability remota assim que ela se torna conhecida.
      void close().catch(error => logger.warn('Chat completion cleanup failed', error));
      reject(signal?.reason ?? new DOMException('Aborted', 'AbortError'));
    };
  });
  signal?.addEventListener('abort', abort, { once: true });
  try {
    // Open consulta o backend depois do terminal. A entrega independente do
    // evento visual não exige polling nem leitura de uma árvore anterior.
    await Promise.race([client.open(completion.conversationId, surface, null, {
      cursor: '', anchor: '', anchorItemId: completion.itemId, direction: 'around', limit: 1,
    }), cancelled]);
    signal?.throwIfAborted();
    const item = client.getItem(completion.conversationId, surface, completion.itemId);
    if (client.getError(completion.conversationId) || !item || item.state !== 'completed'
      || item.turnId !== completion.turnId || item.representativeMessageId !== completion.assistantMessageId) {
      throw new Error('Chat completion replaced or unavailable');
    }
    return canonicalConclusion(item);
  } finally {
    signal?.removeEventListener('abort', abort);
    if (!signal?.aborted && client.hasSurface(completion.conversationId, surface)) await close();
  }
}
