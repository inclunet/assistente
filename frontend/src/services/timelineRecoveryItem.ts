import type { apidto } from '@wailsjs/go/models';
import type { TimelineItem } from '../types/chatTimelineEvents';

export type RecoveryItemIdentity = Pick<apidto.TimelineRecoveryItemRequest, 'reference' | 'itemId' | 'epoch' | 'revision'>;
export interface RecoveryItemTransport {
  reserveItem(windowId: string, identity: RecoveryItemIdentity): Promise<apidto.TimelineRecoveryItemReservation>;
  readItem(windowId: string, identity: RecoveryItemIdentity): Promise<TimelineItem>;
  releaseItem(windowId: string, identity: RecoveryItemIdentity): Promise<void>;
}

export interface RecoveredTimelineItem {
  readonly surfaceId: string;
  readonly identity: RecoveryItemIdentity;
}

export function recoveryItemIdentity(epoch: string, item: TimelineItem): RecoveryItemIdentity | undefined {
  if (!item.recoveryReference) return;
  return { reference: { ...item.recoveryReference }, itemId: item.itemId, epoch, revision: item.revision };
}

export function recoveryItemKey(identity: RecoveryItemIdentity): string {
  const ref = identity.reference;
  return JSON.stringify([ref.projectionId, ref.incarnation, ref.turnId, ref.executionId,
    identity.itemId, identity.epoch, identity.revision]);
}

// Compara o DTO inteiro, sem inferir texto, segmentos ou ordem conversacional.
// A ordenação é apenas das chaves JSON; arrays mantêm a ordem emitida pelo backend.
export function sameRecoveryItem(expected: TimelineItem, actual: TimelineItem): boolean {
  const encode = (value: TimelineItem) => {
    const raw = JSON.stringify(value);
    if (new TextEncoder().encode(raw).length > 512 * 1024) throw new Error('Timeline recovery item exceeds limit');
    return JSON.stringify(value, (_key, field: unknown) => {
      if (!field || typeof field !== 'object' || Array.isArray(field)) return field;
      return Object.fromEntries(Object.entries(field).sort(([a], [b]) => a.localeCompare(b)));
    });
  };
  return encode(expected) === encode(actual);
}
