export type ApplyTextReplacementByOffsetResult =
  | { ok: true; nextText: string }
  | { ok: false; error: 'offset_out_of_bounds' | 'selected_text_mismatch' };

export function applyTextReplacementByOffset(params: {
  current: string;
  startOffset: number;
  endOffset: number;
  expectedSelectedText: string;
  replacement: string;
}): ApplyTextReplacementByOffsetResult {
  const current = String(params.current ?? '');
  const startOffset = Number(params.startOffset);
  const endOffset = Number(params.endOffset);
  const expectedSelectedText = String(params.expectedSelectedText ?? '');
  const replacement = String(params.replacement ?? '');

  if (!Number.isFinite(startOffset) || !Number.isFinite(endOffset)) {
    return { ok: false, error: 'offset_out_of_bounds' };
  }

  if (startOffset < 0 || endOffset < 0 || startOffset > endOffset || endOffset > current.length) {
    return { ok: false, error: 'offset_out_of_bounds' };
  }

  const selectedInCurrent = current.slice(startOffset, endOffset);
  if (selectedInCurrent !== expectedSelectedText) {
    return { ok: false, error: 'selected_text_mismatch' };
  }

  const nextText = current.slice(0, startOffset) + replacement + current.slice(endOffset);
  return { ok: true, nextText };
}
