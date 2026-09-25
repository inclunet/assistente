import { useEffect, useRef } from 'react';
import { DOMParser as ProseMirrorDOMParser } from '@tiptap/pm/model';
import type { Content } from '@tiptap/core';
import { registerEditorTransferSurface, EditorTransferError, type EditorTransferTarget } from '../lib/commandEditorTransfer';
import { useAuthStore } from '../store/authStore';
import { useWorkspaceStore } from '../store/workspaceStore';
import { isModalOpen } from '../lib/modalRegistry';
import { ReadFocusContext } from '../lib/commandContextProviders';
import type { RefObject } from 'react';

import { useEditorStore, type EditorDocument } from '../store/editorStore';
import { normalizeEditorInsertContent } from '../lib/editorInsertNormalize';
import { markdownToHtml } from '../lib/markdownToHtml';
import { computeMonacoInsertText } from '../lib/monacoInsertHeuristics';
import type { MonacoCodeEditor, MonacoNamespace, TipTapEditor } from './editorTypes';

interface UseEditorInsertArgs {
  commandSurface: { root: RefObject<HTMLDivElement | null>; active: boolean; asking: boolean };
  activeTab: EditorDocument | null;
  currentDocumentId: string | null;
  sessionLoaded: boolean;
  editorReadyNonce: number;
  editorRef: RefObject<MonacoCodeEditor | null>;
  monacoRef: RefObject<MonacoNamespace | null>;
  richEditorRef: RefObject<TipTapEditor | null>;
  flushActiveRichMarkdownNow: () => void;
}

/** Registers the mounted editor receiver; commands ACK only a real edit. */
export function useEditorInsert({
  commandSurface,
  activeTab,
  currentDocumentId,
  sessionLoaded,
  editorReadyNonce,
  editorRef,
  monacoRef,
  richEditorRef,
  flushActiveRichMarkdownNow,
}: UseEditorInsertArgs) {
  const live = useRef({ commandSurface, currentDocumentId, sessionLoaded, flushActiveRichMarkdownNow });
  live.current = { commandSurface, currentDocumentId, sessionLoaded, flushActiveRichMarkdownNow };
  useEffect(() => {
    const root = commandSurface?.root.current;
    if (!root || !currentDocumentId || !sessionLoaded) return;
    const documentId = currentDocumentId;
    const owner = useAuthStore.getState().user;
    const workspace = useWorkspaceStore.getState().workspace;
    if (!owner || !workspace) return;
    let mounted = true;
    let composing = false;
    const leases = new Set<() => void>();
    const startComposition = () => { composing = true; };
    const endComposition = () => { composing = false; };
    root.addEventListener('compositionstart', startComposition);
    root.addEventListener('compositionend', endComposition);
    const current = () => {
      const auth = useAuthStore.getState();
      const ws = useWorkspaceStore.getState().workspace;
      const doc = useEditorStore.getState().documents[documentId];
      return mounted && root.isConnected && !live.current.commandSurface?.asking &&
        live.current.sessionLoaded && live.current.currentDocumentId === documentId && !composing &&
        ReadFocusContext().composition !== 'active' && auth.isAuthenticated && auth.user?.userId === owner.userId &&
        auth.user.sessionId === owner.sessionId && useEditorStore.getState().ownerUserId === owner.userId &&
        ws?.id === workspace.id && ws.tabs.some(tab => tab.id === documentId && tab.type === 'editor') &&
        !!doc && !doc.readOnly && !doc.loadError && (doc.mode === 'rich' || doc.mode === 'markdown');
    };
    const unregister = registerEditorTransferSurface({ documentId, capture(expectedDocument) {
      const doc = useEditorStore.getState().documents[documentId];
      if (doc && (doc.readOnly || doc.loadError || doc.mode === 'view') || live.current.commandSurface?.asking) throw new EditorTransferError('unavailable');
      if (!current() || !doc || expectedDocument !== undefined && expectedDocument !== doc) throw new EditorTransferError('stale');
      if (doc.sessionHydrated === false) return undefined;
      const editor = editorRef.current;
      const monaco = monacoRef.current;
      const model = editor?.getModel();
      const selection = editor?.getSelection();
      const rich = richEditorRef.current;
      const state = rich?.state;
      if (doc.mode === 'markdown' && editor && monaco && editor.getOption(monaco.editor.EditorOption.readOnly) ||
          doc.mode === 'rich' && rich && !rich.isEditable) throw new EditorTransferError('unavailable');
      if (doc.mode === 'markdown' && (!editor || !monaco || !model || !selection || model.isDisposed() ||
          !editor.getDomNode()?.isConnected || editor.getOption(monaco.editor.EditorOption.readOnly))) return undefined;
      if (doc.mode === 'rich' && (!rich || rich.isDestroyed || !rich.isEditable || !rich.view.dom.isConnected || rich.view.composing)) return undefined;
      const version = model?.getVersionId();
      const range = selection ? { ...selection } : undefined;
      const selectionKey = (value: typeof selection) => value ? [value.selectionStartLineNumber, value.selectionStartColumn, value.positionLineNumber, value.positionColumn].join(':') : '';
      const originalSelection = selectionKey(selection);
      let invalid = false;
      let contextInvalid = false;
      let used = false;
      const off: Array<() => void> = [];
      const invalidate = () => { invalid = true; };
      const isCurrent = () => {
        const valid = !invalid && !used && current() && useEditorStore.getState().documents[documentId] === doc &&
          (doc.mode === 'markdown'
            ? editorRef.current === editor && monacoRef.current === monaco && editor?.getModel() === model && !model?.isDisposed() &&
              model?.getVersionId() === version && selectionKey(editor?.getSelection()) === originalSelection &&
              !!editor?.getDomNode()?.isConnected && !editor.getOption(monaco!.editor.EditorOption.readOnly)
            : richEditorRef.current === rich && !!rich && !rich.isDestroyed && rich.isEditable && rich.view.dom.isConnected && !rich.view.composing && rich.state === state);
        if (!valid) invalidate();
        return !!valid;
      };
      const changed = () => { if (!current()) contextInvalid = true; isCurrent(); };
      off.push(useAuthStore.subscribe(changed), useWorkspaceStore.subscribe(changed), useEditorStore.subscribe(changed));
      root.addEventListener('compositionstart', invalidate); off.push(() => root.removeEventListener('compositionstart', invalidate));
      if (doc.mode === 'markdown') {
        const subscriptions = [model!.onDidChangeContent(invalidate), editor!.onDidChangeCursorSelection(invalidate),
          editor!.onDidChangeModel(invalidate), editor!.onDidDispose(invalidate), editor!.onDidCompositionStart(invalidate),
          editor!.onDidChangeConfiguration(changed)];
        off.push(...subscriptions.map(subscription => () => subscription.dispose()));
      } else {
        rich!.on('transaction', invalidate); rich!.on('destroy', invalidate);
        off.push(() => { rich!.off('transaction', invalidate); rich!.off('destroy', invalidate); });
      }
      const target: EditorTransferTarget = {
        documentId, isCurrent,
        apply(payload) {
          if (!isCurrent() || !live.current.commandSurface?.active || useWorkspaceStore.getState().workspace?.activeTabId !== documentId || isModalOpen() || root.closest('[inert],[hidden],[aria-hidden="true"]')) throw new EditorTransferError('stale');
          const normalized = normalizeEditorInsertContent({ content: payload.content, format: payload.format, targetMode: doc.mode });
          if (!normalized.content) throw new EditorTransferError('unavailable');
          if (doc.mode === 'markdown') {
            const text = computeMonacoInsertText({ hasFocus: true, selectionIsEmpty: selection!.isEmpty(), selectionStart: selection!.getStartPosition(), currentText: model!.getValue(), content: normalized.content }).textToInsert;
            if (model!.getValueInRange(range!) === text) throw new EditorTransferError('unavailable');
            editor!.pushUndoStop();
            if (!isCurrent() || isModalOpen()) throw new EditorTransferError('stale');
            used = true;
            const applied = editor!.executeEdits('chat-command-transfer', [{ range: range!, text, forceMoveMarkers: true }]);
            if (!applied || model!.getVersionId() === version) throw new EditorTransferError(model!.getVersionId() === version ? 'unavailable' : 'outcome_unknown');
            editor!.pushUndoStop();
          } else {
            let content: Content = { type: 'text', text: normalized.content };
            if (normalized.format !== 'plain') {
              const container = document.createElement('div');
              container.innerHTML = normalized.format === 'markdown' ? markdownToHtml(normalized.content) : normalized.content;
              content = ProseMirrorDOMParser.fromSchema(rich!.schema).parseSlice(container).content.toJSON();
            }
            if (!isCurrent() || isModalOpen()) throw new EditorTransferError('stale');
            used = true;
            const applied = rich!.chain().insertContentAt({ from: state!.selection.from, to: state!.selection.to }, content).run();
            if (!applied || rich!.state.doc.eq(state!.doc)) throw new EditorTransferError(rich!.state.doc.eq(state!.doc) ? 'unavailable' : 'outcome_unknown');
            live.current.flushActiveRichMarkdownNow();
          }
          // Presentation follows the confirmed edit, never a delayed generic
          // focus callback that could target another tab or a new modal.
          if (!contextInvalid && current() && live.current.commandSurface.active &&
              useWorkspaceStore.getState().workspace?.activeTabId === documentId && !isModalOpen() &&
              !root.closest('[inert],[hidden],[aria-hidden="true"]')) {
            try {
              if (doc.mode === 'markdown' && editor && editorRef.current === editor && editor.getModel() === model) editor.focus();
              if (doc.mode === 'rich' && richEditorRef.current === rich && rich?.view.dom.isConnected) rich.view.focus();
            } catch { /* Focus failure does not undo or retry a confirmed document edit. */ }
          }
        },
        dispose() { invalid = true; off.splice(0).forEach(dispose => dispose()); leases.delete(target.dispose); },
      };
      leases.add(target.dispose);
      return target;
    } });
    return () => {
      mounted = false; unregister(); leases.forEach(dispose => dispose());
      root.removeEventListener('compositionstart', startComposition); root.removeEventListener('compositionend', endComposition);
    };
  }, [commandSurface?.root, currentDocumentId, sessionLoaded, editorReadyNonce, activeTab?.mode]);

}
