import { useRef } from 'react';
import { useTranslation } from 'react-i18next';

import { logger } from '../utils/logger';
import { useEditorStore, type EditorDocument } from '../store/editorStore';
import { useUIStore } from '../store/uiStore';
import { useQuestionnaireUIStore } from '../store/questionnaireUIStore';
import { basenameFromPath, normalizePathKey } from '../utils/path';
import {
  EditorDeleteDraft,
  EditorGetFileInfo,
  EditorReadFile,
  EditorSaveFileDialog,
  EditorWriteDraft,
  EditorWriteFile,
} from '@wailsjs/go/app/App';
import {
  type DiskInfo,
  type TextPreview,
  buildUnifiedDiff,
  hashStringFNV1a32,
  makeGitStyleConflictText,
  normalizeDiskInfo,
  safeDraftIdPart,
  truncatePreview,
} from '../lib/editorMergeUtils';
import type { MergeSession } from './editorTypes';

const errorMessage = (e: unknown): string => String((e as Error)?.message || e || '').trim();

/**
 * Hook que concentra o estado e a lógica de merge/conflito externo do editor:
 * - cache do markdown atual por aba (base para comparar com o disco);
 * - metadados/baseline de disco e detecção de mudança externa;
 * - sessões de merge no estilo Git e o questionário de resolução de conflito.
 *
 * As funções retornadas fecham sobre refs estáveis e setters do store, então
 * podem ser usadas dentro de efeitos sem alterar suas dependências.
 */
export function useEditorMerge() {
  const { t } = useTranslation();
  const addToast = useUIStore((s) => s.addToast);
  const requestQuestionnaire = useQuestionnaireUIStore((s) => s.request);

  // Compõe o texto de um preview, anexando o sufixo traduzido quando truncado.
  const composePreviewText = (p: TextPreview): string =>
    p.truncated ? p.preview + t('editor.preview.truncatedSuffix', { total: p.total }) : p.preview;

  const setDocMarkdown = useEditorStore((s) => s.setDocMarkdown);
  const setDocFilePath = useEditorStore((s) => s.setDocFilePath);
  const setDocDraftId = useEditorStore((s) => s.setDocDraftId);
  const setDocDirty = useEditorStore((s) => s.setDocDirty);
  const renameDocument = useEditorStore((s) => s.renameDocument);

  // Autosave robusto: mantém a última versão conhecida do markdown por aba.
  const latestMarkdownByTabRef = useRef<Record<string, string>>({});

  const diskInfoByTabRef = useRef<Record<string, DiskInfo>>({});
  const diskContentHashByTabRef = useRef<Record<string, number>>({});
  const diskBaselineContentByTabRef = useRef<Record<string, string>>({});
  const externalConflictLockedByTabRef = useRef<Record<string, boolean>>({});
  const lastSelfWriteAtByPathRef = useRef<Record<string, number>>({});
  const mergeSessionByTabRef = useRef<Record<string, MergeSession>>({});
  const externalPromptInFlightByTabRef = useRef<Record<string, boolean>>({});

  const getMergeSession = (tabId: string): MergeSession | null => {
    const id = String(tabId || '');
    if (!id) return null;
    return mergeSessionByTabRef.current[id] || null;
  };

  const markSelfWrite = (filePath: string) => {
    const key = normalizePathKey(String(filePath || ''));
    if (!key) return;
    lastSelfWriteAtByPathRef.current[key] = Date.now();
  };

  const isProbablySelfWrite = (filePath: string, withinMs = 900) => {
    const key = normalizePathKey(String(filePath || ''));
    if (!key) return false;
    const at = Number(lastSelfWriteAtByPathRef.current[key] || 0);
    if (!at) return false;
    return Date.now() - at < Math.max(0, withinMs);
  };

  const updateLatestMarkdownForTab = (tabId: string, markdown: string) => {
    latestMarkdownByTabRef.current[String(tabId || '')] = String(markdown ?? '');
  };

  const setExternalConflictLocked = (tabId: string, locked: boolean) => {
    externalConflictLockedByTabRef.current[String(tabId || '')] = !!locked;
  };

  const isExternalPromptInFlight = (tabId: string) => {
    return !!externalPromptInFlightByTabRef.current[String(tabId || '')];
  };

  const setExternalPromptInFlight = (tabId: string, inFlight: boolean) => {
    externalPromptInFlightByTabRef.current[String(tabId || '')] = !!inFlight;
  };

  const isExternalConflictLocked = (tabId: string) => {
    return !!externalConflictLockedByTabRef.current[String(tabId || '')];
  };

  const refreshDiskInfoForTab = async (tab: EditorDocument): Promise<DiskInfo | null> => {
    const filePath = tab?.filePath ? String(tab.filePath) : '';
    if (!filePath) return null;
    try {
      const di = normalizeDiskInfo(await EditorGetFileInfo(filePath));
      diskInfoByTabRef.current[String(tab.id)] = di;
      return di;
    } catch {
      return null;
    }
  };

  const getCachedMarkdownForTab = (tab: EditorDocument): string => {
    if (!tab) return '';
    return latestMarkdownByTabRef.current[tab.id] ?? String(tab.markdown ?? '');
  };

  const setDiskBaselineForTab = (tabId: string, content: string) => {
    diskContentHashByTabRef.current[String(tabId || '')] = hashStringFNV1a32(content);
    diskBaselineContentByTabRef.current[String(tabId || '')] = String(content ?? '');
  };

  const startMergeSessionForTab = async (tabId: string, filePath: string, diskContent: string, localContent: string) => {
    const stamp = Date.now();
    const safeTab = safeDraftIdPart(tabId);
    const mineDraftId = `merge-mine-${safeTab}-${stamp}`;
    const diskDraftId = `merge-disk-${safeTab}-${stamp}`;
    const conflictDraftId = `merge-conflict-${safeTab}-${stamp}`;

    const conflictText = makeGitStyleConflictText(diskContent, localContent, { disk: 'disco', local: 'minha' });

    await EditorWriteDraft(mineDraftId, localContent);
    await EditorWriteDraft(diskDraftId, diskContent);
    await EditorWriteDraft(conflictDraftId, conflictText);

    mergeSessionByTabRef.current[String(tabId || '')] = {
      originalPath: String(filePath || ''),
      mineDraftId,
      diskDraftId,
      conflictDraftId,
      createdAt: stamp,
    };

    // Garante travamento mesmo se o fluxo tiver sido iniciado fora do questionário.
    setExternalConflictLocked(tabId, true);

    setDocMarkdown(tabId, conflictText);
    updateLatestMarkdownForTab(tabId, conflictText);
    setDocDirty(tabId, true);

    addToast(t('editor.toast.mergeConflict'), 'warning');
  };

  const cleanupMergeSessionForTab = async (tabId: string) => {
    const sess = getMergeSession(tabId);
    if (!sess) return;
    delete mergeSessionByTabRef.current[String(tabId || '')];
    const ids = [sess.mineDraftId, sess.diskDraftId, sess.conflictDraftId].filter(Boolean);
    await Promise.all(ids.map((id) => EditorDeleteDraft(id).catch(() => null)));
  };

  const promptResolveExternalChangeForTab = async (
    tabId: string,
    filePath: string,
    opts?: { diskContent?: string; diskReadError?: string }
  ) => {
    if (isExternalPromptInFlight(tabId)) return;

    const { documents: currentDocs } = useEditorStore.getState();
    const tab = currentDocs[tabId] || null;
    if (!tab || !tab.filePath) return;

    setExternalPromptInFlight(tabId, true);
    try {
      setExternalConflictLocked(tabId, true);
      setDocDirty(tabId, true);

      const localContent = getCachedMarkdownForTab(tab);
      const localPreviewText = composePreviewText(truncatePreview(localContent));

      let diskContent = typeof opts?.diskContent === 'string' ? String(opts?.diskContent) : '';
      let diskReadError = typeof opts?.diskReadError === 'string' ? String(opts?.diskReadError) : '';

      if (!diskReadError && opts?.diskContent === undefined) {
        try {
          diskContent = String((await EditorReadFile(filePath)) || '');
        } catch (e) {
          diskReadError = errorMessage(e);
        }
      }

      // Se o conteúdo no disco é igual ao local, não há conflito real.
      if (!diskReadError && diskContent === localContent) {
        setDiskBaselineForTab(tabId, localContent);
        setDocDirty(tabId, false);
        const { documents: afterDocs } = useEditorStore.getState();
        const afterTab = afterDocs[tabId] || tab;
        void refreshDiskInfoForTab(afterTab);
        setExternalConflictLocked(tabId, false);
        return;
      }

      const diskPreviewText = diskReadError
        ? `${t('editor.errors.diskReadFailed')}\n${diskReadError}`
        : composePreviewText(truncatePreview(diskContent));

      const diffText = diskReadError ? '' : buildUnifiedDiff(diskContent, localContent);
      const diffPreviewText = diffText ? composePreviewText(truncatePreview(diffText, 30000)) : '';

      const resp = await requestQuestionnaire({
        id: `ui-editor-external-change-${Date.now()}`,
        title: t('editor.questionnaire.externalChangeTitle'),
        description: t('editor.questionnaire.externalChangeDesc'),
        submitLabel: t('editor.buttons.apply'),
        cancelLabel: t('editor.buttons.notNow'),
        allowCancel: true,
        questions: [
          {
            id: 'path',
            type: 'readonly_code' as const,
            prompt: t('editor.prompts.file'),
            content: String(filePath || ''),
          },
          ...(diffPreviewText
            ? [
                {
                  id: 'diff',
                  type: 'readonly_code' as const,
                  prompt: t('editor.prompts.diff'),
                  content: diffPreviewText,
                },
              ]
            : []),
          {
            id: 'disk',
            type: 'readonly_code' as const,
            prompt: t('editor.prompts.diskPreview'),
            content: diskPreviewText,
          },
          {
            id: 'local',
            type: 'readonly_code' as const,
            prompt: t('editor.prompts.localPreview'),
            content: localPreviewText,
          },
          {
            id: 'choice',
            type: 'single_choice' as const,
            prompt: t('editor.prompts.action'),
            required: true,
            options: [
              t('editor.options.useDisk'),
              t('editor.options.resolveMerge'),
              t('editor.options.useMine'),
              t('editor.options.saveAs'),
            ],
            default: t('editor.options.useDisk'),
          },
        ],
      });

      if (resp.cancelled) {
        addToast(t('editor.toast.externalChange'), 'warning');
        return;
      }

      const choice = String(resp.answers?.choice || '').trim();

      if (choice === t('editor.options.resolveMerge')) {
        if (diskReadError) {
          addToast(t('editor.toast.diskReadFailed'), 'error');
          return;
        }
        try {
          await startMergeSessionForTab(tabId, filePath, diskContent, localContent);
        } catch (e) {
          logger.error('[EditorPage] startMergeSession error:', e);
          addToast(errorMessage(e) || t('editor.toast.mergeStartFailed'), 'error');
        }
        return;
      }

      if (choice === t('editor.options.useDisk')) {
        if (diskReadError) {
          addToast(t('editor.toast.diskReadFailed'), 'error');
          return;
        }

        try {
          setDocMarkdown(tabId, diskContent);
          updateLatestMarkdownForTab(tabId, diskContent);
          setDiskBaselineForTab(tabId, diskContent);
          setDocDirty(tabId, false);
          const { documents: afterDocs } = useEditorStore.getState();
          const afterTab = afterDocs[tabId] || tab;
          void refreshDiskInfoForTab(afterTab);
          setExternalConflictLocked(tabId, false);
          addToast(t('editor.toast.reloaded'), 'success');
        } catch (e) {
          addToast(errorMessage(e) || t('editor.toast.reloadFailed'), 'error');
        }
        return;
      }

      if (choice.startsWith(t('editor.options.saveAs'))) {
        const suggested = basenameFromPath(filePath) || 'documento.md';
        const newPath = String((await EditorSaveFileDialog(suggested)) || '').trim();
        if (!newPath) return;

        updateLatestMarkdownForTab(tabId, localContent);
        markSelfWrite(newPath);
        await EditorWriteFile(newPath, localContent);
        setDiskBaselineForTab(tabId, localContent);

        const title = basenameFromPath(newPath);
        setDocFilePath(tabId, newPath);
        renameDocument(tabId, title);
        setDocDraftId(tabId, null);
        setDocDirty(tabId, false);

        // filePath+title são sincronizados pelo controller do painel de editor.

        const { documents: afterDocs } = useEditorStore.getState();
        const afterTab = afterDocs[tabId] || tab;
        void refreshDiskInfoForTab(afterTab);
        setExternalConflictLocked(tabId, false);
        addToast(t('editor.toast.savedAs'), 'success');
        return;
      }

      // Usar minha versão (sobrescrever no disco)
      try {
        markSelfWrite(filePath);
        await EditorWriteFile(filePath, localContent);
        setDiskBaselineForTab(tabId, localContent);
        setDocDirty(tabId, false);
        const { documents: afterDocs } = useEditorStore.getState();
        const afterTab = afterDocs[tabId] || tab;
        void refreshDiskInfoForTab(afterTab);
        setExternalConflictLocked(tabId, false);
        addToast(t('editor.toast.overwritten'), 'success');
      } catch (e) {
        addToast(errorMessage(e) || t('editor.toast.overwriteFailed'), 'error');
      }
    } finally {
      setExternalPromptInFlight(tabId, false);
    }
  };

  return {
    // Refs compartilhadas (somente leitura/escrita pontual por outros hooks).
    latestMarkdownByTabRef,
    diskInfoByTabRef,
    diskContentHashByTabRef,
    mergeSessionByTabRef,

    // Helpers de estado.
    getMergeSession,
    markSelfWrite,
    isProbablySelfWrite,
    updateLatestMarkdownForTab,
    getCachedMarkdownForTab,
    setExternalConflictLocked,
    isExternalConflictLocked,
    isExternalPromptInFlight,
    setExternalPromptInFlight,
    refreshDiskInfoForTab,
    setDiskBaselineForTab,

    // Ciclo de vida de merge + resolução de conflito.
    startMergeSessionForTab,
    cleanupMergeSessionForTab,
    promptResolveExternalChangeForTab,
  };
}

export type UseEditorMergeResult = ReturnType<typeof useEditorMerge>;
