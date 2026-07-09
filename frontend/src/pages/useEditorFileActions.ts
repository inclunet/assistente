import type { MutableRefObject } from 'react';
import { useTranslation } from 'react-i18next';

import { logger } from '../utils/logger';
import { useEditorStore, DEFAULT_MD, type EditorDocument, type EditorMode } from '../store/editorStore';
import { useWorkspaceStore } from '../store/workspaceStore';
import { useQuestionnaireUIStore } from '../store/questionnaireUIStore';
import { useUIStore } from '../store/uiStore';
import { getErrorMessage, getMaybeContent } from '../lib/editorContent';
import { composePreviewText, hasConflictMarkers } from '../lib/editorMergeUtils';
import { basenameFromPath, normalizePathKey } from '../utils/path';
import {
  EditorDeleteDraft,
  EditorOpenFile,
  EditorReadDraft,
  EditorSaveFileDialog,
  EditorWriteFile,
} from '@wailsjs/go/app/App';
import type { UseEditorMergeResult } from './useEditorMerge';

interface UseEditorFileActionsArgs {
  merge: UseEditorMergeResult;
  activeTab: EditorDocument | null;
  documents: Record<string, EditorDocument>;
  fileModeByPathRef: MutableRefObject<Record<string, EditorMode>>;
  flushActiveRichMarkdownNow: () => void;
  focusEditorSoon: () => void;
}

/**
 * Hook com as ações de arquivo do editor: abrir, salvar, salvar como cópia e
 * abortar merge (estilo Git). Concentra a integração com os diálogos nativos
 * (Wails) e com o estado de merge/conflito externo por aba.
 */
export function useEditorFileActions({
  merge,
  activeTab,
  documents,
  fileModeByPathRef,
  flushActiveRichMarkdownNow,
  focusEditorSoon,
}: UseEditorFileActionsArgs) {
  const { t } = useTranslation();
  const addToast = useUIStore((s) => s.addToast);
  const requestQuestionnaire = useQuestionnaireUIStore((s) => s.request);

  const createDocument = useEditorStore((s) => s.createDocument);
  const setDocMarkdown = useEditorStore((s) => s.setDocMarkdown);
  const renameDocument = useEditorStore((s) => s.renameDocument);
  const setDocFilePath = useEditorStore((s) => s.setDocFilePath);
  const setDocDraftId = useEditorStore((s) => s.setDocDraftId);
  const setDocDirty = useEditorStore((s) => s.setDocDirty);
  const addWorkspaceTab = useWorkspaceStore((s) => s.addTab);
  const setActiveWsTab = useWorkspaceStore((s) => s.setActiveTab);
  const wsTabs = useWorkspaceStore((s) => s.workspace?.tabs);

  const {
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

  return { openFile, abortMerge, saveFile, saveFileAsCopy };
}
