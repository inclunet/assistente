import { useCallback, useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal } from '../ui/Modal';
import { ChatSessionView } from '../chat/ChatSessionView';
import { getAdapter, useMiniChatStore } from '../../store/miniChatStore';
import { useWorkspaceStore } from '../../store/workspaceStore';
import { useChatStore } from '../../store/chatStore';
import { ttsService } from '../../services/tts';
import { messageAudioService } from '../../services/messageAudio';
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
    ttsService.stop();
    messageAudioService.stopCurrentAudio();
    close();
  }, [close]);

  /** Evita enviar com o adaptador da aba errada se o utilizador mudar de painel com o modal aberto. */
  useEffect(() => {
    if (!isOpen || !boundTabId) return;
    if (activeWorkspaceTab?.id !== boundTabId) {
      handleClose();
    }
  }, [isOpen, boundTabId, activeWorkspaceTab?.id, handleClose]);

  const handleSend = useCallback(
    async (content: string, mediaFiles?: MediaFile[]) => {
      const tabId = useMiniChatStore.getState().boundTabId;
      const adapter = getAdapter(tabId);
      const meta = useMiniChatStore.getState().sessionMeta;
      if (!adapter || !tabId) {
        handleClose();
        return;
      }
      await adapter.send(content, mediaFiles, meta);
    },
    [handleClose],
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
