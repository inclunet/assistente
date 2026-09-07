import { logger } from '../../utils/logger';
import { useEffect, useRef } from 'react';
import { EditorReadFile } from '@wailsjs/go/wailsapi/Editor';
import i18next from 'i18next';
import {
  useEditorStore,
  DEFAULT_MD,
  resolveEditorDisplayMode,
} from '../../store/editorStore';
import { useWorkspaceStore, type WorkspaceTab } from '../../store/workspaceStore';
import { basenameFromPath } from '../../utils/path';
import { normalizeEditorDocumentResult } from '../../lib/editorContent';

function isWorkspaceTabActive(tabId: string): boolean {
  return useWorkspaceStore.getState().workspace?.activeTabId === tabId;
}

export function useEditorSurfaceController(tab: WorkspaceTab, isActive: boolean) {
  const isWsInitialized = useWorkspaceStore((state) => state.isInitialized);
  const creatingRef = useRef(false);
  const tabId = tab.id;
  const tabType = tab.type;
  const filePath = (tab.state?.filePath as string) || '';
  const draftId = (tab.state?.draftId as string) || '';

  useEffect(() => {
    if (!isWsInitialized || !isActive || tabType !== 'editor') return;

    const store = useEditorStore.getState();
    const exists = !!store.documents[tabId];

    if (exists) {
      return;
    }

    if (!creatingRef.current) {
      void createDocumentFromTab(tabId, filePath, draftId);
    }
  }, [draftId, filePath, isActive, isWsInitialized, tabId, tabType]);

  useEffect(() => {
    const syncEditorTab = (state: ReturnType<typeof useEditorStore.getState>) => {
      const ws = useWorkspaceStore.getState();
      const wsTab = ws.workspace?.tabs.find((candidate) => candidate.id === tabId);
      if (!wsTab || wsTab.type !== 'editor') return;

      const doc = state.documents[wsTab.id];
      if (!doc) return;

      const updates: Record<string, unknown> = {};
      if (doc.title !== wsTab.title) {
        updates.title = doc.title;
      }

      const wsFilePath = (wsTab.state?.filePath as string) || '';
      const docFilePath = (doc.filePath as string) || '';
      if (docFilePath && docFilePath !== wsFilePath) {
        updates.state = { ...(wsTab.state ?? {}), filePath: docFilePath };
      }

      if (Object.keys(updates).length > 0) {
        ws.updateTab(wsTab.id, updates).catch((error: unknown) => {
          logger.warn('[EditorSurfaceController] falha ao sincronizar tab', wsTab.id, error);
        });
      }
    };

    syncEditorTab(useEditorStore.getState());

    let prevKey = '';
    const unsub = useEditorStore.subscribe((state) => {
      const doc = state.documents[tabId];
      const key = doc ? `${tabId}:${doc.title}:${(doc.filePath as string) || ''}` : '';
      if (key === prevKey) return;
      prevKey = key;
      syncEditorTab(state);
    });

    return unsub;
  }, [tabId]);

  useEffect(() => () => {
    const tabStillOpen = useWorkspaceStore.getState().workspace?.tabs.some((candidate) => candidate.id === tabId) ?? false;
    if (!tabStillOpen) {
      useEditorStore.getState().removeDocument(tabId);
    }
  }, [tabId]);

  async function createDocumentFromTab(tabId: string, filePath: string, draftId: string) {
    creatingRef.current = true;
    try {
      let markdown = DEFAULT_MD;
      let projection: ReturnType<typeof normalizeEditorDocumentResult> | null = null;
      let loadError = false;

      try {
        if (filePath) {
          const result = await EditorReadFile(filePath);
          if (!isWorkspaceTabActive(tabId)) return;
          projection = normalizeEditorDocumentResult(result, filePath);
          markdown = projection.content;
        }
      } catch {
        if (!isWorkspaceTabActive(tabId)) return;
        markdown = '';
        loadError = true;
      }

      if (!isWorkspaceTabActive(tabId)) return;
      if (useEditorStore.getState().documents[tabId]) return;
      const title = filePath ? basenameFromPath(filePath) : i18next.t('editor.fallback.newDoc');
      useEditorStore.getState().createDocument({
        id: tabId,
        title,
        markdown,
        mode: resolveEditorDisplayMode(
          tab.state?.displayMode,
          'markdown',
          (projection?.readOnly ?? false) || loadError,
        ),
        filePath: filePath || null,
        draftId: draftId || (filePath ? null : tabId),
        readOnly: (projection?.readOnly ?? false) || loadError,
        projection: projection?.projected
          ? {
              format: projection.format,
              pages: projection.pages,
              warnings: projection.warnings,
              warningCode: projection.warningCode,
            }
          : null,
        loadError,
        sessionHydrated: false,
      });
    } catch (error) {
      logger.error('[EditorSurfaceController] Erro ao criar documento:', error);
    } finally {
      creatingRef.current = false;
    }
  }
}
