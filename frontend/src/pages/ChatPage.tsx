import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { ensureWorkspaceTabHasConversation } from '../lib/workspaceConversation';
import { ChatPanel, type ChatPanelSendContext } from '../components/chat/ChatPanel';
import { useWorkspacePanel } from '../components/workspace/WorkspacePanelContext';
import { useWorkspaceStore } from '../store/workspaceStore';
import { useChatStore, type ChatConversationSession } from '../store/chatStore';
import { useWorkspaceCommandSurface } from '../components/workspace/useWorkspaceCommandSurface';
import { createChatSurfaceIdentity, normalizeChatSurfaceOrigin } from '../services/chatSessionRegistry';
import { sendChatSurfaceMessage } from '../components/chat/ChatSurfaceController';
import { ChatMessagingStaleError } from '../lib/commandChatMessaging';
import { buildChatSurfaceParams } from '../lib/chatSurface';
import { readChatCommandSurface, type ChatCommandExecutionStatus } from '../lib/commandChatSurface';

export default function ChatPage() {
  const { t } = useTranslation();
  const { tab } = useWorkspacePanel();
  const wsProfile = useWorkspaceStore((s) => s.workspace?.profile);
  const updateTab = useWorkspaceStore((s) => s.updateTab);
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

  const commandSurfaceGetter = useCallback(() => {
    if (!conversationId) return null;

    const workspace = useWorkspaceStore.getState().workspace;
    const currentTab = workspace?.tabs.find((candidate) => candidate.id === tab.id);
    if (
      !currentTab ||
      currentTab.type !== 'chat' ||
      currentTab.conversationId !== conversationId
    ) return null;

    const session = useChatStore.getState().sessionsByConversationId[conversationId];
    if (!session || session.conversation?.id !== conversationId) return null;

    const executionStatus: ChatCommandExecutionStatus = session.isLoading || session.isThinking || session.streamingMessageId
      ? 'running'
      : (session.queuedTurnCount ?? 0) > 0 ? 'queued' : 'idle';

    return readChatCommandSurface({
      tabId: tab.id,
      conversationId,
      sessionKey: session.sessionKey,
      sessionLoaded: true,
      executionStatus,
    });
  }, [tab.id, conversationId]);
  const subscribeToCommandChatSurface = useCallback((invalidate: () => void) => {
    type SessionFacts = {
      readonly session: ChatConversationSession | undefined;
      readonly loaded: boolean;
      readonly sessionKey: string;
      readonly isLoading: boolean;
      readonly isThinking: boolean;
      readonly streamingMessageId: string | null;
      readonly queuedTurnCount: number;
    };
    const readFacts = (state: ReturnType<typeof useChatStore.getState>): SessionFacts => {
      const session = state.sessionsByConversationId[conversationId ?? ''];
      const loaded = !!session && session.conversation?.id === conversationId;
      return {
        session,
        loaded,
        sessionKey: session?.sessionKey ?? '',
        isLoading: session?.isLoading ?? false,
        isThinking: session?.isThinking ?? false,
        streamingMessageId: session?.streamingMessageId ?? null,
        queuedTurnCount: session?.queuedTurnCount ?? 0,
      };
    };
    const sameFacts = (left: SessionFacts, right: SessionFacts) => (
      left.session === right.session
      && left.loaded === right.loaded
      && left.sessionKey === right.sessionKey
      && left.isLoading === right.isLoading
      && left.isThinking === right.isThinking
      && left.streamingMessageId === right.streamingMessageId
      && left.queuedTurnCount === right.queuedTurnCount
    );
    let previousFacts = readFacts(useChatStore.getState());
    return useChatStore.subscribe((state) => {
      const currentFacts = readFacts(state);
      // A referência detecta substituição conservadora da sessão com o mesmo
      // ID; os escalares preservados detectam mutação in-place entre avisos.
      if (!sameFacts(currentFacts, previousFacts)) invalidate();
      previousFacts = currentFacts;
    });
  }, [tab.id, conversationId]);
  useWorkspaceCommandSurface('chat', commandSurfaceGetter, subscribeToCommandChatSurface);

  // NOTE: loadConversation já é feita pelo useWorkspaceChatBridge (WorkspaceLayout).
  // Não duplicar aqui — evita 2x GetConversationInfo + GetMessages a cada troca de aba.

  const onSend = useCallback(
    async (content: string, mediaFiles: Parameters<typeof sendChatSurfaceMessage>[2], context: ChatPanelSendContext) => {
      if (!tab || tab.type !== 'chat') {
        throw new Error(t('chat.errors.tabCannotSend'));
      }
      if (context.command && !context.command.isCurrent()) throw new ChatMessagingStaleError();
      const conversationId = context.command ? context.conversationId : await ensureWorkspaceTabHasConversation(tab);
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
        context.command,
      );
    },
    [effectiveProfileSlug, tab, t],
  );

  // Dono da superfície "página": trocar a conversa re-aponta a aba de chat. Como o
  // `surface` é derivado de `tab.conversationId`, atualizar a aba já recompõe a view,
  // e o useWorkspaceChatBridge reage à mudança de `conversationId` carregando a sessão
  // — não chamamos `loadConversationSession` aqui para não duplicar o load (ver NOTE acima).
  const onRequestConversationChange = useCallback(
    async (nextConversationId: string, conversation: { title?: string }) => {
      if (!tab || tab.type !== 'chat') return;
      const nextTitle = conversation.title || t('chat.newConversation');
      await updateTab(tab.id, { conversation_id: nextConversationId, title: nextTitle });
    },
    [tab, updateTab, t],
  );

  return (
    <ChatPanel
      surface={surface}
      onSend={onSend}
      onRequestConversationChange={onRequestConversationChange}
      profileSlug={effectiveProfileSlug}
    />
  );
}
