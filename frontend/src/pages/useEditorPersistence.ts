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
import { decideExternalChange, type ReconcileTrigger } from './editorReconciler';

/** Resultado de uma leitura de conteúdo do disco (best-effort). */
interface DiskReadResult {
  content: string;
  error: string;
  hash?: number;
}

async function readDiskContent(filePath: string): Promise<DiskReadResult> {
  try {
    const content = String((await EditorReadFile(filePath)) || '');
    return { content, error: '', hash: hashStringFNV1a32(content) };
  } catch (e) {
    return { content: '', error: String((e as Error)?.message || e || '').trim() };
  }
}

/**
 * Garante que uma leitura bem-sucedida sempre tenha `hash` preenchido
 * (calculado do conteúdo quando ausente). `hash` é opcional em DiskReadResult,
 * e o reconciliador/auto-reload dependem dele para decidir de forma consistente.
 */
function normalizeDiskRead(read: DiskReadResult): DiskReadResult {
  if (read.error || typeof read.hash === 'number') return read;
  return { ...read, hash: hashStringFNV1a32(read.content) };
}

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
    getDiskStateForTab,
    setDiskInfoForTab,
    setDiskBaselineForTab,
    refreshDiskInfoForTab,
    promptResolveExternalChangeForTab,
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
        const lastDisk = getDiskStateForTab(tabId).info;

        const buildInput = (diskRead?: DiskReadResult) => ({
          trigger: 'pre_save' as const,
          conflictLocked: isExternalConflictLocked(tabId),
          promptInFlight: isExternalPromptInFlight(tabId),
          hasMergeSession: !!getMergeSession(tabId),
          tabIsDirty: !!tab.isDirty,
          // Sem `lastDisk` ainda (o refresh inicial pode não ter completado),
          // "desconhecido" não pode virar "sem mudança": se o arquivo existe,
          // força a comparação por conteúdo ao menos uma vez. Arquivo ainda
          // inexistente não tem o que sobrescrever — segue gravando direto.
          diskInfoChanged: lastDisk ? !diskInfoEquals(lastDisk, currentDisk) : currentDisk.exists,
          ...(diskRead
            ? {
                diskReadError: !!diskRead.error,
                diskHash: diskRead.hash,
                localHash: hashStringFNV1a32(markdown),
                lastKnownDiskHash: getDiskStateForTab(tabId).baselineHash,
              }
            : {}),
        });

        let diskRead: DiskReadResult | undefined;
        let decision = decideExternalChange(buildInput());

        // Metadados divergentes sem conteúdo em mãos: OneDrive/antivírus/
        // indexador tocam o mtime sem mudar conteúdo, então lê o disco e
        // re-decide por hash antes de acusar conflito.
        if (decision.action === 'defer_read') {
          diskRead = normalizeDiskRead(await readDiskContent(filePath));
          decision = decideExternalChange(buildInput(diskRead));
        }

        // Um `editor:fileChanged` ou merge pode ter travado a aba durante os
        // awaits acima: nesse caso a gravação NÃO pode prosseguir (gravaria
        // por cima de um conflito real pendente). `no_change` é o único
        // `ignore` benigno aqui, e mesmo ele ainda passa pela re-checagem
        // direta de lock/merge abaixo antes de seguir para a gravação.
        if (decision.action === 'ignore' && decision.reason !== 'no_change') {
          return;
        }
        if (isExternalConflictLocked(tabId) || getMergeSession(tabId)) {
          return;
        }

        if (decision.action === 'prompt_conflict') {
          setExternalConflictLocked(tabId, true);
          setDocDirty(tabId, true);
          addToast(t('editor.toast.fileModified'), 'warning');
          if (decision.openPrompt) {
            // Só o erro de leitura é repassado: sem `diskContent`, o prompt
            // relê o disco ao abrir, o que habilita o silent-resolve inicial
            // (disco convergiu de volta nesse meio tempo) e evita apresentar
            // conteúdo já stale ao usuário.
            void promptResolveExternalChangeForTab(
              tabId,
              filePath,
              diskRead?.error ? { diskReadError: diskRead.error } : undefined
            );
          }
          return;
        }

        // Revalidação anti-TOCTOU: o await da leitura de conteúdo (defer_read)
        // alargou a janela entre a decisão e a gravação, então um re-stat
        // rápido confere se o disco ainda é o que a decisão avaliou. Como
        // mtime/size podem flutuar sem mudança de conteúdo (OneDrive/
        // antivírus/indexador), metadados divergentes ganham uma re-leitura:
        // conteúdo ainda igual ao já avaliado segue gravando (adotando o stat
        // novo); conteúdo diferente aborta a rodada ANTES de mutar
        // baseline/info (estado antigo intacto força nova comparação por
        // conteúdo no próximo autosave ou evento do watcher) e sem gravar.
        let adoptedDisk = currentDisk;
        if (diskRead) {
          const recheck = normalizeDiskInfo(await EditorGetFileInfo(filePath));
          if (!diskInfoEquals(recheck, currentDisk)) {
            const reread = normalizeDiskRead(await readDiskContent(filePath));
            if (reread.error || reread.hash !== diskRead.hash) {
              return;
            }
            adoptedDisk = recheck;
          }
        }

        if (decision.action === 'update_baseline') {
          // Casos benignos verificados por conteúdo: só metadado tocado
          // (info_only) ou disco já igual ao local (adopt_local). Atualiza o
          // estado conhecido e segue com a gravação normal abaixo.
          if (decision.scope === 'adopt_local') {
            setDiskBaselineForTab(tabId, markdown);
          }
          setDiskInfoForTab(tabId, adoptedDisk);
        } else if (!lastDisk) {
          setDiskInfoForTab(tabId, adoptedDisk);
        }
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

  /**
   * Ponto único de execução do reconciliador: monta o `ReconcileInput` com o
   * estado consolidado da aba, obtém a decisão pura (`decideExternalChange`) e
   * aplica o efeito correspondente. Usado pelo evento `editor:fileChanged`,
   * pelo sync assistido explícito e pela re-checagem de foco.
   *
   * Retorna `true` quando um auto-reload alterou o conteúdo visível da aba.
   */
  const reconcileExternalChangeForTab = async (
    tab: EditorDocument,
    opts: {
      trigger: ReconcileTrigger;
      selfWrite?: boolean;
      assisted?: boolean;
      probablySelfWrite?: boolean;
      allowAutoReload?: boolean;
      notifyAutoReload?: boolean;
      /** Leitura compartilhada/lazy do disco (uma só por evento com várias abas). */
      readDisk?: () => Promise<DiskReadResult>;
    }
  ): Promise<boolean> => {
    const filePath = tab.filePath ? String(tab.filePath).trim() : '';
    if (!filePath) return false;

    const buildInput = (diskRead?: DiskReadResult) => ({
      trigger: opts.trigger,
      selfWrite: opts.selfWrite,
      assisted: opts.assisted,
      probablySelfWrite: opts.probablySelfWrite,
      conflictLocked: isExternalConflictLocked(tab.id),
      promptInFlight: isExternalPromptInFlight(tab.id),
      hasMergeSession: !!getMergeSession(tab.id),
      tabIsDirty: !!tab.isDirty,
      allowAutoReload: opts.allowAutoReload,
      ...(diskRead
        ? {
            diskReadError: !!diskRead.error,
            diskHash: diskRead.hash,
            localHash: hashStringFNV1a32(getCachedMarkdownForTab(tab)),
            lastKnownDiskHash: getDiskStateForTab(tab.id).baselineHash,
          }
        : {}),
    });

    // Primeira passada sem IO: selfWrite, lock, merge session e o fallback de
    // self-write são decididos sem ler o disco.
    let diskRead: DiskReadResult | undefined;
    let decision = decideExternalChange(buildInput());

    if (decision.action === 'defer_read') {
      diskRead = normalizeDiskRead(await (opts.readDisk ? opts.readDisk() : readDiskContent(filePath)));
      decision = decideExternalChange(buildInput(diskRead));
    }

    const openConflictPrompt = (cause: 'external' | 'assisted' = 'external') => {
      setExternalConflictLocked(tab.id, true);
      setDocDirty(tab.id, true);
      if (!isExternalPromptInFlight(tab.id)) {
        void promptResolveExternalChangeForTab(tab.id, filePath, {
          diskContent: diskRead?.content ?? '',
          diskReadError: diskRead?.error ?? '',
          cause,
        });
      }
    };

    switch (decision.action) {
      case 'ignore':
        return false;

      case 'update_baseline': {
        if (decision.scope === 'adopt_local') {
          // Disco e editor já convergiram: adota o local como baseline e limpa o dirty.
          setDiskBaselineForTab(tab.id, getCachedMarkdownForTab(tab));
          setDocDirty(tab.id, false);
        }
        void refreshDiskInfoForTab(tab);
        return false;
      }

      case 'auto_reload': {
        const autoReloadCause = opts.assisted ? ('assisted' as const) : ('external' as const);
        const diskContent = diskRead?.content ?? '';
        // Hash do conteúdo visível ANTES de aplicar o reload (o retorno indica
        // se o conteúdo exibido mudou).
        const visibleHashBeforeReload = hashStringFNV1a32(String(tab.markdown ?? ''));
        try {
          setDocMarkdown(tab.id, diskContent);
          updateLatestMarkdownForTab(tab.id, diskContent);
          setDiskBaselineForTab(tab.id, diskContent);
          setDocDirty(tab.id, false);
          void refreshDiskInfoForTab(tab);
          if (opts.notifyAutoReload && tab.id === currentDocumentId) {
            // Reload assistido é anunciado como alteração do assistente (o
            // usuário acabou de aprovar o diff), não como mudança externa.
            addToast(t(opts.assisted ? 'editor.toast.assistedReloaded' : 'editor.toast.externalReloaded'), 'info');
          }
          const diskHash = diskRead?.hash ?? hashStringFNV1a32(diskContent);
          return diskHash !== visibleHashBeforeReload;
        } catch {
          // Se não der pra aplicar automaticamente, cai pro fluxo de decisão explícita.
          openConflictPrompt(autoReloadCause);
          return false;
        }
      }

      case 'prompt_conflict':
        openConflictPrompt(decision.cause);
        return false;

      default:
        return false;
    }
  };

  const syncAssistedChangeForTab = async (tabId: string) => {
    const id = String(tabId || '');
    if (!sessionLoaded || !id) return false;
    const { documents: currentDocs } = useEditorStore.getState();
    const tab = currentDocs[id] || null;
    if (!tab?.filePath) return false;

    const activeTab = currentDocumentId ? currentDocs[currentDocumentId] || null : null;
    const activePathKey = activeTab?.filePath ? normalizePathKey(String(activeTab.filePath)) : '';
    const tabPathKey = tab.filePath ? normalizePathKey(String(tab.filePath)) : '';
    if (activeTab?.mode === 'rich' && activePathKey && activePathKey === tabPathKey) {
      flushActiveRichMarkdownNow();
    }

    return reconcileExternalChangeForTab(tab, {
      trigger: 'file_changed',
      assisted: true,
      allowAutoReload: true,
      notifyAutoReload: true,
    });
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

      const lastDisk = getDiskStateForTab(tab.id).info;
      const currentDisk = await refreshDiskInfoForTab(tab);
      if (!currentDisk) return;

      if (lastDisk && !diskInfoEquals(lastDisk, currentDisk)) {
        await reconcileExternalChangeForTab(tab, { trigger: 'focus_recheck', notifyAutoReload: true });
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
      const origin = String(data?.origin || '');
      const assisted = data?.assisted === true || origin === 'assistant_tool';
      const selfWrite = data?.selfWrite === true || origin === 'editor_ui';

      const key = normalizePathKey(changedPath);
      if (!key) return;

      const { documents: currentDocs } = useEditorStore.getState();
      const affected = Object.values(currentDocs).filter(
        (tab) => tab.filePath && normalizePathKey(String(tab.filePath)) === key
      );
      if (affected.length === 0) return;

      if (!selfWrite && assisted) {
        const activeTab = currentDocumentId ? currentDocs[currentDocumentId] || null : null;
        const activePathKey = activeTab?.filePath ? normalizePathKey(String(activeTab.filePath)) : '';
        if (activeTab?.mode === 'rich' && activePathKey === key) {
          flushActiveRichMarkdownNow();
        }
      }

      // Fallback defensivo para eventos SEM origin (janela de self-write):
      // repassado ao reconciliador, que só o considera quando não há origin
      // conhecido no evento.
      const probablySelfWrite = !selfWrite && !assisted && isProbablySelfWrite(changedPath);

      // Leitura lazy e compartilhada: o disco é lido no máximo uma vez por
      // evento, e só quando alguma aba realmente precisa comparar conteúdo.
      let sharedRead: Promise<DiskReadResult> | null = null;
      const readDisk = () => {
        if (!sharedRead) sharedRead = readDiskContent(changedPath);
        return sharedRead;
      };

      for (const tab of affected) {
        await reconcileExternalChangeForTab(tab, {
          trigger: 'file_changed',
          selfWrite,
          assisted,
          probablySelfWrite,
          allowAutoReload: true,
          notifyAutoReload: true,
          readDisk,
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
    syncAssistedChangeForTab,
  };
}
