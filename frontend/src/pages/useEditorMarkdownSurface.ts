import { useEffect, useRef, type RefObject } from 'react';
import { registerEditorFormatAdapter } from '../lib/commandEditorFormatting';
import { captureEditorMarkdown } from '../lib/commandEditorMarkdown';
import type { MonacoCodeEditor, MonacoNamespace } from './editorTypes';
import { useAuthStore } from '../store/authStore';
import { useEditorStore } from '../store/editorStore';
import { useWorkspaceStore } from '../store/workspaceStore';

export function useEditorMarkdownSurface(
  root: RefObject<HTMLDivElement | null>,
  editorRef: RefObject<MonacoCodeEditor | null>,
  monacoRef: RefObject<MonacoNamespace | null>,
  documentId: string | null,
  active: boolean,
  ready: number,
  asking: boolean,
) {
  const live = useRef({ documentId, active, asking });
  live.current = { documentId, active, asking };

  useEffect(() => {
    const element = root.current;
    const editor = editorRef.current;
    const monaco = monacoRef.current;
    const user = useAuthStore.getState().user;
    const workspace = useWorkspaceStore.getState().workspace;
    const document = documentId ? useEditorStore.getState().documents[documentId] : undefined;
    if (!element || !editor || !monaco || !user || !workspace || !document || !documentId || !active || asking || document.mode !== 'markdown') return;

    let mounted = true;
    const composing = { current: false };
    const compositionEditor = editor as MonacoCodeEditor & {
      onDidCompositionStart?: (listener: () => void) => { dispose(): void };
      onDidCompositionEnd?: (listener: () => void) => { dispose(): void };
    };
    const current = () => {
      const auth = useAuthStore.getState();
      const ws = useWorkspaceStore.getState().workspace;
      const doc = useEditorStore.getState().documents[documentId];
      return live.current.active && !live.current.asking && live.current.documentId === documentId &&
        useEditorStore.getState().ownerUserId === user.userId &&
        auth.isAuthenticated && auth.user?.userId === user.userId && auth.user.sessionId === user.sessionId &&
        ws?.id === workspace.id && ws.activeTabId === documentId &&
        ws.tabs.some(tab => tab.id === documentId && tab.type === 'editor') &&
        mounted && !composing.current && doc?.mode === 'markdown' && !doc.readOnly && !doc.loadError &&
        editorRef.current === editor && monacoRef.current === monaco;
    };

    const disposers = new Set<() => void>();
    const compositionStart = compositionEditor.onDidCompositionStart?.(() => { composing.current = true; });
    const compositionEnd = compositionEditor.onDidCompositionEnd?.(() => { composing.current = false; });
    const unregister = registerEditorFormatAdapter({
      capture(expectedEditor?: object) {
        const currentDocument = useEditorStore.getState().documents[documentId];
        if (expectedEditor !== undefined && expectedEditor !== currentDocument) return undefined;
        if (!current()) return undefined;
        const capturedDocument = currentDocument;
        let dispose: (() => void) | undefined;
        const target = captureEditorMarkdown(editor, monaco, element, current, (invalidate) => {
          const changed = () => {
            if (!current() || useEditorStore.getState().documents[documentId] !== capturedDocument) invalidate();
          };
          const a = useAuthStore.subscribe(changed);
          const w = useWorkspaceStore.subscribe(changed);
          const e = useEditorStore.subscribe(changed);
          return () => { a(); w(); e(); if (dispose) disposers.delete(dispose); };
        }, () => composing.current);
        if (target) { dispose = target.dispose; disposers.add(dispose); }
        return target;
      },
    });
    return () => {
      mounted = false;
      disposers.forEach(dispose => dispose());
      disposers.clear();
      compositionStart?.dispose();
      compositionEnd?.dispose();
      unregister();
    };
  }, [root, editorRef, monacoRef, documentId, active, ready, asking]);
}
