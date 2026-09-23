import { useEffect, useRef, type RefObject } from 'react';
import type { Editor } from '@tiptap/core';
import type { MonacoCodeEditor, MonacoNamespace } from './editorTypes';
import { registerEditorFormatAdapter, type EditorFormatTarget } from '../lib/commandEditorFormatting';
import { appendEditorSlideMarkdown, buildEditorSlideTemplate, isEditorSlideCommand } from '../lib/commandEditorSlideTemplates';
import { isModalOpen } from '../lib/modalRegistry';
import { useAuthStore } from '../store/authStore';
import { useEditorStore } from '../store/editorStore';
import { useWorkspaceStore } from '../store/workspaceStore';

interface Options {
  root: RefObject<HTMLDivElement | null>;
  rich: RefObject<Editor | null>;
  markdown: RefObject<MonacoCodeEditor | null>;
  monaco: RefObject<MonacoNamespace | null>;
  documentId: string | null;
  active: boolean;
  ready: number;
  asking: boolean;
  composing: RefObject<boolean>;
  flushRich(): boolean;
  commitRich(documentId: string, markdown: string): void;
  appended(): void;
}

/** Slides append to the full document, not to the selected rich slide. */
export function useEditorSlideSurface(options: Options) {
  const live = useRef(options);
  live.current = options;
  const { root, rich, markdown, documentId, active, ready, asking } = options;
  useEffect(() => {
    const element = root.current;
    const user = useAuthStore.getState().user;
    const workspace = useWorkspaceStore.getState().workspace;
    if (!element || !user || !workspace || !documentId || !active || asking) return;
    let mounted = true;
    const disposers = new Set<() => void>();
    const current = () => {
      const auth = useAuthStore.getState();
      const ws = useWorkspaceStore.getState().workspace;
      const doc = useEditorStore.getState().documents[documentId];
      return mounted && element.isConnected && live.current.active && !live.current.asking &&
        live.current.documentId === documentId && !live.current.composing.current &&
        auth.isAuthenticated && auth.user?.userId === user.userId && auth.user.sessionId === user.sessionId &&
        useEditorStore.getState().ownerUserId === user.userId && ws?.id === workspace.id && ws.activeTabId === documentId &&
        ws.tabs.some(tab => tab.id === documentId && tab.type === 'editor') &&
        !!doc && !doc.readOnly && !doc.loadError && (doc.mode === 'markdown' || doc.mode === 'rich');
    };
    const unregister = registerEditorFormatAdapter({ capture(expectedEditor) {
      let capturedDoc = useEditorStore.getState().documents[documentId];
      if (!current() || isModalOpen() || (expectedEditor !== undefined && expectedEditor !== capturedDoc)) return undefined;
      const mode = capturedDoc.mode;
      const richEditor = capturedDoc.mode === 'rich' ? rich.current : null;
      const codeEditor = capturedDoc.mode === 'markdown' ? markdown.current : null;
      const model = codeEditor?.getModel();
      const selection = codeEditor?.getSelection();
      const readonlyOption = live.current.monaco.current?.editor.EditorOption.readOnly;
      const codeEditable = () => !!codeEditor && readonlyOption !== undefined && !codeEditor.getOption(readonlyOption);
      if (capturedDoc.mode === 'rich' && (!richEditor || richEditor.isDestroyed || !richEditor.isEditable)) return undefined;
      if (capturedDoc.mode === 'markdown' && (!codeEditable() || !codeEditor || !model || !selection || model.isDisposed() ||
        !codeEditor.getDomNode()?.isConnected)) return undefined;
      const state = richEditor?.state;
      const version = model?.getVersionId();
      let invalid = false;
      let used = false;
      let flushing = false;
      let prepared: string | undefined;
      let content: string | undefined;
      const invalidate = () => { invalid = true; };
      const sameEditor = () => richEditor
        ? rich.current === richEditor && !richEditor.isDestroyed && richEditor.isEditable && !richEditor.view.composing &&
          richEditor.state.doc === state?.doc && richEditor.state.selection.eq(state.selection)
        : !!codeEditor && codeEditable() && markdown.current === codeEditor && codeEditor.getModel() === model &&
          !model?.isDisposed() && model?.getVersionId() === version &&
          !!codeEditor.getSelection() && selection?.equalsSelection(codeEditor.getSelection()!) === true && !!codeEditor.getDomNode()?.isConnected;
      const changed = () => {
        if (!current() || !sameEditor() || useEditorStore.getState().documents[documentId]?.mode !== mode ||
          (!flushing && useEditorStore.getState().documents[documentId] !== capturedDoc)) invalidate();
      };
      const subscriptions = [useAuthStore.subscribe(changed), useWorkspaceStore.subscribe(changed), useEditorStore.subscribe(changed)];
      richEditor?.on('transaction', changed);
      const editorSubscriptions = codeEditor ? [codeEditor.onDidChangeModelContent(invalidate), codeEditor.onDidChangeModel(invalidate),
        codeEditor.onDidChangeCursorSelection(changed), codeEditor.onDidDispose(invalidate), codeEditor.onDidCompositionStart(invalidate)] : [];
      element.addEventListener('compositionstart', invalidate, true);
      const isCurrent = () => {
        changed();
        return !invalid && !used;
      };
      const canExecute = (id: string) => isEditorSlideCommand(id) && isCurrent();
      let disposed = false;
      const dispose = () => {
        if (disposed) return;
        disposed = true;
        invalidate();
        richEditor?.off('transaction', changed);
        subscriptions.forEach(unsubscribe => unsubscribe());
        editorSubscriptions.forEach(subscription => subscription.dispose());
        element.removeEventListener('compositionstart', invalidate, true);
        disposers.delete(dispose);
      };
      disposers.add(dispose);
      const target: EditorFormatTarget = {
        isCurrent, canExecute, dispose,
        async prepare(id, input) {
          if (!canExecute(id) || input !== undefined || isModalOpen()) return false;
          if (richEditor) {
            // Flush only this still-current rich document. Our own synchronous
            // store refresh is allowed; owner/model/selection changes are not.
            flushing = true;
            try { if (!live.current.flushRich()) return false; }
            finally { flushing = false; }
            capturedDoc = useEditorStore.getState().documents[documentId];
          }
          if (!isCurrent()) return false;
          content = buildEditorSlideTemplate(id);
          prepared = content === undefined ? undefined : id;
          return prepared !== undefined;
        },
        execute(id) {
          if (!canExecute(id) || prepared !== id || content === undefined || isModalOpen()) return false;
          const currentMarkdown = model ? model.getValue() : capturedDoc.markdown;
          const next = appendEditorSlideMarkdown(currentMarkdown, content);
          used = true;
          if (codeEditor && model) {
            codeEditor.pushUndoStop();
            const applied = codeEditor.executeEdits('command-slide-insert', [{ range: model.getFullModelRange(), text: next, forceMoveMarkers: true }]);
            codeEditor.pushUndoStop();
            if (!applied) return false;
            const end = model.getPositionAt(next.length);
            codeEditor.setPosition(end);
            codeEditor.revealPositionInCenter(end);
            codeEditor.focus();
          } else {
            live.current.commitRich(documentId, next);
          }
          live.current.appended();
          return true;
        },
      };
      return target;
    } });
    return () => { mounted = false; unregister(); disposers.forEach(dispose => dispose()); };
  }, [root, rich, markdown, documentId, active, ready, asking]);
}
