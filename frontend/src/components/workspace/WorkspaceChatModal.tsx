import { logger } from '../../utils/logger';
import { useCallback, useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal } from '../ui/Modal';
import { ChatPanel, useEffectiveProfileSlug, type ChatPanelSendContext } from '../chat/ChatPanel';
import { sendChatSurfaceMessage, useChatConversationTimeline } from '../chat/ChatSurfaceController';
import { useWorkspaceChatModalStore } from '../../store/workspaceChatModalStore';
import { useWorkspaceStore, useActiveTab } from '../../store/workspaceStore';
import { useUIStore } from '../../store/uiStore';
import { ensureWorkspaceTabConversationId } from '../../lib/workspaceConversation';
import { isBackendId } from '../../lib/idUtils';
import type { MediaFile } from '../../services/mediaService';
import { normalizeChatSurfaceOrigin } from '../../services/chatSessionRegistry';
import { WorkspacePanelProvider } from './WorkspacePanelContext';

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
  const boundSurface = useWorkspaceChatModalStore((s) => s.boundSurface);
  const setBoundConversation = useWorkspaceChatModalStore((s) => s.setBoundConversation);
  const workspaceTabs = useWorkspaceStore((s) => s.workspace?.tabs ?? []);
  const activeConversation = useChatConversationTimeline(boundConversationId);
  const activeWorkspaceTab = useActiveTab();
  const boundWorkspaceTab = useMemo(
    () => workspaceTabs.find((tab) => tab.id === boundTabId) ?? null,
    [boundTabId, workspaceTabs],
  );
  const modalTitle = useMemo(() => {
    const conversationTitle = activeConversation?.title || t('editor.chatModal.conversation');
    return `${t('editor.chatModal.title')} — ${conversationTitle}`;
  }, [activeConversation?.title, activeConversation?.id, t]);
  const effectiveProfileSlug = useEffectiveProfileSlug(boundTabId ?? undefined);

  const handleClose = useCallback(() => {
    close();
  }, [close]);

  // Dono da superfície "modal embutido": trocar a conversa no HistoryPicker recria a
  // superfície vinculada e persiste o vínculo na aba. Painéis que observam
  // `boundConversationId` (ex.: TaskListView) re-vinculam a lista automaticamente.
  const handleRequestConversationChange = useCallback(
    (nextConversationId: string) => {
      setBoundConversation(nextConversationId);
    },
    [setBoundConversation],
  );

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

      // A conversa vinculada ao modal (boundConversationId) é a fonte de verdade do
      // que a superfície está exibindo. Quando já é um ID válido do backend, enviamos
      // para ela diretamente, sem reconsultar o workspaceStore. Isso evita um race:
      // `setBoundConversation` atualiza o vínculo de forma síncrona, mas persiste a aba
      // via `updateTab()` (assíncrono); ler a aba aqui poderia devolver o ID antigo e
      // mandar a mensagem para a conversa errada se o envio acontecer logo após a troca.
      let targetConversationId = storedConversationId;
      if (!targetConversationId || !isBackendId(targetConversationId)) {
        try {
          targetConversationId = await ensureWorkspaceTabConversationId(tab);
        } catch (e) {
          logger.error('[workspaceChatModal] falha ao garantir conversa no envio:', e);
          useUIStore.getState().addToast(t('editor.chatModal.newConversationError'), 'error');
          return;
        }
      }

      if (!targetConversationId) {
        logger.error('[workspaceChatModal] conversationId ausente após ensure — envio cancelado');
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
        await sendChatSurfaceMessage(
          targetConversationId,
          sendPlan.content,
          sendPlan.mediaFiles,
          sendPlan.paramsOverride,
          sendOrigin,
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
          <div className="workspace-chat-modal__error">
            {adapterError}
          </div>
        )}

        {boundWorkspaceTab && boundSurface && (
          <WorkspacePanelProvider
            value={{
              tab: boundWorkspaceTab,
              isActive: activeWorkspaceTab?.id === boundWorkspaceTab.id,
            }}
          >
            <div className="workspace-chat-modal__session">
              <ChatPanel
                surface={boundSurface}
                onSend={handleSend}
                onRequestConversationChange={handleRequestConversationChange}
                showShortcutsHelp={false}
                profileSlug={effectiveProfileSlug}
              />
            </div>
          </WorkspacePanelProvider>
        )}
      </div>
    </Modal>
  );
}
