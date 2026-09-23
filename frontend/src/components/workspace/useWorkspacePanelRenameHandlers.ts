import { useEffect } from 'react';
import { useEditorStore } from '../../store/editorStore';
import { useTaskListStore } from '../../store/taskListStore';
import { registerTabRenameHandler } from '../../store/workspaceStore';

export function useWorkspacePanelRenameHandlers() {
  useEffect(() => {
    const unregisterEditor = registerTabRenameHandler('editor', (tabId, newTitle) => {
      useEditorStore.getState().renameDocument(tabId, newTitle);
    });
    const unregisterTaskList = registerTabRenameHandler('tasklist', (id, newTitle) => {
      if (id) {
        // updateTaskList repropaga falha (para callers com feedback); aqui o
        // rename é best-effort e o erro já foi registrado no store.
        void useTaskListStore.getState().updateTaskList(id, newTitle).catch(() => {});
      }
    });

    return () => {
      unregisterEditor();
      unregisterTaskList();
    };
  }, []);
}
