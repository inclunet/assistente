import { describe, expect, it } from 'vitest';
import { canContinueChatMessage } from './chatContinuation';
import { canonicalTestFixture, canonicalTestItem } from '../services/chatTimelineTestFixture';
import type { TimelineItem } from '../types/chatTimelineEvents';

const persisted = '01926b90-7a5a-7c4e-8d3f-000000000001';
describe('elegibilidade de continuação da fonte canônica', () => {
  it.each(['error', 'cancelled', 'output_limit', 'limit_reached'])('aceita parcial terminal %s após reabrir', async reason => {
    const item = canonicalTestItem('response', 'resposta parcial', {
      state: reason === 'error' ? 'failed' : 'interrupted', turnId: persisted, representativeMessageId: persisted,
    });
    item.metadata.terminalReason = reason;
    const fixture = canonicalTestFixture([item]);
    try {
      const presentation = fixture.presentation(); await presentation.open();
      expect(canContinueChatMessage(presentation.getMessage('response'))).toBe(true);
      await presentation.close();
      const reopened = fixture.presentation('reopened'); await reopened.open();
      expect(canContinueChatMessage(reopened.getMessage('response'))).toBe(true);
    } finally { await fixture.client.dispose(); }
  });

  it.each<Partial<TimelineItem>>([
    { state: 'preparing' }, { state: 'streaming' }, { state: 'completed' }, { state: 'historical_unknown' },
    { durability: 'volatile' },
    { metadata: { ...canonicalTestItem('question').metadata, role: 'user' } }, { scopeParentId: persisted }, { turnId: null }, { representativeMessageId: null },
    { turnId: 'local-turn' }, { representativeMessageId: 'local-message' },
  ])('recusa alvo inelegível: %j', async overrides => {
    const item = canonicalTestItem('response', 'resposta parcial', {
      state: 'interrupted', turnId: persisted, representativeMessageId: persisted, ...overrides,
    });
    item.metadata.terminalReason = 'cancelled';
    const fixture = canonicalTestFixture([item]);
    try {
      const presentation = fixture.presentation(); await presentation.open();
      expect(canContinueChatMessage(presentation.getMessage('response'))).toBe(false);
    } finally { await fixture.client.dispose(); }
  });
  it.each(['no-reason', 'no-content'])('recusa resposta sem fundamento para continuar: %s', async missing => {
    const item = canonicalTestItem('response', missing === 'no-content' ? ' ' : 'parcial', {
      state: 'interrupted', turnId: persisted, representativeMessageId: persisted,
    });
    if (missing !== 'no-reason') item.metadata.terminalReason = 'cancelled';
    const fixture = canonicalTestFixture([item]);
    try {
      const presentation = fixture.presentation(); await presentation.open();
      expect(canContinueChatMessage(presentation.getMessage('response'))).toBe(false);
    } finally { await fixture.client.dispose(); }
  });
});
