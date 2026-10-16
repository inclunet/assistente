import { useEffect, useRef, type RefObject } from 'react';
import type { Editor } from '@tiptap/core';
import { registerEditorFormatting } from '../lib/commandEditorFormatting';
import { useAuthStore } from '../store/authStore';
import { useEditorStore } from '../store/editorStore';
import { useWorkspaceStore } from '../store/workspaceStore';

export function useEditorFormattingSurface(root: RefObject<HTMLDivElement | null>, editorRef: RefObject<Editor | null>,
  documentId: string | null, active: boolean, ready: number, asking: boolean) {
  const live = useRef({ documentId, active, asking });
  live.current = { documentId, active, asking };
  useEffect(() => {
    const element = root.current;
    const editor = editorRef.current;
    const user = useAuthStore.getState().user;
    const workspace = useWorkspaceStore.getState().workspace;
    if (!element || !editor || !user || !workspace || !documentId || !active || asking) return;
    const current = () => {
      const auth = useAuthStore.getState();
      const ws = useWorkspaceStore.getState().workspace;
      const doc = useEditorStore.getState().documents[documentId];
      return live.current.active && !live.current.asking && live.current.documentId === documentId &&
        useEditorStore.getState().ownerUserId === user.userId &&
        auth.isAuthenticated && auth.user?.userId === user.userId && auth.user.sessionId === user.sessionId &&
        ws?.id === workspace.id && ws.activeTabId === documentId &&
        ws.tabs.some(tab => tab.id === documentId && tab.type === 'editor') &&
        doc?.mode === 'rich' && !doc.readOnly && !doc.loadError && editorRef.current === editor;
    };
    return registerEditorFormatting({ root: element, editor, isCurrent: current, subscribe(invalidate) {
      // A lease pins the store document too: hydration/replacement must not
      // keep a stale Tiptap selection eligible while React catches up.
      const capturedDocument = useEditorStore.getState().documents[documentId];
      const changed = () => {
        if (!current() || useEditorStore.getState().documents[documentId] !== capturedDocument) invalidate();
      };
      const a = useAuthStore.subscribe(changed);
      const w = useWorkspaceStore.subscribe(changed);
      const e = useEditorStore.subscribe(changed);
      return () => { a(); w(); e(); };
    } });
  }, [root, editorRef, documentId, active, ready, asking]);
}
