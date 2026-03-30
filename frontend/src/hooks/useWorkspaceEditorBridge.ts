import { useEffect, useRef } from 'react';
import { useWorkspaceStore, registerTabRenameHandler } from '../store/workspaceStore';
import { useEditorStore } from '../store/editorStore';

/**
 * Sincroniza abas de editor do workspace com o editorStore (cache de documentos).
 *
 * - contentId vazio -> cria documento via editorStore.createDocument() e salva id como contentId
 * - contentId existente -> ativa documento via editorStore.setActiveDocument()
 * - contentId existente mas documento sumiu -> recria documento vazio
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
        lastSyncedRef.current = syncKey;
      } else if (!creatingRef.current) {
        console.warn('[WorkspaceEditorBridge] Documento %s não encontrado no store; recriando.', docId);
        createDocumentForWsTab(activeTab.id);
      }
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

  // Sincroniza titulo do editorStore -> workspace tab via handleContentRenamed
  useEffect(() => {
    const unsub = useEditorStore.subscribe((state) => {
      const ws = useWorkspaceStore.getState();
      const wsTabs = ws.workspace?.tabs || [];
      for (const wsTab of wsTabs) {
        if (wsTab.type !== 'editor' || !wsTab.contentId) continue;
        const doc = state.documents[wsTab.contentId];
        if (doc && doc.title !== wsTab.title) {
          ws.handleContentRenamed('editor', wsTab.contentId, doc.title);
        }
      }
    });
    return unsub;
  }, []);

  // F2 tab rename → rename document in editor store
  useEffect(() => {
    return registerTabRenameHandler('editor', (contentId, newTitle) => {
      useEditorStore.getState().renameDocument(contentId, newTitle);
    });
  }, []);

  // Cleanup: remover documento quando aba de editor é definitivamente removida.
  // Só remove se o tab.id desapareceu completamente (não apenas se o contentId está vazio
  // temporariamente — isso pode acontecer durante race conditions com eventos do backend).
  const prevEditorTabsRef = useRef<Map<string, string>>(new Map());
  useEffect(() => {
    const unsub = useWorkspaceStore.subscribe((state) => {
      const wsTabs = state.workspace?.tabs || [];

      const currentEditorTabs = new Map<string, string>();
      const allWsTabIds = new Set<string>();
      for (const t of wsTabs) {
        allWsTabIds.add(t.id);
        if (t.type === 'editor' && t.contentId) {
          currentEditorTabs.set(t.id, t.contentId);
        }
      }

      // Coleta docIds que AINDA são referenciados por alguma aba.
      const activeDocIds = new Set(currentEditorTabs.values());

      for (const [wsTabId, docId] of prevEditorTabsRef.current) {
        // Só remove se a aba sumiu completamente do workspace E o doc não é referenciado por outra.
        if (!allWsTabIds.has(wsTabId) && docId && !activeDocIds.has(docId)) {
          useEditorStore.getState().removeDocument(docId);
        }
      }

      prevEditorTabsRef.current = currentEditorTabs;
    });
    return unsub;
  }, []);
}
