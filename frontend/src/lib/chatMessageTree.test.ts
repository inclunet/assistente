import { afterEach, describe, expect, it } from 'vitest';
import { canonicalTestFixture, canonicalTestItem } from '../services/chatTimelineTestFixture';
import type { TimelineItem } from '../types/chatTimelineEvents';

const clients: ReturnType<typeof canonicalTestFixture>['client'][] = [];
afterEach(async () => { await Promise.all(clients.splice(0).map(client => client.dispose())); });
function fixture(items: TimelineItem[]) {
  const f = canonicalTestFixture(items);
  clients.push(f.client);
  return { ...f, view: f.presentation() };
}
const item = (id: string, content = id, scope: string | null = null) => canonicalTestItem(id, content, {
  scopeParentId: scope, representativeMessageId: id, messageIds: [id],
});

describe('regressões da árvore migradas para a apresentação canônica', () => {
  it('encontra mensagens em threads sem reconstruir a árvore por IDs físicos', async () => {
    const f = fixture([item('root'), item('child', 'child', 'root'), item('grandchild', 'grandchild', 'child')]);
    await f.view.open(); await f.view.openThread('root'); await f.view.openThread('child');
    expect(f.view.nodes()[0].children?.[0].children?.[0].itemId).toBe('grandchild');
    expect(f.view.getMessage('grandchild')?.content).toBe('grandchild');
    expect(f.view.getMessage('missing')).toBeUndefined();
  });

  it('atualiza filho por publicação canônica e preserva referências dos irmãos', async () => {
    const child = item('child', 'old', 'root');
    const f = fixture([item('root'), item('sibling'), child, item('untouched', 'same', 'root')]);
    await f.view.open(); await f.view.openThread('root');
    const before = f.view.nodes();
    const untouched = before[0].children?.[1];
    f.publish('surface:thread:root', { ...child, segments: [{ segmentId: 'text', kind: 'text', text: 'new' }] });
    const after = f.view.nodes();
    expect(after[0].children?.[0].message.content).toBe('new');
    expect(after[0].children?.[0].message).not.toBe(before[0].children?.[0].message);
    expect(after[0].children?.[1]).toBe(untouched);
    expect(after[1]).toBe(before[1]);
    expect(f.view.nodes()).toEqual(after);
  });

  it('edição substitui a projeção recebida sem overrides e preserva filhos', async () => {
    const original = item('root', 'old');
    const f = fixture([original, item('child', 'child', 'root')]);
    await f.view.open(); await f.view.openThread('root');
    const child = f.view.nodes()[0].children?.[0];
    f.publish('surface', { ...original, state: 'streaming', segments: [{ segmentId: 'text', kind: 'text', text: 'delta' }] });
    expect(f.view.getMessage('root')?.content).toBe('delta');
    f.publish('surface', item('root', 'edited'));
    const message = f.view.getMessage('root')!;
    expect(message.content).toBe('edited');
    expect(message.turnSegments).toEqual([{ type: 'text', content: 'edited' }]);
    expect(message).not.toHaveProperty('_turnSegments');
    expect(message).not.toHaveProperty('_resumableContent');
    expect(f.view.nodes()[0].children?.[0]).toBe(child);
  });

  it('retry substitui segmentos da execução anterior no mesmo item visual', async () => {
    const original = item('response', 'Parcial');
    const f = fixture([original]); await f.view.open();
    f.publish('surface', { ...original, executionId: 'retry', state: 'preparing', segments: [],
      metadata: { ...original.metadata, conclusionSegmentIds: [] } });
    expect(f.view.nodes()[0].itemId).toBe('response');
    expect(f.view.getMessage('response')?.turnSegments).toEqual([]);
    expect(f.view.getMessage('response')?.content).toBe('');
    f.publish('surface', { ...item('response', 'Novo'), executionId: 'retry' });
    expect(f.view.getMessage('response')?.content).toBe('Novo');
    expect(f.view.getMessage('response')?.turnSegments).toEqual([{ type: 'text', content: 'Novo' }]);
  });

  it('projeta edição sem ferramentas diretamente dos segmentos do backend', async () => {
    const original = item('root', 'old');
    const f = fixture([original]); await f.view.open();
    f.publish('surface', item('root', 'new'));
    expect(f.view.getMessage('root')?.turnSegments).toEqual([{ type: 'text', content: 'new' }]);
    expect(original.segments[0].text).toBe('old');
  });

  it('abre filhos apenas no escopo solicitado sem substituir as mensagens ancestrais', async () => {
    const f = fixture([item('root'), item('parent', 'parent', 'root'), item('loaded', 'loaded', 'parent')]);
    await f.view.open(); await f.view.openThread('root');
    const before = f.view.nodes();
    await f.view.openThread('parent');
    const after = f.view.nodes();
    expect(after[0].children?.[0].children?.[0].itemId).toBe('loaded');
    expect(after[0].message).toBe(before[0].message);
    expect(after[0].children?.[0].message).toBe(before[0].children?.[0].message);
  });

  it('atribui alvo persistido sem substituir a identidade visual durante streaming', async () => {
    const pending = item('turn:question', 'partial');
    const f = fixture([{ ...pending, state: 'streaming', representativeMessageId: null, messageIds: [] }]);
    await f.view.open();
    const before = f.view.nodes()[0];
    f.publish('surface', { ...pending, representativeMessageId: 'backend', messageIds: ['backend'] });
    expect(f.view.nodes()[0].itemId).toBe(before.itemId);
    expect(f.view.nodes()[0].message.id).toBe('turn:question');
    expect(f.view.nodes()[0].message.timelineItem?.representativeMessageId).toBe('backend');
    expect(f.view.nodes()[0].message.isStreaming).toBe(false);
  });

  it('preserva o turno fornecido pelo backend ao concluir streaming', async () => {
    const response = { ...item('response', 'partial'), turnId: 'turn-1', state: 'streaming' };
    const f = fixture([response]); await f.view.open();
    f.publish('surface', { ...response, state: 'completed' });
    expect(f.view.nodes()[0].message.turnId).toBe('turn-1');
    expect(f.view.nodes()[0].message.isStreaming).toBe(false);
    expect(f.view.nodes()[0].itemId).toBe('response');
  });
});
