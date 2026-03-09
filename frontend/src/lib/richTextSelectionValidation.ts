export type RichTextSelectionLike = {
  from: number;
  to: number;
  empty: boolean;
};

export type ValidateRichTextSelectionSnapshotResult =
  | { ok: true }
  | {
      ok: false;
      reason:
        | 'no_selection'
        | 'range_changed'
        | 'empty_mismatch'
        | 'selected_text_mismatch'
        | 'cannot_read_selected_text';
    };

export function validateRichTextSelectionSnapshot(params: {
  currentSelection: RichTextSelectionLike | null | undefined;
  expectedFrom: number;
  expectedTo: number;
  expectedEmpty: boolean;
  expectedSelectedText?: string;
  getCurrentSelectedText?: () => string;
}): ValidateRichTextSelectionSnapshotResult {
  const sel = params.currentSelection;
  if (!sel) return { ok: false, reason: 'no_selection' };

  const expectedFrom = Number(params.expectedFrom);
  const expectedTo = Number(params.expectedTo);
  const expectedEmpty = !!params.expectedEmpty;

  if (sel.from !== expectedFrom || sel.to !== expectedTo) {
    return { ok: false, reason: 'range_changed' };
  }

  if (expectedEmpty && !sel.empty) {
    return { ok: false, reason: 'empty_mismatch' };
  }

  if (!expectedEmpty) {
    try {
      const currentSelectedText = String(params.getCurrentSelectedText?.() ?? '');
      const expectedSelectedText = String(params.expectedSelectedText ?? '');
      if (currentSelectedText !== expectedSelectedText) {
        return { ok: false, reason: 'selected_text_mismatch' };
      }
    } catch {
      return { ok: false, reason: 'cannot_read_selected_text' };
    }
  }

  return { ok: true };
}
