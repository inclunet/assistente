import { logger } from '../utils/logger';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { MessageOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { EventsOn } from '@wailsjs/runtime/runtime';
import { Menu, type MenuItem } from '../components/menu';
import { useAnchoredContextMenu } from '../hooks/useAnchoredContextMenu';
import { MermaidEditorModal } from '../components/editor/MermaidEditorModal';
import type { RichTextEditorHandle } from '../components/editor/RichTextEditor';
import { EditorToolbar } from '../components/editor/EditorToolbar';
import { EditorContentArea } from '../components/editor/EditorContentArea';
import { useRichEditorFlushEvents } from './useRichEditorFlushEvents';
import { useRegisterWorkspaceChatAdapter } from '../hooks/useRegisterWorkspaceChatAdapter';
import { useWorkspaceChatModalStore } from '../store/workspaceChatModalStore';
import type {
  WorkspaceChatModalAdapter,
  WorkspaceChatModalPrepareResult,
  WorkspaceChatSendPlan,
  WorkspaceChatModalSession,
} from '../store/workspaceChatModalStore';
import { useEditorStore, DEFAULT_MD, type EditorMode, type EditorInsertRequest } from '../store/editorStore';
import { useWorkspaceStore, type WorkspaceTab } from '../store/workspaceStore';
import { useDebouncedValue } from '../hooks/useDebouncedValue';
import { useQuestionnaireUIStore } from '../store/questionnaireUIStore';
import { useUIStore } from '../store/uiStore';
import { useChatStore } from '../store/chatStore';
import { applyTextReplacementByOffset } from '../lib/editorPatchApply';
import { normalizeEditorInsertContent } from '../lib/editorInsertNormalize';
import { applyRichTextInsert, applyRichTextInsertAtEnd, type RichTextEditorLike } from '../lib/richTextPatchApply';
import { validateRichTextSelectionSnapshot } from '../lib/richTextSelectionValidation';
import { markdownToHtml } from '../lib/markdownToHtml';
import { computeMonacoInsertText } from '../lib/monacoInsertHeuristics';
import { buildChatSurfaceParams, createSurfaceSnapshotVersion, type SurfaceContext } from '../lib/chatSurface';
import { findMermaidFenceByIndex, removeMermaidFence, replaceMermaidFenceCode } from '../lib/mermaidFence';
import { parseRevealMarkdown, type RevealSlide } from '../lib/revealMarkdown';
import { getErrorMessage, getMaybeContent } from '../lib/editorContent';
import { composePreviewText, hasConflictMarkers } from '../lib/editorMergeUtils';
import { basenameFromPath, normalizePathKey } from '../utils/path';
import { useEditorInlineChatPatch } from '../hooks/useEditorInlineChatPatch';
import { isModalOpen } from '../components/ui/Modal';
import type { MediaFile } from '../services/mediaService';
import type { Message } from '../store/chatStore';
import {
  buildFileMenuItemsForContextMenu,
  buildFormatMenuItemsForContextMenu,
  buildInsertMenuItemsForContextMenu,
  buildModeMenuItemsForContextMenu,
} from './editorMenus';
import {
  GetProfile,
  EditorDeleteDraft,
  EditorGetDraftPath,
  EditorOpenFile,
  EditorReadDraft,
  EditorSaveFileDialog,
  EditorWriteFile,
} from '@wailsjs/go/app/App';
import { useEditorMerge } from './useEditorMerge';
import { useEditorDocument } from './useEditorDocument';
import { useEditorPersistence } from './useEditorPersistence';
import type {
  EditorPatch,
  EditorFileChangedEvent,
  InlineChatSelection,
  MarkdownSelectionSnapshot,
  MonacoCodeEditor,
  MonacoNamespace,
  RichMermaidSession,
  RichSelectionSnapshot,
  TipTapEditor,
} from './editorTypes';
import './EditorPage.css';

interface EditorPageProps {
  documentId?: string;
  workspaceTab?: WorkspaceTab;
  isPanelActive?: boolean;
}

type EditorSelectionSnapshot =
  | { mode: 'markdown'; snapshot: MarkdownSelectionSnapshot }
  | { mode: 'rich'; snapshot: RichSelectionSnapshot };

const EDITOR_SELECTION_CACHE_STALE_AFTER_MS = 120000;
const EDITOR_APPLY_TOOL_NAMES = new Set(['edit_file', 'text_edit', 'write_file']);

export default function EditorPage({ documentId, workspaceTab, isPanelActive = true }: EditorPageProps = {}) {
  const { t } = useTranslation();
  const addToast = useUIStore((s) => s.addToast);
  const requestQuestionnaire = useQuestionnaireUIStore((s) => s.request);

  const { waitForChatDone, waitForEditorPatch, getMaxMessageId } = useEditorInlineChatPatch();

  const documents = useEditorStore((s) => s.documents);
  const createDocument = useEditorStore((s) => s.createDocument);
  const setDocMarkdown = useEditorStore((s) => s.setDocMarkdown);
  const renameDocument = useEditorStore((s) => s.renameDocument);
  const setDocFilePath = useEditorStore((s) => s.setDocFilePath);
  const setDocDraftId = useEditorStore((s) => s.setDocDraftId);
  const setDocDirty = useEditorStore((s) => s.setDocDirty);
  const addWorkspaceTab = useWorkspaceStore((s) => s.addTab);
  const setActiveWsTab = useWorkspaceStore((s) => s.setActiveTab);
  const wsTabs = useWorkspaceStore((s) => s.workspace?.tabs);
  const wsProfile = useWorkspaceStore((s) => s.workspace?.profile);

  const isWsInitialized = useWorkspaceStore((s) => s.isInitialized);

  const tabProfileSlug = workspaceTab?.profileOverride?.slug as string | undefined;
  const effectiveProfileSlug = tabProfileSlug || wsProfile || 'editor-texto';

  const currentDocumentId = documentId ?? workspaceTab?.id ?? null;
  const activeTab = currentDocumentId ? documents[currentDocumentId] ?? null : null;

  const pageRootRef = useRef<HTMLDivElement>(null);
  const insertMenuButtonRef = useRef<HTMLButtonElement | null>(null);
  const modeMenuButtonRef = useRef<HTMLButtonElement | null>(null);
  const revealSlidePickerButtonRef = useRef<HTMLButtonElement | null>(null);
  const editorRef = useRef<MonacoCodeEditor | null>(null);
  const monacoRef = useRef<MonacoNamespace | null>(null);
  const richEditorRef = useRef<TipTapEditor | null>(null);
  const richEditorHandleRef = useRef<RichTextEditorHandle | null>(null);
  const currentRevealSlideIndexRef = useRef(0);
  const lastExplicitSelectionRef = useRef<{
    tabId: string;
    capturedAt: number;
    selection: EditorSelectionSnapshot;
  } | null>(null);

  const [isAsking, setIsAsking] = useState(false);
  const [currentRevealSlideIndex, setCurrentRevealSlideIndex] = useState(0);
  const [revealSlideNavigationRequest, setRevealSlideNavigationRequest] = useState<{ index: number; nonce: number } | null>(null);
  const [revealFullscreenRequestNonce, setRevealFullscreenRequestNonce] = useState(0);

  const [activeMermaidIndex, setActiveMermaidIndex] = useState<number | null>(null);
  const [mermaidInitialCode, setMermaidInitialCode] = useState('');
  const [mermaidInsertText, setMermaidInsertText] = useState('');
  const [richMermaidSession, setRichMermaidSession] = useState<RichMermaidSession | null>(null);

  const [editorReadyNonce, setEditorReadyNonce] = useState(0);
  const [revealAppendNonce, setRevealAppendNonce] = useState(0);
  const [pendingInsert, setPendingInsert] = useState<EditorInsertRequest | null>(null);

  const inlineChatRunIdRef = useRef(0);
  const inlineChatToolCloseCleanupsRef = useRef<Set<() => void>>(new Set());
  const chatModalOpen = useWorkspaceChatModalStore((s) => s.isOpen);

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
  const prevChatModalOpenRef = useRef(false);

  // Foco previsível após fechar o modal Mermaid.
  const prevMermaidOpenRef = useRef(false);
  useEffect(() => {
    const isOpen = activeMermaidIndex !== null;
    if (prevMermaidOpenRef.current && !isOpen) {
      focusEditorSoon();
    }
    prevMermaidOpenRef.current = isOpen;
  }, [activeMermaidIndex]);

  // ----- Hooks de lógica extraída -----
  const merge = useEditorMerge();
  const {
    mergeStateRevision,
    getMergeSession,
    getCachedMarkdownForTab,
    updateLatestMarkdownForTab,
    markSelfWrite,
    isExternalConflictLocked,
    setExternalConflictLocked,
    setDiskBaselineForTab,
    refreshDiskInfoForTab,
    cleanupMergeSessionForTab,
    promptResolveExternalChangeForTab,
  } = merge;

  const allDocs = useMemo(() => Object.values(documents), [documents]);

  const { sessionLoaded, fileModeByPathRef, saveEditorState } = useEditorDocument({
    merge,
    isWsInitialized,
    currentDocumentId,
    activeTab,
    allDocs,
    documents,
  });

  const flushActiveRichMarkdownNow = useCallback(() => {
    try {
      const st = useEditorStore.getState();
      const tab = currentDocumentId ? st.documents[currentDocumentId] ?? null : null;
      if (!tab || tab.mode !== 'rich') return;
      richEditorHandleRef.current?.flushMarkdown?.();
    } catch {
      // best-effort
    }
  }, [currentDocumentId]);

  const { schedulePersistForTab } = useEditorPersistence({
    merge,
    sessionLoaded,
    currentDocumentId,
    allDocs,
    flushActiveRichMarkdownNow,
    saveEditorState,
  });

  useRichEditorFlushEvents({ flushNow: flushActiveRichMarkdownNow });

  const debouncedMarkdownForPreview = useDebouncedValue(activeTab?.markdown || '', 120);
  const revealToolbarDeck = useMemo(
    () => parseRevealMarkdown(activeTab?.markdown || ''),
    [activeTab?.markdown]
  );
  const isRevealToolbarDocument = revealToolbarDeck.detection.kind === 'reveal' && revealToolbarDeck.slides.length > 0;

  // Ao entrar no Editor (e ao trocar de aba/modo), foca automaticamente a área de texto.
  // Não rouba foco de modais nem de campos de digitação.
  const didInitialEditorAutofocusRef = useRef(false);
  useEffect(() => {
    if (!sessionLoaded) return;
    if (!activeTab) return;
    if (chatModalOpen) return;
    if (isModalOpen()) return;

    const el = document.activeElement as HTMLElement | null;
    const tag = el?.tagName || '';
    const isTypingTarget =
      !!el &&
      (tag === 'INPUT' ||
        tag === 'TEXTAREA' ||
        el.isContentEditable ||
        el.getAttribute?.('role') === 'textbox');

    // Primeira entrada: sempre tenta focar o editor.
    if (!didInitialEditorAutofocusRef.current) {
      didInitialEditorAutofocusRef.current = true;
      focusEditorSoon();
      return;
    }

    // Mudança de aba/modo: só foca automaticamente se não houver um alvo de foco claro.
    // (Evita “puxar” o foco de tabs/toolbar, o que quebra navegação por teclado, ex: F6.)
    const isEditorZone =
      !!el &&
      (!!el.closest?.('.rich-text-editor__content') || !!el.closest?.('.monaco-editor'));
    const isDocumentBody = !el || el === document.body;

    if (!isTypingTarget && (isDocumentBody || isEditorZone)) {
      focusEditorSoon();
    }
  }, [sessionLoaded, activeTab?.id, activeTab?.mode, chatModalOpen]);

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

  const findRevealSlideForMarkdownOffsets = (
    markdown: string,
    startOffset: number,
    endOffset: number,
    cursorOffset: number,
  ): RevealSlide | null => {
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
  };

  const getRichSelectionSnapshot = (): RichSelectionSnapshot | null => {
    const editor = richEditorRef.current;
    if (!editor) return null;

    const sel = editor.state?.selection;
    if (!sel) return null;

    const { from, to, empty } = sel;
    const selectedText = editor.state.doc.textBetween(from, to, '\n');

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

  function focusEditorSoon() {
    window.setTimeout(() => {
      try {
        if (!activeTab) return;
        if (activeTab.mode === 'markdown') {
          editorRef.current?.focus?.();
        } else if (activeTab.mode === 'rich') {
          richEditorRef.current?.commands?.focus?.();
          richEditorRef.current?.view?.focus?.();
        }
      } catch {
        // best-effort
      }
    }, 20);
  }

  useEffect(() => {
    if (prevChatModalOpenRef.current && !chatModalOpen) {
      inlineChatRunIdRef.current += 1;
      setIsAsking(false);
      focusEditorSoon();
    }
    prevChatModalOpenRef.current = chatModalOpen;
  }, [chatModalOpen, activeTab]);

  const applyInsertRequest = async (req: EditorInsertRequest): Promise<boolean> => {
    const r = req;
    const rawContent = String(r?.content ?? '');
    if (!rawContent) return true;

    const requestedDocumentId = String(r.targetDocumentId || '').trim();
    if (r.target === 'document' && !requestedDocumentId) {
      logger.error('[EditorPage] applyInsertRequest rejected: document target requires targetDocumentId');
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
    const liveRevealDeck = parseRevealMarkdown(latestActiveTab.markdown);
    const preparedMarkdownRevealDeck = inlineChatSelection.mode === 'markdown'
      ? parseRevealMarkdown(inlineChatSelection.snapshot)
      : null;
    const revealDeck = preparedMarkdownRevealDeck?.detection.kind === 'reveal'
      ? preparedMarkdownRevealDeck
      : liveRevealDeck;
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
    const currentRevealSlide = inlineChatSelection.mode === 'rich'
      ? getRichRevealSlideSnapshot()
      : revealDeck.detection.kind === 'reveal'
        ? findRevealSlideForMarkdownSelection()
        : null;
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
    const surfaceContext: SurfaceContext = {
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

    const normalizeReplacementForEditor = (raw: string, patchFormat: string | undefined, selectedText: string) => {
      const text = String(raw ?? '');
      const sel = String(selectedText ?? '');

      // Alguns modelos colocam o conteúdo dentro de um bloco ```markdown ... ```.
      // Para o editor, isso costuma ser ruído (a não ser que o usuário já tenha selecionado um bloco fence).
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
        setDocMarkdown(s.tabId, nextMarkdown);
        updateLatestMarkdownForTab(s.tabId, nextMarkdown);
        schedulePersistForTab(s.tabId);
        addToast(t('editor.chatModal.patchApplied'), 'success');

        requestAnimationFrame(() => {
          try {
            const editor = editorRef.current;
            const m = editor?.getModel?.();
            if (!editor || !m) return;
            if (currentDocumentId !== s.tabId) return;
            const startPos = m.getPositionAt(s.startOffset);
            const endPos = m.getPositionAt(s.startOffset + replacement.length);
            editor.setSelection({
              startLineNumber: startPos.lineNumber,
              startColumn: startPos.column,
              endLineNumber: endPos.lineNumber,
              endColumn: endPos.column,
            });
            editor.focus();
          } catch {
            // best-effort
          }
        });
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
        title: 'Aplicar alteração?',
        description: notes || 'Revise o antes/depois. Se estiver ok, confirme para aplicar no trecho selecionado.',
        submitLabel: 'Sim, aplicar',
        cancelLabel: 'Não, cancelar',
        allowCancel: true,
        questions: [
          {
            id: 'before',
            type: 'readonly_code',
            prompt: 'Antes',
            content: before,
          },
          {
            id: 'after',
            type: 'readonly_code',
            prompt: 'Depois',
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
      const toolTurnTab = useEditorStore.getState().documents[latestActiveTab.id] ?? latestActiveTab;
      const toolTurnFilePath = String(toolTurnTab.filePath || latestActiveTab.filePath || activeTab?.filePath || '');
      const canUseToolCalling = toolCallingEnabled && !!toolTurnFilePath;
      const filePathBeforeToolTurn = canUseToolCalling ? normalizePathKey(toolTurnFilePath) : '';
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
      const closeModalAfterAppliedToolEdit = () => {
        if (runId !== inlineChatRunIdRef.current) {
          stopTrackingAssistedFileChange();
          return;
        }
        if (!useWorkspaceChatModalStore.getState().isOpen) {
          stopTrackingAssistedFileChange();
          return;
        }
        stopTrackingAssistedFileChange();
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
        unsubscribeEditorApplyToolStart = EventsOn('chat:tool_start', (data: { conversationId?: string; name?: string }) => {
          if (String(data?.conversationId || '') !== expectedConversationId) return;
          if (EDITOR_APPLY_TOOL_NAMES.has(String(data?.name || ''))) {
            sawEditorApplyTool = true;
          }
        });
        unsubscribeEditorApplyToolEnd = EventsOn('chat:tool_end', (data: { conversationId?: string; name?: string; status?: string }) => {
          if (String(data?.conversationId || '') !== expectedConversationId) return;
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
              closeModalAfterAppliedToolEdit();
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
              if (!sawEditorApplyToolSuccess && !sawAssistedFileChange) {
                stopTrackingAssistedFileChange();
                useWorkspaceChatModalStore.getState().bumpFocus();
                setIsAsking(false);
                return;
              }
              if (sawAssistedFileChange) {
                closeModalAfterAppliedToolEdit();
              } else {
                useWorkspaceChatModalStore.getState().bumpFocus();
                setIsAsking(false);
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
            logger.error('[EditorPage] inline chat error:', e);
            useWorkspaceChatModalStore.getState().setAdapterError(getErrorMessage(e) || t('editor.chatModal.requestChangeError'));
            setIsAsking(false);
          }
        },
        onSendError: (e: unknown) => {
          stopTrackingAssistedFileChange();
          logger.error('[EditorPage] inline chat error:', e);
          useWorkspaceChatModalStore.getState().setAdapterError(getErrorMessage(e) || t('editor.chatModal.requestChangeError'));
          setIsAsking(false);
        },
      };
    } catch (e: unknown) {
      logger.error('[EditorPage] inline chat error:', e);
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

  const openFile = async () => {
    try {
      const res = await EditorOpenFile();
      const path = String(res?.path || '').trim();
      if (!path) return;

      const key = normalizePathKey(path);
      const content = String(res?.content || '');

      // Se o arquivo já está aberto em outra aba, apenas ativa essa aba.
      const existingDoc = Object.values(documents).find(
        (tab) => tab.filePath && normalizePathKey(String(tab.filePath)) === key,
      );
      if (existingDoc) {
        const wsTab = (wsTabs || []).find(
          (tab) => tab.type === 'editor' && tab.id === existingDoc.id,
        );
        if (wsTab) {
          await setActiveWsTab(wsTab.id);
          addToast(t('editor.toast.fileAlreadyOpen'), 'info');
          focusEditorSoon();
          return;
        }
      }

      const preferredMode: EditorMode =
        fileModeByPathRef.current[key] || (existingDoc?.mode === 'rich' ? 'rich' : 'markdown');
      const title = basenameFromPath(path);

      // Se a aba atual está "virgem" (sem arquivo, conteúdo padrão), reutiliza-a.
      const isPristine = activeTab && !activeTab.filePath && !activeTab.isDirty && activeTab.markdown === DEFAULT_MD;
      let id: string;

      if (isPristine) {
        id = activeTab.id;
        renameDocument(id, title);
        setDocMarkdown(id, content);
        useEditorStore.getState().setDocMode(id, preferredMode);
        // filePath+title são sincronizados pelo controller do painel de editor.
      } else {
        const tabId = await addWorkspaceTab('editor', title, { filePath: path });
        id = tabId;
        createDocument({ id: tabId, title, markdown: content, mode: preferredMode, filePath: path });
      }

      setDocFilePath(id, path);
      setDocDraftId(id, null);
      setDocDirty(id, false);

      updateLatestMarkdownForTab(id, content);
      setDiskBaselineForTab(id, content);
      const diskTab = {
        id,
        title,
        markdown: content,
        mode: preferredMode,
        filePath: path,
      };
      void refreshDiskInfoForTab(diskTab);

      fileModeByPathRef.current[key] = preferredMode === 'rich' ? 'rich' : 'markdown';

      EditorDeleteDraft(id).catch(() => null);
      addToast(t('editor.toast.fileOpened'), 'success');
      focusEditorSoon();
    } catch (e: unknown) {
      logger.error('[EditorPage] openFile error:', e);
      addToast(getErrorMessage(e) || t('editor.toast.openFailed'), 'error');
    }
  };

  const abortMerge = async () => {
    if (!activeTab?.filePath) return;

    const sess = getMergeSession(activeTab.id);
    if (!sess) return;

    let mineContent = '';
    try {
      const res = await EditorReadDraft(sess.mineDraftId);
      mineContent = getMaybeContent(res);
    } catch {
      mineContent = '';
    }

    const minePreviewText = composePreviewText(mineContent, t);

    const resp = await requestQuestionnaire({
      id: `ui-editor-abort-merge-${Date.now()}`,
      title: 'Abortar merge (estilo Git)?',
      description:
        'Isso vai descartar o texto com marcadores de conflito nesta aba e restaurar a sua versão original. O arquivo continuará com salvamento travado até você escolher como resolver a modificação externa.',
      submitLabel: 'Abortar merge',
      cancelLabel: 'Continuar editando',
      allowCancel: true,
      questions: [
        {
          id: 'path',
          type: 'readonly_code' as const,
          prompt: 'Arquivo',
          content: String(activeTab.filePath || ''),
        },
        {
          id: 'mine',
          type: 'readonly_code' as const,
          prompt: 'Sua versão original (preview)',
          content: minePreviewText || '(vazio)',
        },
      ],
    });

    if (resp.cancelled) return;

    // Mantém travado: evita autosave sobrescrever o arquivo real sem decisão explícita.
    setExternalConflictLocked(activeTab.id, true);

    setDocMarkdown(activeTab.id, mineContent);
    updateLatestMarkdownForTab(activeTab.id, mineContent);
    setDocDirty(activeTab.id, true);

    await cleanupMergeSessionForTab(activeTab.id);

    addToast(t('editor.toast.mergeAborted'), 'info');
    focusEditorSoon();
  };

  const saveFile = async () => {
    if (!activeTab) return;
    try {
      if (activeTab.mode === 'rich') flushActiveRichMarkdownNow();
      const content = getCachedMarkdownForTab(activeTab);
      updateLatestMarkdownForTab(activeTab.id, content);

      if (activeTab.filePath) {
        if (isExternalConflictLocked(activeTab.id)) {
          const mergeSession = getMergeSession(activeTab.id);
          if (mergeSession) {
            if (hasConflictMarkers(content)) {
              addToast(t('editor.toast.conflictMarkersRemain'), 'warning');
              return;
            }
            markSelfWrite(activeTab.filePath);
            await EditorWriteFile(activeTab.filePath, content);
            setDiskBaselineForTab(activeTab.id, content);
            setDocDirty(activeTab.id, false);
            void refreshDiskInfoForTab(activeTab);
            setExternalConflictLocked(activeTab.id, false);
            await cleanupMergeSessionForTab(activeTab.id);
            addToast(t('editor.toast.conflictResolvedSaved'), 'success');
            focusEditorSoon();
            return;
          }

          addToast(t('editor.toast.saveLockedExternal'), 'warning');
          void promptResolveExternalChangeForTab(activeTab.id, String(activeTab.filePath));
          return;
        }
        markSelfWrite(activeTab.filePath);
        await EditorWriteFile(activeTab.filePath, content);
        setDiskBaselineForTab(activeTab.id, content);
        setDocDirty(activeTab.id, false);
        void refreshDiskInfoForTab(activeTab);
        addToast(t('editor.toast.fileSaved'), 'success');
        focusEditorSoon();
        return;
      }

      // Ainda não tem destino: pedir path
      const suggested = (activeTab.title || 'documento') + '.md';
      const path = String(await EditorSaveFileDialog(suggested) || '').trim();
      if (!path) return;

      markSelfWrite(path);
      await EditorWriteFile(path, content);
      setDiskBaselineForTab(activeTab.id, content);
      const title = basenameFromPath(path);
      setDocFilePath(activeTab.id, path);
      renameDocument(activeTab.id, title);
      setDocDirty(activeTab.id, false);

      // filePath+title são sincronizados pelo controller do painel de editor.

      void refreshDiskInfoForTab({ ...activeTab, filePath: path });

      const draftId = activeTab.draftId || activeTab.id;
      setDocDraftId(activeTab.id, null);
      await EditorDeleteDraft(draftId);

      addToast(t('editor.toast.fileSaved'), 'success');
      focusEditorSoon();
    } catch (e: unknown) {
      logger.error('[EditorPage] saveFile error:', e);
      addToast(getErrorMessage(e) || t('editor.toast.saveFailed'), 'error');
    }
  };

  const saveFileAsCopy = async () => {
    if (!activeTab?.filePath) return;
    try {
      if (activeTab.mode === 'rich') flushActiveRichMarkdownNow();
      const suggested = basenameFromPath(activeTab.filePath);
      const path = String(await EditorSaveFileDialog(suggested) || '').trim();
      if (!path) return;
      const content = getCachedMarkdownForTab(activeTab);
      updateLatestMarkdownForTab(activeTab.id, content);
      markSelfWrite(path);
      await EditorWriteFile(path, content);
      addToast(t('editor.toast.copySaved'), 'success');
      focusEditorSoon();
    } catch (e: unknown) {
      logger.error('[EditorPage] saveAs error:', e);
      addToast(getErrorMessage(e) || t('editor.toast.saveAsFailed'), 'error');
    }
  };

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

  const fileMenuItems = useMemo(() => {
    // "Salvar" funciona em qualquer aba ativa: grava o arquivo quando há
    // filePath, pede destino quando é rascunho sem path, ou resolve o conflito
    // externo quando está locked (ver saveFile). Por isso fica habilitado
    // sempre que houver aba ativa — não só nos casos sem path/locked.
    const canSave = !!activeTab;
    const canSaveAs = !!activeTab?.filePath;
    const hasMergeSession = !!activeTab && !!getMergeSession(activeTab.id);

    const items = [
      { value: 'new', label: 'Novo', sublabel: 'Ctrl+N' },
      { value: 'open', label: 'Abrir', sublabel: 'Ctrl+O' },
      { value: 'save', label: 'Salvar', sublabel: 'Ctrl+S', disabled: !canSave },
      ...(hasMergeSession
        ? [{ value: 'abort-merge', label: 'Abortar merge (Git)', sublabel: 'Descarta marcadores de conflito' }]
        : []),
      { value: 'saveas', label: 'Salvar como...', sublabel: 'Ctrl+Shift+S', disabled: !canSaveAs },
    ];

    return items;
    // `mergeStateRevision` força recomputo quando a merge session muda (lida
    // via ref em `getMergeSession` para o item "Abortar merge"), já que esse
    // estado não deriva de `activeTab`.
  }, [activeTab, mergeStateRevision]);

  const onFileMenuSelect = useCallback(
    async (value: string) => {
      const v = String(value || '').trim();
      if (!v) return;

      switch (v) {
        case 'new': {
          const draftId = (typeof crypto !== 'undefined' && crypto.randomUUID) ? crypto.randomUUID() : `editor-${Date.now()}`;
          const draftPath = String(await EditorGetDraftPath(draftId) ?? '');
          const tabId = await addWorkspaceTab('editor', 'Novo documento', { filePath: draftPath, draftId });
          createDocument({ id: tabId, draftId, filePath: draftPath });
          focusEditorSoon();
          return;
        }
        case 'open':
          await openFile();
          return;
        case 'save':
          await saveFile();
          return;
        case 'abort-merge':
          await abortMerge();
          return;
        case 'saveas':
          await saveFileAsCopy();
          return;
        default:
          return;
      }
    },
    [createDocument, addWorkspaceTab, openFile, saveFile, abortMerge, saveFileAsCopy, activeTab]
  );

  const appendMarkdownToDocument = useCallback((content: string) => {
    if (!activeTab) return;
    if (activeTab.mode === 'rich') {
      flushActiveRichMarkdownNow();
    }

    const latestTab = useEditorStore.getState().documents[activeTab.id] ?? activeTab;
    const current = String(latestTab.markdown ?? '');
    const trimmedContent = String(content || '').trim();
    const currentWithoutTrailingNewlines = current.replace(/[\r\n]+$/, '');
    const hasTrailingSlideSeparator = /(^|\r?\n)\s*-{3,4}\s*$/.test(currentWithoutTrailingNewlines);
    const separator = current.trim()
      ? hasTrailingSlideSeparator
        ? '\n\n'
        : '\n\n---\n\n'
      : '';
    const nextMarkdown = `${currentWithoutTrailingNewlines}${separator}${trimmedContent}\n`;
    setDocMarkdown(activeTab.id, nextMarkdown);
    updateLatestMarkdownForTab(activeTab.id, nextMarkdown);
    schedulePersistForTab(activeTab.id);
    setRevealAppendNonce((n) => n + 1);
  }, [activeTab, flushActiveRichMarkdownNow, setDocMarkdown, updateLatestMarkdownForTab, schedulePersistForTab]);

  const requestRevealSlideNavigation = useCallback((index: number) => {
    setRevealSlideNavigationRequest((prev) => ({
      index,
      nonce: (prev?.nonce ?? 0) + 1,
    }));
  }, []);

  const createRevealSlideFromToolbar = useCallback(() => {
    appendMarkdownToDocument(`<!-- .slide: class="content-slide" -->

## ${t('editor.presentation.newSlideTitle')}`);
  }, [appendMarkdownToDocument, t]);

  const requestRevealFullscreen = useCallback(() => {
    setRevealFullscreenRequestNonce((nonce) => nonce + 1);
  }, []);

  const showRevealSlidePicker = !!activeTab && activeTab.mode === 'rich' && isRevealToolbarDocument;
  const getRevealToolbarSlideLabel = useCallback(
    (index: number) => revealToolbarDeck.slides[index]?.label || t('editor.presentation.slideOption', { index: index + 1 }),
    [revealToolbarDeck.slides, t]
  );
  const revealSlideMenuItemsForShortcut = useMemo((): MenuItem[] => {
    if (!showRevealSlidePicker) return [];
    return [
      ...revealToolbarDeck.slides.map((_, index) => ({
        id: `reveal-slide-${index}`,
        label: getRevealToolbarSlideLabel(index),
        checked: index === Math.min(currentRevealSlideIndex, Math.max(0, revealToolbarDeck.slides.length - 1)),
        action: () => requestRevealSlideNavigation(index),
      })),
      { id: 'reveal-slide-separator', separator: true },
      {
        id: 'reveal-slide-new',
        label: t('editor.presentation.newSlide'),
        action: createRevealSlideFromToolbar,
      },
    ];
  }, [
    createRevealSlideFromToolbar,
    currentRevealSlideIndex,
    getRevealToolbarSlideLabel,
    requestRevealSlideNavigation,
    revealToolbarDeck.slides,
    showRevealSlidePicker,
    t,
  ]);

  const {
    menu: toolbarMenu,
    openForTrigger: openToolbarMenu,
    closeMenu: closeToolbarMenu,
    onSelectItem: handleToolbarMenuSelect,
  } = useAnchoredContextMenu();

  const openToolbarMenuFromShortcut = useCallback(
    (anchor: HTMLButtonElement | null, ariaLabel: string, items: MenuItem[]) => {
      if (!anchor || anchor.disabled) return false;
      anchor.focus();
      openToolbarMenu(anchor, ariaLabel, items);
      return true;
    },
    [openToolbarMenu]
  );

  const setActiveTabMode = useCallback(
    (nextMode: EditorMode) => {
      if (!activeTab) return;

      if (activeTab.mode === 'rich' && nextMode !== 'rich') {
        flushActiveRichMarkdownNow();
      }

      useEditorStore.getState().setDocMode(activeTab.id, nextMode);

      // Se for arquivo real, memoriza preferência apenas de modos de edição.
      if (activeTab.filePath && (nextMode === 'markdown' || nextMode === 'rich')) {
        fileModeByPathRef.current[normalizePathKey(String(activeTab.filePath))] = nextMode;
      }

      focusEditorSoon();
    },
    [activeTab]
  );

  const fileMenuItemsForContextMenu = useMemo((): MenuItem[] => {
    return buildFileMenuItemsForContextMenu({
      ctx: {
        fileMenuItems,
        onSelect: onFileMenuSelect,
      },
    });
  }, [fileMenuItems, onFileMenuSelect]);

  const insertMenuItemsForContextMenu = useMemo((): MenuItem[] => {
    return buildInsertMenuItemsForContextMenu({
      ctx: {
        activeTab,
        isAsking,
        editorReadyNonce,
        richEditorRef,
        applyInsertRequest,
        appendMarkdownToDocument,
        focusEditorSoon,
        addToast,
      },
    });
  }, [activeTab, isAsking, editorReadyNonce, addToast, applyInsertRequest, appendMarkdownToDocument]);

  const formatMenuItemsForContextMenu = useMemo((): MenuItem[] => {
    return buildFormatMenuItemsForContextMenu({
      ctx: {
        activeTab,
        isAsking,
        editorReadyNonce,
        richEditorRef,
        richEditorHandleRef,
      },
    });
  }, [activeTab, isAsking, editorReadyNonce]);

  const modeMenuItemsForContextMenu = useMemo((): MenuItem[] => {
    return buildModeMenuItemsForContextMenu({
      ctx: {
        activeTab,
        isAsking,
        setActiveTabMode,
      },
    });
  }, [activeTab, isAsking, setActiveTabMode]);

  const actions = useMemo(() => {
    return [
      {
        key: 'ask',
        label: t('editor.actions.askChat'),
        icon: <MessageOutlined />,
        shortcut: 'Ctrl+Shift+I',
        onMouseDown: () => {
          rememberCurrentExplicitSelection();
        },
        onClick: async () => {
          if (isAsking) return;
          if (activeTab?.mode === 'view') {
            addToast(t('editor.chatModal.prepareNeedCodeOrRich'), 'info');
            return;
          }
          if (!workspaceTab?.id) return;
          await useWorkspaceChatModalStore.getState().requestOpen(workspaceTab.id);
        },
        disabled: !activeTab || isAsking,
      },
    ];
  }, [activeTab, isAsking, addToast, t, workspaceTab?.id]);

  // Atalhos do editor
  useEffect(() => {
    if (!isPanelActive || !activeTab?.id) return;

    const onKeyDown = async (e: KeyboardEvent) => {
      if (isModalOpen()) return;

      if (
        e.key === 'F5' &&
        !e.ctrlKey &&
        !e.shiftKey &&
        !e.altKey &&
        !e.metaKey &&
        activeTab?.mode === 'view' &&
        isRevealToolbarDocument &&
        !isAsking
      ) {
        e.preventDefault();
        e.stopPropagation();
        requestRevealFullscreen();
        return;
      }

      if (e.altKey && !e.ctrlKey && !e.metaKey) {
        const key = e.key.toLowerCase();

        if (!e.shiftKey) {
          const modesByShortcut: Record<string, EditorMode> = {
            '1': 'markdown',
            '2': 'rich',
            '3': 'view',
          };
          const modeShortcut = modesByShortcut[key];
          if (modeShortcut && !isAsking) {
            e.preventDefault();
            e.stopPropagation();
            setActiveTabMode(modeShortcut);
            return;
          }

          if (key === 'i') {
            const didOpen = openToolbarMenuFromShortcut(
              insertMenuButtonRef.current,
              t('editor.aria.insertMenu'),
              insertMenuItemsForContextMenu
            );
            if (didOpen) {
              e.preventDefault();
              e.stopPropagation();
            }
            return;
          }

          if (key === 's') {
            const didOpen = openToolbarMenuFromShortcut(
              revealSlidePickerButtonRef.current,
              t('editor.presentation.goToSlide'),
              showRevealSlidePicker ? revealSlideMenuItemsForShortcut : []
            );
            if (didOpen) {
              e.preventDefault();
              e.stopPropagation();
            }
            return;
          }
        }
      }

      if (e.ctrlKey && !e.shiftKey && (e.key === 's' || e.key === 'S') && !e.altKey) {
        e.preventDefault();
        await saveFile();
        return;
      }

      if (e.ctrlKey && e.shiftKey && (e.key === 's' || e.key === 'S') && !e.altKey) {
        e.preventDefault();
        await saveFileAsCopy();
        return;
      }

      if (e.ctrlKey && !e.shiftKey && (e.key === 'o' || e.key === 'O') && !e.altKey) {
        e.preventDefault();
        await openFile();
        return;
      }
    };

    window.addEventListener('keydown', onKeyDown, true);
    return () => window.removeEventListener('keydown', onKeyDown, true);
  }, [
    activeTab?.id,
    activeTab?.mode,
    isPanelActive,
    isAsking,
    isRevealToolbarDocument,
    insertMenuItemsForContextMenu,
    openFile,
    openToolbarMenuFromShortcut,
    requestRevealFullscreen,
    saveFile,
    saveFileAsCopy,
    setActiveTabMode,
    showRevealSlidePicker,
    revealSlideMenuItemsForShortcut,
    t,
  ]);

  return (
    <div className="editor-page" ref={pageRootRef}>
      <EditorToolbar
        activeTab={activeTab}
        isAsking={isAsking}
        richEditorRef={richEditorRef}
        shortcutRefs={{
          insertMenu: insertMenuButtonRef,
          modeMenu: modeMenuButtonRef,
          revealSlidePicker: revealSlidePickerButtonRef,
        }}
        actions={actions}
        onOpenMenu={openToolbarMenu}
        fileMenuItems={fileMenuItemsForContextMenu}
        formatMenuItems={formatMenuItemsForContextMenu}
        insertMenuItems={insertMenuItemsForContextMenu}
        modeMenuItems={modeMenuItemsForContextMenu}
        revealSlidePicker={{
          enabled: !!activeTab && activeTab.mode === 'rich' && isRevealToolbarDocument,
          slideCount: revealToolbarDeck.slides.length,
          currentSlideIndex: Math.min(currentRevealSlideIndex, Math.max(0, revealToolbarDeck.slides.length - 1)),
          slideLabels: revealToolbarDeck.slides.map((slide) => slide.label),
          onSelectSlide: requestRevealSlideNavigation,
          onCreateSlide: createRevealSlideFromToolbar,
        }}
        revealFullscreen={{
          enabled: !!activeTab && activeTab.mode === 'view' && isRevealToolbarDocument,
          onRequest: requestRevealFullscreen,
        }}
      />

      <EditorContentArea
        activeTab={activeTab}
        isAsking={isAsking}
        debouncedMarkdownForPreview={debouncedMarkdownForPreview}
        onMarkdownChange={(v) => {
          if (!activeTab) return;
          setDocMarkdown(activeTab.id, v);
          updateLatestMarkdownForTab(activeTab.id, v);
          schedulePersistForTab(activeTab.id);
        }}
        onMonacoMount={(editor, monaco) => {
          editorRef.current = editor as unknown as MonacoCodeEditor;
          monacoRef.current = monaco as MonacoNamespace;
          setEditorReadyNonce((n) => n + 1);
        }}
        onRichMarkdownChange={(md) => {
          if (!activeTab) return;
          setDocMarkdown(activeTab.id, md);
          updateLatestMarkdownForTab(activeTab.id, md);
          schedulePersistForTab(activeTab.id);
        }}
        onRichEditorReady={(ed) => {
          richEditorRef.current = ed;
          setEditorReadyNonce((n) => n + 1);
        }}
        onRevealSlideIndexChange={(index) => {
          currentRevealSlideIndexRef.current = index;
          setCurrentRevealSlideIndex(index);
        }}
        revealAppendNonce={revealAppendNonce}
        revealSlideNavigationRequest={revealSlideNavigationRequest}
        revealFullscreenRequestNonce={revealFullscreenRequestNonce}
        richEditorHandleRef={richEditorHandleRef}
        onRequestEditMermaid={(ctx) => {
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
        }}
        onOpenMermaid={openMermaidEditorByIndex}
        onRemoveMermaid={(index) => {
          void removeMermaidBlockByIndex(index);
        }}
      />

      <MermaidEditorModal
        isOpen={activeMermaidIndex !== null || richMermaidSession !== null}
        title="Editar diagrama Mermaid"
        initialCode={
          activeMermaidIndex !== null
            ? mermaidInitialCode
            : richMermaidSession?.initialCode || ''
        }
        initialInsertText={
          activeMermaidIndex !== null
            ? mermaidInsertText
            : String(richMermaidSession?.insertText || '')
        }
        onConsumeInsertText={() => {
          if (activeMermaidIndex !== null) setMermaidInsertText('');
          if (richMermaidSession) {
            setRichMermaidSession((prev) => (prev ? { ...prev, insertText: '' } : prev));
          }
        }}
        onCancel={() => {
          if (activeMermaidIndex !== null) setActiveMermaidIndex(null);
          if (richMermaidSession) setRichMermaidSession(null);
        }}
        onApply={(code) => {
          if (activeMermaidIndex !== null) {
            applyMermaidCode(code);
            return;
          }
          if (richMermaidSession) {
            richMermaidSession.apply(code);
            addToast(t('editor.toast.mermaidUpdated'), 'success');
            setRichMermaidSession(null);
          }
        }}
        onRemove={async () => {
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
        }}
      />

      <Menu
        items={toolbarMenu.items}
        x={toolbarMenu.x}
        y={toolbarMenu.y}
        visible={toolbarMenu.visible}
        ariaLabel={toolbarMenu.ariaLabel}
        onClose={closeToolbarMenu}
        onSelect={handleToolbarMenuSelect}
      />
    </div>
  );
}
