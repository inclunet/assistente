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
  // Baseline: último markdown conhecido na forma SERIALIZADA pelo editor
  // (round-trip). Comparações de emissão usam sempre este valor.
  lastMarkdownRef: { current: string };
  // Forma BRUTA do último markdown externo aplicado (store/disco). O
  // serializador do TipTap normaliza o conteúdo (bullets, escapes, CRLF→LF),
  // então a forma bruta pode diferir do baseline serializado sem que nada
  // tenha mudado de verdade. `null` quando o usuário editou depois do último
  // sync externo (a correspondência bruta↔editor deixou de valer).
  lastExternalMarkdownRef: { current: string | null };
  // Indica se o baseline já foi re-baseado para o round-trip serializado do
  // editor montado (feito uma única vez, no primeiro syncFromExternal com
  // editor disponível).
  hasEditorBaselineRef: { current: boolean };
  pendingMarkdownRef: { current: string | null };
  markdownEmitTimerRef: { current: TimerHandle | null };
};

export function createRichMarkdownSyncRefs(initialMarkdown: string): RichMarkdownSyncRefs {
  return {
    isApplyingExternalMarkdownRef: { current: false },
    lastMarkdownRef: { current: String(initialMarkdown || '') },
    lastExternalMarkdownRef: { current: String(initialMarkdown || '') },
    hasEditorBaselineRef: { current: false },
    pendingMarkdownRef: { current: null },
    markdownEmitTimerRef: { current: null },
  };
}

export function getMarkdownNow(editor: EditorLike): string {
  try {
    const storage = editor.storage as Record<string, unknown> | undefined;
    const markdown = storage?.markdown as { getMarkdown?: () => string } | undefined;
    const md = markdown?.getMarkdown?.();
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
  // Edição real do usuário: a forma bruta externa deixa de corresponder ao editor.
  refs.lastExternalMarkdownRef.current = null;
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
    refs.lastExternalMarkdownRef.current = null;
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

  // Primeiro sync com editor disponível (mount): o editor foi criado com o
  // markdown bruto inicial (useEditor({ content })), mas o baseline precisa
  // ser o round-trip SERIALIZADO — senão a primeira flush emite apenas ruído
  // de normalização do serializador (edição fantasma).
  if (!refs.hasEditorBaselineRef.current) {
    refs.hasEditorBaselineRef.current = true;
    if (nextMarkdown === refs.lastExternalMarkdownRef.current) {
      refs.lastMarkdownRef.current = getMarkdownNow(editor);
      return;
    }
  }

  // A forma bruta externa não mudou desde o último sync: mesmo que difira do
  // baseline serializado (só normalização), não há nada novo a aplicar. Evita
  // reaplicar setContent em loop quando store (bruto) e editor (serializado)
  // representam o mesmo conteúdo.
  if (nextMarkdown === refs.lastExternalMarkdownRef.current) return;

  // O externo coincide com o que o editor já serializa (ex.: o store ecoou de
  // volta a última emissão do próprio editor): registra a forma bruta e sai.
  if (nextMarkdown === refs.lastMarkdownRef.current) {
    refs.lastExternalMarkdownRef.current = nextMarkdown;
    return;
  }

  if (refs.markdownEmitTimerRef.current) {
    timers.clearTimeout(refs.markdownEmitTimerRef.current);
    refs.markdownEmitTimerRef.current = null;
  }
  // Sync externo é autoritativo: descarta a edição pendente SEM emitir.
  // Emitir aqui deixaria o store com o pendente e o editor com o externo,
  // fazendo o efeito de sync reaplicar o pendente por cima (ping-pong) e
  // perder a mudança externa.
  refs.pendingMarkdownRef.current = null;

  refs.isApplyingExternalMarkdownRef.current = true;
  try {
    editor.commands?.setContent?.(nextMarkdown);
    // Baseline = round-trip serializado, não o texto bruto de entrada.
    refs.lastMarkdownRef.current = getMarkdownNow(editor);
    refs.lastExternalMarkdownRef.current = nextMarkdown;
  } finally {
    timers.setTimeout(() => {
      refs.isApplyingExternalMarkdownRef.current = false;
    }, 0);
  }
}

export function disposeRichMarkdownSync(
  refs: RichMarkdownSyncRefs,
  timers: Timers = defaultTimers,
  onMarkdownChange?: (markdown: string) => void
) {
  if (refs.markdownEmitTimerRef.current) {
    try {
      timers.clearTimeout(refs.markdownEmitTimerRef.current);
    } catch {
      // best-effort
    }
  }

  refs.markdownEmitTimerRef.current = null;
  const pending = refs.pendingMarkdownRef.current;
  refs.pendingMarkdownRef.current = null;
  if (typeof pending === 'string' && pending !== refs.lastMarkdownRef.current) {
    refs.lastMarkdownRef.current = pending;
    onMarkdownChange?.(pending);
  }
}
