import { useEffect, useRef, useState } from 'react';

import { logger } from '../utils/logger';
import {
  useEditorStore,
  DEFAULT_MD,
  type EditorDocument,
  type EditorMode,
} from '../store/editorStore';
import { useWorkspaceStore } from '../store/workspaceStore';
import { basenameFromPath, normalizePathKey } from '../utils/path';
import { getMaybeContent } from '../lib/editorContent';
import { EditorDeleteDraft, EditorLoadState, EditorReadDraft, EditorReadFile, EditorSaveState } from '@wailsjs/go/app/App';
import type { app } from '@wailsjs/go/models';
import type { UseEditorMergeResult } from './useEditorMerge';

interface UseEditorDocumentArgs {
  merge: UseEditorMergeResult;
  isWsInitialized: boolean;
  currentDocumentId: string | null;
  activeTab: EditorDocument | null;
  allDocs: EditorDocument[];
  documents: Record<string, EditorDocument>;
}

/**
 * Hook responsável pelo ciclo de vida do documento do editor:
 * - restauração da sessão (abas abertas) via workspace + `EditorLoadState`;
 * - persistência das preferências por arquivo (`fileModeByPath`) e merge sessions;
 * - limpeza de drafts órfãos quando abas são fechadas.
 *
 * Mantém o estado bruto (`sessionLoaded`, `fileModeByPathRef`) e expõe
 * `saveEditorState` para que a persistência possa salvar no fechamento.
 */
export function useEditorDocument({
  merge,
  isWsInitialized,
  currentDocumentId,
  activeTab,
  allDocs,
  documents,
}: UseEditorDocumentArgs) {
  const hydrate = useEditorStore((s) => s.hydrate);

  const fileModeByPathRef = useRef<Record<string, EditorMode>>({});
  const prevDocsRef = useRef<Record<string, EditorDocument>>({});

  const [sessionLoaded, setSessionLoaded] = useState(false);

  const {
    updateLatestMarkdownForTab,
    setDiskBaselineForTab,
    refreshDiskInfoForTab,
    setExternalConflictLocked,
    mergeSessionByTabRef,
  } = merge;

  // Salva o estado do editor (fileModeByPath + mergeSessionsByTabId) em disco.
  const saveEditorState = () => {
    try {
      const docs = useEditorStore.getState().documents;
      for (const doc of docs ? Object.values(docs) : []) {
        if (doc.filePath && (doc.mode === 'markdown' || doc.mode === 'rich')) {
          fileModeByPathRef.current[normalizePathKey(String(doc.filePath))] = doc.mode;
        }
      }
    } catch {
      // best-effort
    }
    const payload = {
      fileModeByPath: fileModeByPathRef.current,
      mergeSessionsByTabId: mergeSessionByTabRef.current,
    };
    EditorSaveState(payload as unknown as app.EditorState).catch((e: unknown) => {
      logger.warn('[EditorPage] falha ao salvar estado:', e);
    });
  };

  // Restaura sessão (abas abertas) via workspace YAML + EditorLoadState (arquivo JSON).
  useEffect(() => {
    if (!isWsInitialized) return;
    let cancelled = false;

    (async () => {
      try {
        const wsState = useWorkspaceStore.getState();
        const wsEditorTabs = (wsState.workspace?.tabs || []).filter((tab) => tab.type === 'editor');

        const editorState = await EditorLoadState();
        if (cancelled) return;

        // Preferências por arquivo.
        try {
          const fromState = editorState?.fileModeByPath || {};
          const next: Record<string, EditorMode> = {};
          for (const [k, v] of Object.entries(fromState)) {
            const key = normalizePathKey(String(k || ''));
            if (!key) continue;
            next[key] = v === 'rich' ? 'rich' : 'markdown';
          }
          fileModeByPathRef.current = next;
        } catch {
          // best-effort
        }

        const mergeFromState = editorState?.mergeSessionsByTabId || {};

        const loadedTabs: EditorDocument[] = [];

        for (const tab of wsEditorTabs) {
          const tabId = tab.id;
          const filePath = String(tab.state?.filePath || '').trim();
          const draftId = String(tab.state?.draftId || '').trim();

          const mergeSessRaw = mergeFromState[tabId];
          const hasMergeSess =
            !!mergeSessRaw && typeof mergeSessRaw === 'object' && !!String(mergeSessRaw?.conflictDraftId || '').trim();

          let markdown = '';
          try {
            if (filePath) {
              if (hasMergeSess) {
                const conflictDraftId = String(mergeSessRaw?.conflictDraftId || '').trim();
                const resDraft = await EditorReadDraft(conflictDraftId);
                markdown = getMaybeContent(resDraft);
              } else {
                const res = await EditorReadFile(filePath);
                markdown = getMaybeContent(res);
              }
            }
          } catch {
            markdown = '';
          }

          const pathKey = filePath ? normalizePathKey(filePath) : '';
          const mode: EditorMode = pathKey ? fileModeByPathRef.current[pathKey] || 'markdown' : 'markdown';
          const title = filePath ? basenameFromPath(filePath) : tab.title || 'Novo documento';

          loadedTabs.push({
            id: tabId,
            title,
            markdown: markdown || DEFAULT_MD,
            mode,
            filePath: filePath || null,
            draftId: filePath ? null : draftId || null,
            isDirty: !!hasMergeSess,
          });
        }

        // Popula o cache do autosave com o conteúdo carregado.
        try {
          for (const tab of loadedTabs) {
            updateLatestMarkdownForTab(tab.id, String(tab.markdown ?? ''));
          }
        } catch {
          // best-effort
        }

        // Baseline do disco para arquivos reais (best-effort).
        try {
          for (const tab of loadedTabs) {
            if (tab.filePath) {
              setDiskBaselineForTab(tab.id, String(tab.markdown ?? ''));
              void refreshDiskInfoForTab(tab);
            }
          }
        } catch {
          // best-effort
        }

        const loadedDocs: Record<string, EditorDocument> = {};
        for (const tab of loadedTabs) {
          loadedDocs[tab.id] = tab;
        }

        hydrate({
          documents: loadedDocs,
        });

        // Restaura merge sessions em refs antes de liberar autosave.
        try {
          for (const tab of loadedTabs) {
            if (!tab?.id) continue;
            const raw = mergeFromState[tab.id];
            if (raw && typeof raw === 'object') {
              const conflictDraftId = String(raw?.conflictDraftId || '').trim();
              const mineDraftId = String(raw?.mineDraftId || '').trim();
              const diskDraftId = String(raw?.diskDraftId || '').trim();
              const originalPath = String(raw?.originalPath || tab.filePath || '').trim();
              if (conflictDraftId && mineDraftId && diskDraftId && originalPath) {
                mergeSessionByTabRef.current[String(tab.id)] = {
                  originalPath,
                  mineDraftId,
                  diskDraftId,
                  conflictDraftId,
                  createdAt: Number(raw?.createdAt || Date.now()),
                };
                setExternalConflictLocked(tab.id, true);
              }
            }
          }
        } catch {
          // best-effort
        }

        setSessionLoaded(true);
      } catch {
        setSessionLoaded(true);
      }
    })();

    return () => {
      cancelled = true;
    };  }, [isWsInitialized]);

  // Mantém cache do markdown atual da aba ativa para flush/abas.
  useEffect(() => {
    if (!sessionLoaded) return;
    if (!activeTab) return;
    updateLatestMarkdownForTab(activeTab.id, String(activeTab.markdown ?? ''));  }, [sessionLoaded, activeTab?.id]);

  // Persiste o estado do editor (fileModeByPath + mergeSessionsByTabId).
  useEffect(() => {
    if (!sessionLoaded) return;

    const timer = window.setTimeout(() => {
      saveEditorState();
    }, 500);

    return () => window.clearTimeout(timer);  }, [sessionLoaded, allDocs, currentDocumentId]);

  // Remove drafts órfãos quando abas (sem arquivo) são fechadas.
  useEffect(() => {
    if (!sessionLoaded) return;

    const prev = prevDocsRef.current;
    prevDocsRef.current = documents;

    const removedIds = Object.keys(prev).filter((id) => !documents[id]);
    for (const id of removedIds) {
      const was = prev[id];
      if (!was) continue;
      if (was.filePath) continue;
      const draftId = was.draftId || was.id;
      EditorDeleteDraft(draftId).catch(() => null);
    }  }, [sessionLoaded, documents]);

  return {
    sessionLoaded,
    fileModeByPathRef,
    saveEditorState,
  };
}
