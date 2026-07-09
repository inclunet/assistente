import { useEffect, useRef, useState } from 'react';
import type { RefObject } from 'react';
import { useTranslation } from 'react-i18next';

import { useQuestionnaireUIStore } from '../store/questionnaireUIStore';
import { useUIStore } from '../store/uiStore';
import type { EditorDocument } from '../store/editorStore';
import { findMermaidFenceByIndex, removeMermaidFence, replaceMermaidFenceCode } from '../lib/mermaidFence';
import type { RichTextEditorHandle } from '../components/editor/RichTextEditor';
import type { RichMermaidSession } from './editorTypes';

/** Contexto recebido do editor rico ao pedir edição de um bloco Mermaid. */
export interface RichMermaidEditRequest {
  mermaidBlockId?: string;
  code?: string;
  insertText?: string;
  apply: (nextCode: string) => void;
  remove: () => void;
}

interface UseMermaidSessionArgs {
  activeTab: EditorDocument | null;
  richEditorHandleRef: RefObject<RichTextEditorHandle | null>;
  setDocMarkdown: (tabId: string, markdown: string) => void;
  updateLatestMarkdownForTab: (tabId: string, markdown: string) => void;
  schedulePersistForTab: (tabId: string) => void;
  focusEditorSoon: () => void;
}

/**
 * Hook que gerencia a sessão de edição de diagramas Mermaid do editor:
 * - modo Markdown: blocos endereçados por índice de fence no documento;
 * - modo rico: sessão vinda do TipTap (`RichMermaidSession`), com apply/remove
 *   preferindo a API por id do editor rico.
 * Expõe os handlers prontos para o `MermaidEditorModal`.
 */
export function useMermaidSession({
  activeTab,
  richEditorHandleRef,
  setDocMarkdown,
  updateLatestMarkdownForTab,
  schedulePersistForTab,
  focusEditorSoon,
}: UseMermaidSessionArgs) {
  const { t } = useTranslation();
  const addToast = useUIStore((s) => s.addToast);
  const requestQuestionnaire = useQuestionnaireUIStore((s) => s.request);

  const [activeMermaidIndex, setActiveMermaidIndex] = useState<number | null>(null);
  const [mermaidInitialCode, setMermaidInitialCode] = useState('');
  const [mermaidInsertText, setMermaidInsertText] = useState('');
  const [richMermaidSession, setRichMermaidSession] = useState<RichMermaidSession | null>(null);

  // Foco previsível após fechar o modal Mermaid.
  const prevMermaidOpenRef = useRef(false);
  useEffect(() => {
    const isOpen = activeMermaidIndex !== null;
    if (prevMermaidOpenRef.current && !isOpen) {
      focusEditorSoon();
    }
    prevMermaidOpenRef.current = isOpen;
  }, [activeMermaidIndex]);

  const openMermaidEditorByIndex = (index: number, opts?: { insertText?: string }) => {
    if (!activeTab) return;
    const fence = findMermaidFenceByIndex(activeTab.markdown, index);
    if (!fence) {
      addToast(t('editor.chatModal.mermaidBlockNotFound'), 'error');
      return;
    }
    setActiveMermaidIndex(index);
    setMermaidInitialCode(fence.code);
    setMermaidInsertText(opts?.insertText ? String(opts.insertText) : '');
  };

  const applyMermaidCode = (code: string) => {
    if (!activeTab) return;
    if (activeMermaidIndex === null) return;
    const fence = findMermaidFenceByIndex(activeTab.markdown, activeMermaidIndex);
    if (!fence) {
      addToast(t('editor.toast.mermaidBlockGone'), 'error');
      return;
    }
    const nextMarkdown = replaceMermaidFenceCode(activeTab.markdown, fence, code);
    setDocMarkdown(activeTab.id, nextMarkdown);
    updateLatestMarkdownForTab(activeTab.id, nextMarkdown);
    schedulePersistForTab(activeTab.id);
    addToast(t('editor.toast.mermaidUpdated'), 'success');
    setActiveMermaidIndex(null);
  };

  const removeMermaidBlockByIndex = async (index: number, reopenOnCancel?: { code: string }) => {
    if (!activeTab) return;

    const confirm = await requestQuestionnaire({
      id: `ui-editor-mermaid-remove-${Date.now()}`,
      title: 'Remover diagrama Mermaid',
      description: 'Tem certeza que deseja remover este bloco Mermaid do documento?',
      submitLabel: 'Remover',
      cancelLabel: 'Cancelar',
      allowCancel: true,
      questions: [
        {
          id: 'note',
          type: 'readonly_code',
          prompt: 'Dica',
          content: 'Essa ação remove o bloco ```mermaid``` inteiro.',
        },
      ],
    });

    if (confirm.cancelled) {
      if (reopenOnCancel) {
        setActiveMermaidIndex(index);
        setMermaidInitialCode(reopenOnCancel.code);
      }
      return;
    }

    const fence = findMermaidFenceByIndex(activeTab.markdown, index);
    if (!fence) {
      addToast(t('editor.toast.mermaidBlockGone'), 'error');
      return;
    }

    const nextMarkdown = removeMermaidFence(activeTab.markdown, fence);
    setDocMarkdown(activeTab.id, nextMarkdown);
    updateLatestMarkdownForTab(activeTab.id, nextMarkdown);
    schedulePersistForTab(activeTab.id);
    addToast(t('editor.toast.mermaidRemoved'), 'success');
  };

  const requestEditRichMermaid = (ctx: RichMermaidEditRequest) => {
    const mermaidBlockId = String(ctx.mermaidBlockId || '').trim();
    const api = richEditorHandleRef.current;
    setRichMermaidSession({
      mermaidBlockId,
      initialCode: String(ctx.code || ''),
      insertText: String(ctx.insertText || ''),
      apply: (nextCode: string) => {
        if (mermaidBlockId && api?.applyMermaidById?.(mermaidBlockId, nextCode)) return;
        ctx.apply(nextCode);
      },
      remove: () => {
        if (mermaidBlockId && api?.removeMermaidById?.(mermaidBlockId)) return;
        ctx.remove();
      },
    });
  };

  const isMermaidModalOpen = activeMermaidIndex !== null || richMermaidSession !== null;
  const mermaidModalInitialCode =
    activeMermaidIndex !== null
      ? mermaidInitialCode
      : richMermaidSession?.initialCode || '';
  const mermaidModalInitialInsertText =
    activeMermaidIndex !== null
      ? mermaidInsertText
      : String(richMermaidSession?.insertText || '');

  const consumeMermaidInsertText = () => {
    if (activeMermaidIndex !== null) setMermaidInsertText('');
    if (richMermaidSession) {
      setRichMermaidSession((prev) => (prev ? { ...prev, insertText: '' } : prev));
    }
  };

  const cancelMermaidModal = () => {
    if (activeMermaidIndex !== null) setActiveMermaidIndex(null);
    if (richMermaidSession) setRichMermaidSession(null);
  };

  const applyMermaidModal = (code: string) => {
    if (activeMermaidIndex !== null) {
      applyMermaidCode(code);
      return;
    }
    if (richMermaidSession) {
      richMermaidSession.apply(code);
      addToast(t('editor.toast.mermaidUpdated'), 'success');
      setRichMermaidSession(null);
    }
  };

  const removeMermaidFromModal = async () => {
    if (activeTab?.mode === 'markdown') {
      if (activeMermaidIndex === null) return;
      const index = activeMermaidIndex;
      const code = mermaidInitialCode;
      setActiveMermaidIndex(null);
      await removeMermaidBlockByIndex(index, { code });
      return;
    }

    if (richMermaidSession) {
      const confirm = await requestQuestionnaire({
        id: `ui-editor-rich-mermaid-remove-${Date.now()}`,
        title: 'Remover diagrama Mermaid',
        description: 'Tem certeza que deseja remover este bloco Mermaid do documento? ',
        submitLabel: 'Remover',
        cancelLabel: 'Cancelar',
        allowCancel: true,
        questions: [
          {
            id: 'note',
            type: 'readonly_code',
            prompt: 'Dica',
            content: 'Essa ação remove o bloco ```mermaid``` inteiro.',
          },
        ],
      });

      if (confirm.cancelled) return;
      richMermaidSession.remove();
      addToast(t('editor.toast.mermaidRemoved'), 'success');
      setRichMermaidSession(null);
    }
  };

  return {
    openMermaidEditorByIndex,
    removeMermaidBlockByIndex,
    requestEditRichMermaid,
    isMermaidModalOpen,
    mermaidModalInitialCode,
    mermaidModalInitialInsertText,
    consumeMermaidInsertText,
    cancelMermaidModal,
    applyMermaidModal,
    removeMermaidFromModal,
  };
}
