import { useEffect, useMemo, useRef, useState } from 'react';
import type { MutableRefObject, RefObject } from 'react';
import { useTranslation } from 'react-i18next';
import { EventsOn } from '@wailsjs/runtime/runtime';

import { logger } from '../utils/logger';
import { useRegisterWorkspaceChatAdapter } from '../hooks/useRegisterWorkspaceChatAdapter';
import { useWorkspaceChatModalStore } from '../store/workspaceChatModalStore';
import type {
  WorkspaceChatModalAdapter,
  WorkspaceChatModalPrepareResult,
  WorkspaceChatSendPlan,
  WorkspaceChatModalSession,
} from '../store/workspaceChatModalStore';
import { useEditorStore, type EditorDocument } from '../store/editorStore';
import type { WorkspaceTab } from '../store/workspaceStore';
import { useQuestionnaireUIStore } from '../store/questionnaireUIStore';
import { useUIStore } from '../store/uiStore';
import { useChatStore } from '../store/chatStore';
import { applyTextReplacementByOffset } from '../lib/editorPatchApply';
import { applyRichTextInsert, type RichTextEditorLike } from '../lib/richTextPatchApply';
import { validateRichTextSelectionSnapshot } from '../lib/richTextSelectionValidation';
import { markdownToHtml } from '../lib/markdownToHtml';
import { buildChatSurfaceParams } from '../lib/chatSurface';
import { parseRevealMarkdown } from '../lib/revealMarkdown';
import { getErrorMessage } from '../lib/editorContent';
import { normalizePathKey } from '../utils/path';
import { useEditorInlineChatPatch } from '../hooks/useEditorInlineChatPatch';
import type { MediaFile } from '../services/mediaService';
import type { Message } from '../store/chatStore';
import { isAppToolEvent } from '../types/chat';
import { GetProfile } from '@wailsjs/go/wailsapi/Profiles';
import {
  buildEditorInlineChatSurfaceContext,
  findRevealSlideForMarkdownOffsets,
  normalizeReplacementForEditor,
} from './editorInlineChatContext';
import type { EditorSelectionSnapshot } from './useEditorSelectionSnapshots';
import type {
  EditorPatch,
  EditorFileChangedEvent,
  InlineChatSelection,
  MarkdownSelectionSnapshot,
  MonacoCodeEditor,
  RichSelectionSnapshot,
  TipTapEditor,
} from './editorTypes';

const EDITOR_APPLY_TOOL_NAMES = new Set(['edit_file', 'text_edit', 'write_file']);

interface UseEditorInlineChatArgs {
  activeTab: EditorDocument | null;
  workspaceTab?: WorkspaceTab;
  currentDocumentId: string | null;
  effectiveProfileSlug: string;
  editorReadyNonce: number;
  editorRef: RefObject<MonacoCodeEditor | null>;
  richEditorRef: RefObject<TipTapEditor | null>;
  currentRevealSlideIndexRef: MutableRefObject<number>;
  flushActiveRichMarkdownNow: () => void;
  persistTabContentNow: (tabId: string) => Promise<void>;
  syncAssistedChangeForTab: (tabId: string) => Promise<boolean>;
  setDocMarkdown: (tabId: string, markdown: string) => void;
  updateLatestMarkdownForTab: (tabId: string, markdown: string) => void;
  schedulePersistForTab: (tabId: string) => void;
  focusEditorSoon: () => void;
  getPreparedSelectionSnapshot: () => EditorSelectionSnapshot | null;
  clearPendingInlineChatEditorRestore: () => void;
  queueMarkdownEditorRestore: (params: {
    tabId: string;
    startOffset: number;
    endOffset: number;
    sourceMarkdown?: string;
    expectedMarkdown?: string;
  }) => void;
  queueRichEditorRestore: (params: { tabId: string; from: number; to: number }) => void;
  queueEditorRestoreForInlineSelection: (params: {
    selection: InlineChatSelection;
    markdownBefore: string;
    markdownAfter: string;
    expectedMarkdown?: string;
  }) => void;
}

/**
 * Hook do chat inline do editor (Ctrl+Shift+I): prepara a seleção para o
 * modal de chat do workspace, monta o plano de envio e trata o resultado do
 * turno (tool `edit_file`/`text_edit` ou patch body-only com confirmação).
 *
 * AEP-0040 (backend-driven messaging): este hook NÃO cria fluxo alternativo
 * de envio. Ele devolve um `WorkspaceChatSendPlan` para o pipeline
 * compartilhado do chat modal do workspace, que faz o envio real.
 */
export function useEditorInlineChat({
  activeTab,
  workspaceTab,
  currentDocumentId,
  effectiveProfileSlug,
  editorReadyNonce,
  editorRef,
  richEditorRef,
  currentRevealSlideIndexRef,
  flushActiveRichMarkdownNow,
  persistTabContentNow,
  syncAssistedChangeForTab,
  setDocMarkdown,
  updateLatestMarkdownForTab,
  schedulePersistForTab,
  focusEditorSoon,
  getPreparedSelectionSnapshot,
  clearPendingInlineChatEditorRestore,
  queueMarkdownEditorRestore,
  queueRichEditorRestore,
  queueEditorRestoreForInlineSelection,
}: UseEditorInlineChatArgs) {
  const { t } = useTranslation();
  const addToast = useUIStore((s) => s.addToast);
  const requestQuestionnaire = useQuestionnaireUIStore((s) => s.request);
  const { waitForChatDone, waitForEditorPatch, getMaxMessageId } = useEditorInlineChatPatch();

  const [isAsking, setIsAsking] = useState(false);
  const inlineChatRunIdRef = useRef(0);
  const inlineChatToolCloseCleanupsRef = useRef<Set<() => void>>(new Set());
  const chatModalOpen = useWorkspaceChatModalStore((s) => s.isOpen);
  const prevChatModalOpenRef = useRef(false);

  useEffect(() => {
    if (chatModalOpen) return;
    for (const cleanup of inlineChatToolCloseCleanupsRef.current) {
      cleanup();
    }
    inlineChatToolCloseCleanupsRef.current.clear();
  }, [chatModalOpen]);

  useEffect(() => {
    return () => {
      for (const cleanup of inlineChatToolCloseCleanupsRef.current) {
        cleanup();
      }
      inlineChatToolCloseCleanupsRef.current.clear();
    };
  }, []);

  useEffect(() => {
    if (prevChatModalOpenRef.current && !chatModalOpen) {
      inlineChatRunIdRef.current += 1;
      setIsAsking(false);
      focusEditorSoon();
    }
    prevChatModalOpenRef.current = chatModalOpen;
  }, [chatModalOpen, activeTab]);

  const sendEditorChatModalMessage = async (
    instruction: string,
    mediaFiles: MediaFile[] | undefined,
    inlineChatSelection: InlineChatSelection,
    session?: WorkspaceChatModalSession,
  ): Promise<WorkspaceChatSendPlan> => {
    if (!activeTab) return null;

    const expectedConversationId = session?.conversationId || workspaceTab?.conversationId || undefined;
    if (!expectedConversationId) return null;

    const beforeMessages = useChatStore.getState().getConversationMessages(expectedConversationId);
    const afterMessageId = getMaxMessageId(beforeMessages as Message[]);

    const trimmed = String(instruction || '').trim();
    if (!trimmed) return null;

    const prompt = trimmed;
    if (activeTab.mode === 'rich') {
      flushActiveRichMarkdownNow();
    }
    // Persiste o buffer no disco antes do turno: tools de edição do editor
    // (text_edit/edit_file) leem o arquivo do disco, e o autosave debounced
    // pode deixar o disco atrás do que o usuário está vendo.
    await persistTabContentNow(activeTab.id);
    const latestActiveTab = useEditorStore.getState().documents[activeTab.id] ?? activeTab;
    const editorSurfaceTab = workspaceTab ?? {
      id: latestActiveTab.id,
      type: 'editor',
      title: latestActiveTab.title,
      state: {
        filePath: latestActiveTab.filePath ?? undefined,
        draftId: latestActiveTab.draftId ?? undefined,
      },
    };
    const surfaceContext = buildEditorInlineChatSurfaceContext(latestActiveTab, inlineChatSelection);

    clearPendingInlineChatEditorRestore();
    const runId = (inlineChatRunIdRef.current += 1);
    useWorkspaceChatModalStore.getState().setAdapterError(null);

    const isToolCallingEnabledForProfileSlug = async (slug: string): Promise<boolean> => {
      const s = String(slug || '').trim();
      if (!s) return true;
      try {
        const prof = await GetProfile(s);
        const disabled = !!(prof as { chat?: { disable_tools?: boolean } })?.chat?.disable_tools;
        return !disabled;
      } catch {
        // Best-effort: se não conseguimos ler o perfil, assume tools on.
        return true;
      }
    };

    const applyInlinePatchNow = (selection: InlineChatSelection, patch: EditorPatch) => {
      const replacement = normalizeReplacementForEditor(String(patch?.replacement || ''), patch?.format, selection?.selectedText);
      const { documents: currentDocs } = useEditorStore.getState();
      const tab = currentDocs[selection.tabId] || null;
      if (!tab) {
        addToast(t('editor.chatModal.editorTabNotFound'), 'error');
        setIsAsking(false);
        focusEditorSoon();
        return;
      }

      if (selection.mode === 'markdown') {
        const s = selection;

        if (currentDocumentId !== s.tabId) {
          addToast(t('editor.chatModal.openOriginalTabToApply'), 'info');
          setIsAsking(false);
          focusEditorSoon();
          return;
        }

        const model = editorRef.current?.getModel?.();
        const current = model?.getValue?.() ?? String(tab.markdown ?? '');

        const applied = applyTextReplacementByOffset({
          current,
          startOffset: s.startOffset,
          endOffset: s.endOffset,
          expectedSelectedText: s.selectedText,
          replacement,
        });

        // Se o conteúdo mudou desde o snapshot, evita aplicar offsets errados.
        if (!applied.ok) {
          addToast(t('editor.chatModal.selectionChangedRetry'), 'error');
          setIsAsking(false);
          focusEditorSoon();
          return;
        }

        const nextMarkdown = applied.nextText;
        queueMarkdownEditorRestore({
          tabId: s.tabId,
          startOffset: s.startOffset,
          endOffset: s.startOffset + replacement.length,
          sourceMarkdown: current,
          expectedMarkdown: nextMarkdown,
        });
        setDocMarkdown(s.tabId, nextMarkdown);
        updateLatestMarkdownForTab(s.tabId, nextMarkdown);
        schedulePersistForTab(s.tabId);
        addToast(t('editor.chatModal.patchApplied'), 'success');
      } else {
        const s = selection;
        if (currentDocumentId !== s.tabId) {
          addToast(t('editor.chatModal.openOriginalTabToApply'), 'info');
          setIsAsking(false);
          focusEditorSoon();
          return;
        }
        const rich = richEditorRef.current;
        if (!rich) {
          addToast(t('editor.chatModal.richEditorNotReady'), 'error');
          setIsAsking(false);
          focusEditorSoon();
          return;
        }

        // Evita aplicar em um range errado caso a seleção tenha mudado enquanto o chat estava aberto.
        try {
          const currentSel = rich.state?.selection;
          const expectedEmpty = !!s.selectionIsEmpty;
          const expectedFrom = Number(s.from);
          const expectedTo = Number(s.to);
          const expectedSelectedText = String(s.selectedText || '');

          const validation = validateRichTextSelectionSnapshot({
            currentSelection: currentSel
              ? { from: Number(currentSel.from), to: Number(currentSel.to), empty: !!currentSel.empty }
              : null,
            expectedFrom,
            expectedTo,
            expectedEmpty,
            expectedSelectedText,
            getCurrentSelectedText: expectedEmpty
              ? undefined
              : () => String(rich.state?.doc?.textBetween?.(currentSel!.from, currentSel!.to, '\n') ?? ''),
          });

          if (!validation.ok) {
            if (validation.reason === 'no_selection') {
              addToast(t('editor.chatModal.richSelectionReadFailed'), 'error');
            } else if (validation.reason === 'selected_text_mismatch') {
              addToast(t('editor.chatModal.selectionChangedRetry'), 'error');
            } else if (validation.reason === 'cannot_read_selected_text') {
              addToast(t('editor.chatModal.richSelectionValidateFailed'), 'error');
            } else {
              addToast(t('editor.chatModal.selectionSnapshotChanged'), 'error');
            }
            setIsAsking(false);
            focusEditorSoon();
            return;
          }
        } catch {
          addToast(t('editor.chatModal.richSelectionValidateFailed'), 'error');
          setIsAsking(false);
          focusEditorSoon();
          return;
        }

        const isMarkdown = patch?.format === 'markdown';
        const contentToInsert = !isMarkdown ? replacement : markdownToHtml(replacement);
        applyRichTextInsert({ rich: rich as unknown as RichTextEditorLike, from: s.from, to: s.to, contentToInsert });
        queueRichEditorRestore({
          tabId: s.tabId,
          from: s.from,
          to: s.from,
        });
        addToast(t('editor.chatModal.patchApplied'), 'success');
        flushActiveRichMarkdownNow();
      }

      useWorkspaceChatModalStore.getState().setAdapterError(null);
      useWorkspaceChatModalStore.getState().close();
      setIsAsking(false);
      focusEditorSoon();
    };

    const confirmInlinePatch = async (selection: InlineChatSelection, patch: EditorPatch) => {
      const before =
        selection.mode === 'rich'
          ? String(selection.selectedMarkdown || selection.selectedText || '')
          : String(selection.selectedText || '');
      const after = normalizeReplacementForEditor(String(patch?.replacement || ''), patch?.format, selection?.selectedText);
      const notes = String(patch?.notes || '').trim();

      const normalizedPatch = { ...patch, replacement: after };

      const resp = await requestQuestionnaire({
        id: `ui-editor-inline-patch-confirm-${Date.now()}`,
        title: t('editor.questionnaire.patchConfirmTitle'),
        description: notes || t('editor.questionnaire.patchConfirmDesc'),
        submitLabel: t('editor.questionnaire.patchConfirmSubmit'),
        cancelLabel: t('editor.questionnaire.patchConfirmCancel'),
        allowCancel: true,
        questions: [
          {
            id: 'before',
            type: 'readonly_code',
            prompt: t('editor.questionnaire.patchBefore'),
            content: before,
          },
          {
            id: 'after',
            type: 'readonly_code',
            prompt: t('editor.questionnaire.patchAfter'),
            content: after,
          },
        ],
      });

      if (!resp.cancelled) {
        applyInlinePatchNow(selection, normalizedPatch);
        return;
      }

      addToast(t('editor.chatModal.patchRejected'), 'info');
      setIsAsking(false);
      // Mantém o chat modal aberto para você criticar/explicar detalhes.
      // Apenas devolve o foco para o input do chat.
      useWorkspaceChatModalStore.getState().bumpFocus();
    };

    try {
      setIsAsking(true);

      // Regra importante:
      // - tools ON  => edit_file com confirmação contextual (Go-side); fecha só se o documento mudou
      // - tools OFF => body-only (extrai ```editor_patch``` do texto e confirma aqui)
      const toolCallingEnabled = await isToolCallingEnabledForProfileSlug(effectiveProfileSlug);

      // Drafts sem filePath não conseguem usar edit_file; nesse caso, cai para o
      // mesmo fluxo principal com fallback body-only e aplicação local do patch.
      const toolTurnTab = useEditorStore.getState().documents[inlineChatSelection.tabId] ?? latestActiveTab;
      const toolTurnFilePath = String(toolTurnTab.filePath || latestActiveTab.filePath || activeTab?.filePath || '');
      const canUseToolCalling = toolCallingEnabled && !!toolTurnFilePath;
      const filePathBeforeToolTurn = canUseToolCalling ? normalizePathKey(toolTurnFilePath) : '';
      const markdownBeforeToolTurn = canUseToolCalling ? String(toolTurnTab.markdown ?? latestActiveTab.markdown ?? activeTab?.markdown ?? '') : '';
      let sawEditorApplyTool = false;
      let sawEditorApplyToolSuccess = false;
      let sawAssistedFileChange = false;
      let toolTurnDone = false;
      let unsubscribeEditorApplyToolStart: (() => void) | null = null;
      let unsubscribeEditorApplyToolEnd: (() => void) | null = null;
      let unsubscribeAssistedFileChange: (() => void) | null = null;
      const stopTrackingAssistedFileChange = () => {
        if (unsubscribeEditorApplyToolStart) {
          try {
            unsubscribeEditorApplyToolStart();
          } catch {
            // best-effort cleanup
          }
          unsubscribeEditorApplyToolStart = null;
        }
        if (unsubscribeEditorApplyToolEnd) {
          try {
            unsubscribeEditorApplyToolEnd();
          } catch {
            // best-effort cleanup
          }
          unsubscribeEditorApplyToolEnd = null;
        }
        if (!unsubscribeAssistedFileChange) return;
        try {
          unsubscribeAssistedFileChange();
        } catch {
          // best-effort cleanup
        }
        unsubscribeAssistedFileChange = null;
        inlineChatToolCloseCleanupsRef.current.delete(stopTrackingAssistedFileChange);
      };
      const didToolTurnChangeEditorMarkdown = () => {
        const currentTab = useEditorStore.getState().documents[inlineChatSelection.tabId] ?? null;
        return String(currentTab?.markdown ?? '') !== markdownBeforeToolTurn;
      };
      const closeModalAfterAppliedToolEdit = async (syncBeforeClose = true) => {
        if (runId !== inlineChatRunIdRef.current) {
          stopTrackingAssistedFileChange();
          return;
        }
        if (!useWorkspaceChatModalStore.getState().isOpen) {
          stopTrackingAssistedFileChange();
          return;
        }
        stopTrackingAssistedFileChange();
        if (syncBeforeClose) {
          await syncAssistedChangeForTab(inlineChatSelection.tabId);
        }
        if (!didToolTurnChangeEditorMarkdown()) {
          useWorkspaceChatModalStore.getState().bumpFocus();
          setIsAsking(false);
          return;
        }
        const markdownAfterToolTurn = String(useEditorStore.getState().documents[inlineChatSelection.tabId]?.markdown ?? '');
        queueEditorRestoreForInlineSelection({
          selection: inlineChatSelection,
          markdownBefore: markdownBeforeToolTurn,
          markdownAfter: markdownAfterToolTurn,
          expectedMarkdown: inlineChatSelection.mode === 'markdown' ? markdownAfterToolTurn : undefined,
        });
        useWorkspaceChatModalStore.getState().setAdapterError(null);
        useWorkspaceChatModalStore.getState().close();
        setIsAsking(false);
        focusEditorSoon();
      };
      const surfaceParams = buildChatSurfaceParams(editorSurfaceTab, {
        profileSlug: effectiveProfileSlug,
        context: surfaceContext,
      });

      const donePromise = waitForChatDone(expectedConversationId);
      if (filePathBeforeToolTurn) {
        unsubscribeEditorApplyToolStart = EventsOn('chat:tool_start', (data: { conversationId?: string; name?: string; origin?: string }) => {
          if (String(data?.conversationId || '') !== expectedConversationId) return;
          if (!isAppToolEvent(data?.origin)) return;
          if (EDITOR_APPLY_TOOL_NAMES.has(String(data?.name || ''))) {
            sawEditorApplyTool = true;
          }
        });
        unsubscribeEditorApplyToolEnd = EventsOn('chat:tool_end', (data: { conversationId?: string; name?: string; status?: string; origin?: string }) => {
          if (String(data?.conversationId || '') !== expectedConversationId) return;
          if (!isAppToolEvent(data?.origin)) return;
          if (!EDITOR_APPLY_TOOL_NAMES.has(String(data?.name || ''))) return;
          if (String(data?.status || '') !== 'error') {
            sawEditorApplyToolSuccess = true;
          }
        });
        unsubscribeAssistedFileChange = EventsOn('editor:fileChanged', (data: EditorFileChangedEvent) => {
          const changedPath = normalizePathKey(String(data?.path || data?.filePath || ''));
          const assisted = data?.assisted === true || String(data?.origin || '') === 'assistant_tool';
          if (sawEditorApplyTool && assisted && changedPath === filePathBeforeToolTurn) {
            sawAssistedFileChange = true;
            if (toolTurnDone) {
              void closeModalAfterAppliedToolEdit();
            }
          }
        });
        inlineChatToolCloseCleanupsRef.current.add(stopTrackingAssistedFileChange);
      }
      return {
        content: prompt,
        mediaFiles,
        paramsOverride: surfaceParams,
        afterSend: async () => {
          try {
            const completedConversationId = await donePromise;

            if (runId !== inlineChatRunIdRef.current) {
              stopTrackingAssistedFileChange();
              return;
            }

            if (canUseToolCalling) {
              toolTurnDone = true;
              if (!sawEditorApplyTool) {
                stopTrackingAssistedFileChange();
                useWorkspaceChatModalStore.getState().bumpFocus();
                setIsAsking(false);
                return;
              }
              if (!sawEditorApplyToolSuccess) {
                stopTrackingAssistedFileChange();
                useWorkspaceChatModalStore.getState().bumpFocus();
                setIsAsking(false);
                return;
              }
              if (sawAssistedFileChange) {
                await closeModalAfterAppliedToolEdit();
              } else {
                const appliedBySync = await syncAssistedChangeForTab(inlineChatSelection.tabId);
                if (appliedBySync) {
                  await closeModalAfterAppliedToolEdit(false);
                } else {
                  stopTrackingAssistedFileChange();
                  useWorkspaceChatModalStore.getState().bumpFocus();
                  setIsAsking(false);
                }
              }
              return;
            }

            // Fallback (sem tool calling): extrai patch do corpo da resposta e confirma.
            const extracted = await waitForEditorPatch({
              conversationId: completedConversationId,
              afterMessageId,
              timeoutMs: 8000,
            });
            if (!extracted.ok) {
              const errText = String(extracted.error || '').trim();
              if (/nenhum patch encontrado|não contém patch|patch vazio|json inválido|patch inválido|muito grande/i.test(errText)) {
                addToast(t('editor.chatModal.patchNotApplicable'), 'error');
              }
              useWorkspaceChatModalStore.getState().setAdapterError(errText || t('editor.chatModal.patchExtractDefault'));
              setIsAsking(false);
              return;
            }

            await confirmInlinePatch(inlineChatSelection, extracted.patch as EditorPatch);
          } catch (e: unknown) {
            stopTrackingAssistedFileChange();
            logger.error('[useEditorInlineChat] inline chat error:', e);
            useWorkspaceChatModalStore.getState().setAdapterError(getErrorMessage(e) || t('editor.chatModal.requestChangeError'));
            setIsAsking(false);
          }
        },
        onSendError: (e: unknown) => {
          stopTrackingAssistedFileChange();
          logger.error('[useEditorInlineChat] inline chat error:', e);
          useWorkspaceChatModalStore.getState().setAdapterError(getErrorMessage(e) || t('editor.chatModal.requestChangeError'));
          setIsAsking(false);
        },
      };
    } catch (e: unknown) {
      logger.error('[useEditorInlineChat] inline chat error:', e);
      useWorkspaceChatModalStore.getState().setAdapterError(getErrorMessage(e) || t('editor.chatModal.requestChangeError'));
      setIsAsking(false);
      return null;
    }
  };

  const sendEditorChatModalRef = useRef(sendEditorChatModalMessage);
  sendEditorChatModalRef.current = sendEditorChatModalMessage;

  const editorChatModalAdapter = useMemo((): WorkspaceChatModalAdapter | null => {
    if (!workspaceTab || workspaceTab.type !== 'editor') return null;

    return {
      prepare: async (): Promise<WorkspaceChatModalPrepareResult> => {
        if (!activeTab) return { ok: false, message: t('workspace.chatModal.panelLoading') };
        if (activeTab.mode === 'view') {
          addToast(t('editor.chatModal.prepareNeedCodeOrRich'), 'info');
          return { ok: false };
        }
        if (isAsking) {
          return { ok: false, message: t('workspace.chatModal.panelLoading') };
        }

        const preparedSnapshot = getPreparedSelectionSnapshot();
        const selectionRaw =
          preparedSnapshot?.mode === activeTab.mode
            ? preparedSnapshot.snapshot
            : null;

        if (!selectionRaw) {
          addToast(t('editor.chatModal.prepareSelectionFailed'), 'error');
          return { ok: false };
        }

        if (selectionRaw.selectedText.length > 20000) {
          addToast(t('editor.chatModal.prepareSelectionTooLarge', { max: 20000 }), 'error');
          return { ok: false };
        }

        const revealSelectionDeck = parseRevealMarkdown(activeTab.markdown);
        const richRevealSlide = activeTab.mode === 'rich' && revealSelectionDeck.detection.kind === 'reveal'
          ? revealSelectionDeck.slides[currentRevealSlideIndexRef.current] ?? revealSelectionDeck.slides[0] ?? null
          : null;
        const richRevealSlideCount = richRevealSlide ? revealSelectionDeck.slides.length : undefined;

        const selection: InlineChatSelection =
          activeTab.mode === 'markdown'
            ? (() => {
                const md = selectionRaw as MarkdownSelectionSnapshot;
                const snapshot = editorRef.current?.getModel?.()?.getValue?.() ?? activeTab.markdown;
                const markdownRevealSlide = findRevealSlideForMarkdownOffsets(
                  snapshot,
                  md.startOffset,
                  md.endOffset,
                  md.cursorOffset,
                );
                const markdownRevealSlideCount = markdownRevealSlide
                  ? parseRevealMarkdown(snapshot).slides.length
                  : undefined;
                return {
                  mode: 'markdown',
                  tabId: activeTab.id,
                  selectedText: md.selectedText,
                  selectionIsEmpty: !!md.selectionIsEmpty,
                  cursorContext: md.cursorContext,
                  displayText: md.displayText,
                  startOffset: md.startOffset,
                  endOffset: md.endOffset,
                  startLine: md.startLine,
                  startColumn: md.startColumn,
                  endLine: md.endLine,
                  endColumn: md.endColumn,
                  cursorLine: md.cursorLine,
                  cursorColumn: md.cursorColumn,
                  cursorOffset: md.cursorOffset,
                  snapshot,
                  revealSlideIndex: markdownRevealSlide?.index,
                  revealSlideLabel: markdownRevealSlide?.label,
                  revealSlideMarkdown: markdownRevealSlide?.markdown,
                  revealSlideCount: markdownRevealSlideCount,
                };
              })()
            : (() => {
                const rich = selectionRaw as RichSelectionSnapshot;
                return {
                  mode: 'rich',
                  tabId: activeTab.id,
                  selectedText: rich.selectedText,
                  selectedMarkdown: rich.selectedMarkdown,
                  selectionIsEmpty: !!rich.selectionIsEmpty,
                  cursorContext: rich.cursorContext,
                  displayText: rich.displayText,
                  displayMarkdown: rich.displayMarkdown,
                  textBeforeSelection: rich.textBeforeSelection,
                  textAfterSelection: rich.textAfterSelection,
                  from: rich.from,
                  to: rich.to,
                  snapshot: rich.snapshot ?? activeTab.markdown,
                  revealSlideIndex: richRevealSlide?.index,
                  revealSlideLabel: richRevealSlide?.label,
                  revealSlideMarkdown: richRevealSlide?.markdown,
                  revealSlideCount: richRevealSlideCount,
                };
              })();

        const contextDisplay =
          selection.displayText ||
          (selection.mode === 'rich' ? selection.selectedMarkdown : '') ||
          selection.selectedText ||
          '';

        useWorkspaceChatModalStore.getState().setAdapterError(null);
        return { ok: true, contextDisplay, meta: selection };
      },
      send: (instruction, media, meta, session) =>
        sendEditorChatModalRef.current(instruction, media, meta as InlineChatSelection, session),
    };
  }, [workspaceTab, activeTab, isAsking, addToast, editorReadyNonce, t]);

  useRegisterWorkspaceChatAdapter(workspaceTab?.id, editorChatModalAdapter);

  return { isAsking, chatModalOpen };
}
