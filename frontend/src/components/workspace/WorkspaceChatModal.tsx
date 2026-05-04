import { useCallback, useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal } from '../ui/Modal';
import { ChatPanel, type ChatPanelSendContext } from '../chat/ChatPanel';
import { useChatConversationTimeline } from '../chat/ChatSurfaceController';
import { useWorkspaceChatModalStore } from '../../store/workspaceChatModalStore';
import { useWorkspaceStore, useActiveTab } from '../../store/workspaceStore';
import { useChatStore } from '../../store/chatStore';
import { useUIStore } from '../../store/uiStore';
import { ensureWorkspaceTabConversationId } from '../../lib/workspaceConversation';
import type { MediaFile } from '../../services/mediaService';
import { normalizeChatSurfaceOrigin } from '../../services/chatSessionRegistry';

import './WorkspaceChatModal.css';

export function WorkspaceChatModal() {
  const { t } = useTranslation();
  const isOpen = useWorkspaceChatModalStore((s) => s.isOpen);
  const boundTabId = useWorkspaceChatModalStore((s) => s.boundTabId);
  const contextDisplay = useWorkspaceChatModalStore((s) => s.contextDisplay);
  const focusNonce = useWorkspaceChatModalStore((s) => s.focusNonce);
  const adapterError = useWorkspaceChatModalStore((s) => s.adapterError);
  const close = useWorkspaceChatModalStore((s) => s.close);
  const boundConversationId = useWorkspaceChatModalStore((s) => s.boundConversationId);
  const activeConversation = useChatConversationTimeline(boundConversationId);
  const activeWorkspaceTab = useActiveTab();
  const modalSurfaceId = useMemo(
    () => `embedded:workspace-chat-modal:${boundTabId ?? 'standalone'}`,
    [boundTabId],
  );

  const modalTitle = useMemo(() => {
    const conversationTitle = activeConversation?.title || t('editor.chatModal.conversation');
    return `${t('editor.chatModal.title')} — ${conversationTitle}`;
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
        '.workspace-chat-modal .chat-input__textarea',
      ) as HTMLTextAreaElement | null;
      ta?.focus();
    });
    return () => cancelAnimationFrame(id);
  }, [isOpen, focusNonce]);

  const handleSend = useCallback(
    async (content: string, mediaFiles: MediaFile[] | undefined, context: ChatPanelSendContext) => {
      const {
        boundTabId: tabId,
        boundConversationId: storedConversationId,
        sessionMeta: meta,
        boundSend,
      } = useWorkspaceChatModalStore.getState();
      if (!boundSend || !tabId) {
        useUIStore.getState().addToast(t('workspace.chatModal.adapterUnavailable'), 'error');
        handleClose();
        return;
      }

      const ws = useWorkspaceStore.getState().workspace;
      const tab = ws?.tabs.find((x) => x.id === tabId);
      if (!tab) {
        useUIStore.getState().addToast(t('workspace.chatModal.adapterUnavailable'), 'error');
        handleClose();
        return;
      }

      let targetConversationId = storedConversationId;
      try {
        targetConversationId = await ensureWorkspaceTabConversationId(tab);
      } catch (e) {
        console.error('[workspaceChatModal] falha ao garantir conversa no envio:', e);
        useUIStore.getState().addToast(t('editor.chatModal.newConversationError'), 'error');
        return;
      }

      if (!targetConversationId) {
        console.error('[workspaceChatModal] conversationId ausente após ensure — envio cancelado');
        useUIStore.getState().addToast(t('editor.chatModal.newConversationError'), 'error');
        return;
      }

      const sendPlan = await boundSend(content, mediaFiles, meta, {
        tabId,
        conversationId: targetConversationId,
      });
      if (!sendPlan) return;

      try {
        const sendOrigin = normalizeChatSurfaceOrigin(context.origin, targetConversationId);
        await useChatStore.getState().sendMessageToConversation(
          targetConversationId,
          sendPlan.content,
          sendPlan.mediaFiles,
          sendPlan.paramsOverride,
          { origin: sendOrigin },
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
      <div className="workspace-chat-modal">
        <details className="workspace-chat-modal__context">
          <summary className="workspace-chat-modal__context-summary">
            {t('editor.chatModal.contextBtn')}
          </summary>
          <pre className="workspace-chat-modal__context-pre">{contextDisplay}</pre>
        </details>

        {adapterError && (
          <div className="workspace-chat-modal__error" role="alert">
            {adapterError}
          </div>
        )}

        <div className="workspace-chat-modal__session">
          <ChatPanel
            surfaceType="embedded"
            conversationId={boundConversationId}
            surfaceId={modalSurfaceId}
            onSend={handleSend}
            showShortcutsHelp={false}
          />
        </div>
      </div>
    </Modal>
  );
}
