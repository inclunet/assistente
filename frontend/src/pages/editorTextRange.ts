/**
 * Funções puras de cálculo de ranges de texto usadas pelo EditorPage:
 * - diff prefixo/sufixo para descobrir o trecho alterado após uma substituição;
 * - mapeamento de offsets de texto "plano" para posições de documento TipTap.
 *
 * Extraídas do EditorPage.tsx (decomposição da onda 2 do editor) sem mudança
 * de comportamento.
 */

export function clampNumber(value: number, min: number, max: number): number {
  if (!Number.isFinite(value)) return min;
  return Math.max(min, Math.min(max, value));
}

export function getChangedRangeAfterTextReplacement(params: {
  before: string;
  after: string;
  fallbackStartOffset: number;
  fallbackEndOffset: number;
  fallbackSelectedText?: string;
}): { startOffset: number; endOffset: number } {
  const before = String(params.before ?? '');
  const after = String(params.after ?? '');
  const fallbackStartOffset = clampNumber(params.fallbackStartOffset, 0, after.length);
  const fallbackEndOffset = clampNumber(params.fallbackEndOffset, fallbackStartOffset, after.length);
  const fallbackSelectedText = String(params.fallbackSelectedText || '');
  const fallbackSelectedEndOffset = fallbackStartOffset + fallbackSelectedText.length;
  if (
    fallbackSelectedText &&
    after.slice(fallbackStartOffset, fallbackSelectedEndOffset) === fallbackSelectedText &&
    (before[fallbackEndOffset] ?? '') === (after[fallbackEndOffset] ?? '')
  ) {
    return {
      startOffset: fallbackStartOffset,
      endOffset: fallbackEndOffset,
    };
  }

  const minLength = Math.min(before.length, after.length);

  let prefixLength = 0;
  while (prefixLength < minLength && before[prefixLength] === after[prefixLength]) {
    prefixLength += 1;
  }

  let suffixLength = 0;
  while (
    suffixLength < before.length - prefixLength &&
    suffixLength < after.length - prefixLength &&
    before[before.length - 1 - suffixLength] === after[after.length - 1 - suffixLength]
  ) {
    suffixLength += 1;
  }

  if (prefixLength === before.length && prefixLength === after.length) {
    const startOffset = clampNumber(params.fallbackStartOffset, 0, after.length);
    const endOffset = clampNumber(params.fallbackEndOffset, startOffset, after.length);
    return { startOffset, endOffset };
  }

  const startOffset = clampNumber(prefixLength, 0, after.length);
  const endOffset = clampNumber(after.length - suffixLength, startOffset, after.length);
  if (
    fallbackSelectedText &&
    prefixLength >= fallbackSelectedEndOffset &&
    after.slice(fallbackStartOffset, fallbackSelectedEndOffset) === fallbackSelectedText &&
    endOffset > fallbackSelectedEndOffset
  ) {
    return { startOffset: fallbackStartOffset, endOffset };
  }
  if (endOffset > startOffset) return { startOffset, endOffset };

  const fallbackStart = clampNumber(params.fallbackStartOffset, 0, after.length);
  const fallbackEnd = clampNumber(params.fallbackEndOffset, fallbackStart, after.length);
  return { startOffset: fallbackStart, endOffset: fallbackEnd };
}

export function getRichDocTextBefore(doc: unknown, pos: number): string {
  const textBetween = (doc as { textBetween?: (from: number, to: number, separator?: string) => string } | null)?.textBetween;
  if (typeof textBetween !== 'function') return '';
  try {
    return String(textBetween.call(doc, 0, Math.max(0, pos), '\n') ?? '');
  } catch {
    return '';
  }
}

export function getRichDocPosForTextOffset(doc: unknown, targetOffset: number, side: 'start' | 'end'): number | null {
  const docSize = Number((doc as { content?: { size?: number } } | null)?.content?.size ?? 0);
  if (!Number.isFinite(docSize) || docSize < 0) return null;

  const target = Math.max(0, targetOffset);
  for (let pos = 0; pos <= docSize; pos += 1) {
    const length = getRichDocTextBefore(doc, pos).length;
    if (side === 'start' && length > target) return Math.max(0, pos - 1);
    if (side === 'end' && length >= target) return pos;
  }
  return null;
}

export function findTextRangeInRichDoc(doc: unknown, text: string, textBefore?: string): { from: number; to: number } | null {
  const needle = String(text || '').trim();
  if (!needle) return null;

  const docSize = Number((doc as { content?: { size?: number } } | null)?.content?.size ?? 0);
  const flatText = getRichDocTextBefore(doc, docSize);
  let startInFlatText = -1;
  let searchFrom = 0;
  const before = String(textBefore || '');
  while (searchFrom <= flatText.length) {
    const found = flatText.indexOf(needle, searchFrom);
    if (found < 0) break;
    if (!before || flatText.slice(0, found).endsWith(before)) {
      startInFlatText = found;
      break;
    }
    searchFrom = found + needle.length;
  }
  if (startInFlatText < 0) return null;
  const endInFlatText = startInFlatText + needle.length;
  const from = getRichDocPosForTextOffset(doc, startInFlatText, 'start');
  const to = getRichDocPosForTextOffset(doc, endInFlatText, 'end');

  return from !== null && to !== null && to >= from ? { from, to } : null;
}

export function findTextRangeInRichDocByContext(doc: unknown, textBefore?: string, textAfter?: string): { from: number; to: number } | null {
  const docSize = Number((doc as { content?: { size?: number } } | null)?.content?.size ?? 0);
  const fullText = getRichDocTextBefore(doc, docSize);
  const before = String(textBefore || '');
  const after = String(textAfter || '');
  if (before && !fullText.startsWith(before)) return null;
  if (after && !fullText.endsWith(after)) return null;

  const startOffset = before.length;
  const endOffset = Math.max(startOffset, after ? fullText.length - after.length : fullText.length);
  const from = getRichDocPosForTextOffset(doc, startOffset, 'start');
  const to = getRichDocPosForTextOffset(doc, endOffset, 'end');
  return from !== null && to !== null && to >= from ? { from, to } : null;
}
