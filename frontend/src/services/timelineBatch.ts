import type {
  TimelineItem,
  TimelineOperation,
  TimelineUpdate,
  TimelineWindowMetadata,
} from '../types/chatTimelineEvents';

/** Uma janela aceita; sequence é local à capability, não ao cache compartilhado. */
export interface TimelineBatchWindow {
  conversationId: string;
  scopeParentId: string | null;
  epoch: string;
  windowId: string;
  revision: string;
  sequence: string;
  orderedItemIds: readonly string[];
  metadata: TimelineWindowMetadata;
}

/** Tombstones participam da comparação de revisão até o descarte seguro pelo dono do cache. */
export type TimelineCacheEntry =
  | { revision: string; item: TimelineItem }
  | { revision: string; item: null };

export type TimelineBatchResult =
  | { kind: 'duplicate' }
  | { kind: 'recover'; reason: 'epoch' | 'gap' | 'patch' | 'content' }
  | {
      kind: 'applied';
      window: TimelineBatchWindow;
      writes: ReadonlyMap<string, TimelineCacheEntry>;
    };

const maxUint64 = (1n << 64n) - 1n;

function revision(value: string): bigint {
  if (!/^(0|[1-9][0-9]*)$/.test(value) || value.length > 20) throw new Error('revision');
  const parsed = BigInt(value);
  if (parsed > maxUint64) throw new Error('revision');
  return parsed;
}

function validItem(item: TimelineItem, window: TimelineBatchWindow, sequence: bigint): void {
  if (!item.itemId || item.conversationId !== window.conversationId ||
      revision(item.revision) > sequence ||
      new Set(item.segments.map(segment => segment.segmentId)).size !== item.segments.length ||
      item.segments.some(segment => !segment.segmentId)) throw new Error('item');
}

/**
 * Prepara um único lote sem alterar janela ou cache. O proprietário deve publicar
 * writes + window na mesma transação síncrona, antes de preparar outro lote.
 * read deve consultar exclusivamente o cache da conversa/época da janela.
 * Não ordena itens, não cria identidades e não controla RPCs, retenção ou sessão.
 */
export function prepareTimelineBatch(
  window: TimelineBatchWindow,
  update: TimelineUpdate,
  read: (itemId: string) => TimelineCacheEntry | undefined,
): TimelineBatchResult {
  if (update.conversationId !== window.conversationId || update.epoch !== window.epoch) {
    return { kind: 'recover', reason: 'epoch' };
  }
  let sequence: bigint;
  try {
    sequence = revision(update.sequence);
    const previous = revision(window.sequence);
    if (sequence <= previous) return { kind: 'duplicate' };
    if (sequence !== previous + 1n) return { kind: 'recover', reason: 'gap' };
  } catch {
    return { kind: 'recover', reason: 'gap' };
  }

  const writes = new Map<string, TimelineCacheEntry>();
  const current = (id: string) => writes.get(id) ?? read(id);
  let nextWindow = { ...window, sequence: update.sequence };
  const patches = update.windowPatches.filter(patch => patch.windowId === window.windowId);
  if (patches.length > 1) return { kind: 'recover', reason: 'patch' };
  const patch = patches[0];
  let orderedIds = window.orderedItemIds;

  try {
    if (patch) {
      if (patch.syncRequired || patch.previousRevision !== window.revision ||
          revision(patch.revision) <= revision(window.revision)) throw new Error('patch');
      if (patch.splices?.length) {
        const ids = [...orderedIds];
        for (const splice of patch.splices) {
          if (!Number.isSafeInteger(splice.index) || !Number.isSafeInteger(splice.deleteCount) ||
              splice.index < 0 || splice.index > ids.length || splice.deleteCount < 0 ||
              splice.deleteCount > ids.length - splice.index || splice.insertIds.some(id => !id)) {
            throw new Error('splice');
          }
          ids.splice(splice.index, splice.deleteCount, ...splice.insertIds);
        }
        if (new Set(ids).size !== ids.length) throw new Error('duplicate ID');
        orderedIds = ids;
      }
      nextWindow = {
        ...nextWindow, revision: patch.revision, orderedItemIds: orderedIds,
        metadata: patch.metadata ? structuredClone(patch.metadata) : window.metadata,
      };
    }
  } catch {
    return { kind: 'recover', reason: 'patch' };
  }

  try {
    // Hidratação completa pode ser mais antiga que o cache de outra janela.
    const hydrate = (item: TimelineItem) => {
      validItem(item, window, sequence);
      const existing = current(item.itemId);
      if (!existing || revision(existing.revision) < revision(item.revision)) {
        writes.set(item.itemId, { revision: item.revision, item: structuredClone(item) });
      }
    };
    for (const item of patch?.items ?? []) hydrate(item);

    // Vários segmentos do mesmo item compartilham a revisão do lote. Comparar
    // revisão por operação descartaria o segundo segmento após aplicar o primeiro.
    const groups = new Map<string, TimelineOperation[]>();
    for (const operation of update.operations) {
      if (!operation.itemId || revision(operation.revision) !== sequence ||
          !['item_upsert', 'item_remove', 'segment_append', 'segment_replace'].includes(operation.kind)) {
        throw new Error('operation');
      }
      const group = groups.get(operation.itemId) ?? [];
      group.push(operation);
      groups.set(operation.itemId, group);
    }
    for (const [id, operations] of groups) {
      const existing = current(id);
      if (existing && revision(existing.revision) >= sequence) continue;
      let item = existing?.item;
      for (const operation of operations) {
        switch (operation.kind) {
          case 'item_remove':
            item = null;
            writes.set(id, { revision: update.sequence, item });
            break;
          case 'item_upsert':
            if (!operation.item || operation.item.itemId !== id ||
                operation.item.revision !== update.sequence) throw new Error('upsert');
            validItem(operation.item, window, sequence);
            // Operação fora da janela só atualiza item já conhecido.
            if (existing || orderedIds.includes(id)) {
              item = structuredClone(operation.item);
              writes.set(id, { revision: update.sequence, item });
            }
            break;
          default: {
            if (!item) {
              if (existing || orderedIds.includes(id)) throw new Error('missing item');
              break;
            }
            const index = item.segments.findIndex(segment => segment.segmentId === operation.segmentId);
            if (index < 0) throw new Error('segment');
            const previous = item.segments[index];
            const segments = [...item.segments];
            if (operation.kind === 'segment_append') {
              if (typeof operation.text !== 'string') throw new Error('text');
              segments[index] = { ...previous, text: (previous.text ?? '') + operation.text };
            } else {
              if (!operation.segment || operation.segment.segmentId !== operation.segmentId ||
                  operation.segment.kind !== previous.kind) throw new Error('replace');
              segments[index] = structuredClone(operation.segment);
            }
            item = { ...item, revision: update.sequence, segments };
            writes.set(id, { revision: update.sequence, item });
          }
        }
      }
    }

    // Não inferir remoção global a partir da ausência em uma página. Um ID novo
    // precisa de corpo válido; tombstone nunca é uma fonte de hidratação.
    if (patch?.splices?.length) {
      for (const id of orderedIds) {
        const entry = current(id);
        if (!entry?.item || entry.item.scopeParentId !== window.scopeParentId) throw new Error('membership');
      }
    }
    for (const [id, entry] of writes) {
      if ((!entry.item || entry.item.scopeParentId !== window.scopeParentId) && orderedIds.includes(id)) {
        throw new Error('membership');
      }
    }
  } catch {
    return { kind: 'recover', reason: 'content' };
  }
  return { kind: 'applied', window: nextWindow, writes };
}
