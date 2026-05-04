import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { ensureWorkspaceTabHasConversation } from '../lib/workspaceConversation';
import { ChatPanel, type ChatPanelSendContext } from '../components/chat/ChatPanel';
import { useWorkspacePanel } from '../components/workspace/WorkspacePanelContext';
import { useWorkspaceStore } from '../store/workspaceStore';
import { createChatSurfaceIdentity, normalizeChatSurfaceOrigin } from '../services/chatSessionRegistry';
import { sendChatSurfaceMessage } from '../components/chat/ChatSurfaceController';
import { buildChatSurfaceParams } from '../lib/chatSurface';

export default function ChatPage() {
  const { t } = useTranslation();
  const { tab } = useWorkspacePanel();
  const wsProfile = useWorkspaceStore((s) => s.workspace?.profile);
  const conversationId = tab?.type === 'chat' ? tab.conversationId : undefined;
  const tabProfileSlug = tab?.type === 'chat'
    ? (tab.profileOverride?.slug as string | undefined)
    : undefined;
  const effectiveProfileSlug = tabProfileSlug || wsProfile || undefined;
  const surface = useMemo(() => createChatSurfaceIdentity({
    conversationId: conversationId ?? null,
    surfaceType: 'page',
    tabId: tab.id,
  }), [conversationId, tab.id]);

  // NOTE: loadConversation já é feita pelo useWorkspaceChatBridge (WorkspaceLayout).
  // Não duplicar aqui — evita 2x GetConversationInfo + GetMessages a cada troca de aba.

  const onSend = useCallback(
    async (content: string, mediaFiles: Parameters<typeof sendChatSurfaceMessage>[2], context: ChatPanelSendContext) => {
      if (!tab || tab.type !== 'chat') {
        throw new Error(t('chat.errors.tabCannotSend'));
      }
      const conversationId = await ensureWorkspaceTabHasConversation(tab);
      if (!conversationId) {
        throw new Error(t('chat.errors.chatTabNotReady'));
      }
      const sendOrigin = normalizeChatSurfaceOrigin(context.origin, conversationId);
      await sendChatSurfaceMessage(
        conversationId,
        content,
        mediaFiles,
        buildChatSurfaceParams(tab, { profileSlug: effectiveProfileSlug }),
        sendOrigin,
      );
    },
    [effectiveProfileSlug, tab, t],
  );

  return <ChatPanel surface={surface} onSend={onSend} />;
}
