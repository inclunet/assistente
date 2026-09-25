import { useEffect, useRef, type RefObject } from 'react';
import { useChatStore, type Message } from '../../store/chatStore';
import { isBackendId } from '../../lib/idUtils';
import { ChatMessagingStaleError, CHAT_MESSAGE_ACTION_IDS, isChatMessageAction, type PreparedChatMessagingTarget } from '../../lib/commandChatMessaging';
import { prepareChatMessageCommand, prepareChatMessageEditCommand, commitChatMessageCommand } from '../../lib/commandChatMessageWails';
import { captureChatMessageEditDraft } from './useChatMessageEditDraft';
import { useMessageActions } from '../../hooks/useContextMenu';
import { clearToolInvocationDetailsCache } from '../../services/toolInvocationDetailsCache';
import { ttsService } from '../../services/tts';
import { useAuthStore } from '../../store/authStore';
import { useWorkspaceStore } from '../../store/workspaceStore';
import { useWorkspaceChatModalStore } from '../../store/workspaceChatModalStore';
import { getModalSnapshotGeneration } from '../../lib/modalRegistry';
import { logger } from '../../utils/logger';
import { handleError, ErrorSeverity } from '../../utils/errorHandler';
import i18next from 'i18next';
import { prepareChatEditorTransfer, CHAT_EDITOR_COMMAND } from '../../lib/commandChatEditorTransfer';
import type { SendToEditorPayload } from '../../lib/editorSendMenu';

export type ChatMessageCommandID = typeof CHAT_MESSAGE_ACTION_IDS[number];
export const isChatMessageCommand = isChatMessageAction;

/** Selection is surface-local, never inferred from the last timeline item. */
export function useChatMessageCommands(rootRef: RefObject<HTMLDivElement | null>, conversationId: string | null | undefined, sessionKey: string, announce: (text: string) => void) {
  const selection = useRef({ id: '', revision: 0 });
  const listeners = useRef(new Set<() => void>());
  const playbackLifetimes = useRef(new Set<() => void>());
  const actions = useMessageActions({ onAnnounce: announce });
  const live = useRef({ conversationId, sessionKey, actions });
  live.current = { conversationId, sessionKey, actions };
  useEffect(() => {
    const root = rootRef.current;
    const focus = (event: FocusEvent) => {
      const node = event.target instanceof Element ? event.target.closest('[data-message-node][data-message-id]') : null;
      if (!node || !root?.contains(node)) return;
      const id = node.getAttribute('data-message-id') ?? '';
      if (selection.current.id === id) return;
      selection.current = { id, revision: selection.current.revision + 1 };
      listeners.current.forEach(changed => changed());
    };
    selection.current = { id: '', revision: selection.current.revision + 1 };
    root?.addEventListener('focusin', focus);
    return () => { root?.removeEventListener('focusin', focus); playbackLifetimes.current.forEach(cancel => cancel()); };
  }, [rootRef, conversationId, sessionKey]);
  const getMessage = (id: string) => live.current.conversationId
    ? useChatStore.getState().getConversationMessages(live.current.conversationId).find(message => message.id === id)
    : undefined;
  const eligible = (message: Message | undefined, id: ChatMessageCommandID): message is Message => !!message &&
    isBackendId(message.id) && !message.isStreaming && !message.internal && message.conversationId === live.current.conversationId &&
    ((id !== 'chat.message.edit.open' && id !== 'chat.message.edit.save') || message.role === 'user') && (id !== 'chat.message.speak' || ttsService.hasVoiceConfig());
  return {
    subscribe(changed: () => void) { listeners.current.add(changed); return () => { listeners.current.delete(changed); }; },
    canStart(id: ChatMessageCommandID, keyboardTarget?: EventTarget | null) {
      if (id === 'chat.message.edit.save') {
        const root = rootRef.current;
        const draft = root && live.current.conversationId ? captureChatMessageEditDraft(root, live.current.conversationId, live.current.sessionKey, selection.current.id) : undefined;
        try { return !!draft && draft.isCurrent() && (!(keyboardTarget instanceof Node) || draft.contains(keyboardTarget)); }
        finally { draft?.dispose(); }
      }
      if (keyboardTarget instanceof Element && keyboardTarget.closest('input,textarea,select,[contenteditable]')) return false;
      // Menu overrides are resolved in prepare; menu focus need not be a message.
      return eligible(getMessage(selection.current.id), id) || keyboardTarget == null;
    },
    prepare(id: ChatMessageCommandID, override: unknown, current: () => boolean, available: () => boolean): PreparedChatMessagingTarget | undefined {
      const explicit = override && typeof override === 'object' && 'messageId' in override ? override.messageId : undefined;
      if (explicit !== undefined && typeof explicit !== 'string') return undefined;
      if (typeof explicit === 'string' && selection.current.id !== explicit) {
        selection.current = { id: explicit, revision: selection.current.revision + 1 };
        listeners.current.forEach(changed => changed());
      }
      const selected = selection.current;
      const capturedModalGeneration = getModalSnapshotGeneration();
      const message = getMessage(typeof explicit === 'string' ? explicit : selected.id);
      if (!eligible(message, id)) return undefined;
      if (id === CHAT_EDITOR_COMMAND) {
        const provided = override && typeof override === 'object' && 'transfer' in override ? override.transfer : undefined;
        let payload: SendToEditorPayload;
        if (provided !== undefined) {
          if (!provided || typeof provided !== 'object' || !('originalContent' in provided) || provided.originalContent !== message.content ||
              !('messageId' in provided) || provided.messageId !== message.id || !('content' in provided) || typeof provided.content !== 'string' ||
              !('format' in provided) || !['markdown', 'html', 'plain'].includes(String(provided.format)) || !('target' in provided)) return undefined;
          payload = { ...provided } as SendToEditorPayload;
        } else payload = { target: 'new_document', content: message.content, format: 'markdown', title: i18next.t('editor.fallback.fromChat') };
        return prepareChatEditorTransfer(message, payload, () => current() && available() && selection.current.revision === selected.revision);
      }
      const capturedConversation = live.current.conversationId!;
      const capturedSession = live.current.sessionKey;
      const editDraft = id === 'chat.message.edit.save' && rootRef.current
        ? captureChatMessageEditDraft(rootRef.current, capturedConversation, capturedSession, message.id) : undefined;
      if (id === 'chat.message.edit.save' && (!editDraft || editDraft.originalMessage !== message)) { editDraft?.dispose(); return undefined; }
      const snapshot = { ...message };
      let disposed = false;
      let invalid = false;
      const messageCurrent = () => {
        const latest = getMessage(snapshot.id);
        return Boolean(rootRef.current?.isConnected && current() && live.current.conversationId === capturedConversation &&
          live.current.sessionKey === capturedSession && latest === message && eligible(latest, id) &&
          latest.content === snapshot.content && selection.current.revision === selected.revision && (!editDraft || editDraft.isCurrent()));
      };
      const isCurrent = () => {
        const valid = !disposed && !invalid && messageCurrent();
        if (!valid) invalid = true;
        return valid;
      };
      const canCommit = () => isCurrent() && available();
      const unsubscribeDraft = editDraft?.subscribe(() => { isCurrent(); });
      const guard = () => { if (!canCommit()) throw new ChatMessagingStaleError(); };
      return {
        executionKind: id === 'chat.message.pin.toggle' || id === 'chat.message.delete' || id === 'chat.message.edit.save' ? 'backend' : 'ui',
        isCurrent, canCommit,
        async prepareAdmission(ticket) {
          if (!isCurrent()) throw new ChatMessagingStaleError();
          if (editDraft) await prepareChatMessageEditCommand(ticket, snapshot.id, editDraft.originalContent, editDraft.content);
          else await prepareChatMessageCommand(ticket, snapshot.id);
          if (!isCurrent()) throw new ChatMessagingStaleError();
        },
        async execute(handoff) {
          guard();
          if (id === 'chat.message.copy' || id === 'chat.message.copy_markdown') await live.current.actions.copyMessage(snapshot, id === 'chat.message.copy_markdown', canCommit);
          else if (id === 'chat.message.speak') await new Promise<void>((resolve, reject) => {
            // Playback outlives the command lease. Its guard deliberately does
            // not depend on dispose() after CompleteSucceeded.
            let started = false;
            let playbackInvalid = false;
            let startTimeout: ReturnType<typeof setTimeout> | undefined;
            const modalGeneration = getModalSnapshotGeneration();
            const off: Array<() => void> = [];
            const release = () => { clearTimeout(startTimeout); off.splice(0).forEach(unsubscribe => unsubscribe()); playbackLifetimes.current.delete(cancel); };
            const cancel = () => { playbackInvalid = true; release(); if (!started) reject(new ChatMessagingStaleError()); };
            const playbackCurrent = () => {
              if (playbackInvalid) return false;
              if (!messageCurrent() || !available() || getModalSnapshotGeneration() !== modalGeneration) { cancel(); return false; }
              return true;
            };
            const changed = () => { playbackCurrent(); };
            off.push(useChatStore.subscribe(changed), useAuthStore.subscribe(changed), useWorkspaceStore.subscribe(changed), useWorkspaceChatModalStore.subscribe(changed));
            listeners.current.add(changed);
            off.push(() => { listeners.current.delete(changed); });
            playbackLifetimes.current.add(cancel);
            const onStarted = () => { if (playbackCurrent()) { started = true; clearTimeout(startTimeout); resolve(); } };
            // No playback may begin after the audited UI effect has timed out.
            startTimeout = setTimeout(() => { playbackInvalid = true; release(); reject(new Error('chat-message-playback-start-timeout')); }, 20_000);
            void live.current.actions.speakMessage(snapshot, playbackCurrent, onStarted).then(() => {
              if (!started) reject(new Error('chat-message-playback-not-started'));
            }, error => {
              if (!started) reject(error);
              else if (!playbackInvalid) logger.error('[ChatMessageCommands] playback failed after start', error);
            }).finally(release);
          });
          else if (id === 'chat.message.edit.open') useChatStore.getState().startConversationEditing(capturedConversation, snapshot.id, capturedSession);
          else await commitChatMessageCommand(handoff.ticket, handoff.handoffId);
        },
        succeeded() {
          if (!current()) return;
          if (editDraft?.accept(() => current() && available() && document.hasFocus() &&
              selection.current.revision === selected.revision && getModalSnapshotGeneration() === capturedModalGeneration)) announce(i18next.t('chat.messageEdited'));
          else editDraft?.rebaseConfirmed(() => current() && getModalSnapshotGeneration() === capturedModalGeneration);
          if (id === 'chat.message.delete' || id === 'chat.message.pin.toggle') {
            clearToolInvocationDetailsCache();
            void useChatStore.getState().loadConversationSession(capturedConversation, { refreshSurfaceWindows: true }).catch(error => {
              if (current()) handleError(error, { source: 'ChatMessageCommands.refresh', userMessage: i18next.t('chat.loadError'), severity: ErrorSeverity.RECOVERABLE, metadata: { conversationId: capturedConversation, messageId: snapshot.id } });
            });
          }
        },
        dispose() { disposed = true; unsubscribeDraft?.(); editDraft?.dispose(); },
      };
    },
  };
}
