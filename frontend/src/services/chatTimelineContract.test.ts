import { describe, expect, it } from 'vitest';
import { sortTimelineNodes } from './chatSessionRegistry';
import type { MessageNode } from '../lib/chatMessageTree';
import { chat } from '../../wailsjs/go/models';

const node = (id: string, timestamp: number, originalIndex?: number): MessageNode => chat.MessageNode.createFrom({
  message: { id, role: id === 'user' ? 'user' : 'assistant', timestamp,
    conversationId: 'conversation', content: id,
    turnId: id === 'user' ? undefined : id === 'assistant' ? 'user' : `question-${id}` },
  originalIndex, children: [], childCount: 0, level: 0,
});

// AEP-0116 fase 1: contratos desejados ainda violados pelo algoritmo atual.
// it.fails exige a falha: uma correção torna o teste vermelho até retirar .fails.
// Não substituir a expectativa pela ordem defeituosa para deixar o CI verde.
describe('AEP-0116 — lacunas conhecidas de ordenação', () => {
  it.fails('preserva pergunta antes da resposta mesmo com horário local posterior', () => {
    const ordered = sortTimelineNodes([node('user', 200), node('assistant', 150)]);
    expect(ordered.map(item => item.message.id)).toEqual(['user', 'assistant']);
  });

  it('preserva a precedência dos índices quando ambos os itens têm índice', () => {
    const a = node('a', 300, 0);
    const b = node('b', 100, 1);
    expect(sortTimelineNodes([b, a])).toEqual([a, b]);
  });

  it.fails('define uma relação transitiva ao misturar índices e horários', () => {
    const a = node('a', 300, 0);
    const b = node('b', 100, 1);
    const c = node('c', 200);
    const before = (left: MessageNode, right: MessageNode) =>
      sortTimelineNodes([right, left])[0] === left;
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
  });
});
