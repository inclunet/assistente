import { useEffect, useRef } from 'react';
import { EditorReadFile } from '@wailsjs/go/app/App';
import { useEditorStore, DEFAULT_MD } from '../../store/editorStore';
import { useWorkspaceStore, type WorkspaceTab } from '../../store/workspaceStore';
import { basenameFromPath } from '../../utils/path';

export function useEditorSurfaceController(tab: WorkspaceTab, isActive: boolean) {
  const isWsInitialized = useWorkspaceStore((state) => state.isInitialized);
  const lastSyncedRef = useRef<string | null>(null);
  const creatingRef = useRef(false);

  useEffect(() => {
    if (!isWsInitialized || !isActive || tab.type !== 'editor') return;

    const tabId = tab.id;
    if (lastSyncedRef.current === tabId) return;

    const store = useEditorStore.getState();
    const exists = !!store.documents[tabId];

    if (exists) {
      if (store.activeDocumentId !== tabId) {
        store.setActiveDocument(tabId);
      }
      lastSyncedRef.current = tabId;
      return;
    }

    if (!creatingRef.current) {
      void createDocumentFromTab(tabId, tab.state);
    }
  }, [isActive, isWsInitialized, tab.id, tab.state, tab.type]);

  useEffect(() => {
    const syncEditorTab = (state: ReturnType<typeof useEditorStore.getState>) => {
      const ws = useWorkspaceStore.getState();
      const wsTab = ws.workspace?.tabs.find((candidate) => candidate.id === tab.id);
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
          console.warn('[EditorSurfaceController] falha ao sincronizar tab', wsTab.id, error);
        });
      }
    };

    syncEditorTab(useEditorStore.getState());

    let prevKey = '';
    const unsub = useEditorStore.subscribe((state) => {
      const doc = state.documents[tab.id];
      const key = doc ? `${tab.id}:${doc.title}:${(doc.filePath as string) || ''}` : '';
      if (key === prevKey) return;
      prevKey = key;
      syncEditorTab(state);
    });

    return unsub;
  }, [tab.id]);

  useEffect(() => () => {
    useEditorStore.getState().removeDocument(tab.id);
  }, [tab.id]);

  async function createDocumentFromTab(tabId: string, state?: Record<string, unknown>) {
    creatingRef.current = true;
    try {
      const filePath = (state?.filePath as string) || '';
      const draftId = (state?.draftId as string) || '';
      let markdown = DEFAULT_MD;

      try {
        if (filePath) {
          const result = await EditorReadFile(filePath);
          markdown = String((result as unknown as { content?: string })?.content ?? result ?? '');
        }
      } catch {
        markdown = DEFAULT_MD;
      }

      const title = filePath ? basenameFromPath(filePath) : 'Novo documento';
      useEditorStore.getState().createDocument({
        id: tabId,
        title,
        markdown,
        filePath: filePath || null,
        draftId: draftId || (filePath ? null : tabId),
      });
      useEditorStore.getState().setActiveDocument(tabId);
      lastSyncedRef.current = tabId;
    } catch (error) {
      console.error('[EditorSurfaceController] Erro ao criar documento:', error);
    } finally {
      creatingRef.current = false;
    }
  }
}
