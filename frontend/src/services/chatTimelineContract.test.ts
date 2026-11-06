import { describe, expect, it } from 'vitest';
import { canonicalTestFixture, canonicalTestItem } from './chatTimelineTestFixture';

const node = (id: string, timestamp: number, position?: number) => {
  const item = canonicalTestItem(id, id, { representativeMessageId: id });
  return { ...item, position: position === undefined ? 'opaque' : String(position),
    metadata: { ...item.metadata, timestamp, role: id === 'user' ? 'user' : 'assistant' } };
};

// O backend determina a ordem; timestamps e positions opacas nunca a reordenam.
// A classificação no produtor é coberta por TestTimelineOrderSnapshotKeepsSQLClassificationAndIndexes.
describe('AEP-0116 — ordenação canônica', () => {
  it('preserva pergunta antes da resposta mesmo com horário local posterior', async () => {
    const f = canonicalTestFixture([node('user', 200), node('assistant', 150)]);
    const presentation = f.presentation('order'); await presentation.open();
    const ordered = presentation.nodes();
    expect(ordered.map(item => item.message.id)).toEqual(['user', 'assistant']);
    await presentation.close();
  });

  it('preserva a precedência das posições publicada pelo backend', async () => {
    const a = node('a', 300, 0);
    const b = node('b', 100, 1);
    const f = canonicalTestFixture([a, b]);
    const presentation = f.presentation('order'); await presentation.open();
    expect(presentation.nodes().map(item => item.itemId)).toEqual(['a', 'b']);
    await presentation.close();
  });

  it('mantém relação transitiva sem comparar posições opacas com horários', async () => {
    const a = node('a', 300, 0);
    const b = node('b', 100, 1);
    const c = node('c', 200);
    const f = canonicalTestFixture([a, b, c]);
    const presentation = f.presentation('order'); await presentation.open();
    const ordered = presentation.nodes().map(item => item.itemId);
    expect(ordered).toEqual(['a', 'b', 'c']);
    const before = (left: typeof a, right: typeof a) =>
      ordered.indexOf(left.itemId) < ordered.indexOf(right.itemId);
    const nodes = [a, b, c];
    for (const x of nodes) {
      for (const y of nodes) {
        for (const z of nodes) {
          if (x !== y && y !== z && x !== z && before(x, y) && before(y, z)) {
            expect(before(x, z)).toBe(true);
          }
        }
      }
    }
    await presentation.close();
  });
});
