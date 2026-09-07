import { useEffect, useRef } from 'react';
import type { RefObject } from 'react';

import type { EditorDocument } from '../store/editorStore';
import {
  clampNumber,
  findTextRangeInRichDoc,
  findTextRangeInRichDocByContext,
  getChangedRangeAfterTextReplacement,
} from './editorTextRange';
import type { InlineChatSelection, MonacoCodeEditor, TipTapEditor } from './editorTypes';

/** Restauração pendente de seleção/foco no editor após o chat inline fechar. */
export type PendingInlineChatEditorRestore =
  | {
      mode: 'markdown';
      tabId: string;
      startOffset: number;
      endOffset: number;
      sourceMarkdown?: string;
      expectedMarkdown?: string;
    }
  | {
      mode: 'rich';
      tabId: string;
      from: number;
      to: number;
      anchorText?: string;
      anchorTextBefore?: string;
      anchorTextAfter?: string;
    };

interface UseInlineChatSelectionRestoreArgs {
  activeTab: EditorDocument | null;
  chatModalOpen: boolean;
  editorReadyNonce: number;
  editorRef: RefObject<MonacoCodeEditor | null>;
  richEditorRef: RefObject<TipTapEditor | null>;
  focusEditorSoon: () => void;
}

/**
 * Hook que enfileira e executa a restauração de seleção do editor depois que
 * o chat inline aplica uma mudança (patch local ou tool do assistente).
 *
 * A restauração só acontece quando o modal fecha, na aba/modo corretos e —
 * no Markdown — quando o conteúdo esperado já está visível no Monaco (senão
 * espera o próximo `onDidChangeModelContent`).
 */
export function useInlineChatSelectionRestore({
  activeTab,
  chatModalOpen,
  editorReadyNonce,
  editorRef,
  richEditorRef,
  focusEditorSoon,
}: UseInlineChatSelectionRestoreArgs) {
  const pendingInlineChatEditorRestoreRef = useRef<PendingInlineChatEditorRestore | null>(null);
  const pendingInlineChatEditorRestoreDisposeRef = useRef<(() => void) | null>(null);

  const clearPendingInlineChatEditorRestore = () => {
    pendingInlineChatEditorRestoreDisposeRef.current?.();
    pendingInlineChatEditorRestoreDisposeRef.current = null;
    pendingInlineChatEditorRestoreRef.current = null;
  };

  const restoreMarkdownEditorSelection = (restore: Extract<PendingInlineChatEditorRestore, { mode: 'markdown' }>) => {
    const editor = editorRef.current;
    const model = editor?.getModel?.();
    if (!editor || !model) return false;

    const current = String(model.getValue?.() ?? activeTab?.markdown ?? '');
    if (restore.expectedMarkdown !== undefined && current !== restore.expectedMarkdown) return false;

    const startOffset = clampNumber(restore.startOffset, 0, current.length);
    const endOffset = clampNumber(restore.endOffset, startOffset, current.length);
    const startPos = model.getPositionAt(startOffset);
    const endPos = model.getPositionAt(endOffset);
    const range = {
      startLineNumber: startPos.lineNumber,
      startColumn: startPos.column,
      endLineNumber: endPos.lineNumber,
      endColumn: endPos.column,
    };

    try {
      editor.setSelection(range);
      if (startOffset === endOffset) {
        editor.setPosition?.(startPos);
        editor.revealPositionInCenter?.(startPos);
      } else {
        editor.revealRangeInCenter?.(range);
      }
      editor.focus();
      return true;
    } catch {
      return false;
    }
  };

  const restoreRichEditorSelection = (restore: Extract<PendingInlineChatEditorRestore, { mode: 'rich' }>) => {
    const rich = richEditorRef.current;
    if (!rich) return false;

    const doc = rich.state?.doc;
    const docSize = Number(doc?.content?.size ?? restore.to);
    const contextRange = restore.anchorTextAfter
      ? findTextRangeInRichDocByContext(doc, restore.anchorTextBefore, restore.anchorTextAfter)
      : null;
    const anchorRange = contextRange ?? (restore.anchorText
      ? findTextRangeInRichDoc(doc, restore.anchorText, restore.anchorTextBefore)
      : null);
    const from = clampNumber(anchorRange?.from ?? restore.from, 0, Math.max(0, docSize));
    const to = clampNumber(anchorRange?.to ?? restore.to, from, Math.max(from, docSize));

    try {
      rich.chain?.().focus().setTextSelection({ from, to }).run();
      rich.view?.focus?.();
      return true;
    } catch {
      try {
        rich.commands?.focus?.();
        rich.view?.focus?.();
      } catch {
        // best-effort
      }
      return false;
    }
  };

  const queueMarkdownEditorRestore = (params: {
    tabId: string;
    startOffset: number;
    endOffset: number;
    sourceMarkdown?: string;
    expectedMarkdown?: string;
  }) => {
    pendingInlineChatEditorRestoreDisposeRef.current?.();
    pendingInlineChatEditorRestoreDisposeRef.current = null;
    pendingInlineChatEditorRestoreRef.current = {
      mode: 'markdown',
      tabId: params.tabId,
      startOffset: params.startOffset,
      endOffset: params.endOffset,
      sourceMarkdown: params.sourceMarkdown,
      expectedMarkdown: params.expectedMarkdown,
    };
  };

  const queueRichEditorRestore = (params: {
    tabId: string;
    from: number;
    to: number;
    anchorText?: string;
    anchorTextBefore?: string;
    anchorTextAfter?: string;
  }) => {
    pendingInlineChatEditorRestoreDisposeRef.current?.();
    pendingInlineChatEditorRestoreDisposeRef.current = null;
    pendingInlineChatEditorRestoreRef.current = {
      mode: 'rich',
      tabId: params.tabId,
      from: params.from,
      to: params.to,
      anchorText: params.anchorText,
      anchorTextBefore: params.anchorTextBefore,
      anchorTextAfter: params.anchorTextAfter,
    };
  };

  const queueEditorRestoreForInlineSelection = (params: {
    selection: InlineChatSelection;
    markdownBefore: string;
    markdownAfter: string;
    expectedMarkdown?: string;
  }) => {
    const { selection } = params;
    if (selection.mode === 'markdown') {
      const range = getChangedRangeAfterTextReplacement({
        before: params.markdownBefore,
        after: params.markdownAfter,
        fallbackStartOffset: selection.startOffset,
        fallbackEndOffset: selection.endOffset,
        fallbackSelectedText: selection.selectedText,
      });
      queueMarkdownEditorRestore({
        tabId: selection.tabId,
        startOffset: range.startOffset,
        endOffset: range.endOffset,
        sourceMarkdown: params.markdownBefore,
        expectedMarkdown: params.expectedMarkdown,
      });
      return;
    }

    const richAnchorSource = String(selection.selectedText || selection.selectedMarkdown || '').trim();
    queueRichEditorRestore({
      tabId: selection.tabId,
      from: selection.from,
      to: selection.from,
      anchorText: richAnchorSource || selection.selectedText,
      anchorTextBefore: selection.textBeforeSelection,
      anchorTextAfter: selection.textAfterSelection,
    });
  };

  useEffect(() => {
    const pending = pendingInlineChatEditorRestoreRef.current;
    if (!pending || !activeTab) return;
    if (chatModalOpen) return;
    if (pending.tabId !== activeTab.id) return;
    if (pending.mode !== activeTab.mode) {
      clearPendingInlineChatEditorRestore();
      return;
    }

    if (pending.mode === 'markdown') {
      if (pending.expectedMarkdown !== undefined && activeTab.markdown !== pending.expectedMarkdown) {
        if (pending.sourceMarkdown !== undefined && activeTab.markdown !== pending.sourceMarkdown) {
          clearPendingInlineChatEditorRestore();
        }
        return;
      }
      if (!restoreMarkdownEditorSelection(pending)) return;
      clearPendingInlineChatEditorRestore();
      return;
    }

    if (!restoreRichEditorSelection(pending)) {
      focusEditorSoon();
      return;
    }
    clearPendingInlineChatEditorRestore();
  }, [activeTab?.id, activeTab?.mode, activeTab?.markdown, chatModalOpen, editorReadyNonce]);

  useEffect(() => {
    const pending = pendingInlineChatEditorRestoreRef.current;
    if (!pending || pending.mode !== 'markdown' || pending.expectedMarkdown === undefined) return;
    if (!activeTab) return;
    if (chatModalOpen) return;
    if (pending.tabId !== activeTab.id) return;
    if (activeTab.mode !== 'markdown') {
      clearPendingInlineChatEditorRestore();
      return;
    }
    if (activeTab.markdown !== pending.expectedMarkdown) return;
    if (restoreMarkdownEditorSelection(pending)) {
      clearPendingInlineChatEditorRestore();
      return;
    }

    const editor = editorRef.current;
    const onDidChangeModelContent = editor?.onDidChangeModelContent;
    if (typeof onDidChangeModelContent !== 'function') {
      focusEditorSoon();
      return;
    }

    const disposable = onDidChangeModelContent.call(editor, () => {
      const latestPending = pendingInlineChatEditorRestoreRef.current;
      if (!latestPending || latestPending.mode !== 'markdown') return;
      if (latestPending.tabId !== activeTab.id) return;
      if (restoreMarkdownEditorSelection(latestPending)) {
        clearPendingInlineChatEditorRestore();
      }
    }) as { dispose?: () => void } | undefined;

    const dispose = () => disposable?.dispose?.();
    pendingInlineChatEditorRestoreDisposeRef.current = dispose;
    return () => {
      if (pendingInlineChatEditorRestoreDisposeRef.current === dispose) {
        pendingInlineChatEditorRestoreDisposeRef.current = null;
      }
      dispose();
    };
  }, [activeTab?.id, activeTab?.mode, activeTab?.markdown, chatModalOpen, editorReadyNonce]);

  // Dispose do listener pendente ao desmontar a página.
  useEffect(() => {
    return () => {
      pendingInlineChatEditorRestoreDisposeRef.current?.();
      pendingInlineChatEditorRestoreDisposeRef.current = null;
    };
  }, []);

  return {
    clearPendingInlineChatEditorRestore,
    queueMarkdownEditorRestore,
    queueRichEditorRestore,
    queueEditorRestoreForInlineSelection,
  };
}
