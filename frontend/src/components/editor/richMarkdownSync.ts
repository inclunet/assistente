export type EditorLike = {
  storage?: unknown;
  commands?: {
    setContent?: (markdown: string) => void;
  };
};

export type UpdateCtx = { editor: EditorLike };

type TimerHandle = ReturnType<typeof setTimeout>;

type Timers = {
  setTimeout: (fn: () => void, ms: number) => TimerHandle;
  clearTimeout: (handle: TimerHandle) => void;
};

const defaultTimers: Timers = {
  setTimeout: (fn, ms) => globalThis.setTimeout(fn, ms),
  clearTimeout: (handle) => globalThis.clearTimeout(handle),
};

export type RichMarkdownSyncRefs = {
  isApplyingExternalMarkdownRef: { current: boolean };
  lastMarkdownRef: { current: string };
  pendingMarkdownRef: { current: string | null };
  markdownEmitTimerRef: { current: TimerHandle | null };
};

export function createRichMarkdownSyncRefs(initialMarkdown: string): RichMarkdownSyncRefs {
  return {
    isApplyingExternalMarkdownRef: { current: false },
    lastMarkdownRef: { current: String(initialMarkdown || '') },
    pendingMarkdownRef: { current: null },
    markdownEmitTimerRef: { current: null },
  };
}

export function getMarkdownNow(editor: EditorLike): string {
  try {
    const md = (editor.storage as any)?.markdown?.getMarkdown?.() as string | undefined;
    return typeof md === 'string' ? md : '';
  } catch {
    return '';
  }
}

export function flushNow(args: {
  refs: RichMarkdownSyncRefs;
  editor: EditorLike;
  onMarkdownChange: (markdown: string) => void;
  timers?: Timers;
}) {
  const { refs, editor, onMarkdownChange, timers = defaultTimers } = args;

  if (refs.isApplyingExternalMarkdownRef.current) return;

  if (refs.markdownEmitTimerRef.current) {
    timers.clearTimeout(refs.markdownEmitTimerRef.current);
    refs.markdownEmitTimerRef.current = null;
  }

  const next = refs.pendingMarkdownRef.current ?? getMarkdownNow(editor);
  refs.pendingMarkdownRef.current = null;

  if (next === refs.lastMarkdownRef.current) return;
  refs.lastMarkdownRef.current = next;
  onMarkdownChange(next);
}

export function onUpdate(args: {
  refs: RichMarkdownSyncRefs;
  ctx: UpdateCtx;
  onMarkdownChange: (markdown: string) => void;
  debounceMs: number;
  timers?: Timers;
}) {
  const { refs, ctx, onMarkdownChange, debounceMs, timers = defaultTimers } = args;

  if (refs.isApplyingExternalMarkdownRef.current) return;
  const next = getMarkdownNow(ctx.editor);

  refs.pendingMarkdownRef.current = next;

  if (refs.markdownEmitTimerRef.current) {
    timers.clearTimeout(refs.markdownEmitTimerRef.current);
  }

  refs.markdownEmitTimerRef.current = timers.setTimeout(() => {
    refs.markdownEmitTimerRef.current = null;

    const pending = refs.pendingMarkdownRef.current;
    refs.pendingMarkdownRef.current = null;

    if (typeof pending !== 'string') return;
    if (pending === refs.lastMarkdownRef.current) return;

    refs.lastMarkdownRef.current = pending;
    onMarkdownChange(pending);
  }, Math.max(0, debounceMs));
}

export function syncFromExternal(args: {
  refs: RichMarkdownSyncRefs;
  editor: EditorLike | null;
  nextMarkdown: string;
  timers?: Timers;
}) {
  const { refs, editor, nextMarkdown, timers = defaultTimers } = args;

  if (!editor) return;
  if (nextMarkdown === refs.lastMarkdownRef.current) return;

  refs.isApplyingExternalMarkdownRef.current = true;
  try {
    editor.commands?.setContent?.(nextMarkdown);
    refs.lastMarkdownRef.current = nextMarkdown;
  } finally {
    timers.setTimeout(() => {
      refs.isApplyingExternalMarkdownRef.current = false;
    }, 0);
  }
}

export function disposeRichMarkdownSync(refs: RichMarkdownSyncRefs, timers: Timers = defaultTimers) {
  if (refs.markdownEmitTimerRef.current) {
    try {
      timers.clearTimeout(refs.markdownEmitTimerRef.current);
    } catch {
      // best-effort
    }
  }

  refs.markdownEmitTimerRef.current = null;
  refs.pendingMarkdownRef.current = null;
}
