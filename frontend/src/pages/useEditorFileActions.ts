import { useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';

import { logger } from '../utils/logger';
import { useEditorStore, type EditorDocument } from '../store/editorStore';
import { useWorkspaceStore } from '../store/workspaceStore';
import { useAuthStore } from '../store/authStore';
import { useQuestionnaireUIStore } from '../store/questionnaireUIStore';
import { requestConfirm } from '../store/confirmStore';
import { useUIStore } from '../store/uiStore';
import { getMaybeContent } from '../lib/editorContent';
import { composePreviewText, hasConflictMarkers } from '../lib/editorMergeUtils';
import { editorFileDialogLabels } from '../lib/editorDialogLabels';
import { basenameFromPath } from '../utils/path';
import {
  EditorDeleteDraft,
  EditorReadDraft,
} from '@wailsjs/go/wailsapi/Editor';
import type { UseEditorMergeResult } from './useEditorMerge';
import type { EditorFileCommandID, EditorFilePreparation } from '../lib/commandEditorFile';

interface UseEditorFileActionsArgs {
  merge: UseEditorMergeResult;
  activeTab: EditorDocument | null;
  flushActiveRichMarkdownNow: () => void;
  focusEditorSoon: () => void;
}

interface EditorFileCommandSnapshot {
  ownerId: string;
  sessionId: string;
  workspaceId: string;
  tabId: string;
  documentId: string;
  filePath: string | null;
  sentContent: string | undefined;
  draftId: string | null;
  mergeSession: unknown;
}

/**
 * Hook com as ações de arquivo do editor: abrir, salvar, salvar como cópia e
 * abortar merge (estilo Git). Concentra a integração com os diálogos nativos
 * (Wails) e com o estado de merge/conflito externo por aba.
 */
export function useEditorFileActions({
  merge,
  activeTab,
  flushActiveRichMarkdownNow,
  focusEditorSoon,
}: UseEditorFileActionsArgs) {
  const { t } = useTranslation();
  const addToast = useUIStore((s) => s.addToast);
  const requestQuestionnaire = useQuestionnaireUIStore((s) => s.request);

  const setDocMarkdown = useEditorStore((s) => s.setDocMarkdown);
  const setDocDraftId = useEditorStore((s) => s.setDocDraftId);
  const setDocDirty = useEditorStore((s) => s.setDocDirty);
  const fileCommandSnapshots = useRef(new Map<EditorFileCommandID, EditorFileCommandSnapshot>());
  const setDocFilePathAndTitle = (documentId: string, path: string) => {
    const title = basenameFromPath(path);
    useEditorStore.setState((state) => {
      const document = state.documents[documentId];
      if (!document) return state;
      return {
        documents: {
          ...state.documents,
          [documentId]: { ...document, filePath: path, title },
        },
      };
    });
  };
  useEffect(() => {
    const snapshots = fileCommandSnapshots.current;
    return () => {
      snapshots.clear();
    };
  }, []);

  const {
    getMergeSession,
    getCachedMarkdownForTab,
    updateLatestMarkdownForTab,
    isExternalConflictLocked,
    setExternalConflictLocked,
    setDiskBaselineForTab,
    cleanupMergeSessionForTab,
  } = merge;

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
      title: t('editor.questionnaire.abortMergeTitle'),
      description: t('editor.questionnaire.abortMergeDesc'),
      submitLabel: t('editor.questionnaire.abortMergeSubmit'),
      cancelLabel: t('editor.questionnaire.abortMergeCancel'),
      allowCancel: true,
      questions: [
        {
          id: 'path',
          type: 'readonly_code' as const,
          prompt: t('editor.prompts.file'),
          content: String(activeTab.filePath || ''),
        },
        {
          id: 'mine',
          type: 'readonly_code' as const,
          prompt: t('editor.questionnaire.abortMergeMinePreview'),
          content: minePreviewText || t('editor.questionnaire.emptyPreview'),
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

  /** Captura somente dados locais; diálogos nativos são responsabilidade do backend prepare. */
  const prepareFileCommand = async (commandID: EditorFileCommandID): Promise<EditorFilePreparation | undefined> => {
    if (!activeTab || (activeTab.readOnly && commandID !== 'editor.file.open') || !useWorkspaceStore.getState().workspace ||
        useWorkspaceStore.getState().workspace?.activeTabId !== activeTab.id) return undefined;
    if (activeTab.mode === 'rich') flushActiveRichMarkdownNow();
    const content = commandID === 'editor.file.open' ? undefined : getCachedMarkdownForTab(activeTab);
    if (commandID !== 'editor.file.open') {
      if (content !== undefined && hasConflictMarkers(content)) {
        addToast(t('editor.toast.conflictMarkersRemain'), 'warning');
        return undefined;
      }
      if (isExternalConflictLocked(activeTab.id) && !getMergeSession(activeTab.id)) {
        addToast(t('editor.toast.saveLockedExternal'), 'warning');
        return undefined;
      }
    }
    const auth = useAuthStore.getState();
    const workspace = useWorkspaceStore.getState().workspace;
    const user = auth.user;
    if (!auth.isAuthenticated || !user || !workspace) return undefined;
    fileCommandSnapshots.current.set(commandID, {
      ownerId: user.userId,
      sessionId: user.sessionId,
      workspaceId: workspace.id,
      tabId: activeTab.id,
      documentId: activeTab.id,
      filePath: activeTab.filePath ? String(activeTab.filePath) : null,
      sentContent: content,
      draftId: activeTab.draftId ? String(activeTab.draftId) : null,
      mergeSession: commandID === 'editor.file.open' ? null : getMergeSession(activeTab.id),
    });
    return {
      content,
      labels: { ...editorFileDialogLabels(t, commandID === 'editor.file.open' ? 'open' : 'save') },
      suggestedFilename: activeTab.filePath ? basenameFromPath(activeTab.filePath) : `${activeTab.title || t('editor.fallback.newDoc')}.md`,
      confirmOverwrite: false,
      path: activeTab.filePath ? String(activeTab.filePath) : undefined,
    };
  };

  const confirmOverwrite = async (path: string): Promise<boolean> => {
    const confirmed = await requestConfirm({
      title: t('app.questionnaire.editConfirmation.overwriteTitle'),
      message: t('app.questionnaire.editConfirmation.overwriteDescription', { path }),
      confirmText: t('app.questionnaire.editConfirmation.overwriteConfirm'),
      cancelText: t('app.questionnaire.editConfirmation.overwriteCancel'),
      variant: 'warning',
    });
    // confirmStore resolves before its queued focus restoration frame. Give
    // the renderer one frame plus a macrotask so the coordinator's immediate
    // canCommit check does not observe the just-closed dialog as still active.
    await new Promise<void>((resolve) => {
      requestAnimationFrame(() => setTimeout(resolve, 0));
    });
    return confirmed;
  };

  const applyCommittedFileCommand = (commandID: EditorFileCommandID, raw: unknown): void => {
    if (!raw || typeof raw !== 'object') return;
    const result = raw as { tabId?: unknown; path?: unknown; opened?: unknown; written?: unknown };
    const snapshot = fileCommandSnapshots.current.get(commandID);
    fileCommandSnapshots.current.delete(commandID);
    if (!snapshot) return;
    const auth = useAuthStore.getState();
    const workspace = useWorkspaceStore.getState().workspace;
    const current = useEditorStore.getState().getDocument(snapshot.documentId);
    if (!auth.isAuthenticated || auth.user?.userId !== snapshot.ownerId || auth.user.sessionId !== snapshot.sessionId ||
        workspace?.id !== snapshot.workspaceId || typeof result.tabId !== 'string') return;

    if (commandID === 'editor.file.open') {
      // The workspace event/loader owns creation and hydration. Never replace
      // an already-live document here, especially one that may be dirty.
      return;
    }

    if (result.tabId !== snapshot.tabId || result.written !== true || !current) return;
    const announceSuccess = (message: string) => {
      const active = workspace?.activeTabId === snapshot.tabId
        && workspace.tabs.some((tab) => tab.id === snapshot.tabId && tab.type === 'editor');
      if (active) addToast(message, 'success');
    };
    if (commandID === 'editor.file.save_copy') {
      // A copy is a second file only: it never changes the source document,
      // baseline, draft, merge lock, path, or dirty bit.
      announceSuccess(t('editor.toast.copySaved'));
      return;
    }
    const resultPath = typeof result.path === 'string' ? result.path : null;
    const currentPath = current.filePath ? String(current.filePath) : null;
    if (currentPath !== snapshot.filePath && currentPath !== resultPath) return;
    const currentMergeSession = getMergeSession(snapshot.documentId);
    if (resultPath && commandID === 'editor.file.save' &&
        (currentPath !== resultPath || current.title !== basenameFromPath(resultPath))) {
      setDocFilePathAndTitle(snapshot.documentId, resultPath);
    }
    const currentContent = getCachedMarkdownForTab(current);
    const unchanged = snapshot.sentContent !== undefined && currentContent === snapshot.sentContent;
    // Uma nova decisão de conflito pode ter observado uma escrita posterior.
    // Não substituir sua baseline pelo resultado atrasado desta operação.
    if (currentMergeSession !== snapshot.mergeSession ||
        (snapshot.mergeSession === null && isExternalConflictLocked(snapshot.documentId))) return;
    setDiskBaselineForTab(snapshot.documentId, snapshot.sentContent ?? '');
    if (!unchanged) return;

    setDocDirty(snapshot.documentId, false);
    if (snapshot.mergeSession !== null) {
      setExternalConflictLocked(snapshot.documentId, false);
      void cleanupMergeSessionForTab(snapshot.documentId).catch((error) => logger.warn('[useEditorFileActions] merge cleanup failed', error));
    }
    if (snapshot.draftId && current.draftId === snapshot.draftId) {
      setDocDraftId(snapshot.documentId, null);
      void EditorDeleteDraft(snapshot.draftId).catch((error) => logger.warn('[useEditorFileActions] draft cleanup failed', error));
    }
    announceSuccess(t('editor.toast.fileSaved'));
  };

  return { abortMerge, prepareFileCommand, confirmOverwrite, applyCommittedFileCommand };
}
