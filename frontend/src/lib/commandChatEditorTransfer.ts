import { flushSync } from 'react-dom';
import { useAuthStore } from '../store/authStore';
import { useWorkspaceStore } from '../store/workspaceStore';
import { useWorkspaceChatModalStore } from '../store/workspaceChatModalStore';
import { useChatStore, type Message } from '../store/chatStore';
import { useEditorStore } from '../store/editorStore';
import { getModalRegistrySnapshot } from './modalRegistry';
import { ReadFocusContext } from './commandContextProviders';
import { ChatMessagingStaleError, type PreparedChatMessagingTarget } from './commandChatMessaging';
import { captureEditorTransferDestination, transferEditorContent, type EditorTransferTarget } from './commandEditorTransfer';
import { prepareChatEditorCommand, openChatEditorCommand, validateChatEditorCommand, type ChatEditorPlan } from './commandChatEditorWails';
import type { SendToEditorPayload } from './editorSendMenu';

export const CHAT_EDITOR_COMMAND = 'chat.message.send_to_editor';

/** The source lease ends at the authorized transition; the destination lease
 * then owns the effect. Neither lease may retarget following navigation. */
export function prepareChatEditorTransfer(
  message: Message, payload: SendToEditorPayload, sourceCurrent: () => boolean,
): PreparedChatMessagingTarget | undefined {
  if (!payload.content || !['markdown', 'plain', 'html'].includes(payload.format)) return undefined;
  const owner = useAuthStore.getState().user;
  const workspace = useWorkspaceStore.getState().workspace;
  if (!owner || !workspace || !workspace.activeTabId || !sourceCurrent()) return undefined;
  const sourceTabId = workspace.activeTabId;
  const sourceTab = workspace.tabs.find(tab => tab.id === sourceTabId);
  const sourceBinding = JSON.stringify(sourceTab);
  const messageContent = message.content;
  const pathname = window.location.pathname;
  const initialModal = getModalRegistrySnapshot();
  const chatModal = useWorkspaceChatModalStore.getState();
  const ownsModal = initialModal.ids.length === 1 && chatModal.isOpen &&
    chatModal.boundTabId === sourceTabId && chatModal.boundConversationId === message.conversationId;
  if (initialModal.ids.length && !ownsModal) return undefined;
  const input = Object.freeze({ ...payload });
  const targetId = input.target === 'document' ? input.targetDocumentId : '';
  if (input.target !== 'document' && input.target !== 'new_document') return undefined;
  if (input.target === 'document' && (!targetId || !workspace.tabs.some(tab => tab.id === targetId && tab.type === 'editor'))) return undefined;
  let expectedDocument = targetId ? useEditorStore.getState().documents[targetId] : undefined;
  if (expectedDocument?.readOnly || expectedDocument?.loadError || expectedDocument?.mode === 'view') return undefined;
  if (expectedDocument?.sessionHydrated === false) expectedDocument = undefined;
  let destination: EditorTransferTarget | undefined;
  try { destination = targetId ? captureEditorTransferDestination(targetId, expectedDocument) : undefined; }
  catch { return undefined; }
  let planned: ChatEditorPlan | undefined;
  let phase: 'source' | 'transition' | 'destination' | 'applying' = 'source';
  let invalid = false;
  let disposed = false;
  let running = false;
  let destinationSeen = false;
  let closingModal = false;
  let modalGeneration = initialModal.generationNumber;
  const off: Array<() => void> = [];
  const cleanup = () => { off.splice(0).forEach(unsubscribe => unsubscribe()); destination?.dispose(); };
  const current = () => {
    if (invalid || disposed) return false;
    const auth = useAuthStore.getState();
    const ws = useWorkspaceStore.getState().workspace;
    const latest = useChatStore.getState().getConversationMessages(message.conversationId!).find(item => item.id === message.id);
    const modals = getModalRegistrySnapshot();
    let valid = auth.isAuthenticated && auth.user?.userId === owner.userId && auth.user?.sessionId === owner.sessionId &&
      ws?.id === workspace.id && window.location.pathname === pathname && latest === message && latest.content === messageContent &&
      JSON.stringify(ws.tabs.find(tab => tab.id === sourceTabId)) === sourceBinding;
    if (phase === 'source') valid = valid && sourceCurrent();
    else {
      const active = ws?.activeTabId;
      if (active === planned?.tabId) destinationSeen = true;
      valid = valid && !!planned && (active === planned.tabId || phase === 'transition' && !destinationSeen && active === sourceTabId);
    }
    if (!closingModal) valid = valid && modals.generationNumber === modalGeneration;
    const docId = planned?.tabId ?? targetId;
    const doc = docId ? useEditorStore.getState().documents[docId] : undefined;
    if (phase !== 'applying') {
      if (expectedDocument) valid = valid && doc === expectedDocument;
      else if (doc?.sessionHydrated !== false && doc) expectedDocument = doc;
      valid = valid && !doc?.readOnly && !doc?.loadError && doc?.mode !== 'view';
    }
    if (!valid) invalid = true;
    return valid;
  };
  const observe = () => { current(); };
  off.push(useAuthStore.subscribe(observe), useWorkspaceStore.subscribe(observe), useEditorStore.subscribe(observe), useChatStore.subscribe(observe));
  const waitForDestination = async () => {
    const deadline = Date.now() + 10_000;
    while (current() && (useWorkspaceStore.getState().workspace?.activeTabId !== planned?.tabId)) {
      if (Date.now() >= deadline) throw new Error('Editor transition timed out');
      await new Promise(resolve => setTimeout(resolve, 20));
    }
    if (!current()) throw new Error('Editor transition became stale');
  };
  return {
    executionKind: 'ui', isCurrent: current,
    canCommit: () => current() && phase === 'source',
    async prepareAdmission(ticket) {
      if (!current()) throw new ChatMessagingStaleError();
      planned = await prepareChatEditorCommand(ticket, message.id, messageContent, targetId);
      if (targetId && planned.tabId !== targetId || !current()) throw new ChatMessagingStaleError();
    },
    async execute(handoff) {
      if (!current() || !planned || running) throw new ChatMessagingStaleError();
      running = true;
      phase = 'transition';
      try {
        const opened = await openChatEditorCommand(handoff.ticket, handoff.handoffId, input.title ?? '');
        if (opened.tabId !== planned.tabId || !current()) throw new Error('Editor destination mismatch');
        await waitForDestination();
        if (ownsModal) {
          const modal = useWorkspaceChatModalStore.getState();
          if (!modal.isOpen || modal.boundTabId !== sourceTabId || modal.boundConversationId !== message.conversationId) throw new Error('Chat modal changed');
          closingModal = true;
          flushSync(() => useWorkspaceChatModalStore.getState().close());
          const after = getModalRegistrySnapshot();
          const expectedGeneration = initialModal.generationNumber + 1 + (initialModal.chatPresentationCommandIds ? 1 : 0);
          closingModal = false;
          if (after.ids.length || after.generationNumber !== expectedGeneration) throw new Error('Unexpected modal transition');
          modalGeneration = after.generationNumber;
        }
        phase = 'destination';
        if (!current()) throw new Error('Editor destination became stale');
        await transferEditorContent({
          targetDocumentId: planned.tabId, content: input.content, format: input.format, title: input.title,
          expectedDocument, destination,
          isCurrent: () => current() && document.hasFocus() && ReadFocusContext().composition !== 'active',
          beforeApply: async () => {
            await validateChatEditorCommand(handoff.ticket, handoff.handoffId);
            if (!current()) throw new Error('Editor destination became stale');
            phase = 'applying';
          },
        });
      } finally { running = false; cleanup(); }
    },
    // Closing the originating contextual chat is an intentional transition.
    // The independent owner/destination observers remain until execution ends.
    dispose() { if (!running) { disposed = true; cleanup(); } },
  };
}
