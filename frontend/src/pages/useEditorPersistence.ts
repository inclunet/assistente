import { useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';

import { logger } from '../utils/logger';
import { useEditorStore, type EditorDocument } from '../store/editorStore';
import { useUIStore } from '../store/uiStore';
import { normalizePathKey } from '../utils/path';
import { diskInfoEquals, hashStringFNV1a32, normalizeDiskInfo } from '../lib/editorMergeUtils';
import { EventsOn } from '@wailsjs/runtime/runtime';
import {
  EditorGetFileInfo,
  EditorReadFile,
  EditorUnwatchFile,
  EditorWatchFile,
  EditorWriteDraft,
  EditorWriteFile,
} from '@wailsjs/go/app/App';
import type { EditorFileChangedEvent } from './editorTypes';
import type { UseEditorMergeResult } from './useEditorMerge';

interface UseEditorPersistenceArgs {
  merge: UseEditorMergeResult;
  sessionLoaded: boolean;
  currentDocumentId: string | null;
  allDocs: EditorDocument[];
  flushActiveRichMarkdownNow: () => void;
  saveEditorState: () => void;
}

/**
 * Hook que cuida da persistência e do watch de arquivos do editor:
 * - autosave debounced por aba e gravação imediata (`persistTabContentNow`);
 * - flush ao fechar/ocultar a janela e re-checagem ao focar;
 * - watch de arquivos externos e tratamento do evento `editor:fileChanged`.
 */
export function useEditorPersistence({
  merge,
  sessionLoaded,
  currentDocumentId,
  allDocs,
  flushActiveRichMarkdownNow,
  saveEditorState,
}: UseEditorPersistenceArgs) {
  const { t } = useTranslation();
  const addToast = useUIStore((s) => s.addToast);
  const setDocMarkdown = useEditorStore((s) => s.setDocMarkdown);
  const setDocDirty = useEditorStore((s) => s.setDocDirty);

  const {
    getMergeSession,
    getCachedMarkdownForTab,
    updateLatestMarkdownForTab,
    isExternalConflictLocked,
    setExternalConflictLocked,
    isExternalPromptInFlight,
    isProbablySelfWrite,
    markSelfWrite,
    setDiskBaselineForTab,
    refreshDiskInfoForTab,
    promptResolveExternalChangeForTab,
    diskInfoByTabRef,
    diskContentHashByTabRef,
  } = merge;

  const autosaveTimersByTabRef = useRef<Record<string, number>>({});
  const watchedFilesRef = useRef<Record<string, { path: string; count: number }>>({});

  const persistTabContentNow = async (tabId: string) => {
    if (!sessionLoaded) return;
    const { documents: currentDocs } = useEditorStore.getState();
    const tab = currentDocs[tabId] || null;
    if (!tab) return;

    if (tab.mode === 'rich' && currentDocumentId === tabId) {
      flushActiveRichMarkdownNow();
    }

    const mergeSession = getMergeSession(tabId);
    if (mergeSession) {
      const markdown = getCachedMarkdownForTab(tab);
      updateLatestMarkdownForTab(tab.id, markdown);
      try {
        await EditorWriteDraft(mergeSession.conflictDraftId, markdown);
      } catch {
        // best-effort
      }
      return;
    }

    if (isExternalConflictLocked(tabId)) return;

    const markdown = getCachedMarkdownForTab(tab);
    updateLatestMarkdownForTab(tab.id, markdown);

    const filePath = tab.filePath ? String(tab.filePath) : '';
    const draftId = tab.draftId ? String(tab.draftId) : String(tab.id);

    try {
      if (!filePath) {
        if (!draftId) return;
        await EditorWriteDraft(draftId, markdown);
        return;
      }

      // Detecta mudança externa antes de escrever (evita sobrescrever sem avisar).
      try {
        const currentDisk = normalizeDiskInfo(await EditorGetFileInfo(filePath));
        const lastDisk = diskInfoByTabRef.current[String(tabId)];

        if (lastDisk && !diskInfoEquals(lastDisk, currentDisk)) {
          setExternalConflictLocked(tabId, true);
          setDocDirty(tabId, true);
          addToast(t('editor.toast.fileModified'), 'warning');
          if (!isExternalPromptInFlight(tabId)) {
            void promptResolveExternalChangeForTab(tabId, filePath);
          }
          return;
        }

        if (!lastDisk) diskInfoByTabRef.current[String(tabId)] = currentDisk;
      } catch {
        // best-effort
      }

      markSelfWrite(filePath);
      await EditorWriteFile(filePath, markdown);
      setDiskBaselineForTab(tab.id, markdown);
      setDocDirty(tab.id, false);

      // Atualiza baseline após salvar.
      void refreshDiskInfoForTab(tab);
    } catch (e) {
      logger.warn('[EditorPage] falha ao salvar:', e);
    }
  };

  const schedulePersistForTab = (tabId: string, delayMs = 650) => {
    if (!sessionLoaded) return;
    const id = String(tabId || '');
    if (!id) return;
    if (isExternalConflictLocked(id) && !getMergeSession(id)) return;
    const prev = autosaveTimersByTabRef.current[id];
    if (prev) window.clearTimeout(prev);
    autosaveTimersByTabRef.current[id] = window.setTimeout(() => {
      void persistTabContentNow(id);
    }, Math.max(0, delayMs));
  };

  const syncOrPromptExternalChangeForTab = async (
    tab: EditorDocument,
    opts?: {
      diskContent?: string;
      diskReadError?: string;
      diskHash?: number;
      assisted?: boolean;
      notifyAutoReload?: boolean;
      allowAutoReload?: boolean;
    }
  ) => {
    const filePath = tab.filePath ? String(tab.filePath).trim() : '';
    if (!filePath) return;
    if (isExternalConflictLocked(tab.id)) return;

    let diskContent = typeof opts?.diskContent === 'string' ? String(opts.diskContent) : '';
    let diskReadError = typeof opts?.diskReadError === 'string' ? String(opts.diskReadError) : '';

    if (!diskReadError && opts?.diskContent === undefined) {
      try {
        diskContent = String((await EditorReadFile(filePath)) || '');
      } catch (e) {
        diskReadError = String((e as Error)?.message || e || '').trim();
      }
    }

    if (!diskReadError) {
      const localContent = getCachedMarkdownForTab(tab);
      const diskHash = typeof opts?.diskHash === 'number' ? opts.diskHash : hashStringFNV1a32(diskContent);
      const localHash = hashStringFNV1a32(localContent);
      const lastDiskHash = Number(diskContentHashByTabRef.current[String(tab.id)] || 0);
      const localMatchesKnownDisk = !!lastDiskHash && lastDiskHash === localHash;

      // Sem conflito real: disco e editor já convergiram para o mesmo conteúdo.
      if (diskHash === localHash) {
        setDiskBaselineForTab(tab.id, localContent);
        setDocDirty(tab.id, false);
        void refreshDiskInfoForTab(tab);
        return;
      }

      // Mudou o metadado, mas o conteúdo continua sendo o baseline conhecido.
      if (lastDiskHash && lastDiskHash === diskHash) {
        void refreshDiskInfoForTab(tab);
        return;
      }

      // Aba limpa pode acompanhar uma escrita externa/assistida sem intervenção.
      if (opts?.allowAutoReload && (!tab.isDirty || (opts?.assisted && localMatchesKnownDisk))) {
        try {
          setDocMarkdown(tab.id, diskContent);
          updateLatestMarkdownForTab(tab.id, diskContent);
          setDiskBaselineForTab(tab.id, diskContent);
          setDocDirty(tab.id, false);
          void refreshDiskInfoForTab(tab);
          if (opts?.notifyAutoReload && tab.id === currentDocumentId) {
            addToast(t('editor.toast.externalReloaded'), 'info');
          }
          return;
        } catch {
          // Se não der pra aplicar automaticamente, cai pro fluxo de decisão explícita.
        }
      }
    }

    setExternalConflictLocked(tab.id, true);
    setDocDirty(tab.id, true);
    if (!isExternalPromptInFlight(tab.id)) {
      void promptResolveExternalChangeForTab(tab.id, filePath, { diskContent, diskReadError });
    }
  };

  // Flush imediato ao fechar/minimizar para reduzir chance de perder o estado.
  useEffect(() => {
    if (!sessionLoaded) return;

    const persistNow = () => {
      try {
        if (currentDocumentId) {
          void persistTabContentNow(currentDocumentId);
        }
      } catch {
        // best-effort
      }
      saveEditorState();
    };

    const onBeforeUnload = () => persistNow();
    const onPageHide = () => persistNow();
    const checkActiveFileExternalChange = async () => {
      const { documents: currentDocs } = useEditorStore.getState();
      const tab = currentDocumentId ? currentDocs[currentDocumentId] || null : null;
      if (!tab?.filePath) return;
      if (isExternalConflictLocked(tab.id)) return;

      const lastDisk = diskInfoByTabRef.current[String(tab.id)];
      const currentDisk = await refreshDiskInfoForTab(tab);
      if (!currentDisk) return;

      if (lastDisk && !diskInfoEquals(lastDisk, currentDisk)) {
        await syncOrPromptExternalChangeForTab(tab, { notifyAutoReload: true });
      }
    };

    const onVisibilityChange = () => {
      if (document.visibilityState === 'hidden') {
        persistNow();
        return;
      }
      if (document.visibilityState === 'visible') {
        void checkActiveFileExternalChange();
      }
    };

    const onFocus = () => {
      void checkActiveFileExternalChange();
    };

    window.addEventListener('beforeunload', onBeforeUnload);
    window.addEventListener('pagehide', onPageHide);
    document.addEventListener('visibilitychange', onVisibilityChange);
    window.addEventListener('focus', onFocus);

    return () => {
      window.removeEventListener('beforeunload', onBeforeUnload);
      window.removeEventListener('pagehide', onPageHide);
      document.removeEventListener('visibilitychange', onVisibilityChange);
      window.removeEventListener('focus', onFocus);
    };
  }, [sessionLoaded, currentDocumentId]);

  // Watcher de mudanças externas (sincroniza watch/unwatch com as abas abertas).
  useEffect(() => {
    if (!sessionLoaded) return;

    const next: Record<string, { path: string; count: number }> = {};
    for (const tab of allDocs) {
      if (!tab.filePath) continue;
      const p = String(tab.filePath || '').trim();
      const key = normalizePathKey(p);
      if (!key) continue;
      if (!next[key]) next[key] = { path: p, count: 0 };
      next[key].count += 1;
    }

    const prev = watchedFilesRef.current;

    for (const [key, entry] of Object.entries(prev)) {
      const prevCount = entry.count;
      const nextCount = next[key]?.count ?? 0;
      const diff = prevCount - nextCount;
      if (diff <= 0) continue;
      for (let i = 0; i < diff; i++) {
        EditorUnwatchFile(entry.path).catch(() => null);
      }
    }

    for (const [key, entry] of Object.entries(next)) {
      const prevCount = prev[key]?.count ?? 0;
      const diff = entry.count - prevCount;
      if (diff <= 0) continue;
      for (let i = 0; i < diff; i++) {
        EditorWatchFile(entry.path).catch(() => null);
      }
    }

    watchedFilesRef.current = next;
  }, [sessionLoaded, allDocs]);

  useEffect(() => {
    return () => {
      const prev = watchedFilesRef.current;
      watchedFilesRef.current = {};
      for (const entry of Object.values(prev)) {
        for (let i = 0; i < entry.count; i++) {
          EditorUnwatchFile(entry.path).catch(() => null);
        }
      }
    };
  }, []);

  useEffect(() => {
    if (!sessionLoaded) return;

    const unsub = EventsOn('editor:fileChanged', async (data: EditorFileChangedEvent) => {
      const changedPath = String(data?.path || data?.filePath || '').trim();
      if (!changedPath) return;
      const assisted = data?.assisted === true || String(data?.origin || '') === 'assistant_tool';
      if (assisted) {
        flushActiveRichMarkdownNow();
      }

      if (!assisted && isProbablySelfWrite(changedPath)) {
        return;
      }

      const key = normalizePathKey(changedPath);
      if (!key) return;

      const { documents: currentDocs } = useEditorStore.getState();
      const affected = Object.values(currentDocs).filter(
        (tab) => tab.filePath && normalizePathKey(String(tab.filePath)) === key
      );
      if (affected.length === 0) return;
      let diskContent = '';
      let diskReadError = '';
      try {
        diskContent = String((await EditorReadFile(changedPath)) || '');
      } catch (e) {
        diskReadError = String((e as Error)?.message || e || '').trim();
      }
      const diskHash = !diskReadError ? hashStringFNV1a32(diskContent) : undefined;

      for (const tab of affected) {
        await syncOrPromptExternalChangeForTab(tab, {
          diskContent,
          diskReadError,
          diskHash,
          assisted,
          allowAutoReload: true,
          notifyAutoReload: true,
        });
      }
    });

    return () => {
      try {
        unsub();
      } catch {
        // ignore
      }
    };
  }, [sessionLoaded, currentDocumentId]);

  // Limpa timers de autosave ao desmontar.
  useEffect(() => {
    return () => {
      const timers = autosaveTimersByTabRef.current;
      for (const k of Object.keys(timers)) {
        try {
          window.clearTimeout(timers[k]);
        } catch {
          // best-effort
        }
      }
      autosaveTimersByTabRef.current = {};
    };
  }, []);

  return {
    persistTabContentNow,
    schedulePersistForTab,
  };
}
