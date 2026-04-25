import { useEffect, useRef } from 'react';
import { useWorkspaceStore, useActiveTab, registerTabRenameHandler } from '../store/workspaceStore';
import { useEditorStore, DEFAULT_MD } from '../store/editorStore';
import { EditorReadFile } from '@wailsjs/go/app/App';
import { basenameFromPath } from '../utils/path';

/**
 * Sincroniza abas de editor do workspace com o editorStore.
 *
 * Modelo novo: o workspace tab.id é o doc.id no editorStore.
 *   - tab.state.filePath → caminho em disco (draft ou arquivo real)
 *   - tab.state.draftId  → UUID do draft (se for rascunho)
 *
 * - Aba ativa sem doc no store → cria doc lendo filePath
 * - Aba ativa com doc existente → setActiveDocument
 * - Remoção de aba → removeDocument
 * - Título do editorStore → workspace tab via updateTab
 * - F2 rename → renameDocument no editorStore
 */
export function useWorkspaceEditorBridge() {
  const activeTab = useActiveTab();
  const isWsInitialized = useWorkspaceStore((s) => s.isInitialized);

  const lastSyncedRef = useRef<string | null>(null);
  const creatingRef = useRef(false);

  useEffect(() => {
    if (!isWsInitialized) return;
    if (!activeTab || activeTab.type !== 'editor') return;

    const tabId = activeTab.id;
    const syncKey = tabId;
    if (lastSyncedRef.current === syncKey) return;

    const store = useEditorStore.getState();
    const exists = !!store.documents[tabId];

    if (exists) {
      if (store.activeDocumentId !== tabId) {
        store.setActiveDocument(tabId);
      }
      lastSyncedRef.current = syncKey;
    } else if (!creatingRef.current) {
      createDocFromWsTab(tabId, activeTab.state);
    }
  }, [activeTab?.id, activeTab?.type, isWsInitialized]);

  async function createDocFromWsTab(tabId: string, state?: Record<string, unknown>) {
    creatingRef.current = true;
    try {
      const filePath = (state?.filePath as string) || '';
      const draftId = (state?.draftId as string) || '';
      let markdown = DEFAULT_MD;
      try {
        if (filePath) {
          const res = await EditorReadFile(filePath);
          markdown = String((res as unknown as { content?: string })?.content ?? res ?? '');
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
      console.error('[WorkspaceEditorBridge] Erro ao criar documento:', error);
    } finally {
      creatingRef.current = false;
    }
  }

  // Sincroniza título e filePath do editorStore → workspace tab
  useEffect(() => {
    const syncEditorToWs = (state: ReturnType<typeof useEditorStore.getState>) => {
      const ws = useWorkspaceStore.getState();
      const wsTabs = ws.workspace?.tabs || [];
      for (const wsTab of wsTabs) {
        if (wsTab.type !== 'editor') continue;
        const doc = state.documents[wsTab.id];
        if (!doc) continue;
        const updates: Record<string, unknown> = {};
        if (doc.title !== wsTab.title) {
          updates.title = doc.title;
        }
        // Sincroniza filePath: se o doc tem filePath mas a aba não, propaga para o backend
        const wsFilePath = (wsTab.state?.filePath as string) || '';
        const docFilePath = (doc.filePath as string) || '';
        if (docFilePath && docFilePath !== wsFilePath) {
          updates.state = { ...(wsTab.state ?? {}), filePath: docFilePath };
        }
        if (Object.keys(updates).length > 0) {
          ws.updateTab(wsTab.id, updates).catch((err: unknown) => {
            console.warn('[WorkspaceEditorBridge] falha ao sincronizar tab', wsTab.id, err);
          });
        }
      }
    };

    // Sync inicial: propaga filePath que já existe no editorStore mas não no workspace
    syncEditorToWs(useEditorStore.getState());

    const unsub = useEditorStore.subscribe(syncEditorToWs);
    return unsub;
  }, []);

  // F2 tab rename → rename document in editor store
  useEffect(() => {
    return registerTabRenameHandler('editor', (tabId, newTitle) => {
      useEditorStore.getState().renameDocument(tabId, newTitle);
    });
  }, []);

  // Cleanup: remover documento quando aba de editor é removida
  const prevEditorTabsRef = useRef<Set<string>>(new Set());
  useEffect(() => {
    const unsub = useWorkspaceStore.subscribe((state) => {
      const wsTabs = state.workspace?.tabs || [];
      const currentTabIds = new Set<string>();
      for (const t of wsTabs) {
        if (t.type === 'editor') {
          currentTabIds.add(t.id);
        }
      }

      for (const tabId of prevEditorTabsRef.current) {
        if (!currentTabIds.has(tabId)) {
          useEditorStore.getState().removeDocument(tabId);
        }
      }

      prevEditorTabsRef.current = currentTabIds;
    });
    return unsub;
  }, []);
}
