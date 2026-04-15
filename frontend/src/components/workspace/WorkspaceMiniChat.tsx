import { useCallback, useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal } from '../ui/Modal';
import { ChatSessionView } from '../chat/ChatSessionView';
import { useMiniChatStore } from '../../store/miniChatStore';
import { useWorkspaceStore } from '../../store/workspaceStore';
import { useChatStore } from '../../store/chatStore';
import { useUIStore } from '../../store/uiStore';
import { ensureWorkspaceTabConversationId } from '../../lib/workspaceConversation';
import type { MediaFile } from '../../services/mediaService';

import './WorkspaceMiniChat.css';

export function WorkspaceMiniChat() {
  const { t } = useTranslation();
  const isOpen = useMiniChatStore((s) => s.isOpen);
  const boundTabId = useMiniChatStore((s) => s.boundTabId);
  const contextDisplay = useMiniChatStore((s) => s.contextDisplay);
  const focusNonce = useMiniChatStore((s) => s.focusNonce);
  const adapterError = useMiniChatStore((s) => s.adapterError);
  const close = useMiniChatStore((s) => s.close);
  const activeConversation = useChatStore((s) => s.activeConversation);
  const activeWorkspaceTab = useWorkspaceStore((s) => s.getActiveTab());

  const modalTitle = useMemo(() => {
    const conversationTitle = activeConversation?.title || t('editor.inlineChat.conversation');
    return `${t('editor.inlineChat.title')} — ${conversationTitle}`;
  }, [activeConversation?.title, activeConversation?.id, t]);

  const handleClose = useCallback(() => {
    close();
  }, [close]);

  /** Evita enviar com o adaptador da aba errada se o utilizador mudar de painel com o modal aberto. */
  useEffect(() => {
    if (!isOpen || !boundTabId) return;
    if (activeWorkspaceTab?.id !== boundTabId) {
      handleClose();
    }
  }, [isOpen, boundTabId, activeWorkspaceTab?.id, handleClose]);

  /** `bumpFocus()` altera o nonce; sem isto o textarea do ChatInput não volta a receber foco. */
  useEffect(() => {
    if (!isOpen) return;
    const id = requestAnimationFrame(() => {
      const ta = document.querySelector(
        '.workspace-mini-chat .chat-input__textarea',
      ) as HTMLTextAreaElement | null;
      ta?.focus();
    });
    return () => cancelAnimationFrame(id);
  }, [isOpen, focusNonce]);

  const handleSend = useCallback(
    async (content: string, mediaFiles?: MediaFile[]) => {
      const {
        boundTabId: tabId,
        boundConversationId: storedConversationId,
        sessionMeta: meta,
        boundSend,
      } = useMiniChatStore.getState();
      if (!boundSend || !tabId) {
        useUIStore.getState().addToast(t('workspace.miniChat.adapterUnavailable'), 'error');
        handleClose();
        return;
      }

      const ws = useWorkspaceStore.getState().workspace;
      const tab = ws?.tabs.find((x) => x.id === tabId);
      if (!tab) {
        useUIStore.getState().addToast(t('workspace.miniChat.adapterUnavailable'), 'error');
        handleClose();
        return;
      }

      let targetConversationId = storedConversationId;
      try {
        targetConversationId = await ensureWorkspaceTabConversationId(tab);
      } catch (e) {
        console.error('[miniChat] falha ao garantir conversa no envio:', e);
        useUIStore.getState().addToast(t('editor.inlineChat.newConversationError'), 'error');
        return;
      }

      if (!targetConversationId) {
        console.error('[miniChat] conversationId ausente após ensure — envio cancelado');
        useUIStore.getState().addToast(t('editor.inlineChat.newConversationError'), 'error');
        return;
      }

      const sendPlan = await boundSend(content, mediaFiles, meta, {
        tabId,
        conversationId: targetConversationId,
      });
      if (!sendPlan) return;

      try {
        await useChatStore.getState().sendMessageToConversation(
          targetConversationId,
          sendPlan.content,
          sendPlan.mediaFiles,
          sendPlan.paramsOverride,
        );
        await sendPlan.afterSend?.();
      } catch (error) {
        sendPlan.onSendError?.(error);
        if (!sendPlan.onSendError) {
          throw error;
        }
      }
    },
    [handleClose, t],
  );

  return (
    <Modal isOpen={isOpen} title={modalTitle} onClose={handleClose} size="lg">
      <div className="editor-inline-chat workspace-mini-chat" key={focusNonce}>
        <details className="editor-inline-chat__context" open={false}>
          <summary className="editor-inline-chat__context-summary">
            {t('editor.inlineChat.contextBtn')}
          </summary>
          <pre className="editor-inline-chat__context-pre">{contextDisplay}</pre>
        </details>

        {adapterError && (
          <div className="editor-inline-chat__error" role="alert">
            {adapterError}
          </div>
        )}

        <div className="workspace-mini-chat__session">
          <ChatSessionView variant="embedded" onSend={handleSend} showShortcutsHelp={false} />
        </div>
      </div>
    </Modal>
  );
}
