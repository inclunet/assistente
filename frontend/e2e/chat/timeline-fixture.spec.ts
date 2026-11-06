import { test, expect } from '../fixtures';

test('fixture limpa uma conversa explícita sem alterar outra, antes e depois da navegação', async ({ page, wails }) => {
  const rows = (conversationId: string) => [{ message: { id: conversationId + '-message', conversationId, role: 'user', content: 'conteúdo', createdAt: '2026-01-01T00:00:00Z' }, children: [] }];
  await wails.setTimelineRows(rows('other-a'));
  await wails.setTimelineRows(rows('other-b'));
  await wails.setTimelineRows([], undefined, 'other-a');
  await wails.waitForApp();
  const read = (conversationId: string) => page.evaluate(id => window.go.wailsapi.ChatTimeline.Open({ conversationId: id, surfaceId: id + '-fixture', scopeParentId: null, anchor: 'start', direction: 'after', limit: 20 }), conversationId);
  expect(await read('other-a')).toMatchObject({ items: [] });
  expect(await read('other-b')).toMatchObject({ items: [{ representativeMessageId: 'other-b-message' }] });
  await wails.setTimelineRows(rows('other-a'));
  await wails.setTimelineRows([], undefined, 'other-b');
  expect(await read('other-b')).toMatchObject({ items: [] });
  expect(await read('other-a')).toMatchObject({ items: [{ representativeMessageId: 'other-a-message' }] });
});
