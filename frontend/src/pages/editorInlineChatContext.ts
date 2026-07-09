/**
 * Funções puras do chat inline do editor: resolução do slide Reveal em foco,
 * montagem do `SurfaceContext` enviado ao backend (AEP-0040: o contrato de
 * envio não muda aqui — apenas a montagem do payload de contexto) e
 * normalização do replacement retornado pelo modelo.
 *
 * Extraídas do EditorPage.tsx (decomposição da onda 2 do editor) sem mudança
 * de comportamento.
 */

import { createSurfaceSnapshotVersion, type SurfaceContext } from '../lib/chatSurface';
import { parseRevealMarkdown, type ParsedRevealDeck, type RevealSlide } from '../lib/revealMarkdown';
import type { InlineChatSelection } from './editorTypes';

/** Subconjunto da aba do editor necessário para montar o contexto do chat inline. */
export interface InlineChatSurfaceTabInfo {
  id: string;
  title: string;
  markdown: string;
  filePath?: string | null;
  draftId?: string | null;
}

export function findRevealSlideForMarkdownOffsets(
  markdown: string,
  startOffset: number,
  endOffset: number,
  cursorOffset: number,
): RevealSlide | null {
  const deck = parseRevealMarkdown(markdown);
  if (deck.detection.kind !== 'reveal') return null;
  const start = Number(startOffset);
  const end = Number(endOffset);
  const cursor = Number(cursorOffset);
  return deck.slides.find((slide) => {
    if (Number.isFinite(start) && Number.isFinite(end) && end > start) {
      return start < slide.endOffset && end > slide.startOffset;
    }
    return Number.isFinite(cursor) && cursor >= slide.startOffset && cursor <= slide.endOffset;
  }) ?? null;
}

/**
 * Escolhe o deck Reveal de referência: prefere o snapshot congelado da
 * seleção Markdown (o que o usuário via ao abrir o chat) e cai para o deck
 * "vivo" da aba.
 */
export function resolveRevealDeckForInlineSelection(
  latestMarkdown: string,
  inlineChatSelection: InlineChatSelection,
): ParsedRevealDeck {
  const liveRevealDeck = parseRevealMarkdown(latestMarkdown);
  const preparedMarkdownRevealDeck = inlineChatSelection.mode === 'markdown'
    ? parseRevealMarkdown(inlineChatSelection.snapshot)
    : null;
  return preparedMarkdownRevealDeck?.detection.kind === 'reveal'
    ? preparedMarkdownRevealDeck
    : liveRevealDeck;
}

/** Resolve o slide Reveal em foco para a seleção do chat inline (rich ou markdown). */
export function resolveRevealSlideForInlineSelection(
  revealDeck: ParsedRevealDeck,
  inlineChatSelection: InlineChatSelection,
): RevealSlide | null {
  const getRichRevealSlideSnapshot = (): RevealSlide | null => {
    if (inlineChatSelection.mode !== 'rich') return null;
    const frozenIndex = inlineChatSelection.revealSlideIndex;
    if (!Number.isInteger(frozenIndex)) return null;

    const snapshotMarkdown = String(inlineChatSelection.revealSlideMarkdown || '');
    const frozenSlide: RevealSlide | null = snapshotMarkdown
      ? {
          index: frozenIndex as number,
          level: 'horizontal',
          markdown: snapshotMarkdown,
          label: inlineChatSelection.revealSlideLabel,
          separatorBefore: '',
          startOffset: 0,
          endOffset: snapshotMarkdown.length,
        }
      : null;

    const currentSlide = revealDeck.detection.kind === 'reveal'
      ? revealDeck.slides[frozenIndex as number] ?? null
      : null;
    if (currentSlide && snapshotMarkdown && currentSlide.markdown === snapshotMarkdown) {
      return currentSlide;
    }
    if (currentSlide && !snapshotMarkdown) {
      const selectedMarkdown = String(inlineChatSelection.selectedMarkdown || inlineChatSelection.selectedText || '').trim();
      if (selectedMarkdown && currentSlide.markdown.includes(selectedMarkdown)) return currentSlide;
    }

    return frozenSlide;
  };

  const findRevealSlideForMarkdownSelection = (): RevealSlide | null => {
    if (revealDeck.detection.kind !== 'reveal' || inlineChatSelection.mode !== 'markdown') return null;
    const snapshotMarkdown = String(inlineChatSelection.revealSlideMarkdown || '');
    const frozenIndex = inlineChatSelection.revealSlideIndex;
    if (Number.isInteger(frozenIndex)) {
      const currentSlide = revealDeck.slides[frozenIndex as number] ?? null;
      if (currentSlide && (!snapshotMarkdown || currentSlide.markdown === snapshotMarkdown)) {
        return currentSlide;
      }
    }

    if (snapshotMarkdown) {
      return {
        index: Number.isInteger(frozenIndex) ? frozenIndex as number : 0,
        level: 'horizontal',
        markdown: snapshotMarkdown,
        label: inlineChatSelection.revealSlideLabel,
        separatorBefore: '',
        startOffset: 0,
        endOffset: snapshotMarkdown.length,
      };
    }

    return findRevealSlideForMarkdownOffsets(
      inlineChatSelection.snapshot,
      inlineChatSelection.startOffset,
      inlineChatSelection.endOffset,
      inlineChatSelection.cursorOffset,
    );
  };

  return inlineChatSelection.mode === 'rich'
    ? getRichRevealSlideSnapshot()
    : revealDeck.detection.kind === 'reveal'
      ? findRevealSlideForMarkdownSelection()
      : null;
}

/**
 * Monta o `SurfaceContext` do turno de chat inline do editor a partir da aba
 * mais recente e da seleção congelada. Não altera o contrato com o backend.
 */
export function buildEditorInlineChatSurfaceContext(
  latestActiveTab: InlineChatSurfaceTabInfo,
  inlineChatSelection: InlineChatSelection,
): SurfaceContext {
  const revealDeck = resolveRevealDeckForInlineSelection(latestActiveTab.markdown, inlineChatSelection);
  const currentRevealSlide = resolveRevealSlideForInlineSelection(revealDeck, inlineChatSelection);
  const isRevealSurface = revealDeck.detection.kind === 'reveal' || !!currentRevealSlide;
  const frozenRevealSlideCount = Number.isInteger(inlineChatSelection.revealSlideCount) && (inlineChatSelection.revealSlideCount ?? 0) > 0
    ? inlineChatSelection.revealSlideCount
    : undefined;
  const hasPreparedRevealSnapshot = !!currentRevealSlide && (
    Number.isInteger(inlineChatSelection.revealSlideIndex) ||
    !!inlineChatSelection.revealSlideMarkdown
  );
  const revealSlideCount = frozenRevealSlideCount ??
    (revealDeck.detection.kind === 'reveal'
      ? revealDeck.slides.length
      : hasPreparedRevealSnapshot
        ? undefined
        : 1);
  const presentationContext = isRevealSurface
    ? {
        slideCount: revealSlideCount,
        currentSlideIndex: currentRevealSlide?.index,
        currentSlideLabel: currentRevealSlide?.label,
        currentSlideMarkdown: currentRevealSlide?.markdown,
        presentationDetection: revealDeck.detection.confidence,
      }
    : {};
  const surfaceId = latestActiveTab.id;
  const surfaceMode = isRevealSurface ? 'reveal' : inlineChatSelection.mode;
  const selectionSnapshotSeed = inlineChatSelection.mode === 'rich'
    ? `${inlineChatSelection.from}:${inlineChatSelection.to}:${inlineChatSelection.revealSlideIndex ?? ''}:${inlineChatSelection.revealSlideMarkdown?.length ?? 0}:${inlineChatSelection.selectedText.length}:${String(inlineChatSelection.selectedMarkdown || '').length}`
    : `${inlineChatSelection.startOffset}:${inlineChatSelection.endOffset}:${inlineChatSelection.cursorOffset ?? ''}:${inlineChatSelection.revealSlideIndex ?? ''}:${inlineChatSelection.revealSlideMarkdown?.length ?? 0}:${inlineChatSelection.selectedText.length}`;
  const snapshotVersion = createSurfaceSnapshotVersion(
    'editor',
    surfaceId,
    `${latestActiveTab.filePath || latestActiveTab.draftId || ''}:${inlineChatSelection.mode}:${selectionSnapshotSeed}`,
  );
  return {
    surfaceType: 'editor',
    surfaceId,
    title: latestActiveTab.title,
    mode: surfaceMode,
    selection: inlineChatSelection.mode === 'rich'
      ? {
          kind: 'text',
          text: inlineChatSelection.selectedText,
          markdown: inlineChatSelection.selectedMarkdown,
          range: { startOffset: inlineChatSelection.from, endOffset: inlineChatSelection.to },
          isEmpty: !!inlineChatSelection.selectionIsEmpty,
          explicit: !inlineChatSelection.selectionIsEmpty,
        }
      : {
          kind: 'text',
          text: inlineChatSelection.selectedText,
          range: {
            startLine: inlineChatSelection.startLine,
            startColumn: inlineChatSelection.startColumn,
            endLine: inlineChatSelection.endLine,
            endColumn: inlineChatSelection.endColumn,
            startOffset: inlineChatSelection.startOffset,
            endOffset: inlineChatSelection.endOffset,
          },
          isEmpty: !!inlineChatSelection.selectionIsEmpty,
          explicit: !inlineChatSelection.selectionIsEmpty,
        },
    focus: inlineChatSelection.mode === 'rich'
      ? {
          kind: currentRevealSlide ? 'slide' : 'cursor',
          label: currentRevealSlide?.label,
          text: inlineChatSelection.cursorContext,
          range: { startOffset: inlineChatSelection.from, endOffset: inlineChatSelection.to },
          entity: currentRevealSlide ? { slideIndex: currentRevealSlide.index } : undefined,
        }
      : {
          kind: currentRevealSlide ? 'slide' : 'cursor',
          label: currentRevealSlide?.label,
          text: inlineChatSelection.cursorContext,
          cursor: {
            line: inlineChatSelection.cursorLine,
            column: inlineChatSelection.cursorColumn,
            offset: inlineChatSelection.cursorOffset,
          },
          entity: currentRevealSlide ? { slideIndex: currentRevealSlide.index } : undefined,
        },
    content: currentRevealSlide
      ? { kind: 'reveal_slide', markdown: currentRevealSlide.markdown }
      : {
          kind: 'document_window',
          text: inlineChatSelection.mode === 'rich'
            ? inlineChatSelection.displayMarkdown || inlineChatSelection.cursorContext
            : inlineChatSelection.cursorContext,
        },
    metadata: {
      documentId: latestActiveTab.id,
      filePath: latestActiveTab.filePath ?? undefined,
      draftId: latestActiveTab.draftId ?? undefined,
      language: 'markdown',
      ...presentationContext,
    },
    snapshotVersion,
    capturedAt: new Date().toISOString(),
    staleAfterMs: 120000,
  };
}

/**
 * Alguns modelos colocam o conteúdo dentro de um bloco ```markdown ... ```.
 * Para o editor, isso costuma ser ruído (a não ser que o usuário já tenha
 * selecionado um bloco fence).
 */
export function normalizeReplacementForEditor(raw: string, patchFormat: string | undefined, selectedText: string): string {
  const text = String(raw ?? '');
  const sel = String(selectedText ?? '');

  const looksLikeUserSelectedFence = /^\s*```/m.test(sel);

  const fence = text.match(/^\s*```\s*([a-z0-9_-]+)?\s*\r?\n([\s\S]*?)\r?\n```\s*$/i);
  if (!fence) return text;

  if (looksLikeUserSelectedFence) return text;

  const lang = String(fence[1] || '').trim().toLowerCase();
  const unwrapped = String(fence[2] || '');

  // Só unwrap para fences de markdown/texto (evita remover mermaid, etc.).
  const unwrapLangs = new Set(['markdown', 'md', 'text', 'plain', 'txt']);
  if (lang && unwrapLangs.has(lang)) return unwrapped;

  // Para patches plain, fences são quase sempre acidentais.
  if (patchFormat === 'plain' && (lang === '' || unwrapLangs.has(lang))) return unwrapped;

  return text;
}
