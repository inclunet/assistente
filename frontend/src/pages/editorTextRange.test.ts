import { describe, expect, it } from 'vitest';

import {
  clampNumber,
  findTextRangeInRichDoc,
  findTextRangeInRichDocByContext,
  getChangedRangeAfterTextReplacement,
  getRichDocPosForTextOffset,
  getRichDocTextBefore,
} from './editorTextRange';

/**
 * Doc "TipTap-like" mínimo para as funções puras: posições do documento
 * correspondem 1:1 aos offsets do texto plano (suficiente porque as funções
 * só dependem de `textBetween` e `content.size`).
 */
function makeRichDoc(text: string) {
  return {
    content: { size: text.length },
    textBetween: (from: number, to: number) => {
      const start = Math.max(0, Math.min(from, text.length));
      const end = Math.max(start, Math.min(to, text.length));
      return text.slice(start, end);
    },
  };
}

describe('clampNumber', () => {
  it('mantém valores dentro do intervalo', () => {
    expect(clampNumber(5, 0, 10)).toBe(5);
  });

  it('clampa abaixo do mínimo e acima do máximo', () => {
    expect(clampNumber(-3, 0, 10)).toBe(0);
    expect(clampNumber(42, 0, 10)).toBe(10);
  });

  it('retorna o mínimo para valores não finitos', () => {
    expect(clampNumber(Number.NaN, 2, 10)).toBe(2);
    expect(clampNumber(Number.POSITIVE_INFINITY, 2, 10)).toBe(2);
    expect(clampNumber(Number.NEGATIVE_INFINITY, 2, 10)).toBe(2);
  });
});

describe('getChangedRangeAfterTextReplacement', () => {
  it('retorna o range alterado por diff de prefixo/sufixo', () => {
    const before = 'um dois tres';
    const after = 'um QUATRO tres';
    const range = getChangedRangeAfterTextReplacement({
      before,
      after,
      fallbackStartOffset: 0,
      fallbackEndOffset: 0,
    });
    expect(after.slice(range.startOffset, range.endOffset)).toBe('QUATRO');
  });

  it('prefere o fallback quando o texto selecionado continua no lugar', () => {
    const before = 'a b c';
    const after = 'a b c';
    const range = getChangedRangeAfterTextReplacement({
      before,
      after,
      fallbackStartOffset: 2,
      fallbackEndOffset: 3,
      fallbackSelectedText: 'b',
    });
    expect(range).toEqual({ startOffset: 2, endOffset: 3 });
  });

  it('usa o fallback quando não houve mudança e não há seleção', () => {
    const range = getChangedRangeAfterTextReplacement({
      before: 'igual',
      after: 'igual',
      fallbackStartOffset: 1,
      fallbackEndOffset: 4,
    });
    expect(range).toEqual({ startOffset: 1, endOffset: 4 });
  });

  it('expande até o fim da mudança quando a seleção precede uma inserção posterior', () => {
    const before = 'sel.';
    const after = 'sel resto';
    const range = getChangedRangeAfterTextReplacement({
      before,
      after,
      fallbackStartOffset: 0,
      fallbackEndOffset: 3,
      fallbackSelectedText: 'sel',
    });
    expect(range.startOffset).toBe(0);
    expect(range.endOffset).toBe(after.length);
  });

  it('clampa offsets de fallback fora do texto', () => {
    const range = getChangedRangeAfterTextReplacement({
      before: 'abc',
      after: 'abc',
      fallbackStartOffset: 99,
      fallbackEndOffset: 120,
    });
    expect(range).toEqual({ startOffset: 3, endOffset: 3 });
  });
});

describe('getRichDocTextBefore', () => {
  it('retorna o texto plano antes da posição', () => {
    const doc = makeRichDoc('hello world');
    expect(getRichDocTextBefore(doc, 5)).toBe('hello');
  });

  it('retorna string vazia para doc sem textBetween', () => {
    expect(getRichDocTextBefore(null, 5)).toBe('');
    expect(getRichDocTextBefore({}, 5)).toBe('');
  });
});

/**
 * Doc onde o mapeamento posição→texto NÃO é 1:1: algumas posições são
 * "fronteiras de nó" que não acrescentam texto (como em docs TipTap reais,
 * onde abrir/fechar blocos consome posições sem emitir caracteres).
 */
function makeRichDocWithBoundaries(paragraphs: string[]) {
  // Posições: 1 de abertura + texto + 1 de fechamento por parágrafo.
  const size = paragraphs.reduce((acc, p) => acc + p.length + 2, 0);
  const textUpTo = (pos: number) => {
    let remaining = pos;
    let out = '';
    for (const p of paragraphs) {
      if (remaining <= 0) break;
      remaining -= 1; // abertura do bloco
      if (remaining <= 0) break;
      out += p.slice(0, Math.min(p.length, remaining));
      remaining -= p.length;
      if (remaining <= 0) break;
      remaining -= 1; // fechamento do bloco
    }
    return out;
  };
  return {
    content: { size },
    textBetween: (from: number, to: number) => textUpTo(to).slice(textUpTo(from).length),
  };
}

/** Referência: o loop linear original, para validar a busca binária. */
function linearPosForTextOffset(
  doc: { content: { size: number }; textBetween: (from: number, to: number) => string },
  targetOffset: number,
  side: 'start' | 'end'
): number | null {
  const target = Math.max(0, targetOffset);
  for (let pos = 0; pos <= doc.content.size; pos += 1) {
    const length = doc.textBetween(0, pos).length;
    if (side === 'start' && length > target) return Math.max(0, pos - 1);
    if (side === 'end' && length >= target) return pos;
  }
  return null;
}

describe('getRichDocPosForTextOffset', () => {
  it('mapeia offsets de texto para posições do doc', () => {
    const doc = makeRichDoc('hello world');
    expect(getRichDocPosForTextOffset(doc, 0, 'start')).toBe(0);
    expect(getRichDocPosForTextOffset(doc, 5, 'end')).toBe(5);
  });

  it('retorna null quando o offset está além do doc', () => {
    const doc = makeRichDoc('abc');
    expect(getRichDocPosForTextOffset(doc, 50, 'start')).toBeNull();
  });

  it('equivale ao loop linear original em todos os offsets (docs 1:1 e com fronteiras de nó)', () => {
    const docs = [
      makeRichDoc('hello world'),
      makeRichDoc(''),
      makeRichDocWithBoundaries(['abc', 'de', '', 'fghi']),
      makeRichDocWithBoundaries(['x']),
    ];
    for (const doc of docs) {
      const textLength = doc.textBetween(0, doc.content.size).length;
      for (let offset = 0; offset <= textLength + 2; offset += 1) {
        for (const side of ['start', 'end'] as const) {
          expect(getRichDocPosForTextOffset(doc, offset, side)).toBe(
            linearPosForTextOffset(doc, offset, side)
          );
        }
      }
    }
  });

  it('usa O(log n) chamadas a textBetween em vez de O(n)', () => {
    const text = 'a'.repeat(4096);
    let calls = 0;
    const doc = {
      content: { size: text.length },
      textBetween: (from: number, to: number) => {
        calls += 1;
        return text.slice(from, to);
      },
    };
    expect(getRichDocPosForTextOffset(doc, 4000, 'end')).toBe(4000);
    expect(calls).toBeLessThan(20);
  });
});

describe('findTextRangeInRichDoc', () => {
  it('encontra o range do texto no doc', () => {
    const doc = makeRichDoc('um dois tres');
    const range = findTextRangeInRichDoc(doc, 'dois');
    expect(range).not.toBeNull();
    expect(doc.textBetween(range!.from, range!.to)).toBe('dois');
  });

  it('usa textBefore para desambiguar ocorrências repetidas', () => {
    const doc = makeRichDoc('x y x y');
    const range = findTextRangeInRichDoc(doc, 'y', 'x y x ');
    expect(range).toEqual({ from: 6, to: 7 });
  });

  it('retorna null para texto ausente ou vazio', () => {
    const doc = makeRichDoc('abc');
    expect(findTextRangeInRichDoc(doc, 'zzz')).toBeNull();
    expect(findTextRangeInRichDoc(doc, '   ')).toBeNull();
  });
});

describe('findTextRangeInRichDocByContext', () => {
  it('resolve o range pelo contexto antes/depois', () => {
    const doc = makeRichDoc('inicio MEIO fim');
    const range = findTextRangeInRichDocByContext(doc, 'inicio ', ' fim');
    expect(range).not.toBeNull();
    expect(doc.textBetween(range!.from, range!.to)).toBe('MEIO');
  });

  it('retorna null quando o contexto não bate', () => {
    const doc = makeRichDoc('inicio MEIO fim');
    expect(findTextRangeInRichDocByContext(doc, 'outro ', ' fim')).toBeNull();
    expect(findTextRangeInRichDocByContext(doc, 'inicio ', ' outra')).toBeNull();
  });

  it('sem contexto, cobre o doc inteiro', () => {
    const doc = makeRichDoc('abc');
    const range = findTextRangeInRichDocByContext(doc);
    expect(range).toEqual({ from: 0, to: 3 });
  });
});
