import { describe, expect, it } from 'vitest';

import {
  buildEditorInlineChatSurfaceContext,
  findRevealSlideForMarkdownOffsets,
  normalizeReplacementForEditor,
  resolveRevealDeckForInlineSelection,
  resolveRevealSlideForInlineSelection,
} from './editorInlineChatContext';
import type { InlineChatSelection } from './editorTypes';

const REVEAL_MARKDOWN = [
  '<!-- .slide: class="content-slide" -->',
  '',
  '## Slide 1',
  'conteudo um',
  '',
  '---',
  '',
  '<!-- .slide: class="content-slide" -->',
  '',
  '## Slide 2',
  'conteudo dois',
].join('\n');

function makeMarkdownSelection(overrides: Partial<Extract<InlineChatSelection, { mode: 'markdown' }>> = {}): InlineChatSelection {
  return {
    mode: 'markdown',
    tabId: 'tab-1',
    selectedText: 'conteudo um',
    selectionIsEmpty: false,
    cursorContext: 'ctx',
    displayText: 'conteudo um',
    startOffset: 0,
    endOffset: 11,
    startLine: 1,
    startColumn: 1,
    endLine: 1,
    endColumn: 12,
    cursorLine: 1,
    cursorColumn: 1,
    cursorOffset: 0,
    snapshot: REVEAL_MARKDOWN,
    ...overrides,
  };
}

function makeRichSelection(overrides: Partial<Extract<InlineChatSelection, { mode: 'rich' }>> = {}): InlineChatSelection {
  return {
    mode: 'rich',
    tabId: 'tab-1',
    selectedText: 'conteudo dois',
    selectedMarkdown: 'conteudo dois',
    selectionIsEmpty: false,
    cursorContext: 'ctx',
    displayText: 'conteudo dois',
    from: 1,
    to: 14,
    snapshot: REVEAL_MARKDOWN,
    ...overrides,
  };
}

describe('normalizeReplacementForEditor', () => {
  it('remove fence de markdown acidental', () => {
    const raw = '```markdown\n# Titulo\n```';
    expect(normalizeReplacementForEditor(raw, undefined, 'texto normal')).toBe('# Titulo');
  });

  it('mantém fence quando a seleção do usuário já era um fence', () => {
    const raw = '```markdown\n# Titulo\n```';
    expect(normalizeReplacementForEditor(raw, undefined, '```js\ncode\n```')).toBe(raw);
  });

  it('não remove fences de linguagens não-texto (ex.: mermaid)', () => {
    const raw = '```mermaid\ngraph TD\n```';
    expect(normalizeReplacementForEditor(raw, undefined, 'sel')).toBe(raw);
  });

  it('remove fence sem linguagem em patches plain', () => {
    const raw = '```\ntexto\n```';
    expect(normalizeReplacementForEditor(raw, 'plain', 'sel')).toBe('texto');
  });

  it('retorna o texto intacto quando não há fence', () => {
    expect(normalizeReplacementForEditor('texto puro', undefined, 'sel')).toBe('texto puro');
  });
});

describe('findRevealSlideForMarkdownOffsets', () => {
  it('encontra o slide pelo range da seleção', () => {
    const secondSlideOffset = REVEAL_MARKDOWN.indexOf('conteudo dois');
    const slide = findRevealSlideForMarkdownOffsets(
      REVEAL_MARKDOWN,
      secondSlideOffset,
      secondSlideOffset + 5,
      secondSlideOffset,
    );
    expect(slide?.index).toBe(1);
  });

  it('usa o cursor quando não há seleção', () => {
    const slide = findRevealSlideForMarkdownOffsets(REVEAL_MARKDOWN, 0, 0, 5);
    expect(slide?.index).toBe(0);
  });

  it('retorna null para markdown comum', () => {
    expect(findRevealSlideForMarkdownOffsets('# apenas um doc', 0, 5, 0)).toBeNull();
  });
});

describe('resolveRevealDeckForInlineSelection', () => {
  it('prefere o snapshot congelado da seleção markdown', () => {
    const selection = makeMarkdownSelection();
    const deck = resolveRevealDeckForInlineSelection('# doc vivo sem reveal', selection);
    expect(deck.detection.kind).toBe('reveal');
    expect(deck.slides.length).toBe(2);
  });

  it('cai para o markdown vivo quando o snapshot não é reveal', () => {
    const selection = makeMarkdownSelection({ snapshot: '# sem reveal' });
    const deck = resolveRevealDeckForInlineSelection(REVEAL_MARKDOWN, selection);
    expect(deck.detection.kind).toBe('reveal');
  });
});

describe('resolveRevealSlideForInlineSelection', () => {
  it('markdown: resolve o slide congelado pelo índice quando o markdown ainda bate', () => {
    const selection = makeMarkdownSelection();
    const deck = resolveRevealDeckForInlineSelection(REVEAL_MARKDOWN, selection);
    const frozen = deck.slides[1];
    const selectionWithFrozen = makeMarkdownSelection({
      revealSlideIndex: 1,
      revealSlideMarkdown: frozen.markdown,
    });
    const slide = resolveRevealSlideForInlineSelection(deck, selectionWithFrozen);
    expect(slide?.index).toBe(1);
  });

  it('rich: usa o slide congelado quando o deck atual divergiu', () => {
    const selection = makeRichSelection({
      revealSlideIndex: 0,
      revealSlideMarkdown: '## Slide congelado',
      revealSlideLabel: 'Slide congelado',
    });
    const deck = resolveRevealDeckForInlineSelection('# doc sem reveal', selection);
    const slide = resolveRevealSlideForInlineSelection(deck, selection);
    expect(slide?.markdown).toBe('## Slide congelado');
    expect(slide?.label).toBe('Slide congelado');
  });

  it('rich sem índice congelado retorna null', () => {
    const selection = makeRichSelection();
    const deck = resolveRevealDeckForInlineSelection(REVEAL_MARKDOWN, selection);
    expect(resolveRevealSlideForInlineSelection(deck, selection)).toBeNull();
  });
});

describe('buildEditorInlineChatSurfaceContext', () => {
  const tab = {
    id: 'tab-1',
    title: 'Documento',
    markdown: REVEAL_MARKDOWN,
    filePath: 'C:/docs/apresentacao.md',
    draftId: null,
  };

  it('monta contexto reveal com slide em foco (markdown)', () => {
    const secondSlideOffset = REVEAL_MARKDOWN.indexOf('conteudo dois');
    const selection = makeMarkdownSelection({
      selectedText: 'conteudo dois',
      startOffset: secondSlideOffset,
      endOffset: secondSlideOffset + 'conteudo dois'.length,
      cursorOffset: secondSlideOffset,
    });
    const ctx = buildEditorInlineChatSurfaceContext(tab, selection);

    expect(ctx.surfaceType).toBe('editor');
    expect(ctx.surfaceId).toBe('tab-1');
    expect(ctx.mode).toBe('reveal');
    expect(ctx.focus?.kind).toBe('slide');
    expect(ctx.content?.kind).toBe('reveal_slide');
    expect(ctx.metadata?.slideCount).toBe(2);
    expect(ctx.metadata?.currentSlideIndex).toBe(1);
    expect(ctx.metadata?.filePath).toBe(tab.filePath);
  });

  it('monta contexto de documento comum sem reveal', () => {
    const plainTab = { ...tab, markdown: '# doc comum' };
    const selection = makeMarkdownSelection({ snapshot: '# doc comum', selectedText: 'doc', startOffset: 2, endOffset: 5 });
    const ctx = buildEditorInlineChatSurfaceContext(plainTab, selection);

    expect(ctx.mode).toBe('markdown');
    expect(ctx.focus?.kind).toBe('cursor');
    expect(ctx.content?.kind).toBe('document_window');
    expect(ctx.selection?.isEmpty).toBe(false);
    expect(ctx.selection?.explicit).toBe(true);
  });

  it('rich: usa offsets from/to na seleção e no foco', () => {
    const plainTab = { ...tab, markdown: '# doc comum' };
    const selection = makeRichSelection({ snapshot: '# doc comum' });
    const ctx = buildEditorInlineChatSurfaceContext(plainTab, selection);

    expect(ctx.mode).toBe('rich');
    expect(ctx.selection?.range?.startOffset).toBe(1);
    expect(ctx.selection?.range?.endOffset).toBe(14);
    expect(ctx.selection?.markdown).toBe('conteudo dois');
  });
});
