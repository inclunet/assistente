import { useEffect, useRef } from 'react';
import type { RefObject } from 'react';

import type { EditorDocument, EditorMode } from '../store/editorStore';
import type {
  MarkdownSelectionSnapshot,
  MonacoCodeEditor,
  MonacoNamespace,
  RichSelectionSnapshot,
  TipTapEditor,
} from './editorTypes';

/** Snapshot de seleção tipado pelo modo do editor ativo. */
export type EditorSelectionSnapshot =
  | { mode: 'markdown'; snapshot: MarkdownSelectionSnapshot }
  | { mode: 'rich'; snapshot: RichSelectionSnapshot };

export const EDITOR_SELECTION_CACHE_STALE_AFTER_MS = 120000;

interface UseEditorSelectionSnapshotsArgs {
  activeTab: EditorDocument | null;
  editorReadyNonce: number;
  editorRef: RefObject<MonacoCodeEditor | null>;
  monacoRef: RefObject<MonacoNamespace | null>;
  richEditorRef: RefObject<TipTapEditor | null>;
}

/**
 * Hook que lê snapshots de seleção do Monaco/TipTap e mantém um cache da
 * última seleção explícita por aba. O cache permite que o chat inline use a
 * seleção mesmo quando o foco já saiu do editor (ex.: clicou na toolbar),
 * com validação de que o texto selecionado ainda está no mesmo lugar.
 */
export function useEditorSelectionSnapshots({
  activeTab,
  editorReadyNonce,
  editorRef,
  monacoRef,
  richEditorRef,
}: UseEditorSelectionSnapshotsArgs) {
  const lastExplicitSelectionRef = useRef<{
    tabId: string;
    capturedAt: number;
    selection: EditorSelectionSnapshot;
  } | null>(null);

  const getSelectionSnapshot = (): MarkdownSelectionSnapshot | null => {
    const editor = editorRef.current;
    const monaco = monacoRef.current;
    if (!editor || !monaco) return null;

    const model = editor.getModel();
    if (!model) return null;

    const selection = editor.getSelection();
    const position = editor.getPosition();
    if (!selection || !position) return null;

    const start = selection.getStartPosition();
    const end = selection.getEndPosition();
    const selectedText = model.getValueInRange(selection);

    const startOffset = model.getOffsetAt(start);
    const endOffset = model.getOffsetAt(end);

    const full = model.getValue();
    const cursorOffset = model.getOffsetAt(position);
    const windowSize = 260;
    const before = full.slice(Math.max(0, cursorOffset - windowSize), cursorOffset);
    const after = full.slice(cursorOffset, Math.min(full.length, cursorOffset + windowSize));
    const cursorContext = (before + '⟂' + after).trimEnd();

    const selectionIsEmpty = !selectedText;
    const displayText = selectionIsEmpty ? cursorContext : selectedText;

    return {
      selectedText,
      selectionIsEmpty,
      cursorContext,
      displayText,
      startOffset,
      endOffset,
      startLine: start.lineNumber,
      startColumn: start.column,
      endLine: end.lineNumber,
      endColumn: end.column,
      cursorLine: position.lineNumber,
      cursorColumn: position.column,
      cursorOffset,
    };
  };

  const getRichSelectionSnapshot = (): RichSelectionSnapshot | null => {
    const editor = richEditorRef.current;
    if (!editor) return null;

    const sel = editor.state?.selection;
    if (!sel) return null;

    const { from, to, empty } = sel;
    const selectedText = editor.state.doc.textBetween(from, to, '\n');
    const textBeforeSelection = editor.state.doc.textBetween(0, from, '\n');
    const docSizeForSelection = Number(editor.state.doc.content?.size ?? to);
    const textAfterSelection = editor.state.doc.textBetween(to, Math.max(to, docSizeForSelection), '\n');

    const markdownStorage = (editor.storage as unknown as Record<string, unknown> | undefined)?.markdown as
      | { serializer?: { serialize?: (node: unknown) => string }; getMarkdown?: () => string }
      | undefined;
    const serializer = markdownStorage?.serializer;
    const serializeNodeToMarkdown = (node: unknown): string => {
      try {
        if (serializer?.serialize) return String(serializer.serialize(node) ?? '');
      } catch {
        // best-effort
      }
      return '';
    };

    const getMarkdownForRange = (fromPos: number, toPos: number) => {
      try {
        const doc = editor.state?.doc;
        if (!doc) return '';
        // `cut` retorna um Node do tipo doc com o conteúdo do range.
        const cut = doc.cut(Math.max(0, fromPos), Math.max(0, toPos));
        return serializeNodeToMarkdown(cut);
      } catch {
        return '';
      }
    };

    let selectedMarkdown = '';
    if (!empty && to > from) {
      selectedMarkdown = getMarkdownForRange(from, to);
    }

    // Contexto ao redor do cursor (para inserção quando empty=true)
    let cursorContext = '';
    try {
      const docSize = editor.state.doc.content.size;
      const windowSize = 260;
      const winFrom = Math.max(0, from - windowSize);
      const winTo = Math.min(docSize, from + windowSize);
      const before = editor.state.doc.textBetween(winFrom, from, '\n');
      const after = editor.state.doc.textBetween(from, winTo, '\n');
      cursorContext = (before + '⟂' + after).trimEnd();
    } catch {
      cursorContext = '';
    }

    const selectionIsEmpty = !!empty || !selectedText;

    // Quando não há seleção, usa o bloco atual como “contexto” em Markdown.
    let displayMarkdown = selectedMarkdown;
    if (selectionIsEmpty) {
      try {
        const $from = sel.$from;
        if ($from) {
          let depth = $from.depth;
          while (depth > 0 && !$from.node(depth)?.isBlock) depth -= 1;
          if (depth > 0) {
            const nodeStart = $from.before(depth);
            const nodeSize = $from.node(depth)?.nodeSize ?? 0;
            const nodeEnd = nodeStart + nodeSize;
            if (nodeSize > 0) {
              displayMarkdown = getMarkdownForRange(nodeStart, nodeEnd);
            }
          }
        }
      } catch {
        // best-effort
      }
    }

    const displayText = selectionIsEmpty ? (cursorContext || '(cursor)') : selectedText;
    const displayForContextPanel = displayMarkdown || (selectionIsEmpty ? (cursorContext || '(cursor)') : selectedText);

    // Snapshot do documento (para debug/consistência): prefere o Markdown atual do TipTap.
    let snapshot = '';
    try {
      snapshot = String(markdownStorage?.getMarkdown?.() ?? '');
    } catch {
      snapshot = '';
    }

    return {
      selectedText,
      selectedMarkdown: selectedMarkdown || undefined,
      selectionIsEmpty,
      cursorContext,
      displayText,
      displayMarkdown: displayForContextPanel || undefined,
      textBeforeSelection,
      textAfterSelection,
      from,
      to,
      snapshot,
    };
  };

  const hasExplicitSelection = (selection: EditorSelectionSnapshot) => {
    if (selection.mode === 'markdown') {
      return !selection.snapshot.selectionIsEmpty && !!selection.snapshot.selectedText;
    }
    return !selection.snapshot.selectionIsEmpty && !!(selection.snapshot.selectedText || selection.snapshot.selectedMarkdown);
  };

  const isEditorFocusedForMode = (mode: EditorMode) => {
    if (mode === 'markdown') return !!editorRef.current?.hasTextFocus?.();
    if (mode === 'rich') {
      const rich = richEditorRef.current;
      return !!(rich?.view?.hasFocus?.() ?? rich?.isFocused);
    }
    return false;
  };

  const readCurrentSelectionSnapshot = (): EditorSelectionSnapshot | null => {
    if (!activeTab) return null;
    if (activeTab.mode === 'markdown') {
      const snapshot = getSelectionSnapshot();
      return snapshot ? { mode: 'markdown', snapshot } : null;
    }
    if (activeTab.mode === 'rich') {
      const snapshot = getRichSelectionSnapshot();
      return snapshot ? { mode: 'rich', snapshot } : null;
    }
    return null;
  };

  const rememberCurrentExplicitSelection = () => {
    if (!activeTab) return null;
    const selection = readCurrentSelectionSnapshot();
    if (!selection) return null;

    if (hasExplicitSelection(selection)) {
      lastExplicitSelectionRef.current = {
        tabId: activeTab.id,
        capturedAt: Date.now(),
        selection,
      };
      return selection;
    }

    if (isEditorFocusedForMode(activeTab.mode)) {
      lastExplicitSelectionRef.current = null;
    }
    return selection;
  };

  const isCachedSelectionStillValid = (cached: EditorSelectionSnapshot) => {
    try {
      if (cached.mode === 'markdown') {
        const model = editorRef.current?.getModel?.();
        if (!model) return false;
        const current = model.getValue?.() ?? activeTab?.markdown ?? '';
        const expected = cached.snapshot.selectedText;
        return current.slice(cached.snapshot.startOffset, cached.snapshot.endOffset) === expected;
      }

      const rich = richEditorRef.current;
      if (!rich) return false;
      const expected = cached.snapshot.selectedText;
      const current = String(
        rich.state?.doc?.textBetween?.(cached.snapshot.from, cached.snapshot.to, '\n') ?? '',
      );
      return current === expected;
    } catch {
      return false;
    }
  };

  const getPreparedSelectionSnapshot = (): EditorSelectionSnapshot | null => {
    const live = readCurrentSelectionSnapshot();
    const cached = lastExplicitSelectionRef.current;
    if (!activeTab || !cached) return live;
    if (cached.tabId !== activeTab.id) return live;
    if (cached.selection.mode !== activeTab.mode) return live;
    if (!hasExplicitSelection(cached.selection)) return live;
    if (Date.now() - cached.capturedAt > EDITOR_SELECTION_CACHE_STALE_AFTER_MS) return live;
    if (live && hasExplicitSelection(live)) return live;
    if (live && isEditorFocusedForMode(activeTab.mode)) return live;
    if (!isCachedSelectionStillValid(cached.selection)) return live;
    return cached.selection;
  };

  useEffect(() => {
    if (!activeTab || activeTab.mode !== 'markdown') return;
    const editor = editorRef.current;
    const onDidChangeCursorSelection = editor?.onDidChangeCursorSelection;
    if (typeof onDidChangeCursorSelection !== 'function') return;

    const disposable = onDidChangeCursorSelection.call(editor, () => {
      rememberCurrentExplicitSelection();
    }) as { dispose?: () => void } | undefined;

    return () => disposable?.dispose?.();
  }, [activeTab?.id, activeTab?.mode, editorReadyNonce]);

  useEffect(() => {
    if (!activeTab || activeTab.mode !== 'rich') return;
    const rich = richEditorRef.current as unknown as {
      on?: (event: string, callback: () => void) => void;
      off?: (event: string, callback: () => void) => void;
    } | null;
    if (typeof rich?.on !== 'function') return;

    const onSelectionUpdate = () => {
      rememberCurrentExplicitSelection();
    };

    rich.on('selectionUpdate', onSelectionUpdate);
    return () => rich.off?.('selectionUpdate', onSelectionUpdate);
  }, [activeTab?.id, activeTab?.mode, editorReadyNonce]);

  return {
    rememberCurrentExplicitSelection,
    getPreparedSelectionSnapshot,
  };
}
