import { beforeEach } from 'vitest';
import type { MessageNode } from '../lib/chatMessageTree';
import type { TimelineWindowMetadata } from '../types/chatTimelineEvents';

export function windowFixture(overrides: Partial<TimelineWindowMetadata> = {}): TimelineWindowMetadata {
  return { totalCount: 1, startIndex: 0, endIndex: 0, hasBefore: false, hasAfter: false,
    retainedItemIds: [], retentionBlocked: false, beforeCursor: '', afterCursor: '', ...overrides };
}

type FixtureOwner = { id?: string; sessionKey?: string; conversation?: { id: string } | null };
type FixtureProjection = { nodes?: MessageNode[]; metadata?: TimelineWindowMetadata };
const projections = new Map<string, FixtureProjection>();
const ownerKey = (owner: FixtureOwner) => {
  if (owner.sessionKey) return owner.sessionKey;
  if (owner.id) return `conversation:${owner.id}`;
  if (owner.conversation?.id) return `conversation:${owner.conversation.id}`;
  return undefined;
};
beforeEach(() => projections.clear());

/** Dados injetados nos testes de apresentação; nunca são gravados no store do produto. */
export function fixtureProjection(owner?: FixtureOwner): FixtureProjection {
  const key = owner && ownerKey(owner);
  return key ? projections.get(key) ?? {} : {};
}

export function withFixtureProjection<T extends FixtureOwner>(owner: T, projection: FixtureProjection): T {
  const key = ownerKey(owner);
  if (!key) throw new Error('A fixture de apresentação exige uma superfície ou conversa identificada');
  projections.set(key, { ...fixtureProjection(owner), ...projection });
  return owner;
}
