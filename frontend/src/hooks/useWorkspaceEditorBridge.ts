import { useEffect, useRef } from 'react';
import { useWorkspaceStore } from '../store/workspaceStore';
import { useEditorStore } from '../store/editorStore';

/**
 * Sincroniza abas de editor do workspace com o editorStore (cache de documentos).
 *
 * - contentId vazio -> cria documento via editorStore.createDocument() e salva id como contentId
 * - contentId existente -> ativa documento via editorStore.setActiveDocument()
 * - Remocao de aba -> remove documento via editorStore.removeDocument()
 * - Sincroniza titulo de volta (editorStore -> workspace)
 */
export function useWorkspaceEditorBridge() {
  const activeTab = useWorkspaceStore((s) => s.getActiveTab());
  const updateWsTab = useWorkspaceStore((s) => s.updateTab);
  const isWsInitialized = useWorkspaceStore((s) => s.isInitialized);

  const lastSyncedRef = useRef<string | null>(null);
  const creatingRef = useRef(false);

  useEffect(() => {
    if (!isWsInitialized) return;
    if (!activeTab || activeTab.type !== 'editor') return;

    const syncKey = `${activeTab.id}:${activeTab.contentId}`;
    if (lastSyncedRef.current === syncKey) return;

    const docId = activeTab.contentId || '';

    if (docId) {
      const store = useEditorStore.getState();
      const exists = !!store.documents[docId];
      if (exists) {
        if (store.activeDocumentId !== docId) {
          store.setActiveDocument(docId);
        }
      }
      lastSyncedRef.current = syncKey;
    } else if (!creatingRef.current) {
      createDocumentForWsTab(activeTab.id);
    }
  }, [activeTab?.id, activeTab?.type, activeTab?.contentId, isWsInitialized]);

  async function createDocumentForWsTab(wsTabId: string) {
    creatingRef.current = true;
    try {
      const newDocId = useEditorStore.getState().createDocument();
      if (newDocId) {
        await updateWsTab(wsTabId, { content_id: newDocId });
        lastSyncedRef.current = `${wsTabId}:${newDocId}`;
      }
    } catch (error) {
      console.error('[WorkspaceEditorBridge] Erro ao criar documento:', error);
    } finally {
      creatingRef.current = false;
    }
  }

  // Sincroniza titulo do editorStore -> workspace tab
  useEffect(() => {
    const unsub = useEditorStore.subscribe((state) => {
      const ws = useWorkspaceStore.getState();
      const wsTabs = ws.workspace?.tabs || [];
      for (const wsTab of wsTabs) {
        if (wsTab.type !== 'editor' || !wsTab.contentId) continue;
        const doc = state.documents[wsTab.contentId];
        if (doc && doc.title !== wsTab.title) {
          void ws.updateTab(wsTab.id, { title: doc.title });
        }
      }
    });
    return unsub;
  }, []);

  // Cleanup: remover documento quando aba de editor e removida do workspace
  const prevEditorTabsRef = useRef<Map<string, string>>(new Map());
  useEffect(() => {
    const unsub = useWorkspaceStore.subscribe((state) => {
      const wsTabs = state.workspace?.tabs || [];
      const currentEditorTabs = new Map<string, string>();
      for (const t of wsTabs) {
        if (t.type === 'editor' && t.contentId) {
          currentEditorTabs.set(t.id, t.contentId);
        }
      }

      for (const [wsTabId, docId] of prevEditorTabsRef.current) {
        if (!currentEditorTabs.has(wsTabId) && docId) {
          useEditorStore.getState().removeDocument(docId);
        }
      }

      prevEditorTabsRef.current = currentEditorTabs;
    });
    return unsub;
  }, []);
}
