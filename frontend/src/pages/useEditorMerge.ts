import { useCallback, useRef, useState } from 'react';
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
  buildUnifiedDiff,
  composePreviewText,
  hashStringFNV1a32,
  makeGitStyleConflictText,
  normalizeDiskInfo,
  safeDraftIdPart,
} from '../lib/editorMergeUtils';
import type { MergeSession } from './editorTypes';
import { createEmptyTabDiskState, type TabDiskState } from './editorReconciler';

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

  const setDocMarkdown = useEditorStore((s) => s.setDocMarkdown);
  const setDocFilePath = useEditorStore((s) => s.setDocFilePath);
  const setDocDraftId = useEditorStore((s) => s.setDocDraftId);
  const setDocDirty = useEditorStore((s) => s.setDocDirty);
  const renameDocument = useEditorStore((s) => s.renameDocument);

  // Autosave robusto: mantém a última versão conhecida do markdown por aba.
  const latestMarkdownByTabRef = useRef<Record<string, string>>({});

  // Estado de disco consolidado por aba (metadados + baseline de conteúdo).
  // Única fonte de verdade para o reconciliador de mudanças externas
  // (antes espalhado em diskInfoByTabRef + diskContentHashByTabRef +
  // diskBaselineContentByTabRef).
  const diskStateByTabRef = useRef<Record<string, TabDiskState>>({});
  // Sequência por aba que ordena escritas em TabDiskState.info: cada refresh
  // assíncrono captura a sequência ao iniciar e descarta o resultado se outro
  // refresh/gravação mais novo aconteceu no meio (um `stat` antigo não pode
  // sobrescrever um mais novo — mtime não serve de guarda porque pode regredir
  // legitimamente, ex.: restauração de arquivo).
  const diskInfoSeqByTabRef = useRef<Record<string, number>>({});
  const externalConflictLockedByTabRef = useRef<Record<string, boolean>>({});
  const lastSelfWriteAtByPathRef = useRef<Record<string, number>>({});
  const mergeSessionByTabRef = useRef<Record<string, MergeSession>>({});
  const externalPromptInFlightByTabRef = useRef<Record<string, boolean>>({});

  // O estado de lock externo e de merge session vive em refs (lidos de forma
  // síncrona dentro de efeitos/timers sem recriar dependências). Refs não
  // disparam re-render, então mantemos um contador reativo que é incrementado
  // sempre que esse estado muda. Consumidores de UI (ex.: menu Arquivo) podem
  // incluí-lo nas dependências de useMemo para recomputar quando lock/merge
  // mudarem sem que `activeTab` tenha mudado.
  const [mergeStateRevision, setMergeStateRevision] = useState(0);
  const bumpMergeStateRevision = useCallback(() => {
    setMergeStateRevision((r) => r + 1);
  }, []);

  const getMergeSession = (tabId: string): MergeSession | null => {
    const id = String(tabId || '');
    if (!id) return null;
    return mergeSessionByTabRef.current[id] || null;
  };

  // markSelfWrite/isProbablySelfWrite são apenas um fallback defensivo por
  // janela de tempo: a supressão principal de eventos da própria gravação é
  // feita no backend, que marca EditorWriteFile/EditorWriteDraft por token e
  // emite `editor:fileChanged` com origin 'editor_ui' + selfWrite. Este
  // fallback só atua em eventos SEM origin (ex.: duplicados do SO que chegam
  // após o TTL da marcação).
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
    const id = String(tabId || '');
    if (!id) return;
    const next = !!locked;
    const prev = !!externalConflictLockedByTabRef.current[id];
    externalConflictLockedByTabRef.current[id] = next;
    if (prev !== next) bumpMergeStateRevision();
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

  const ensureDiskStateForTab = (tabId: string): TabDiskState => {
    const id = String(tabId || '');
    let state = diskStateByTabRef.current[id];
    if (!state) {
      state = createEmptyTabDiskState();
      diskStateByTabRef.current[id] = state;
    }
    return state;
  };

  /** Snapshot (somente leitura) do estado de disco conhecido da aba. */
  const getDiskStateForTab = (tabId: string): TabDiskState => {
    return diskStateByTabRef.current[String(tabId || '')] ?? createEmptyTabDiskState();
  };

  /** Invalida refreshes em voo da aba e devolve a nova sequência. */
  const bumpDiskInfoSeqForTab = (tabId: string): number => {
    const id = String(tabId || '');
    const next = (diskInfoSeqByTabRef.current[id] || 0) + 1;
    diskInfoSeqByTabRef.current[id] = next;
    return next;
  };

  const setDiskInfoForTab = (tabId: string, info: DiskInfo | null) => {
    // Escrita direta é a verdade mais recente: invalida refreshes em voo.
    bumpDiskInfoSeqForTab(tabId);
    ensureDiskStateForTab(tabId).info = info;
  };

  const refreshDiskInfoForTab = async (tab: EditorDocument): Promise<DiskInfo | null> => {
    const filePath = tab?.filePath ? String(tab.filePath) : '';
    if (!filePath) return null;
    // Reivindica um slot na sequência ANTES do IO: se outro refresh/gravação
    // mais novo acontecer durante o await, este resultado é descartado.
    const seq = bumpDiskInfoSeqForTab(tab.id);
    try {
      const di = normalizeDiskInfo(await EditorGetFileInfo(filePath));
      if (diskInfoSeqByTabRef.current[String(tab.id || '')] !== seq) {
        // Resultado descartado: devolve o info mais novo já aplicado ao
        // estado, para o chamador não decidir com um stat que não foi adotado.
        return getDiskStateForTab(tab.id).info;
      }
      ensureDiskStateForTab(tab.id).info = di;
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
    // O baseline muda quando o fluxo de save acabou de gravar: um `stat` lido
    // antes dessa gravação está desatualizado e não pode vencer a corrida.
    bumpDiskInfoSeqForTab(tabId);
    const state = ensureDiskStateForTab(tabId);
    state.baselineHash = hashStringFNV1a32(content);
    state.baselineContent = String(content ?? '');
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
    bumpMergeStateRevision();

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
    bumpMergeStateRevision();
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
      const localPreviewText = composePreviewText(localContent, t);

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
        : composePreviewText(diskContent, t);

      const diffText = diskReadError ? '' : buildUnifiedDiff(diskContent, localContent);
      const diffPreviewText = diffText ? composePreviewText(diffText, t, 30000) : '';

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
            // "Manter minha versão" como padrão: um Enter afobado não pode
            // descartar a digitação recente do usuário (crítico com leitor de
            // telas, onde o diálogo abre no meio da digitação).
            default: t('editor.options.useMine'),
          },
        ],
      });

      if (resp.cancelled) {
        // Re-checa uma vez antes de manter o lock: se disco e local
        // convergiram enquanto o questionário estava aberto (mesmo
        // silent-resolve do início da função), desfaz o lock em vez de deixar
        // o autosave morto.
        try {
          const { documents: nowDocs } = useEditorStore.getState();
          const nowTab = nowDocs[tabId] || tab;
          const latestLocal = getCachedMarkdownForTab(nowTab);
          const diskNow = String((await EditorReadFile(filePath)) || '');
          if (diskNow === latestLocal) {
            setDiskBaselineForTab(tabId, latestLocal);
            setDocDirty(tabId, false);
            void refreshDiskInfoForTab(nowTab);
            setExternalConflictLocked(tabId, false);
            return;
          }
        } catch {
          // best-effort: sem leitura, mantém o lock (comportamento seguro)
        }
        // Mantém o lock, mas avisa explicitamente (toast + anúncio assertivo
        // via addToast) que o autosave fica pausado até o usuário decidir.
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
    // Contador reativo que muda quando o lock externo ou a merge session mudam.
    mergeStateRevision,

    // Refs compartilhadas (somente leitura/escrita pontual por outros hooks).
    latestMarkdownByTabRef,
    diskStateByTabRef,
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
    getDiskStateForTab,
    setDiskInfoForTab,
    refreshDiskInfoForTab,
    setDiskBaselineForTab,

    // Ciclo de vida de merge + resolução de conflito.
    startMergeSessionForTab,
    cleanupMergeSessionForTab,
    promptResolveExternalChangeForTab,
  };
}

export type UseEditorMergeResult = ReturnType<typeof useEditorMerge>;
