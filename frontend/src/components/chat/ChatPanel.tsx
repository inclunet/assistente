import { ChatSessionView } from './ChatSessionView';
import type { ChatSurfaceIdentity } from '../../services/chatSessionRegistry';
import type {
  ChatSurfaceSendContext,
  ChatSurfaceSendHandler,
} from './ChatSurfaceController';

export type ChatPanelSendContext = ChatSurfaceSendContext;
export type ChatPanelSendHandler = ChatSurfaceSendHandler;

/**
 * Solicitação de troca de conversa originada no HistoryPicker do toolbar. A
 * superfície de chat é "controlada": ela não decide o que trocar a conversa
 * significa — apenas notifica o dono (página, modal, etc.), que reage atualizando
 * a identidade da superfície que passa para baixo (ex.: `tab.conversation_id` na
 * página; `setBoundConversation` no modal embutido).
 */
export type ChatPanelConversationChangeHandler = (
  conversationId: string,
  conversation: { title?: string },
) => void | Promise<void>;

export interface ChatPanelProps {
  surface: ChatSurfaceIdentity;
  onSend: ChatPanelSendHandler;
  onRequestConversationChange?: ChatPanelConversationChangeHandler;
  showShortcutsHelp?: boolean;
}

export function ChatPanel({
  surface,
  onSend,
  onRequestConversationChange,
  showShortcutsHelp,
}: ChatPanelProps) {
  const variant = surface.surfaceType === 'embedded' || surface.surfaceType === 'modal'
    ? 'embedded'
    : 'page';

  return (
    <ChatSessionView
      variant={variant}
      surface={surface}
      onSend={(content, mediaFiles, origin) => onSend(content, mediaFiles, {
        conversationId: surface.conversationId || origin.conversationId || null,
        origin,
      })}
      onRequestConversationChange={onRequestConversationChange}
      showShortcutsHelp={showShortcutsHelp}
    />
  );
}
