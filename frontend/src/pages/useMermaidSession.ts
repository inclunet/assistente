import { useCallback, useEffect, useRef, useState, type RefObject } from 'react';
import { flushSync } from 'react-dom';
import { useTranslation } from 'react-i18next';
import { useUIStore } from '../store/uiStore';
import { useEditorStore, type EditorDocument } from '../store/editorStore';
import { useAuthStore } from '../store/authStore';
import { useWorkspaceStore } from '../store/workspaceStore';
import { requestConfirm, useConfirmStore } from '../store/confirmStore';
import { getModalRegistrySnapshot, isModalOpen } from '../lib/modalRegistry';
import { ReadFocusContext } from '../lib/commandContextProviders';
import { findMermaidFenceByIndex, removeMermaidFence, replaceMermaidFenceCode } from '../lib/mermaidFence';
import { registerEditorMermaidSurface, requestEditorMermaidCommand, type EditorMermaidInput, type EditorMermaidTarget } from '../lib/commandEditorMermaid';
import type { RichTextEditorHandle } from '../components/editor/RichTextEditor';
import { findMermaidNodeById } from '../components/editor/richMermaidById';
import type { MonacoCodeEditor, MonacoNamespace, TipTapEditor } from './editorTypes';

export interface RichMermaidEditRequest { mermaidBlockId?: string; code?: string; insertText?: string; expectedEditor?: object }
interface UseMermaidSessionArgs {
  activeTab: EditorDocument | null;
  rootRef: RefObject<HTMLDivElement | null>;
  active: boolean;
  asking: boolean;
  editorRef: RefObject<MonacoCodeEditor | null>;
  monacoRef: RefObject<MonacoNamespace | null>;
  richEditorRef: RefObject<TipTapEditor | null>;
  richEditorHandleRef: RefObject<RichTextEditorHandle | null>;
  setDocMarkdown: (id: string, markdown: string) => void;
  updateLatestMarkdownForTab: (id: string, markdown: string) => void;
  schedulePersistForTab: (id: string) => void;
}
interface Session {
  key: string; code: string; initialCode: string; revision: number; insertText: string;
  current(): boolean; apply(code: string | undefined): boolean; focus(): void; ownsFocus(): boolean; dispose(): void;
}
let sequence = 0;
function settleModal(): Promise<void> {
  return new Promise(resolve => {
    setTimeout(() => {
      let first = 0; let second = 0;
      const finish = () => { clearTimeout(fallback); cancelAnimationFrame(first); cancelAnimationFrame(second); resolve(); };
      const fallback = setTimeout(finish, 100);
      first = requestAnimationFrame(() => { second = requestAnimationFrame(finish); });
    }, 0);
  });
}
export function useMermaidSession(args: UseMermaidSessionArgs) {
  const { t } = useTranslation();
  const live = useRef(args); live.current = args;
  const [session, setSession] = useState<Session | null>(null);
  const sessionRef = useRef<Session | null>(null);
  const modalId = useRef<string | null>(null);
  const setMermaidModalId = useCallback((id: string | null) => { modalId.current = id; }, []);
  const mounted = useRef(true);
  const snapshots = useRef(new Set<() => void>());
  const leases = useRef(new Set<() => void>());
  const close = (captured: Session) => {
    if (sessionRef.current !== captured) return false;
    sessionRef.current = null; setSession(null); return true;
  };
  const ownsTopmost = (captured: Session) => {
    const marker = [...document.querySelectorAll<HTMLElement>('[data-mermaid-command-scope]')]
      .find(element => element.dataset.mermaidCommandScope === captured.key);
    const id = marker?.closest<HTMLElement>('.modal-overlay')?.dataset.modalId;
    return !!id && id === modalId.current && getModalRegistrySnapshot().topID === id;
  };
  const captureBlock = (input?: EditorMermaidInput): Session | undefined => {
    const a = live.current; const doc = a.activeTab; const root = a.rootRef.current;
    const auth = useAuthStore.getState(); const ws = useWorkspaceStore.getState().workspace;
    if (!doc || !root?.isConnected || !document.hasFocus() || !a.active || a.asking || doc.readOnly || doc.loadError || doc.sessionHydrated === false ||
        !auth.isAuthenticated || !auth.user || !ws || ws.activeTabId !== doc.id || useEditorStore.getState().ownerUserId !== auth.user.userId ||
        useEditorStore.getState().documents[doc.id] !== doc || ReadFocusContext().composition === 'active') return;
    const owner = auth.user; const editor = a.editorRef.current; const monaco = a.monacoRef.current;
    const pathname = window.location.pathname;
    const rich = a.richEditorRef.current; const model = editor?.getModel(); const richDoc = rich?.state.doc;
    if (input?.expectedEditor && (doc.mode !== 'rich' || rich !== input.expectedEditor)) return;
    const handle = a.richEditorHandleRef.current;
    let index = input?.index;
    let blockPosition: number | undefined; let blockSize = 0; let code = '';
    if (doc.mode === 'rich') {
      if (!rich || rich.isDestroyed || !rich.isEditable || rich.view.composing || !rich.view.dom.isConnected) return;
      const matches: Array<{ pos: number; size: number; code: string }> = [];
      const explicit = input?.mermaidBlockId ? findMermaidNodeById(rich, input.mermaidBlockId) : null;
      if (input?.mermaidBlockId && !explicit) return;
      rich.state.doc.descendants((node, pos) => {
        if (node.type.name === 'codeBlock' && node.attrs.language === 'mermaid' &&
            (explicit ? pos === explicit.pos : rich.state.selection.from >= pos && rich.state.selection.to <= pos + node.nodeSize)) {
          matches.push({ pos, size: node.nodeSize, code: node.textContent });
        }
      });
      if (matches.length !== 1) return;
      blockPosition = matches[0].pos; blockSize = matches[0].size; code = matches[0].code;
    } else {
      if (doc.mode !== 'view' && (!editor || !monaco || !model || model.isDisposed() || !editor.getDomNode()?.isConnected || editor.getOption(monaco.editor.EditorOption.readOnly))) return;
      const markdown = doc.mode === 'view' ? doc.markdown : model!.getValue();
      if (index === undefined && doc.mode === 'view') {
        const focused = document.activeElement?.closest<HTMLElement>('[data-mermaid-index]');
        if (focused && root.contains(focused) && /^\d+$/.test(focused.dataset.mermaidIndex ?? '')) index = Number(focused.dataset.mermaidIndex);
      }
      if (index === undefined && doc.mode === 'markdown') {
        const position = editor!.getPosition(); if (!position) return;
        const offset = model!.getOffsetAt(position);
        for (let i = 0; ; i++) { const fence = findMermaidFenceByIndex(markdown, i); if (!fence) break;
          if (offset >= fence.fenceStartOffset && offset <= fence.fenceEndOffset) { index = i; break; }
        }
      }
      if (!Number.isInteger(index) || index! < 0) return;
      const fence = findMermaidFenceByIndex(markdown, index!); if (!fence) return; code = fence.code;
    }
    const markdown = doc.mode === 'markdown' ? model!.getValue() : doc.markdown;
    const version = doc.mode === 'markdown' ? model!.getVersionId() : undefined;
    const viewBlock = doc.mode === 'view' ? [...root.querySelectorAll<HTMLElement>('[data-mermaid-index]')]
      .find(element => element.dataset.mermaidIndex === String(index)) : undefined;
    if (doc.mode === 'view' && !viewBlock) return;
    let invalid = false; let used = false;
    const off: Array<() => void> = [];
    const current = () => {
      const now = live.current; const user = useAuthStore.getState(); const workspace = useWorkspaceStore.getState().workspace;
      const valid = !invalid && !used && mounted.current && now.active && !now.asking && root.isConnected && document.hasFocus() &&
        now.activeTab?.id === doc.id && window.location.pathname === pathname && useEditorStore.getState().documents[doc.id] === doc && !doc.readOnly && !doc.loadError &&
        user.isAuthenticated && user.user?.userId === owner.userId && user.user.sessionId === owner.sessionId &&
        useEditorStore.getState().ownerUserId === owner.userId && workspace?.id === ws.id && workspace.activeTabId === doc.id &&
        workspace.tabs.some(tab => tab.id === doc.id && tab.type === 'editor') && ReadFocusContext().composition !== 'active' &&
        (doc.mode === 'rich' ? now.richEditorRef.current === rich && now.richEditorHandleRef.current === handle && !!rich && !rich.isDestroyed && rich.isEditable && !rich.view.composing && rich.view.dom.isConnected && rich.state.doc === richDoc :
          doc.mode === 'view' ? !!viewBlock?.isConnected && root.contains(viewBlock) : now.editorRef.current === editor && editor?.getModel() === model && !model?.isDisposed() && model?.getVersionId() === version &&
          !!editor?.getDomNode()?.isConnected && !editor.getOption(monaco!.editor.EditorOption.readOnly));
      if (!valid) invalid = true; return !!valid;
    };
    const invalidate = () => { invalid = true; };
    const observe = () => { current(); };
    off.push(useAuthStore.subscribe(observe), useEditorStore.subscribe(observe), useWorkspaceStore.subscribe(observe));
    root.addEventListener('compositionstart', invalidate); off.push(() => root.removeEventListener('compositionstart', invalidate));
    window.addEventListener('popstate', observe); off.push(() => window.removeEventListener('popstate', observe));
    if (doc.mode === 'markdown') {
      const subscriptions = [model!.onDidChangeContent(invalidate), editor!.onDidChangeModel(invalidate), editor!.onDidDispose(invalidate), editor!.onDidCompositionStart(invalidate), editor!.onDidChangeConfiguration(observe)];
      off.push(...subscriptions.map(subscription => () => subscription.dispose()));
    } else if (doc.mode === 'rich') {
      rich!.on('transaction', observe); rich!.on('destroy', invalidate);
      off.push(() => { rich!.off('transaction', observe); rich!.off('destroy', invalidate); });
    }
    const captured: Session = {
      key: 'mermaid-' + ++sequence, code, initialCode: code, revision: 0, insertText: input?.insertText ?? '', current,
      ownsFocus() {
        const control = doc.mode === 'rich' ? rich?.view.dom : doc.mode === 'markdown' ? editor?.getDomNode() : viewBlock;
        return !!control && current() && !isModalOpen() && !root.closest('[hidden],[inert],[aria-hidden="true"]') && control.contains(document.activeElement);
      },
      focus() {
        if (!current() || isModalOpen() || root.closest('[hidden],[inert],[aria-hidden="true"]')) return;
        if (doc.mode === 'rich') rich!.view.focus(); else if (doc.mode === 'markdown') editor!.focus(); else viewBlock!.focus();
      },
      apply(nextCode) {
        if (!current() || isModalOpen()) return false;
        const removing = nextCode === undefined;
        if (!removing && nextCode === code) return false;
        used = true;
        if (doc.mode === 'rich') {
          const applied = rich!.commands.command(({ tr, state }) => {
            if (removing) tr.delete(blockPosition!, blockPosition! + blockSize);
            else tr.replaceWith(blockPosition! + 1, blockPosition! + blockSize - 1, nextCode ? state.schema.text(nextCode) : []);
            return true;
          });
          if (!applied || rich!.state.doc.eq(richDoc!)) return false;
          handle?.flushMarkdown();
        } else {
          const fence = findMermaidFenceByIndex(markdown, index!)!;
          const next = removing ? removeMermaidFence(markdown, fence) : replaceMermaidFenceCode(markdown, fence, nextCode!);
          if (next === markdown) return false;
          if (doc.mode === 'markdown') {
            editor!.pushUndoStop();
            const applied = editor!.executeEdits('mermaid-command', [{ range: model!.getFullModelRange(), text: next }]);
            if (!applied || model!.getVersionId() === version) return false;
            editor!.pushUndoStop();
          } else {
            a.setDocMarkdown(doc.id, next);
            if (useEditorStore.getState().documents[doc.id]?.markdown !== next) return false;
            a.updateLatestMarkdownForTab(doc.id, next); a.schedulePersistForTab(doc.id);
          }
        }
        return true;
      },
      dispose() { invalid = true; off.splice(0).forEach(unsubscribe => unsubscribe()); snapshots.current.delete(captured.dispose); },
    };
    snapshots.current.add(captured.dispose); return captured;
  };
  const capture = (id: string, expectedDocument?: object, input?: EditorMermaidInput): EditorMermaidTarget | undefined => {
    if (expectedDocument && live.current.activeTab !== expectedDocument) return;
    const opening = id === 'editor.mermaid.open';
    const directRemove = id === 'editor.mermaid.remove' && !sessionRef.current && !isModalOpen();
    if (opening && (isModalOpen() || sessionRef.current)) return;
    const block = opening || directRemove ? captureBlock(input) : sessionRef.current;
    if (!block || !block.current() || !opening && !directRemove && !ownsTopmost(block)) return;
    const revision = block.revision;
    const code = input?.code ?? block.code;
    if (typeof code !== 'string') return;
    let invalid = false; let prepared = opening; let executed = false; let transferred = false; let closed = directRemove;
    let cancelConfirmation: (() => void) | undefined;
    let generation = getModalRegistrySnapshot().generationNumber;
    const current = () => {
      const valid = !invalid && !executed && block.current() && block.revision === revision &&
        (closed || opening || sessionRef.current === block);
      if (!valid) invalid = true; return valid;
    };
    const target: EditorMermaidTarget = {
      isCurrent: current,
      hasPreparedFocus: () => prepared && closed && !opening && current() && block.ownsFocus(),
      canExecute(command) { return command === id && current() && getModalRegistrySnapshot().generationNumber === generation &&
        (opening || closed ? !isModalOpen() : ownsTopmost(block)) && (id !== 'editor.mermaid.apply' || code !== block.initialCode); },
      async prepare(command) {
        if (!target.canExecute(command)) return false;
        if (opening) return true;
        if (!directRemove) {
          flushSync(() => { closed = close(block); });
          if (!closed) return false;
          await settleModal();
          const after = getModalRegistrySnapshot();
          if (!current() || after.ids.length || after.generationNumber !== generation + 1) return false;
          generation = after.generationNumber;
        }
        if (id === 'editor.mermaid.remove') {
          if (useConfirmStore.getState().active) return false;
          const answer = requestConfirm({ title: t('editor.mermaid.removeConfirmTitle'), message: t('editor.mermaid.removeConfirmMessage'), confirmText: t('editor.mermaid.removeBtn'), cancelText: t('common.cancel'), variant: 'danger' });
          const confirmationId = useConfirmStore.getState().active?.id;
          cancelConfirmation = () => { if (useConfirmStore.getState().active?.id === confirmationId) useConfirmStore.getState().cancel(); };
          const deadline = Date.now() + 300_000;
          const monitor = setInterval(() => { if (!current() || Date.now() >= deadline) { invalid = true; cancelConfirmation?.(); } }, 20);
          let confirmed: boolean;
          try { confirmed = await answer; } finally { clearInterval(monitor); cancelConfirmation = undefined; }
          await settleModal();
          const returned = getModalRegistrySnapshot();
          if (!current() || returned.ids.length || returned.generationNumber !== generation + 2) return false;
          generation = returned.generationNumber;
          if (!confirmed) { block.focus(); return false; }
        }
        if (!current()) return false;
        block.focus(); prepared = true; return target.canExecute(command);
      },
      execute(command) {
        if (!prepared || !target.canExecute(command)) return false;
        executed = true;
        if (opening) { transferred = true; sessionRef.current = block; setSession(block); return true; }
        const applied = block.apply(id === 'editor.mermaid.remove' ? undefined : code);
        if (applied) useUIStore.getState().addToast(t(id === 'editor.mermaid.remove' ? 'editor.toast.mermaidRemoved' : 'editor.toast.mermaidUpdated'), 'success');
        return applied;
      },
      dispose() { invalid = true; cancelConfirmation?.(); leases.current.delete(target.dispose); if (!transferred && (opening || closed)) block.dispose(); },
    };
    leases.current.add(target.dispose); return target;
  };
  const captureRef = useRef(capture); captureRef.current = capture;
  useEffect(() => {
    mounted.current = true;
    const unregister = registerEditorMermaidSurface({ documentId: args.activeTab?.id ?? '', capture: (...params) => captureRef.current(...params) });
    return () => { mounted.current = false; unregister(); leases.current.forEach(dispose => dispose()); snapshots.current.forEach(dispose => dispose()); };
  }, [args.activeTab?.id]);
  const request = (id: 'editor.mermaid.open' | 'editor.mermaid.apply' | 'editor.mermaid.remove', input?: EditorMermaidInput) =>
    requestEditorMermaidCommand(id, input, live.current.activeTab ?? undefined);
  return {
    openMermaidEditorByIndex: (index: number, options?: { insertText?: string }) => request('editor.mermaid.open', { index, ...options }),
    requestEditRichMermaid: (ctx: RichMermaidEditRequest) => request('editor.mermaid.open', { mermaidBlockId: ctx.mermaidBlockId, insertText: ctx.insertText, expectedEditor: ctx.expectedEditor }),
    removeMermaidBlockByIndex: (index: number) => request('editor.mermaid.remove', { index }),
    cancelMermaidModal: () => {
      const captured = sessionRef.current;
      if (!captured || !ownsTopmost(captured)) return;
      const generation = getModalRegistrySnapshot().generationNumber;
      close(captured);
      void settleModal().then(() => {
        try { if (getModalRegistrySnapshot().generationNumber === generation + 1) captured.focus(); }
        finally { captured.dispose(); }
      }).catch(() => captured.dispose());
    },
    isMermaidModalOpen: !!session,
    mermaidModalSessionKey: session?.key ?? '',
    setMermaidModalId,
    mermaidModalTitle: t('editor.modal.mermaidTitle'),
    mermaidModalInitialCode: session?.initialCode ?? '',
    mermaidModalInitialInsertText: session?.insertText ?? '',
    consumeMermaidInsertText: () => { if (sessionRef.current) sessionRef.current.insertText = ''; },
    updateMermaidDraft: (code: string) => { const current = sessionRef.current; if (current && current.code !== code) { current.code = code; current.revision++; } },
  };
}
