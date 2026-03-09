import { describe, expect, it, vi, afterEach } from 'vitest';

import {
  createRichMarkdownSyncRefs,
  disposeRichMarkdownSync,
  flushNow,
  getMarkdownNow,
  onUpdate,
  syncFromExternal,
  type EditorLike,
} from './richMarkdownSync';

afterEach(() => {
  vi.useRealTimers();
});

describe('richMarkdownSync', () => {
  it('getMarkdownNow retorna markdown do storage quando disponível', () => {
    const editor: EditorLike = {
      storage: {
        markdown: {
          getMarkdown: () => 'oi',
        },
      },
    };

    expect(getMarkdownNow(editor)).toBe('oi');
  });

  it('getMarkdownNow retorna string vazia quando lança erro', () => {
    const editor: EditorLike = {
      storage: {
        markdown: {
          getMarkdown: () => {
            throw new Error('boom');
          },
        },
      },
    };

    expect(getMarkdownNow(editor)).toBe('');
  });

  it('onUpdate faz debounce e emite a última versão', () => {
    vi.useFakeTimers();

    const refs = createRichMarkdownSyncRefs('a');
    const onMarkdownChange = vi.fn();

    let current = 'b';
    const editor: EditorLike = {
      storage: {
        markdown: {
          getMarkdown: () => current,
        },
      },
    };

    onUpdate({ refs, ctx: { editor }, onMarkdownChange, debounceMs: 50 });
    expect(onMarkdownChange).not.toHaveBeenCalled();

    current = 'c';
    onUpdate({ refs, ctx: { editor }, onMarkdownChange, debounceMs: 50 });

    vi.advanceTimersByTime(49);
    expect(onMarkdownChange).not.toHaveBeenCalled();

    vi.advanceTimersByTime(1);
    expect(onMarkdownChange).toHaveBeenCalledTimes(1);
    expect(onMarkdownChange).toHaveBeenCalledWith('c');

    disposeRichMarkdownSync(refs);
  });

  it('flushNow cancela debounce e emite imediatamente', () => {
    vi.useFakeTimers();

    const refs = createRichMarkdownSyncRefs('a');
    const onMarkdownChange = vi.fn();

    let current = 'b';
    const editor: EditorLike = {
      storage: {
        markdown: {
          getMarkdown: () => current,
        },
      },
    };

    onUpdate({ refs, ctx: { editor }, onMarkdownChange, debounceMs: 100 });
    flushNow({ refs, editor, onMarkdownChange });

    expect(onMarkdownChange).toHaveBeenCalledTimes(1);
    expect(onMarkdownChange).toHaveBeenCalledWith('b');

    vi.advanceTimersByTime(200);
    expect(onMarkdownChange).toHaveBeenCalledTimes(1);

    disposeRichMarkdownSync(refs);
  });

  it('syncFromExternal aplica conteúdo e ignora updates até liberar flag', () => {
    vi.useFakeTimers();

    const refs = createRichMarkdownSyncRefs('a');
    const onMarkdownChange = vi.fn();

    const setContent = vi.fn();

    const editor: EditorLike = {
      commands: {
        setContent,
      },
      storage: {
        markdown: {
          getMarkdown: () => 'interno',
        },
      },
    };

    syncFromExternal({ refs, editor, nextMarkdown: 'externo' });

    expect(setContent).toHaveBeenCalledTimes(1);
    expect(setContent).toHaveBeenCalledWith('externo');
    expect(refs.isApplyingExternalMarkdownRef.current).toBe(true);

    // Enquanto estiver aplicando, não deve emitir.
    onUpdate({ refs, ctx: { editor }, onMarkdownChange, debounceMs: 10 });
    vi.runOnlyPendingTimers();
    expect(onMarkdownChange).not.toHaveBeenCalled();

    // Libera flag no próximo tick.
    vi.runAllTimers();
    expect(refs.isApplyingExternalMarkdownRef.current).toBe(false);

    disposeRichMarkdownSync(refs);
  });
});
