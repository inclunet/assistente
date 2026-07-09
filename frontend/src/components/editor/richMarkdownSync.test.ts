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

  it('syncFromExternal descarta debounce pendente sem emitir (externo é autoritativo)', () => {
    vi.useFakeTimers();

    const refs = createRichMarkdownSyncRefs('slide antigo');
    const onMarkdownChange = vi.fn();
    const setContent = vi.fn();

    let current = 'edição pendente do slide antigo';
    const editor: EditorLike = {
      commands: {
        setContent: (md: string) => {
          setContent(md);
          current = md;
        },
      },
      storage: {
        markdown: {
          getMarkdown: () => current,
        },
      },
    };

    onUpdate({ refs, ctx: { editor }, onMarkdownChange, debounceMs: 100 });
    syncFromExternal({ refs, editor, nextMarkdown: 'slide novo' });

    vi.advanceTimersByTime(150);

    // O pendente NÃO é emitido: emitir deixaria o store com o pendente e o
    // editor com o externo, reaplicando o pendente por cima (ping-pong).
    expect(onMarkdownChange).not.toHaveBeenCalled();
    expect(setContent).toHaveBeenCalledWith('slide novo');
    expect(refs.lastMarkdownRef.current).toBe('slide novo');

    disposeRichMarkdownSync(refs);
  });

  it('flush após syncFromExternal com serializador que normaliza não emite mudança', () => {
    vi.useFakeTimers();

    const refs = createRichMarkdownSyncRefs('inicial');
    const onMarkdownChange = vi.fn();

    // Simula a normalização do serializador do TipTap: o round-trip difere do
    // texto bruto de entrada (ex.: "* item" vira "- item").
    let current = 'inicial';
    const editor: EditorLike = {
      commands: {
        setContent: (md: string) => {
          current = md.replace(/^\* /gm, '- ');
        },
      },
      storage: {
        markdown: {
          getMarkdown: () => current,
        },
      },
    };

    syncFromExternal({ refs, editor, nextMarkdown: '* item externo' });
    vi.runAllTimers();

    // Baseline deve ser o round-trip serializado, não o texto bruto.
    expect(refs.lastMarkdownRef.current).toBe('- item externo');

    // Flush (ex.: blur/alt-tab) não deve emitir "edição fantasma".
    flushNow({ refs, editor, onMarkdownChange });
    expect(onMarkdownChange).not.toHaveBeenCalled();

    disposeRichMarkdownSync(refs);
  });

  it('primeiro syncFromExternal com editor rebaseia o baseline para o round-trip do mount', () => {
    vi.useFakeTimers();

    const refs = createRichMarkdownSyncRefs('* item inicial');
    const onMarkdownChange = vi.fn();
    const setContent = vi.fn();

    // O editor montou com o markdown bruto e o serializa de forma normalizada.
    const editor: EditorLike = {
      commands: { setContent },
      storage: {
        markdown: {
          getMarkdown: () => '- item inicial',
        },
      },
    };

    // Efeito de sync do RichTextEditor no mount: mesmo markdown bruto do store.
    syncFromExternal({ refs, editor, nextMarkdown: '* item inicial' });

    // Não reaplica conteúdo (evita perder cursor) e rebaseia o baseline.
    expect(setContent).not.toHaveBeenCalled();
    expect(refs.lastMarkdownRef.current).toBe('- item inicial');

    // Primeira flush após montar não emite ruído de normalização.
    flushNow({ refs, editor, onMarkdownChange });
    expect(onMarkdownChange).not.toHaveBeenCalled();

    disposeRichMarkdownSync(refs);
  });

  it('syncFromExternal não reaplica setContent quando o externo bruto não mudou', () => {
    vi.useFakeTimers();

    const refs = createRichMarkdownSyncRefs('inicial');
    const onMarkdownChange = vi.fn();
    const setContent = vi.fn();

    let current = 'inicial';
    const editor: EditorLike = {
      commands: {
        setContent: (md: string) => {
          setContent(md);
          current = md.replace(/^\* /gm, '- ');
        },
      },
      storage: {
        markdown: {
          getMarkdown: () => current,
        },
      },
    };

    syncFromExternal({ refs, editor, nextMarkdown: '* externo' });
    vi.runAllTimers();
    expect(setContent).toHaveBeenCalledTimes(1);

    // O efeito de sync re-executa com o mesmo prop bruto do store, que difere
    // do baseline serializado apenas por normalização: não deve haver loop.
    syncFromExternal({ refs, editor, nextMarkdown: '* externo' });
    syncFromExternal({ refs, editor, nextMarkdown: '* externo' });

    expect(setContent).toHaveBeenCalledTimes(1);
    expect(onMarkdownChange).not.toHaveBeenCalled();

    disposeRichMarkdownSync(refs);
  });

  it('syncFromExternal com pendente não causa ping-pong (externo vence e estado converge)', () => {
    vi.useFakeTimers();

    const refs = createRichMarkdownSyncRefs('base');
    const onMarkdownChange = vi.fn();
    const setContent = vi.fn();

    let current = 'base';
    const editor: EditorLike = {
      commands: {
        setContent: (md: string) => {
          setContent(md);
          current = md;
        },
      },
      storage: {
        markdown: {
          getMarkdown: () => current,
        },
      },
    };

    // Usuário digita (pendente no debounce)...
    current = 'edição pendente';
    onUpdate({ refs, ctx: { editor }, onMarkdownChange, debounceMs: 100 });

    // ...e chega um sync externo autoritativo antes do debounce vencer.
    syncFromExternal({ refs, editor, nextMarkdown: 'externo' });
    vi.runAllTimers();

    // O pendente foi descartado; store (via emissões), baseline e editor convergem.
    expect(onMarkdownChange).not.toHaveBeenCalled();
    expect(refs.lastMarkdownRef.current).toBe('externo');
    expect(getMarkdownNow(editor)).toBe('externo');

    // O efeito de sync re-executa com o mesmo externo: nada a reaplicar.
    syncFromExternal({ refs, editor, nextMarkdown: 'externo' });
    expect(setContent).toHaveBeenCalledTimes(1);

    disposeRichMarkdownSync(refs);
  });

  it('após edição do usuário, eco do próprio conteúdo emitido não reaplica setContent', () => {
    vi.useFakeTimers();

    const refs = createRichMarkdownSyncRefs('base');
    const onMarkdownChange = vi.fn();
    const setContent = vi.fn();

    let current = 'base';
    const editor: EditorLike = {
      commands: {
        setContent: (md: string) => {
          setContent(md);
          current = md;
        },
      },
      storage: {
        markdown: {
          getMarkdown: () => current,
        },
      },
    };

    current = 'digitado pelo usuário';
    onUpdate({ refs, ctx: { editor }, onMarkdownChange, debounceMs: 50 });
    vi.advanceTimersByTime(50);

    expect(onMarkdownChange).toHaveBeenCalledWith('digitado pelo usuário');

    // O store atualiza e o efeito de sync re-executa com o valor emitido.
    syncFromExternal({ refs, editor, nextMarkdown: 'digitado pelo usuário' });

    expect(setContent).not.toHaveBeenCalled();

    disposeRichMarkdownSync(refs);
  });

  it('disposeRichMarkdownSync emite debounce pendente antes de descartar', () => {
    vi.useFakeTimers();

    const refs = createRichMarkdownSyncRefs('a');
    const onMarkdownChange = vi.fn();
    const editor: EditorLike = {
      storage: {
        markdown: {
          getMarkdown: () => 'pendente',
        },
      },
    };

    onUpdate({ refs, ctx: { editor }, onMarkdownChange, debounceMs: 100 });
    disposeRichMarkdownSync(refs, undefined, onMarkdownChange);

    expect(onMarkdownChange).toHaveBeenCalledTimes(1);
    expect(onMarkdownChange).toHaveBeenCalledWith('pendente');

    vi.advanceTimersByTime(150);
    expect(onMarkdownChange).toHaveBeenCalledTimes(1);
  });
});
