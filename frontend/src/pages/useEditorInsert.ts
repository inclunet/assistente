import { useEffect, useState } from 'react';
import type { RefObject } from 'react';
import { useTranslation } from 'react-i18next';

import { logger } from '../utils/logger';
import { useEditorStore, type EditorDocument, type EditorInsertRequest } from '../store/editorStore';
import { normalizeEditorInsertContent } from '../lib/editorInsertNormalize';
import { applyRichTextInsert, applyRichTextInsertAtEnd, type RichTextEditorLike } from '../lib/richTextPatchApply';
import { markdownToHtml } from '../lib/markdownToHtml';
import { computeMonacoInsertText } from '../lib/monacoInsertHeuristics';
import { EditorGetDraftPath } from '@wailsjs/go/wailsapi/Editor';
import type { AddToastFn } from './editorMenus';
import type { MonacoCodeEditor, MonacoNamespace, TipTapEditor } from './editorTypes';

interface UseEditorInsertArgs {
  activeTab: EditorDocument | null;
  currentDocumentId: string | null;
  sessionLoaded: boolean;
  editorReadyNonce: number;
  editorRef: RefObject<MonacoCodeEditor | null>;
  monacoRef: RefObject<MonacoNamespace | null>;
  richEditorRef: RefObject<TipTapEditor | null>;
  addWorkspaceTab: (type: 'editor', title: string, initialState?: Record<string, unknown>) => Promise<string>;
  setDocMarkdown: (tabId: string, markdown: string) => void;
  updateLatestMarkdownForTab: (tabId: string, markdown: string) => void;
  schedulePersistForTab: (tabId: string) => void;
  flushActiveRichMarkdownNow: () => void;
  focusEditorSoon: () => void;
  addToast: AddToastFn;
}

/**
 * Hook que aplica requisições de inserção vindas do Chat → Editor
 * (`EditorInsertRequest`): resolve a aba alvo (atual, direcionada ou nova),
 * normaliza o conteúdo e insere no Monaco ou TipTap com heurísticas de
 * cursor/foco. Também consome a fila de inserções pendentes do editorStore
 * com retry enquanto o editor monta.
 */
export function useEditorInsert({
  activeTab,
  currentDocumentId,
  sessionLoaded,
  editorReadyNonce,
  editorRef,
  monacoRef,
  richEditorRef,
  addWorkspaceTab,
  setDocMarkdown,
  updateLatestMarkdownForTab,
  schedulePersistForTab,
  flushActiveRichMarkdownNow,
  focusEditorSoon,
  addToast,
}: UseEditorInsertArgs) {
  const { t } = useTranslation();
  const [pendingInsert, setPendingInsert] = useState<EditorInsertRequest | null>(null);

  const applyInsertRequest = async (req: EditorInsertRequest): Promise<boolean> => {
    const r = req;
    const rawContent = String(r?.content ?? '');
    if (!rawContent) return true;

    const requestedDocumentId = String(r.targetDocumentId || '').trim();
    if (r.target === 'document' && !requestedDocumentId) {
      logger.error('[useEditorInsert] applyInsertRequest rejected: document target requires targetDocumentId');
      return false;
    }
    const currentEditorState = useEditorStore.getState();
    let targetTab = requestedDocumentId
      ? currentEditorState.documents[requestedDocumentId] ?? null
      : activeTab;

    if (requestedDocumentId && currentDocumentId !== requestedDocumentId) {
      return false;
    }

    if (r.target === 'new_document' || !targetTab) {
      if (requestedDocumentId) return false;
      const title = String(r.title || t('editor.fallback.fromChat'));
      const draftId = (typeof crypto !== 'undefined' && crypto.randomUUID) ? crypto.randomUUID() : `editor-${Date.now()}`;
      const draftPath = String(await EditorGetDraftPath(draftId) ?? '');
      const tabId = await addWorkspaceTab('editor', title, { filePath: draftPath, draftId });
      useEditorStore.getState().createDocument({ id: tabId, title, markdown: '', mode: 'markdown', filePath: draftPath, draftId });
      targetTab = useEditorStore.getState().documents[tabId] ?? null;
      await new Promise((res) => setTimeout(res, 0));
    }

    if (!targetTab) return false;

    const normalized = normalizeEditorInsertContent({
      content: rawContent,
      format: r.format,
      targetMode: targetTab.mode,
    });

    const content = normalized.content;
    const format = normalized.format;

    const focusAfter = r.focus !== false;

    if (targetTab.mode === 'markdown') {
      const editor = editorRef.current;
      const monaco = monacoRef.current;
      const model = editor?.getModel?.();
      const selection = editor?.getSelection?.();

      if (editor && monaco && model && selection) {
        const hasFocus = !!editor.hasTextFocus?.();
        const isEmptySel = !!selection.isEmpty?.();
        const selStart = selection.getStartPosition();
        const currentText = model.getValue?.() ?? '';
        const { useSelection, textToInsert } = computeMonacoInsertText({
          hasFocus,
          selectionIsEmpty: isEmptySel,
          selectionStart: { lineNumber: selStart.lineNumber, column: selStart.column },
          currentText,
          content,
        });

        // Se não há foco e a seleção é vazia (comum após navegação),
        // inserir no fim do documento é mais previsível do que no início.
        const endOffset = model.getValueLength?.() ?? currentText.length;
        const endPos = model.getPositionAt(endOffset);

        const insertRange =
          useSelection
            ? selection
            : typeof monaco.Range === 'function'
              ? new monaco.Range(endPos.lineNumber, endPos.column, endPos.lineNumber, endPos.column)
              : {
                  startLineNumber: endPos.lineNumber,
                  startColumn: endPos.column,
                  endLineNumber: endPos.lineNumber,
                  endColumn: endPos.column,
                };

        const startPos = useSelection ? selStart : endPos;
        const startOffset = model.getOffsetAt(startPos);

        editor.executeEdits('chat-to-editor-insert', [
          {
            range: insertRange,
            text: textToInsert,
            forceMoveMarkers: true,
          },
        ]);

        const nextOffset = startOffset + textToInsert.length;
        const nextPos = model.getPositionAt(nextOffset);
        editor.setPosition(nextPos);
        editor.revealPositionInCenter(nextPos);
        if (focusAfter) editor.focus();
        return true;
      }

      // Fallback: se o Monaco ainda não montou, aplica no markdown da aba (no final) e tenta focar depois.
      const current = String(targetTab.markdown ?? '');
      const nextText = current ? current + '\n\n' + content : content;
      setDocMarkdown(targetTab.id, nextText);
      updateLatestMarkdownForTab(targetTab.id, nextText);
      schedulePersistForTab(targetTab.id);
      if (focusAfter) focusEditorSoon();
      return true;
    }

    // Rich: insere no cursor/seleção atual.
    const rich = richEditorRef.current;
    if (!rich) return false;
    const sel = rich.state?.selection;
    if (!sel) return false;

    const richHasFocus = !!(rich.view?.hasFocus?.() ?? rich.isFocused);

    const from = Number(sel.from);
    const to = Number(sel.to);

    let contentToInsert: unknown = content;
    if (format === 'markdown') {
      contentToInsert = markdownToHtml(content);
    } else if (format === 'plain') {
      // Inserção como texto puro (sem interpretar como HTML).
      // Para manter comportamento previsível, tratamos como texto.
      contentToInsert = { type: 'text', text: content };
    }

    const richLike = rich as unknown as RichTextEditorLike;
    // Se não há foco (comum após navegar do Chat), a seleção pode estar no início.
    // Para um comportamento mais previsível, inserimos no fim do documento.
    if (!richHasFocus) {
      applyRichTextInsertAtEnd({ rich: richLike, contentToInsert });
    } else {
      applyRichTextInsert({ rich: richLike, from, to, contentToInsert });
    }
    flushActiveRichMarkdownNow();
    if (focusAfter) {
      try {
        rich.commands?.focus?.();
        rich.view?.focus?.();
      } catch {
        // best-effort
      }
    }
    return true;
  };

  // Consome requisições vindas do Chat → Editor (aba atual ou nova)
  useEffect(() => {
    if (!sessionLoaded) return;
    if (pendingInsert) return;
    const req = useEditorStore.getState().consumePendingInsert();
    if (req) setPendingInsert(req);
  }, [sessionLoaded, pendingInsert]);

  // Tenta aplicar quando o editor (Monaco/TipTap) estiver pronto.
  useEffect(() => {
    if (!pendingInsert) return;

    let cancelled = false;
    (async () => {
      // Inserções direcionadas podem precisar esperar a aba/documento terminar de sincronizar.
      const targetedInsert = !!String(pendingInsert.targetDocumentId || '').trim();
      const maxAttempts = targetedInsert ? 40 : 10;
      const delayMs = targetedInsert ? 100 : 60;
      for (let i = 0; i < maxAttempts; i += 1) {
        if (cancelled) return;
        const ok = await applyInsertRequest(pendingInsert);
        if (ok) {
          setPendingInsert(null);
          return;
        }
        await new Promise((r) => setTimeout(r, delayMs));
      }

      // Se falhar, mantém pendente mas avisa.
      addToast(t('editor.chatModal.insertExhausted'), 'error');
      setPendingInsert(null);
    })();

    return () => {
      cancelled = true;
    };
  }, [pendingInsert, editorReadyNonce]);

  return { applyInsertRequest };
}
